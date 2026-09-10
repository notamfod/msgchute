package dto

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

type TemplateMetadata map[string]any
type TemplateLabels []string

func (m TemplateMetadata) Value() (driver.Value, error) { return json.Marshal(m) }
func (m *TemplateMetadata) Scan(value any) error {
	if value == nil {
		*m = TemplateMetadata{}
		return nil
	}
	data, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("invalid template metadata database type %T", value)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return decoder.Decode(m)
}
func (l TemplateLabels) Value() (driver.Value, error) {
	if l == nil {
		return nil, nil
	}
	return json.Marshal(l)
}
func (l *TemplateLabels) Scan(value any) error {
	if value == nil {
		*l = nil
		return nil
	}
	data, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("invalid template labels database type %T", value)
	}
	return json.Unmarshal(data, l)
}
