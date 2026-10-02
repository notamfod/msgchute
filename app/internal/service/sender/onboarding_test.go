package sender

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/devian2011/msgchute/internal/dto"
)

func onboardingTestStore(t *testing.T) *OnboardingStore {
	t.Helper()
	store, err := NewOnboardingStore(nil, map[string]*ProviderConfig{
		"tg": {
			Provider:   "telegram",
			Onboarding: &OnboardingConfig{Enabled: true, BotID: "8665735492", SMSTransport: "beeline", ConnectURL: "https://mircli.ru/notification/connect/tg"},
		},
		"beeline": {Provider: "beeline", Channel: "sms"},
	})
	require.NoError(t, err)
	return store
}

func TestExplicitPhoneDoesNotGuessOpaqueMessengerIDs(t *testing.T) {
	tests := []struct {
		value string
		phone string
		ok    bool
	}{
		{value: "+7 (999) 123-45-67", phone: "+79991234567", ok: true},
		{value: "89991234567", phone: "+79991234567", ok: true},
		{value: "79991234567", phone: "+79991234567", ok: true},
		{value: "8665735492", ok: false},
		{value: "150633501", ok: false},
		{value: "@customer", ok: false},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			phone, ok := explicitPhone(test.value)
			require.Equal(t, test.ok, ok)
			require.Equal(t, test.phone, phone)
		})
	}
}

func TestOnboardingContentValidationAppliesOnlyToUnknownPhones(t *testing.T) {
	store := onboardingTestStore(t)
	store.resolver = func(_ context.Context, _ string, _ string, phone string, _ time.Time) (string, time.Time, error) {
		if phone == "+79991234567" {
			return "12345", time.Now(), nil
		}
		return "", time.Time{}, nil
	}

	known := &dto.Message{Transport: "tg", Recipients: dto.Recipients{"+79991234567"}, Meta: dto.MessageMeta{"attachments": []string{"invoice.pdf"}}}
	require.NoError(t, store.ValidateContent(context.Background(), known, "<b>subject</b>", "<b>body</b>"))

	unknown := &dto.Message{Transport: "tg", Recipients: dto.Recipients{"+79997654321"}}
	require.ErrorIs(t, store.ValidateContent(context.Background(), unknown, "", "<b>body</b>"), ErrUnsupportedOnboardingContent)
	unknown.Meta = dto.MessageMeta{"files": []string{"invoice.pdf"}}
	require.ErrorIs(t, store.ValidateContent(context.Background(), unknown, "plain", "plain"), ErrUnsupportedOnboardingContent)
	unknown.Meta = dto.MessageMeta{"format": "plain", "parse_mode": ""}
	require.NoError(t, store.ValidateContent(context.Background(), unknown, "plain", "plain"))
	unknown.Meta = dto.MessageMeta{"format": "markdown"}
	require.ErrorIs(t, store.ValidateContent(context.Background(), unknown, "plain", "plain"), ErrUnsupportedOnboardingContent)
	unknown.Meta = dto.MessageMeta{"Attachments": []string{"invoice.pdf"}}
	require.ErrorIs(t, store.ValidateContent(context.Background(), unknown, "plain", "plain"), ErrUnsupportedOnboardingContent)
	unknown.Meta = dto.MessageMeta{"Format": "markdown"}
	require.ErrorIs(t, store.ValidateContent(context.Background(), unknown, "plain", "plain"), ErrUnsupportedOnboardingContent)

	opaque := &dto.Message{Transport: "tg", Recipients: dto.Recipients{"@legacy"}, Meta: dto.MessageMeta{"files": []string{"legacy.bin"}}}
	require.NoError(t, store.ValidateContent(context.Background(), opaque, "<b>legacy</b>", "<b>legacy</b>"))
}

func TestOnboardingConfigMustReferenceConfiguredSMSTransport(t *testing.T) {
	tests := []map[string]*ProviderConfig{
		{
			"tg": {Provider: "telegram", Channel: "sms", Onboarding: &OnboardingConfig{Enabled: true, BotID: "1", SMSTransport: "tg", ConnectURL: "https://example.test/connect"}},
		},
		{
			"tg": {Provider: "telegram", Onboarding: &OnboardingConfig{Enabled: true, BotID: "1", SMSTransport: "missing", ConnectURL: "https://example.test/connect"}},
		},
		{
			"tg":      {Provider: "telegram", Onboarding: &OnboardingConfig{Enabled: true, BotID: "not-a-number", SMSTransport: "beeline", ConnectURL: "https://example.test/connect"}},
			"beeline": {Provider: "beeline"},
		},
		{
			"tg":      {Provider: "telegram", Onboarding: &OnboardingConfig{Enabled: true, BotID: "1", SMSTransport: "beeline", ConnectURL: "http://example.test/connect"}},
			"beeline": {Provider: "beeline"},
		},
		{
			"tg":   {Provider: "smtp", Onboarding: &OnboardingConfig{Enabled: true, BotID: "1", SMSTransport: "mail", ConnectURL: "https://example.test/connect"}},
			"mail": {Provider: "smtp"},
		},
	}
	for _, providers := range tests {
		_, err := NewOnboardingStore(nil, providers)
		require.ErrorIs(t, err, ErrInvalidOnboardingConfig)
	}
}

func TestSelectedFallbackRouteIsImmutable(t *testing.T) {
	store := onboardingTestStore(t)
	phone := "+79991234567"
	delivery := &onboardingDelivery{
		TaskID:       uuid.New(),
		Transport:    "tg",
		Recipient:    phone,
		Phone:        &phone,
		SelectedPath: "sms_fallback",
	}
	message := &dto.Message{Transport: "tg", Recipients: dto.Recipients{phone}, Subject: "subject", Body: "original"}
	payload, worker := store.routePayload(delivery, message)
	require.Equal(t, "beeline", worker)
	require.Equal(t, "beeline", payload.Transport)
	require.Equal(t, dto.Recipients{phone}, payload.Recipients)
	require.Equal(t, dto.Recipients{phone}, payload.RoutingRecipients)
	require.Empty(t, payload.Subject)
	require.Equal(t, "subject\noriginal", payload.Body)
}

func TestDistinctOnboardingRecipientsUsesCanonicalPhones(t *testing.T) {
	require.Equal(t,
		[]string{"+7 (999) 123-45-67", "@customer"},
		distinctOnboardingRecipients(dto.Recipients{"+7 (999) 123-45-67", "79991234567", "89991234567", "@customer", "@customer"}),
	)
}

func TestOnboardingResolverErrorsAreReturned(t *testing.T) {
	store := onboardingTestStore(t)
	want := errors.New("bindings unavailable")
	store.resolver = func(context.Context, string, string, string, time.Time) (string, time.Time, error) {
		return "", time.Time{}, want
	}
	err := store.ValidateContent(context.Background(), &dto.Message{Transport: "tg", Recipients: dto.Recipients{"+79991234567"}}, "", "plain")
	require.ErrorIs(t, err, want)
}
