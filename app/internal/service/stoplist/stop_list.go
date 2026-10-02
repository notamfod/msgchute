package stoplist

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/devian2011/msgchute/internal/dto"
)

const (
	KindEmail = "email"
	KindPhone = "phone"
)

var (
	ErrInvalidEntry         = errors.New("invalid stop list entry")
	ErrInvalidRecipient     = errors.New("invalid stop list recipient")
	ErrReasonTooLong        = errors.New("stop list reason is too long")
	ErrNotFound             = errors.New("stop list entry not found")
	ErrAlreadyExists        = errors.New("stop list entry already exists")
	ErrInvalidSubscriptions = errors.New("invalid subscriptions")
	subscriptionCatalog     = map[string]dto.SubscriptionList{
		KindEmail: {"email.order", "email.promotion", "email.news"},
		KindPhone: {"sms.order", "sms.promotion", "sms.news", "whatsapp.order", "whatsapp.promotion", "whatsapp.news"},
	}
)

type Repository interface {
	Create(context.Context, *dto.StopListEntry) (*dto.StopListEntry, error)
	Delete(context.Context, string) error
	Find(context.Context, dto.StopListFilter) ([]dto.StopListEntry, uint64, error)
	Blocked(context.Context, string, []string) (map[string]struct{}, error)
	Preferences(context.Context, string, []string) (map[string]dto.StopListEntry, error)
	List(context.Context, dto.StopListFilter) ([]dto.StopListEntry, uint64, error)
	Upsert(context.Context, *dto.StopListEntry) (*dto.StopListEntry, error)
	Block(context.Context, *dto.StopListEntry) (*dto.StopListEntry, error)
	Unblock(context.Context, string) error
}

type Service struct{ repo Repository }

func New(repo Repository) *Service { return &Service{repo: repo} }

func Normalize(kind, recipient string) (string, error) {
	recipient = strings.TrimSpace(recipient)
	switch kind {
	case KindEmail:
		address, err := mail.ParseAddress(recipient)
		if err != nil || address == nil {
			return "", ErrInvalidRecipient
		}
		at := strings.LastIndexByte(address.Address, '@')
		if at <= 0 || at == len(address.Address)-1 || strings.Count(address.Address, "@") != 1 || utf8.RuneCountInString(address.Address) > 320 {
			return "", ErrInvalidRecipient
		}
		return strings.ToLower(address.Address), nil
	case KindPhone:
		explicitPlus, value, err := phoneDigits(recipient)
		if err != nil {
			return "", err
		}
		if len(value) < 7 || len(value) > 15 || value[0] == '0' {
			return "", ErrInvalidRecipient
		}
		if !explicitPlus && len(value) == 11 && value[0] == '8' {
			return "+7" + value[1:], nil
		}
		return "+" + value, nil
	default:
		return "", ErrInvalidRecipient
	}
}

func phoneDigits(recipient string) (bool, string, error) {
	explicitPlus := strings.HasPrefix(recipient, "+")
	var digits strings.Builder
	for i, r := range recipient {
		if r == '+' {
			if i != 0 {
				return false, "", ErrInvalidRecipient
			}
			continue
		}
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
			continue
		}
		if !strings.ContainsRune(" +()-.", r) {
			return false, "", ErrInvalidRecipient
		}
	}
	return explicitPlus, digits.String(), nil
}

func (s *Service) Create(ctx context.Context, entry *dto.StopListEntry) (*dto.StopListEntry, error) {
	if entry == nil {
		return nil, ErrInvalidEntry
	}
	if utf8.RuneCountInString(entry.Reason) > 1000 {
		return nil, ErrReasonTooLong
	}
	recipient, err := Normalize(entry.Kind, entry.Recipient)
	if err != nil {
		return nil, err
	}
	entry.Recipient = recipient
	entry.Subscriptions = defaultSubscriptions(entry.Kind)
	entry.BlockedAll = true
	return s.repo.Block(ctx, entry)
}

func (s *Service) Delete(ctx context.Context, id string) error { return s.repo.Unblock(ctx, id) }

func (s *Service) Find(ctx context.Context, filter dto.StopListFilter) ([]dto.StopListEntry, uint64, error) {
	return s.repo.Find(ctx, filter)
}

func (s *Service) List(ctx context.Context, filter dto.StopListFilter) ([]dto.StopListEntry, uint64, error) {
	return s.repo.List(ctx, filter)
}

