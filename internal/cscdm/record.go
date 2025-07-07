package cscdm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

type ZoneEditReq struct {
	ZoneName string     `json:"zoneName"`
	Edits    []ZoneEdit `json:"edits"`
}

type ZoneEdit struct {
	RecordType      string `json:"recordType"`
	Action          string `json:"action"`
	CurrentKey      string `json:"currentKey,omitempty"`
	CurrentValue    string `json:"currentValue,omitempty"`
	CurrentTtl      int64  `json:"currentTtl,omitempty"`
	CurrentPriority int64  `json:"currentPriority,omitempty"`
	NewKey          string `json:"newKey,omitempty"`
	NewValue        string `json:"newValue,omitempty"`
	NewTtl          int64  `json:"newTtl,omitempty"`
	NewPriority     int64  `json:"newPriority,omitempty"`
}

func (ze *ZoneEdit) KeyId() string {
	if ze.RecordType == "ADD" || ze.RecordType == "EDIT" {
		return ze.NewKey
	} else {
		return ze.CurrentKey
	}
}

type ZoneEditRes struct {
	Content struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	} `json:"content"`
	Links struct {
		Self   string `json:"self"`
		Status string `json:"status"`
	} `json:"links"`
}

type ZoneEditErr struct {
	Code        string `json:"code"`
	Description string `json:"description"`
	Value       string `json:"value"`
}

type ZoneEditStatus struct {
	Content struct {
		Status string `json:"status"`
	} `json:"content"`
}

type Zone struct {
	ZoneName    string          `json:"zoneName"`
	HostingType string          `json:"hostingType"`
	A           []ZoneRecord    `json:"a"`
	CNAME       []ZoneRecord    `json:"cname"`
	AAAA        []ZoneRecord    `json:"aaaa"`
	TXT         []ZoneRecord    `json:"txt"`
	MX          []ZoneRecord    `json:"mx"`
	NS          []ZoneRecord    `json:"ns"`
	SRV         []ZoneSrvRecord `json:"srv"`
	CAA         []ZoneRecord    `json:"caa"`
	SOA         ZoneSoaRecord   `json:"soa"`
}

type ZoneRecord struct {
	Id       string `json:"id"`
	Key      string `json:"key"`
	Value    string `json:"value"`
	Ttl      int64  `json:"ttl,omitempty"`
	Priority int64  `json:"priority"`
	Status   string `json:"status"`
}

type ZoneSrvRecord struct {
	ZoneRecord
	Port int32 `json:"port"`
}

type ZoneSoaRecord struct {
	Serial     int64  `json:"serial"`
	Refresh    int64  `json:"refresh"`
	Retry      int64  `json:"retry"`
	Expire     int64  `json:"expire"`
	TtlMin     int64  `json:"ttlMin"`
	TtlNeg     int64  `json:"ttlNeg"`
	TtlZone    int64  `json:"ttlZone"`
	TechEmail  string `json:"techEmail"`
	MasterHost string `json:"masterHost"`
}

func (c *Client) PerformRecordAction(payload *RecordAction) (*ZoneRecord, error) {
	returnChan := make(chan *ZoneRecord, 1)
	c.enqueue(payload, returnChan)

	zoneRecord, ok := <-returnChan
	if !ok {
		return nil, fmt.Errorf("return channel closed for %s %s in %s", payload.RecordType, payload.ZoneEdit.KeyId(), payload.ZoneName)
	}

	return zoneRecord, nil
}

func (c *Client) editZones() error {
	c.batchMutex.Lock()
	defer c.clear()
	defer c.batchMutex.Unlock()

	zoneEdits := make(map[string][]ZoneEdit)
	for _, recordAction := range c.recordActionQueue {
		zoneEdits[recordAction.ZoneName] = append(
			zoneEdits[recordAction.ZoneName],
			ZoneEdit{
				RecordType:      recordAction.RecordType,
				Action:          recordAction.Action,
				CurrentKey:      recordAction.CurrentKey,
				CurrentValue:    recordAction.CurrentValue,
				CurrentTtl:      recordAction.CurrentTtl,
				CurrentPriority: recordAction.CurrentPriority,
				NewKey:          recordAction.NewKey,
				NewValue:        recordAction.NewValue,
				NewTtl:          recordAction.NewTtl,
				NewPriority:     recordAction.NewPriority,
			},
		)
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(zoneEdits))

	for zone, edits := range zoneEdits {
		payload := ZoneEditReq{
			ZoneName: zone,
			Edits:    edits,
		}

		wg.Add(1)
		go func(payload ZoneEditReq) {
			defer wg.Done()

			editId, err := c.editZone(payload)
			if err != nil {
				errChan <- err
				return
			}

			err = c.waitForZoneEdits(*editId)
			if err != nil {
				errChan <- err
				return
			}

			recordsByType := make(map[string][]string)

			for _, edit := range payload.Edits {
				if edit.Action == "PURGE" {
					err := c.returnRecord(payload.ZoneName, edit.RecordType, edit.KeyId(), nil)
					if err != nil {
						errChan <- err
						return
					}
				} else {
					recordsByType[edit.RecordType] = append(recordsByType[edit.RecordType], edit.KeyId())
				}
			}

			if len(recordsByType) > 0 {
				zone, err := c.FetchZone(payload.ZoneName)
				if err != nil {
					errChan <- err
					return
				}

				for recordType, keys := range recordsByType {
					records := c.GetRecordsByType(zone, recordType)
					if records == nil {
						errChan <- fmt.Errorf("unsupported record type: %s", recordType)
						return
					}

					for key, record := range c.GetRecordsByKeys(records, keys) {
						err := c.returnRecord(payload.ZoneName, recordType, key, record)
						if err != nil {
							errChan <- err
							return
						}
					}
				}
			}
		}(payload)
	}

	wg.Wait()
	close(errChan)

	if len(errChan) > 0 {
		var errStrs []string
		for err := range errChan {
			errStrs = append(errStrs, err.Error())
		}

		return fmt.Errorf("%d error(s) in batch zone edits: %s", len(errStrs), strings.Join(errStrs, ", "))
	}

	return nil
}

