package cscdm

import (
	"fmt"
	"net/http"
	"os"
	"sync"
	"terraform-provider-cscdm/internal/util"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	CSC_DOMAIN_MANAGER_API_URL = "https://apis.cscglobal.com/dbs/api/v2/"
	POLL_INTERVAL              = 5 * time.Second
	FLUSH_IDLE_DURATION        = 5 * time.Second
)

type Client struct {
	http *http.Client

	recordActionQueue   []*RecordAction
	returnChannels      map[string]chan *ZoneRecord
	errorChannels       map[string]chan error
	batchMutex          sync.Mutex
	returnChannelsMutex sync.Mutex

	flushTrigger      *sync.Cond
	flushLoopStopChan chan struct{}

	zoneCache  map[string]*Zone
	zoneGroup  singleflight.Group
	cacheMutex sync.RWMutex
}

func (c *Client) Configure(apiKey string, apiToken string) {
	c.http = &http.Client{Transport: &util.HttpTransport{
		BaseUrl: CSC_DOMAIN_MANAGER_API_URL,
		Headers: map[string]string{
			"accept":        "application/json",
			"apikey":        apiKey,
			"Authorization": fmt.Sprintf("Bearer %s", apiToken),
		},
	}}

	c.returnChannels = make(map[string]chan *ZoneRecord)
	c.errorChannels = make(map[string]chan error)

	c.flushTrigger = sync.NewCond(&sync.Mutex{})
	c.flushLoopStopChan = make(chan struct{})

	c.zoneCache = make(map[string]*Zone)

	go c.flushLoop()
}

func (c *Client) flushLoop() {
	// Single trigger channel used throughout lifetime
	triggerChan := make(chan struct{}, 1)
	// Start the trigger watcher goroutine
	triggerStop := make(chan struct{})
	go func() {
		defer close(triggerChan) // Signal flushLoop to exit when we're done
		for {
			c.flushTrigger.L.Lock()
			c.flushTrigger.Wait()
			c.flushTrigger.L.Unlock()

			select {
			case <-triggerStop:
				return
			default:
				// Non-blocking send - if channel full, trigger already pending
				select {
				case triggerChan <- struct{}{}:
				default:
				}
			}
		}
	}()

	for {
		flushTimer := time.NewTimer(FLUSH_IDLE_DURATION)

		select {
		case <-triggerChan:
			// Flush triggered; reset flush timer
			flushTimer.Stop()
			// Drain the channel in case of multiple signals
			select {
			case <-triggerChan:
			default:
			}
		case <-flushTimer.C:
			// Timer expired; flush queue
			err := c.flush()

			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to flush queue: %s\n", err.Error())
				// Continue - don't return/terminate
			}
		case <-c.flushLoopStopChan:
			// Stop flush loop
			flushTimer.Stop()
			close(triggerStop) // Stop the trigger watcher
			<-triggerChan      // Wait for it to close the channel
			return
		}
	}
}

func (c *Client) triggerFlush() {
	c.flushTrigger.L.Lock()
	defer c.flushTrigger.L.Unlock()

	c.flushTrigger.Signal()
}

func (c *Client) Stop() {
	close(c.flushLoopStopChan)
}
