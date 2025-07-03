package cscdm

import "fmt"

// Record represents a planned DNS record.
type RecordAction struct {
	Action   string
	ZoneName string
	Type     string
	Key      string
	Value    string
	Ttl      int64
	Priority int64
}

func (c *Client) enqueue(recordAction *RecordAction, channel chan *ZoneRecord) {
	c.batchMutex.Lock()
	c.returnChannelsMutex.Lock()
	defer c.batchMutex.Unlock()
	defer c.returnChannelsMutex.Unlock()

	c.recordActionQueue = append(c.recordActionQueue, recordAction)

	id := c.genId(recordAction.ZoneName, recordAction.Type, recordAction.Key)
	c.returnChannels[id] = channel

	c.triggerFlush()
}

func (c *Client) flush() error {
	return c.editZones()
}

func (c *Client) genId(zone string, recordType string, key string) string {
	return fmt.Sprintf("%s:%s:%s", zone, recordType, key)
}

func (c *Client) clear() {
	// Clear queue
	c.recordActionQueue = nil

	// Close pending return channels and clear
	c.returnChannelsMutex.Lock()
	for _, channel := range c.returnChannels {
		close(channel)
	}
	c.returnChannels = make(map[string]chan *ZoneRecord)
	c.returnChannelsMutex.Unlock()

}
