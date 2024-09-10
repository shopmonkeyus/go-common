package dbchange

import (
	"encoding/json"
)

// Event represents a change record from the database.
type Event struct {
	Operation     string          `json:"operation"`
	ID            string          `json:"id"`
	Table         string          `json:"table"`
	Key           []string        `json:"key"`
	Version       int64           `json:"version"`
	ModelVersion  string          `json:"modelVersion"`
	CompanyID     *string         `json:"companyId,omitempty"`
	LocationID    *string         `json:"locationId,omitempty"`
	UserID        *string         `json:"userId,omitempty"`
	Before        json.RawMessage `json:"before,omitempty"`
	After         json.RawMessage `json:"after,omitempty"`
	Diff          []string        `json:"diff,omitempty"`
	Timestamp     int64           `json:"timestamp"`
	MVCCTimestamp string          `json:"mvccTimestamp"`

	object map[string]any
}

func (c *Event) String() string {
	return "Event[op=" + c.Operation + ",table=" + c.Table + ",id=" + c.ID + ",pk=" + c.GetPrimaryKey() + "]"
}

func (c *Event) GetPrimaryKey() string {
	if len(c.Key) >= 1 {
		return c.Key[len(c.Key)-1]
	}
	o, err := c.GetObject()
	if err == nil {
		if id, ok := o["id"].(string); ok {
			return id
		}
	}
	return ""
}

func (c *Event) GetObject() (map[string]any, error) {
	if c.After != nil {
		if c.object == nil {
			res := make(map[string]any)
			if err := json.Unmarshal(c.After, &res); err != nil {
				return nil, err
			}
			c.object = res
		}
		return c.object, nil
	} else if c.Before != nil {
		if c.object == nil {
			res := make(map[string]any)
			if err := json.Unmarshal(c.Before, &res); err != nil {
				return nil, err
			}
			c.object = res
		}
		return c.object, nil
	}
	return nil, nil
}

func (c *Event) Get(res any) error {
	if c.Operation == "DELETE" {
		if err := json.Unmarshal(c.Before, &res); err != nil {
			return err
		}
		return nil
	}
	if err := json.Unmarshal(c.After, &res); err != nil {
		return err
	}
	return nil
}
