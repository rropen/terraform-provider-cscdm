package cscdm

import "fmt"

// Record represents a planned DNS record.
type RecordAction struct {
	ZoneEdit
	ZoneName string
}

func (c *Client) enqueue(recordAction *RecordAction, returnChan chan *ZoneRecord, errorChan chan error) {
	c.batchMutex.Lock()
	c.returnChannelsMutex.Lock()
	defer c.batchMutex.Unlock()
	defer c.returnChannelsMutex.Unlock()

	c.recordActionQueue = append(c.recordActionQueue, recordAction)

	id := c.genId(recordAction.ZoneName, recordAction.RecordType, recordAction.KeyId(), recordAction.ValueId())
	c.returnChannels[id] = returnChan
	c.errorChannels[id] = errorChan

	c.triggerFlush()
}

func (c *Client) flush() error {
	return c.editZones()
}

func (c *Client) genId(zone string, recordType string, key string, value string) string {
	return fmt.Sprintf("%s:%s:%s:%s", zone, recordType, key, value)
}

func (c *Client) clear() {
	c.batchMutex.Lock()
	c.returnChannelsMutex.Lock()
	defer c.batchMutex.Unlock()
	defer c.returnChannelsMutex.Unlock()

	// Clear queue
	c.recordActionQueue = nil

	// Close pending return channels and clear
	for _, returnChan := range c.returnChannels {
		close(returnChan)
	}
	c.returnChannels = make(map[string]chan *ZoneRecord)

	// Close pending error channels and clear
	for _, errorChan := range c.errorChannels {
		close(errorChan)
	}
	c.errorChannels = make(map[string]chan error)
}
