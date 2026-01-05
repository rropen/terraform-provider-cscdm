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
	HTTP_REQUEST_TIMEOUT       = 30 * time.Second
)

type Client struct {
	http *http.Client

	recordActionQueue   []*RecordAction
	returnChannels      map[string]chan *ZoneRecord
	errorChannels       map[string]chan error
	batchMutex          sync.Mutex
	returnChannelsMutex sync.Mutex

	flushTrigger      chan struct{}
	flushLoopStopChan chan struct{}
	stopOnce          sync.Once

	zoneCache  map[string]*Zone
	zoneGroup  singleflight.Group
	cacheMutex sync.RWMutex
}

func (c *Client) Configure(apiKey string, apiToken string) {
	c.http = &http.Client{
		Timeout: HTTP_REQUEST_TIMEOUT,
		Transport: &util.HttpTransport{
			BaseUrl: CSC_DOMAIN_MANAGER_API_URL,
			Headers: map[string]string{
				"accept":        "application/json",
				"apikey":        apiKey,
				"Authorization": fmt.Sprintf("Bearer %s", apiToken),
			},
		}}

	c.returnChannels = make(map[string]chan *ZoneRecord)
	c.errorChannels = make(map[string]chan error)

	c.flushTrigger = make(chan struct{}, 1)
	c.flushLoopStopChan = make(chan struct{})

	c.zoneCache = make(map[string]*Zone)

	go c.flushLoop()
}

func (c *Client) flushLoop() {
	for {
		flushTimer := time.NewTimer(FLUSH_IDLE_DURATION)

		select {
		case <-c.flushTrigger:
			// Flush triggered; reset flush timer
			flushTimer.Stop()
			// Drain the channel in case of multiple signals
			select {
			case <-c.flushTrigger:
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
			return
		}
	}
}

func (c *Client) triggerFlush() {
	// Non-blocking send - if channel full, trigger already pending
	select {
	case c.flushTrigger <- struct{}{}:
	default:
	}
}

func (c *Client) Stop() {
	c.stopOnce.Do(func() {
		close(c.flushLoopStopChan)
	})
}
