package stoplist

import (
	"context"
	"strings"
	"testing"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/stretchr/testify/require"
)

type createStub struct {
	calls   int
	blocked map[string]struct{}
}

func (s *createStub) Create(context.Context, *dto.StopListEntry) (*dto.StopListEntry, error) {
	s.calls++
	return nil, nil
}
func (s *createStub) Delete(context.Context, string) error { return nil }
func (s *createStub) Find(context.Context, dto.StopListFilter) ([]dto.StopListEntry, uint64, error) {
	return nil, 0, nil
}
func (s *createStub) Blocked(_ context.Context, _ string, recipients []string) (map[string]struct{}, error) {
	result := map[string]struct{}{}
	for _, recipient := range recipients {
		if _, ok := s.blocked[recipient]; ok {
			result[recipient] = struct{}{}
		}
	}
	return result, nil
}
func (s *createStub) Preferences(_ context.Context, _ string, recipients []string) (map[string]dto.StopListEntry, error) {
	result := map[string]dto.StopListEntry{}
	for _, recipient := range recipients {
		if _, ok := s.blocked[recipient]; ok {
			result[recipient] = dto.StopListEntry{Recipient: recipient, BlockedAll: true}
		}
	}
	return result, nil
}
func (s *createStub) List(context.Context, dto.StopListFilter) ([]dto.StopListEntry, uint64, error) {
	return nil, 0, nil
}
func (s *createStub) Upsert(context.Context, *dto.StopListEntry) (*dto.StopListEntry, error) {
	return nil, nil
}
func (s *createStub) Block(_ context.Context, entry *dto.StopListEntry) (*dto.StopListEntry, error) {
	s.calls++
	return entry, nil
}
func (s *createStub) Unblock(context.Context, string) error { return nil }

func TestNormalize(t *testing.T) {
	tests := []struct {
		kind, recipient, want string
	}{
		{"email", " User@Example.COM ", "user@example.com"},
		{"email", "Name <User@Example.COM>", "user@example.com"},
		{"phone", "+7 (999) 123-45-67", "+79991234567"},
		{"phone", "8 999 123 45 67", "+79991234567"},
		{"phone", "+12025550123", "+12025550123"},
		{"phone", "+442079460123", "+442079460123"},
		{"phone", "+852 2123 4567", "+85221234567"},
		{"phone", "+84 91234567", "+8491234567"},
	}
	for _, tt := range tests {
		got, err := Normalize(tt.kind, tt.recipient)
		if err != nil || got != tt.want {
			t.Fatalf("Normalize(%q, %q) = %q, %v; want %q", tt.kind, tt.recipient, got, err, tt.want)
		}
	}
}

func TestNormalizePhoneIsIdempotent(t *testing.T) {
	for _, recipient := range []string{"+85221234567", "+8491234567"} {
		canonical, err := Normalize(KindPhone, recipient)
		if err != nil {
			t.Fatal(err)
		}
		again, err := Normalize(KindPhone, canonical)
		if err != nil || again != canonical {
			t.Fatalf("Normalize(%q) = %q, %v", canonical, again, err)
		}
	}
}

func TestNormalizePhoneRejectsInvalidPlusPlacement(t *testing.T) {
	for _, recipient := range []string{"+7+9991234567", "7+9991234567", "01234567"} {
		_, err := Normalize(KindPhone, recipient)
		if err == nil {
			t.Fatalf("Normalize(%q) unexpectedly succeeded", recipient)
		}
	}
}

func TestCreateCountsReasonRunes(t *testing.T) {
	repo := &createStub{}
	service := New(repo)
	_, err := service.Create(context.Background(), &dto.StopListEntry{Kind: KindEmail, Recipient: "user@example.com", Reason: strings.Repeat("я", 1001)})
	if err != ErrReasonTooLong {
		t.Fatalf("Create() error = %v, want %v", err, ErrReasonTooLong)
	}
	if repo.calls != 0 {
		t.Fatal("repository called for invalid reason")
	}
}

func TestNormalizeEmailUsesRuneLimit(t *testing.T) {
	valid := strings.Repeat("x", 308) + "@example.com"
	if _, err := Normalize(KindEmail, valid); err != nil {
		t.Fatalf("valid address rejected: %v", err)
	}
	tooLong := strings.Repeat("x", 309) + "@example.com"
	if _, err := Normalize(KindEmail, tooLong); err == nil {
		t.Fatal("too long unicode address accepted")
	}
}

func TestFilterPhoneAliases(t *testing.T) {
	cases := []struct {
		name        string
		blocked     map[string]struct{}
		recipient   string
		wantBlocked bool
	}{
		{"international plus", map[string]struct{}{"+85221234567": {}}, "+85221234567", true},
		{"international bare", map[string]struct{}{"+85221234567": {}}, "85221234567", true},
		{"russian bare eight", map[string]struct{}{"+79991234567": {}}, "89991234567", true},
		{"russian explicit seven", map[string]struct{}{"+79991234567": {}}, "+79991234567", true},
		{"ambiguous bare eight raw candidate", map[string]struct{}{"+89991234567": {}}, "89991234567", true},
		{"explicit eight is not russian alias", map[string]struct{}{"+79991234567": {}}, "+89991234567", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			filtered, err := New(&createStub{blocked: tc.blocked}).Filter(context.Background(), []string{tc.recipient}, "sms", "order")
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantBlocked && len(filtered) != 0 {
				t.Fatalf("%q was not blocked", tc.recipient)
			}
			if !tc.wantBlocked && len(filtered) != 1 {
				t.Fatalf("%q was unexpectedly blocked", tc.recipient)
			}
		})
	}
}

func TestTaggedPreferenceRules(t *testing.T) {
	if denied(nil, []string{"missing"}, "email", "order") {
		t.Fatal("email.order should be the default")
	}
	if !denied(nil, []string{"missing"}, "email", "promotion") {
		t.Fatal("marketing must default off")
	}
	if !denied(map[string]dto.StopListEntry{"a": {Subscriptions: dto.SubscriptionList{}}}, []string{"a"}, "email", "order") {
		t.Fatal("explicit empty subscriptions must deny")
	}
	entries := map[string]dto.StopListEntry{"allowed": {Subscriptions: dto.SubscriptionList{"sms.order"}}, "blocked": {BlockedAll: true, Subscriptions: dto.SubscriptionList{"sms.order"}}}
	if !denied(entries, []string{"allowed", "blocked"}, "sms", "order") {
		t.Fatal("a blocked alias must dominate an allowed alias")
	}
}

func TestTaggedRecipientsFailClosed(t *testing.T) {
	_, err := New(&createStub{}).Filter(context.Background(), []string{"local-mailbox"}, "email", "order")
	if err != ErrInvalidRecipient {
		t.Fatalf("Filter() error = %v, want %v", err, ErrInvalidRecipient)
	}
	_, err = New(&createStub{}).Filter(context.Background(), []string{"user@example.com"}, "sms", "order")
	if err != ErrInvalidRecipient {
		t.Fatalf("Filter() error = %v, want %v", err, ErrInvalidRecipient)
	}
}

func TestSubscriptionCatalogMatchesValidation(t *testing.T) {
	require.Equal(t, []string{KindEmail, KindPhone}, RecipientKinds())
	for kind, subscriptions := range Subscriptions() {
		require.True(t, validSubscriptions(kind, subscriptions))
	}
	require.False(t, validSubscriptions(KindEmail, dto.SubscriptionList{"sms.order"}))
	require.False(t, validSubscriptions("unknown", nil))
}
