package cscdm

import (
	"fmt"
	"net/http"
	"os"
	"sync"
	"terraform-provider-csc-domain-manager/internal/util"
	"time"
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
	batchMutex          sync.Mutex
	returnChannelsMutex sync.Mutex

	flushTrigger      *sync.Cond
	flushLoopStopChan chan struct{}
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

	c.flushTrigger = sync.NewCond(&sync.Mutex{})
	c.flushLoopStopChan = make(chan struct{})

	go c.flushLoop()
}

func (c *Client) flushLoop() {
	for {
		triggerChan := make(chan struct{})
		go func() {
			c.flushTrigger.L.Lock()
			c.flushTrigger.Wait()
			c.flushTrigger.L.Unlock()
			close(triggerChan)
		}()

		flushTimer := time.NewTimer(FLUSH_IDLE_DURATION)

		select {
		case <-triggerChan:
			// Flush triggered; reset flush timer
			flushTimer.Stop()
			continue
		case <-flushTimer.C:
			// Timer expired; flush queue
			err := c.flush()

			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to flush queue: %s", err.Error())
				return
			}
		case <-c.flushLoopStopChan:
			// Stop flush loop
			flushTimer.Stop()
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