func (c *Client) editZone(payload ZoneEditReq) (*string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal record payload: %s", err)
	}

	for {
		createResp, err := c.http.Post("zones/edits", "application/json", bytes.NewBuffer(body))
		if err != nil {
			return nil, fmt.Errorf("failed to send request: %s", err)
		}
		defer createResp.Body.Close()

		if createResp.StatusCode != 201 {
			var createErrJson ZoneEditErr
			err = json.NewDecoder(createResp.Body).Decode(&createErrJson)
			if err != nil {
				return nil, fmt.Errorf("unable to unmarshal create record error response: %s", err)
			}

			if createErrJson.Code == "OPEN_ZONE_EDITS" {
				time.Sleep(POLL_INTERVAL)
				continue
			}

			return nil, fmt.Errorf("request returned unsuccessful status code: %s", err)
		}

		var createJson ZoneEditRes
		err = json.NewDecoder(createResp.Body).Decode(&createJson)
		if err != nil {
			return nil, fmt.Errorf("unable to unmarshal create record response: %s", err)
		}

		editStatusLink := strings.Split(createJson.Links.Status, "/")
		return &editStatusLink[len(editStatusLink)-1], nil
	}
}

func (c *Client) waitForZoneEdits(editId string) error {
	for {
		editStatusResp, err := c.http.Get(fmt.Sprintf("zones/edits/status/%s", editId))
		if err != nil {
			return fmt.Errorf("failed to send request: %s", err)
		}
		defer editStatusResp.Body.Close()

		var editStatusJson ZoneEditStatus
		err = json.NewDecoder(editStatusResp.Body).Decode(&editStatusJson)
		if err != nil {
			return fmt.Errorf("unable to unmarshal edit status response: %s", err)
		}

		if editStatusJson.Content.Status == "COMPLETED" {
			return nil
		}

		time.Sleep(POLL_INTERVAL)
	}
}

func (c *Client) returnRecord(zone string, recordType string, key string, record *ZoneRecord) error {
	id := c.genId(zone, recordType, key)

	c.returnChannelsMutex.Lock()
	returnChan, ok := c.returnChannels[id]
	if ok {
		delete(c.returnChannels, id)
	}
	c.returnChannelsMutex.Unlock()
	if !ok {
		return fmt.Errorf("failed to get return channel for %s", id)
	}

	returnChan <- record
	close(returnChan)
	return nil
}

func (c *Client) FetchZone(zoneName string) (*Zone, error) {
	zoneResp, err := c.http.Get(fmt.Sprintf("zones/%s", zoneName))
	if err != nil {
		return nil, fmt.Errorf("unable to send request: %s", err)
	}
	defer zoneResp.Body.Close()

	var zoneJson Zone
	err = json.NewDecoder(zoneResp.Body).Decode(&zoneJson)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal zone: %s", err)
	}

	return &zoneJson, nil
}

func (c *Client) GetRecordsByType(zone *Zone, recordType string) []ZoneRecord {
	switch recordType {
	case "A":
		return zone.A
	case "AAAA":
		return zone.AAAA
	case "CNAME":
		return zone.CNAME
	case "MX":
		return zone.MX
	case "NS":
		return zone.NS
	case "TXT":
		return zone.TXT
	default:
		return nil
	}
}

func (c *Client) GetRecordByKey(records []ZoneRecord, key string) *ZoneRecord {
	for _, record := range records {
		if record.Key == key {
			return &record
		}
	}

	return nil
}

func (c *Client) GetRecord(zone *Zone, recordType string, key string) (*ZoneRecord, error) {
	records := c.GetRecordsByType(zone, recordType)
	if records == nil {
		return nil, fmt.Errorf("unsupported record type: %s", recordType)
	}

	record := c.GetRecordByKey(records, key)
	if record == nil {
		return nil, fmt.Errorf("record of type %s with key '%s' was not found in zone %s", recordType, key, zone.ZoneName)
	}

	return record, nil
}

func (c *Client) GetRecordsByKeys(records []ZoneRecord, keys []string) map[string]*ZoneRecord {
	keySet := make(map[string]bool)
	for _, key := range keys {
		keySet[key] = true
	}

	recordMap := make(map[string]*ZoneRecord)
	for _, record := range records {
		if keySet[record.Key] {
			recordMap[record.Key] = &record
		}
	}

	return recordMap
}
