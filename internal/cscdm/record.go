package cscdm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
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
	if ze.Action == "ADD" || ze.Action == "EDIT" {
		return ze.NewKey
	} else {
		return ze.CurrentKey
	}
}

func (ze *ZoneEdit) ValueId() string {
	if ze.Action == "ADD" || ze.Action == "EDIT" {
		return ze.NewValue
	} else {
		return ze.CurrentValue
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
	errorChan := make(chan error, 1)
	c.enqueue(payload, returnChan, errorChan)

	select {
	case zoneRecord, ok := <-returnChan:
		if !ok {
			return nil, fmt.Errorf("return channel closed for %s %s in %s. CHECK TF WARN LOGS.", payload.RecordType, payload.KeyId(), payload.ZoneName)
		}
		return zoneRecord, nil
	case err, ok := <-errorChan:
		if !ok {
			return nil, fmt.Errorf("error channel closed for %s %s in %s. CHECK TF WARN LOGS.", payload.RecordType, payload.KeyId(), payload.ZoneName)
		}
		return nil, err
	}
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
				err = fmt.Errorf("failed to edit zone %s: %s", payload.ZoneName, err)
				rErr := c.returnErrorToZone(payload.ZoneName, err)

				if rErr != nil {
					errChan <- fmt.Errorf("failed to return error: %s", rErr)
				}
				return
			}

			err = c.waitForZoneEdits(*editId)
			if err != nil {
				err = fmt.Errorf("failed to wait for %s zone edits: %s", payload.ZoneName, err)
				rErr := c.returnErrorToZone(payload.ZoneName, err)

				if rErr != nil {
					errChan <- fmt.Errorf("failed to return error: %s", rErr)
				}
				return
			}

			c.invalidateZoneCache(payload.ZoneName)

			recordsByType := make(map[string][]string)

			for _, edit := range payload.Edits {
				if edit.Action == "PURGE" {
					err := c.returnRecord(payload.ZoneName, edit.RecordType, edit.KeyId(), edit.ValueId(), nil)
					if err != nil {
						rErr := c.returnError(payload.ZoneName, edit.RecordType, edit.KeyId(), edit.ValueId(), err)

						if rErr != nil {
							errChan <- fmt.Errorf("failed to return error: %s", rErr)
						}
						return
					}
				} else {
					recordsByType[edit.RecordType] = append(recordsByType[edit.RecordType], edit.KeyId())
				}
			}

			if len(recordsByType) > 0 {
				zone, err := c.GetZone(payload.ZoneName)
				if err != nil {
					rErr := c.returnErrorToZone(payload.ZoneName, err)

					if rErr != nil {
						errChan <- fmt.Errorf("failed to return error: %s", rErr)
					}
					return
				}

				for recordType, keys := range recordsByType {
					records := c.GetRecordsByType(zone, recordType)
					if records == nil {
						err := fmt.Errorf("unsupported record type: %s", recordType)
						rErr := c.returnErrorToZoneWithRecordType(payload.ZoneName, recordType, err)

						if rErr != nil {
							errChan <- fmt.Errorf("failed to return error: %s", rErr)
						}
						return
					}

					for key, record := range c.GetRecordsByKeys(records, keys) {
						err := c.returnRecord(payload.ZoneName, recordType, key, record.Value, record)
						if err != nil {
							rErr := c.returnError(payload.ZoneName, recordType, key, record.Value, err)

							if rErr != nil {
								errChan <- fmt.Errorf("failed to return error: %s", rErr)
							}
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

		if editStatusJson.Content.Status == "FAILED" {
			err = c.cancelZoneEdit(editId)
			if err != nil {
				return fmt.Errorf("zone edits returned status FAILED: failed to cancel zone edits: %s", err)
			}
			return fmt.Errorf("zone edits returned status FAILED: successfully canceled zone edits")
		}

		time.Sleep(POLL_INTERVAL)
	}
}

func (c *Client) returnRecord(zone string, recordType string, key string, value string, record *ZoneRecord) error {
	id := c.genId(zone, recordType, key, value)

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

func (c *Client) returnErrorByIdWithoutLock(id string, err error) error {
	errorChan, ok := c.errorChannels[id]
	if !ok {
		return fmt.Errorf("failed to get error channel for %s", id)
	}

	errorChan <- err
	delete(c.errorChannels, id)
	close(errorChan)
	return nil
}

func (c *Client) returnError(zone string, recordType string, key string, value string, err error) error {
	c.returnChannelsMutex.Lock()
	defer c.returnChannelsMutex.Unlock()

	return c.returnErrorByIdWithoutLock(c.genId(zone, recordType, key, value), err)
}

func (c *Client) returnErrorToZone(zone string, err error) error {
	c.returnChannelsMutex.Lock()
	defer c.returnChannelsMutex.Unlock()

	var rErrs []error

	for id := range c.errorChannels {
		if strings.Split(id, ":")[0] == zone {
			rErr := c.returnErrorByIdWithoutLock(id, err)

			if rErr != nil {
				rErrs = append(rErrs, rErr)
			}
		}
	}

	if len(rErrs) > 0 {
		return fmt.Errorf("failed to return error to %d in zone %s: %s", len(rErrs), zone, err)
	}

	return nil
}

func (c *Client) returnErrorToZoneWithRecordType(zone string, recordType string, err error) error {
	c.returnChannelsMutex.Lock()
	defer c.returnChannelsMutex.Unlock()

	var rErrs []error

	for id := range c.errorChannels {
		idParts := strings.Split(id, ":")

		if idParts[0] == zone && idParts[1] == recordType {
			rErr := c.returnErrorByIdWithoutLock(id, err)

			if rErr != nil {
				rErrs = append(rErrs, rErr)
			}
		}
	}

	if len(rErrs) > 0 {
		return fmt.Errorf("failed to return error to %d in zone %s: %s", len(rErrs), zone, err)
	}

	return nil
}

func (c *Client) cancelZoneEdit(editId string) error {
	req, err := http.NewRequest("DELETE", fmt.Sprintf("zones/edits/%s", editId), nil)
	if err != nil {
		return fmt.Errorf("unable to create request: %s", err)
	}

	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("unable to send request: %s", err)
	}
	defer res.Body.Close()

	if res.StatusCode == 204 {
		return nil
	}

	var zeErr ZoneEditErr
	err = json.NewDecoder(res.Body).Decode(&zeErr)
	if err != nil {
		return fmt.Errorf("unable to unmarshal zone edit cancellation error: %s", err)
	}

	return fmt.Errorf("failed to cancel zone edit: %s: %s: %q", zeErr.Code, zeErr.Description, zeErr.Value)
}

func (c *Client) invalidateZoneCache(zoneName string) {
	c.cacheMutex.Lock()
	defer c.cacheMutex.Unlock()

	delete(c.zoneCache, zoneName)
}

func (c *Client) FetchZone(zoneName string) (*Zone, error) {
	zoneResp, err := c.http.Get(fmt.Sprintf("zones/%s", zoneName))
	if err != nil {
		return nil, fmt.Errorf("unable to send request: %s", err)
	}
	defer zoneResp.Body.Close()

	var zone Zone
	err = json.NewDecoder(zoneResp.Body).Decode(&zone)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal zone: %s", err)
	}

	c.cacheMutex.Lock()
	c.zoneCache[zoneName] = &zone
	c.cacheMutex.Unlock()

	return &zone, nil
}

func (c *Client) GetZone(zoneName string) (*Zone, error) {
	c.cacheMutex.RLock()
	zone, ok := c.zoneCache[zoneName]
	c.cacheMutex.RUnlock()

	if ok {
		return zone, nil
	}

	res, err, _ := c.zoneGroup.Do(zoneName, func() (interface{}, error) {
		zone, err := c.FetchZone(zoneName)
		if err != nil {
			return nil, err
		}

		c.cacheMutex.Lock()
		c.zoneCache[zoneName] = zone
		c.cacheMutex.Unlock()
		return zone, nil
	})

	if err != nil {
		return nil, err
	}

	zone, ok = res.(*Zone)
	if !ok {
		return nil, fmt.Errorf("failed to assert type for *zone")
	}

	return zone, nil
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
	for i, record := range records {
		if record.Key == key {
			return &records[i]
		}
	}

	return nil
}

func (c *Client) GetRecordById(records []ZoneRecord, id string) *ZoneRecord {
	for i, record := range records {
		if record.Id == id {
			return &records[i]
		}
	}

	return nil
}

func (c *Client) GetRecordByTypeByKey(zone *Zone, recordType string, key string) (*ZoneRecord, error) {
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

func (c *Client) GetRecordByTypeById(zone *Zone, recordType string, id string) (*ZoneRecord, error) {
	records := c.GetRecordsByType(zone, recordType)
	if records == nil {
		return nil, fmt.Errorf("unsupported record type: %s", recordType)
	}

	record := c.GetRecordById(records, id)
	if record == nil {
		return nil, fmt.Errorf("record of type %s with id '%s' was not found in zone %s", recordType, id, zone.ZoneName)
	}

	return record, nil
}

func (c *Client) GetRecordsByKeys(records []ZoneRecord, keys []string) map[string]*ZoneRecord {
	keySet := make(map[string]bool)
	for _, key := range keys {
		keySet[key] = true
	}

	recordMap := make(map[string]*ZoneRecord)
	for i, record := range records {
		if keySet[record.Key] {
			recordMap[record.Key] = &records[i]
		}
	}

	return recordMap
}
