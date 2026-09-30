package dto

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/devian2011/retrier"
	"github.com/google/uuid"
)

type MessageStatus string

const (
	MessageStatusRunning   MessageStatus = "running"
	MessageStatusSucceeded MessageStatus = "succeeded"
	MessageStatusFailed    MessageStatus = "failed"
	MessageStatusDeclined  MessageStatus = "declined"
)

// MessageFilter defines evaluation criteria for advanced message querying, pagination, and sorting.
type MessageFilter struct {
	Limit  uint64
	Offset uint64

	Status []MessageStatus

	IDs       []uuid.UUID
	Recipient []string
	SenderIDs []string
	Code      []string
	Transport []string

	SortField *string
	SortOrder *string
}

// MessageRetryRequest
type MessageRetryRequest struct {
	ID       uuid.UUID `json:"id"`
	Deadline time.Time `json:"deadline"`
	Retry    *Retry    `json:"retry,omitempty"`
	Schedule time.Time `json:"schedule,omitempty"`
}

// Message represents the core message entity tracking dispatch metadata and payload definitions.
type Message struct {
	ID         uuid.UUID     `json:"id" db:"id"`
	SenderID   string        `json:"sender_id" db:"sender_id" validate:"required"`
	Recipients Recipients    `json:"recipients" db:"recipients" validate:"required"`
	Status     MessageStatus `json:"status" db:"status"`
	Meta       MessageMeta   `json:"metadata" db:"meta"`                           // Meta some additional fields like CC Bcc or Files etc.
	Code       *string       `json:"code,omitempty" db:"code"`                     // Code template code
	Params     MessageParams `json:"params,omitempty" db:"params"`                 // Params message params for generate templates
	Transport  string        `json:"transport" db:"transport" validate:"required"` // Transport message provider
	Tag        string        `json:"tag,omitempty" db:"tag"`
	Subject    string        `json:"subject" db:"subject"`
	Body       string        `json:"body" db:"body"`
	Deadline   time.Time     `json:"deadline" db:"deadline"`
	Retry      *Retry        `json:"retry,omitempty" db:"retry"`
	Schedule   time.Time     `json:"schedule,omitempty" db:"schedule"`
}

// Recipients represents a list of target delivery addresses or identifiers.
type Recipients []string

// Value implements driver.Valuer to serialize destination collections into database-compatible JSON strings.
func (p Recipients) Value() (driver.Value, error) {
	if p == nil {
		return nil, nil
	}
	return sonic.Marshal(p)
}

// Scan implements sql.Scanner to deserialize raw database JSON arrays into structural target collections.
func (p *Recipients) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed")
	}
	return sonic.Unmarshal(bytes, p)
}

// MessageMeta handles vendor-specific transport flags or contextual properties.
type MessageMeta map[string]any

// Value implements driver.Valuer to serialize metadata frames into database-compatible JSON strings.
func (p MessageMeta) Value() (driver.Value, error) {
	if p == nil {
		return nil, nil
	}
	return sonic.Marshal(p)
}

// Scan implements sql.Scanner to deserialize raw database JSON documents into key-value configuration matrices.
func (p *MessageMeta) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed")
	}
	return sonic.Unmarshal(bytes, p)
}

// MessageParam holds a flexible parameters mapping payload value.
type MessageParam struct {
	Value any `json:"value"`
}

// MessageParams maps raw parameter labels to structured interpolation matrices.
type MessageParams map[string]*MessageParam

// Value implements driver.Valuer to serialize parameter matrices into database-compatible JSON arrays.
func (p MessageParams) Value() (driver.Value, error) {
	if p == nil {
		return nil, nil
	}
	return sonic.Marshal(p)
}

// Scan implements sql.Scanner to deserialize raw database JSON maps into structured value-holder parameters.
func (p *MessageParams) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed")
	}
	return sonic.Unmarshal(bytes, p)
}

// Retry outlines boundary settings, backoff spacing strategies, and rate policies.
type Retry struct {
	Retries  int                          `json:"retries,omitempty"`
	Strategy string                       `json:"strategy,omitempty"`
	Params   map[retrier.BackOffParam]any `json:"params,omitempty"`
}

// Value implements driver.Valuer to serialize dynamic backup boundaries into structured JSON.
func (r *Retry) Value() (driver.Value, error) {
	if r == nil {
		return nil, nil
	}
	return sonic.Marshal(r)
}

// Scan implements sql.Scanner to deserialize backend retry policies into executable application constraints.
func (r *Retry) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed")
	}
	return sonic.Unmarshal(bytes, r)
}

// MessageTemplateFilter defines evaluation criteria for advanced template querying, pagination, and sorting.
type MessageTemplateFilter struct {
	Systems   []string
	Channels  []string
	Limit     uint64
	Offset    uint64
	Code      []string
	Search    *string
	SortField *string
	SortOrder *string
}

// Template establishes baseline layouts, mandatory properties, and default retry bounds for structural content generation.
type Template struct {
	Metadata    TemplateMetadata `json:"metadata" db:"metadata"`
	Systems     TemplateLabels   `json:"systems,omitempty" db:"systems"`
	Channels    TemplateLabels   `json:"channels,omitempty" db:"channels"`
	Code        string         `json:"code" db:"code" validate:"required"`
	Name        string         `json:"name" db:"name" validate:"required"`
	Description string         `json:"description" db:"description"`
	Params      TemplateParams `json:"params" db:"params"`
	Subject     string         `json:"subject" db:"subject" validate:"required"`
	Body        string         `json:"body" db:"body" validate:"required"`
}

// TemplateParam handles structural fallback expectations when explicit variables remain unassigned.
type TemplateParam struct {
	Default string `json:"default"`
	Required *bool `json:"required,omitempty"`
}

// TemplateParams lists baseline injection requirements needed to synthesize functional notifications.
type TemplateParams map[string]*TemplateParam

// Value implements driver.Valuer to serialize baseline templates into database-compatible JSON strings.
func (p TemplateParams) Value() (driver.Value, error) {
	if p == nil {
		return nil, nil
	}
	return sonic.Marshal(p)
}

// Scan implements sql.Scanner to deserialize raw database JSON constraints into structural fallback parameters.
func (p *TemplateParams) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed")
	}
	return sonic.Unmarshal(bytes, p)
}
