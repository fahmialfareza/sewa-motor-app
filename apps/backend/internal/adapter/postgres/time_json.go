package postgres

import (
	"bytes"
	"encoding/json"
	"time"
)

type domainTime struct {
	time.Time
}

func (t *domainTime) UnmarshalJSON(body []byte) error {
	if bytes.Equal(body, []byte("null")) {
		return nil
	}
	var value string
	if err := json.Unmarshal(body, &value); err != nil {
		return err
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return err
	}
	t.Time = parsed
	return nil
}