func (s *Service) Upsert(ctx context.Context, entry *dto.StopListEntry) (*dto.StopListEntry, error) {
	if entry == nil || !validKind(entry.Kind) || utf8.RuneCountInString(entry.Reason) > 1000 {
		return nil, ErrInvalidEntry

	}
	recipient, err := Normalize(entry.Kind, entry.Recipient)
	if err != nil {
		return nil, err
	}
	if !validSubscriptions(entry.Kind, entry.Subscriptions) {
		return nil, ErrInvalidSubscriptions
	}
	entry.Recipient = recipient
	return s.repo.Upsert(ctx, entry)
}

func validKind(kind string) bool { return kind == KindEmail || kind == KindPhone }

func RecipientKinds() []string { return []string{KindEmail, KindPhone} }

func Subscriptions() map[string]dto.SubscriptionList {
	result := make(map[string]dto.SubscriptionList, len(subscriptionCatalog))
	for kind, subscriptions := range subscriptionCatalog {
		result[kind] = append(dto.SubscriptionList(nil), subscriptions...)
	}
	return result
}

func defaultSubscriptions(kind string) dto.SubscriptionList {
	if kind == KindEmail {
		return dto.SubscriptionList{"email.order"}
	}
	return dto.SubscriptionList{"sms.order"}
}

func validSubscriptions(kind string, subscriptions dto.SubscriptionList) bool {
	allowed, ok := subscriptionCatalog[kind]
	if !ok {
		return false
	}
	seen := make(map[string]struct{}, len(subscriptions))
	for _, subscription := range subscriptions {
		if _, ok := seen[subscription]; ok {
			return false
		}
		seen[subscription] = struct{}{}
		if !slices.Contains(allowed, subscription) {
			return false
		}
	}
	return true
}

func (s *Service) Filter(ctx context.Context, recipients []string, channel, tag string) ([]string, error) {
	if tag != "" && !dto.ValidMessageTag(tag) {
		return nil, ErrInvalidEntry
	}
	if tag != "" && channel != "email" && channel != "sms" && channel != "whatsapp" {
		return nil, ErrInvalidEntry
	}
	byKind := map[string][]string{KindEmail: nil, KindPhone: nil}
	normalized := make(map[string][]string, len(recipients))
	for _, recipient := range recipients {
		kind := KindPhone
		if (tag != "" && channel == "email") || (tag == "" && strings.Contains(recipient, "@")) {
			kind = KindEmail
		}
		value, err := Normalize(kind, recipient)
		if err != nil {
			if tag != "" {
				return nil, ErrInvalidRecipient
			}
			continue // Legacy untagged messages may use opaque provider identifiers.
		}
		candidates := []string{value}
		if kind == KindPhone {
			explicitPlus, digits, _ := phoneDigits(strings.TrimSpace(recipient))
			if !explicitPlus && len(digits) == 11 && digits[0] == '8' {
				candidates = append(candidates, "+"+digits)
			}
		}
		normalized[recipient] = candidates
		byKind[kind] = append(byKind[kind], candidates...)
	}
	entries := make(map[string]dto.StopListEntry)
	for kind, values := range byKind {
		if len(values) == 0 {
			continue
		}
		found, err := s.repo.Preferences(ctx, kind, values)
		if err != nil {
			return nil, fmt.Errorf("check stop list: %w", err)
		}
		for value, entry := range found {
			entries[value] = entry
		}
	}
	filtered := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		values, checked := normalized[recipient]
		if !checked || !denied(entries, values, channel, tag) {
			filtered = append(filtered, recipient)
		}
	}
	return filtered, nil
}

func denied(entries map[string]dto.StopListEntry, values []string, channel, tag string) bool {
	if tag == "" {
		for _, value := range values {
			if entry, ok := entries[value]; ok && entry.BlockedAll {
				return true
			}
		}
		return false
	}
	matched := false
	token := channel + "." + tag
	for _, value := range values {
		entry, ok := entries[value]
		if !ok {
			continue
		}
		matched = true
		if entry.BlockedAll {
			return true
		}
		allowed := false
		for _, subscription := range entry.Subscriptions {
			if subscription == token {
				allowed = true
				break
			}
		}
		if !allowed {
			return true
		}
	}
	return !matched && !(tag == "order" && (channel == "email" || channel == "sms"))
}
