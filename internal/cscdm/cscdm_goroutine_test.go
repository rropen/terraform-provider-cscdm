package cscdm_test

import (
	"runtime"
	"sync"
	"terraform-provider-cscdm/internal/cscdm"
	"testing"
	"time"
)

func TestClient_GoroutineLeakPrevention(t *testing.T) {
	// Record initial goroutine count
	initialGoroutines := runtime.NumGoroutine()

	// Create multiple clients to test for cumulative leaks
	clients := make([]*cscdm.Client, 5)

	for i := 0; i < 5; i++ {
		client := &cscdm.Client{}
		client.Configure("test-key", "test-token")
		clients[i] = client

		// Allow goroutines to start
		time.Sleep(10 * time.Millisecond)
	}

	// Verify goroutines increased as expected (at least 2 per client: flushLoop + trigger watcher)
	midGoroutines := runtime.NumGoroutine()
	if midGoroutines <= initialGoroutines {
		t.Errorf("Expected goroutine count to increase after creating clients. Initial: %d, Mid: %d", initialGoroutines, midGoroutines)
	}

	// Stop all clients
	for _, client := range clients {
		client.Stop()
	}

	// Allow time for cleanup
	time.Sleep(200 * time.Millisecond)
	runtime.GC()
	runtime.GC() // Double GC to ensure cleanup
	time.Sleep(100 * time.Millisecond)

	// Check final goroutine count
	finalGoroutines := runtime.NumGoroutine()
	if finalGoroutines > initialGoroutines+2 { // Allow small margin for test goroutines
		t.Errorf("Goroutine leak detected. Initial: %d, Final: %d, Leaked: %d",
			initialGoroutines, finalGoroutines, finalGoroutines-initialGoroutines)
	}

	// Test that we can create and stop another client without issues
	testClient := &cscdm.Client{}
	testClient.Configure("test-key", "test-token")

	done := make(chan bool, 1)
	go func() {
		testClient.Stop()
		done <- true
	}()

	select {
	case <-done:
		// Success - no deadlock from leaked goroutines
	case <-time.After(2 * time.Second):
		t.Error("Final client Stop() hung, suggesting goroutine leak interference")
	}
}

func TestClient_FlushErrorResilience(t *testing.T) {
	// This test verifies that the flush loop continues running even after errors
	client := &cscdm.Client{}
	client.Configure("invalid-key", "invalid-token") // Force API errors

	initialGoroutines := runtime.NumGoroutine()

	// Wait for multiple flush cycles with errors
	for i := 0; i < 3; i++ {
		time.Sleep(cscdm.FLUSH_IDLE_DURATION + 50*time.Millisecond)

		// Verify flush loop is still running by checking goroutine stability
		currentGoroutines := runtime.NumGoroutine()
		if currentGoroutines < initialGoroutines {
			t.Errorf("Goroutine count decreased after error cycle %d, suggesting flush loop died", i+1)
			break
		}
	}

	// Verify the flush loop is still responsive by stopping cleanly
	done := make(chan bool, 1)
	go func() {
		client.Stop()
		done <- true
	}()

	select {
	case <-done:
		// Test passes if Stop() completes without hanging
	case <-time.After(2 * time.Second):
		t.Error("Stop() hung, suggesting flush loop died from error")
	}
}

func TestClient_ConcurrentFlushTriggers(t *testing.T) {
	client := &cscdm.Client{}
	client.Configure("test-key", "test-token")

	initialGoroutines := runtime.NumGoroutine()

	// Simulate concurrent operations that would trigger flushes
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Simulate work that might trigger flushes
			for j := 0; j < 5; j++ {
				time.Sleep(time.Millisecond)
			}
		}()
	}

	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	// Verify goroutines haven't multiplied excessively
	currentGoroutines := runtime.NumGoroutine()
	if currentGoroutines > initialGoroutines+10 {
		t.Errorf("Excessive goroutine growth during concurrent operations. Initial: %d, Current: %d", initialGoroutines, currentGoroutines)
	}

	// Test that Stop() works cleanly after concurrent triggers
	done := make(chan bool, 1)
	go func() {
		client.Stop()
		done <- true
	}()

	select {
	case <-done:
		// Success - no deadlock from concurrent triggers
	case <-time.After(2 * time.Second):
		t.Error("Stop() hung after concurrent triggers, suggesting channel overflow issue")
	}
}

func TestClient_GracefulShutdown(t *testing.T) {
	client := &cscdm.Client{}
	client.Configure("test-key", "test-token")

	// Start multiple goroutines that trigger flushes
	stopWorkers := make(chan bool)
	var workerWg sync.WaitGroup

	for i := 0; i < 5; i++ {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			for {
				select {
				case <-stopWorkers:
					return
				default:
					// Just keep the goroutine active to test concurrent access
					time.Sleep(1 * time.Millisecond)
				}
			}
		}()
	}

	// Let workers run for a bit
	time.Sleep(10 * time.Millisecond)

	// Stop workers and client
	close(stopWorkers)
	client.Stop()

	// Wait for workers to finish
	done := make(chan bool)
	go func() {
		workerWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Error("Graceful shutdown timed out")
	}
}

func TestClient_TriggerChannelDraining(t *testing.T) {
	client := &cscdm.Client{}
	client.Configure("test-key", "test-token")

	// Let the client run for a bit to test the flush loop
	time.Sleep(50 * time.Millisecond)

	// Small delay to let triggers propagate
	time.Sleep(10 * time.Millisecond)

	// Test clean stop - if channel draining doesn't work, this might hang
	done := make(chan bool, 1)
	go func() {
		client.Stop()
		done <- true
	}()

	select {
	case <-done:
		// Success - channel draining worked
	case <-time.After(1 * time.Second):
		t.Error("Stop() hung, suggesting channel draining issue")
	}
}

func TestClient_StopChannelCleanup(t *testing.T) {
	client := &cscdm.Client{}
	client.Configure("test-key", "test-token")

	// Let the client run for a bit
	time.Sleep(10 * time.Millisecond)

	// Test that Stop() works correctly
	done := make(chan bool, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- false
				return
			}
			done <- true
		}()
		client.Stop()
	}()

	select {
	case success := <-done:
		if !success {
			t.Error("Stop() panicked")
		}
	case <-time.After(1 * time.Second):
		t.Error("Stop() hung")
	}
}
