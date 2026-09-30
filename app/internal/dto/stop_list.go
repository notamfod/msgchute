package dto

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
)

type StopListEntry struct {
	ID            uuid.UUID        `json:"id" db:"id"`
	Recipient     string           `json:"recipient" db:"recipient"`
	Kind          string           `json:"kind" db:"kind"`
	Reason        string           `json:"reason,omitempty" db:"reason"`
	Subscriptions SubscriptionList `json:"subscriptions" db:"subscriptions"`
	BlockedAll    bool             `json:"blocked_all" db:"blocked_all"`
	CreatedAt     time.Time        `json:"created_at" db:"created_at"`
}

type StopListFilter struct {
	Search string
	Limit  uint64
	Offset uint64
}

type SubscriptionList []string

func (p SubscriptionList) Value() (driver.Value, error) { return sonic.Marshal(p) }
func (p *SubscriptionList) Scan(src interface{}) error {
	bytes, ok := src.([]byte)
	if !ok {
		return fmt.Errorf("unexpected subscriptions type %T", src)
	}
	return sonic.Unmarshal(bytes, p)
}
