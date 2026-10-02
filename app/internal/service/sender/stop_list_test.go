package sender

import (
	"context"
	"errors"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/devian2011/retrier"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/internal/service/stoplist"
	"github.com/devian2011/msgchute/pkg/shared/provider"
)

type stopListStub struct {
	blocked     map[string]struct{}
	preferences map[string]dto.StopListEntry
	err         error
}

type recordingProvider struct{ calls int }

func (p *recordingProvider) Configure([]byte) error { return nil }
func (p *recordingProvider) Send(*provider.Message) *provider.MessageResponse {
	p.calls++
	return &provider.MessageResponse{Response: "sent"}
}

func (s *stopListStub) Create(context.Context, *dto.StopListEntry) (*dto.StopListEntry, error) {
	return nil, nil
}
func (s *stopListStub) Delete(context.Context, string) error { return nil }
func (s *stopListStub) Find(context.Context, dto.StopListFilter) ([]dto.StopListEntry, uint64, error) {
	return nil, 0, nil
}
func (s *stopListStub) Blocked(_ context.Context, _ string, recipients []string) (map[string]struct{}, error) {
	if s.err != nil {
		return nil, s.err
	}
	result := map[string]struct{}{}
	for _, recipient := range recipients {
		if _, ok := s.blocked[recipient]; ok {
			result[recipient] = struct{}{}
		}
	}
	return result, nil
}
func (s *stopListStub) Preferences(_ context.Context, _ string, recipients []string) (map[string]dto.StopListEntry, error) {
	if s.err != nil {
		return nil, s.err
	}
	result := map[string]dto.StopListEntry{}
	for _, recipient := range recipients {
		if entry, ok := s.preferences[recipient]; ok {
			result[recipient] = entry
			continue
		}
		if _, ok := s.blocked[recipient]; ok {
			result[recipient] = dto.StopListEntry{Recipient: recipient, BlockedAll: true}
		}
	}
	return result, nil
}
func (s *stopListStub) List(context.Context, dto.StopListFilter) ([]dto.StopListEntry, uint64, error) {
	return nil, 0, nil
}
func (s *stopListStub) Upsert(context.Context, *dto.StopListEntry) (*dto.StopListEntry, error) {
	return nil, nil
}
func (s *stopListStub) Block(_ context.Context, entry *dto.StopListEntry) (*dto.StopListEntry, error) {
	return entry, nil
}
func (s *stopListStub) Unblock(context.Context, string) error { return nil }

func TestSenderStopList(t *testing.T) {
	stub := &stopListStub{blocked: map[string]struct{}{"blocked@example.com": {}, "+79991234567": {}}}
	pm, wm, templates := new(MockProviderManager), new(MockWorkerManager), new(MockTemplateGenerator)
	s := NewSender(context.Background(), &Config{Providers: map[string]*ProviderConfig{"mail": {Provider: "smtp"}}}, pm, wm, templates, stoplist.New(stub))
	message := func() *dto.Message {
		return &dto.Message{Transport: "mail", Recipients: dto.Recipients{"ok@example.com", "blocked@example.com"}, Meta: dto.MessageMeta{"cc": []string{"cc@example.com", "blocked@example.com"}, "bcc": []string{"+7 (999) 123-45-67"}}}
	}

	t.Run("mixed filters to cc and bcc", func(t *testing.T) {
		msg := message()
		payload, _ := sonic.Marshal(msg)
		templates.On("GenerateMessage", mock.Anything).Return("subject", "body", nil).Once()
		providerStub := new(MockProvider)
		providerStub.On("Send", assertRecipientSet(t, []string{"ok@example.com"}, []string{"cc@example.com"}, []string{})).Return(&provider.MessageResponse{Response: "sent"}).Once()
		pm.On("GetProvider", "mail").Return(providerStub, nil).Once()
		response, err := s.sendFunc(context.Background(), payload)
		require.Nil(t, err)
		assert.Equal(t, "sent", response)
		providerStub.AssertExpectations(t)
	})

	t.Run("all blocked is terminal", func(t *testing.T) {
		msg := &dto.Message{Transport: "mail", Recipients: dto.Recipients{"blocked@example.com"}}
		payload, _ := sonic.Marshal(msg)
		templates.On("GenerateMessage", mock.Anything).Return("", "", nil).Once()
		pm.On("GetProvider", "mail").Return(new(MockProvider), nil).Once()
		_, err := s.sendFunc(context.Background(), payload)
		require.NotNil(t, err)
		assert.Equal(t, retrier.CriticalState, err.State)
		assert.ErrorContains(t, err.Err, "all recipients blocked by stop list")
	})

	t.Run("database failure retries and does not send", func(t *testing.T) {
		stub.err = errors.New("database unavailable")
		msg := message()
		payload, _ := sonic.Marshal(msg)
		templates.On("GenerateMessage", mock.Anything).Return("", "", nil).Once()
		pm.On("GetProvider", "mail").Return(new(MockProvider), nil).Once()
		_, err := s.sendFunc(context.Background(), payload)
		require.NotNil(t, err)
		assert.Equal(t, retrier.UsualState, err.State)
		assert.ErrorContains(t, err.Err, "check stop list")
		stub.err = nil
	})

	t.Run("lookup is fresh for each attempt", func(t *testing.T) {
		stub.blocked = map[string]struct{}{}
		msg := &dto.Message{Transport: "mail", Recipients: dto.Recipients{"fresh@example.com"}}
		payload, _ := sonic.Marshal(msg)
		templates.On("GenerateMessage", mock.Anything).Return("", "", nil).Twice()
		first := new(MockProvider)
		first.On("Send", assertRecipientSet(t, []string{"fresh@example.com"}, nil, nil)).Return(&provider.MessageResponse{Response: "sent"}).Once()
		pm.On("GetProvider", "mail").Return(first, nil).Once()
		_, err := s.sendFunc(context.Background(), payload)
		require.Nil(t, err)
		stub.blocked["fresh@example.com"] = struct{}{}
		pm.On("GetProvider", "mail").Return(new(MockProvider), nil).Once()
		_, err = s.sendFunc(context.Background(), payload)
		require.NotNil(t, err)
		assert.Equal(t, retrier.CriticalState, err.State)
	})

	t.Run("case-insensitive bcc remains deliverable when to is blocked", func(t *testing.T) {
		stub.blocked = map[string]struct{}{"blocked@example.com": {}}
		msg := &dto.Message{Transport: "mail", Recipients: dto.Recipients{"blocked@example.com"}, Meta: dto.MessageMeta{"BCC": []string{"allowed@example.com"}}}
		payload, _ := sonic.Marshal(msg)
		templates.On("GenerateMessage", mock.Anything).Return("", "", nil).Once()
		providerStub := new(MockProvider)
		providerStub.On("Send", mock.MatchedBy(func(message *provider.Message) bool {
			var meta dto.MessageMeta
			require.NoError(t, sonic.Unmarshal(message.Params, &meta))
			return assert.Empty(t, message.To) && assert.Equal(t, []string{"allowed@example.com"}, metaStrings(meta["BCC"]))
		})).Return(&provider.MessageResponse{Response: "sent"}).Once()
		pm.On("GetProvider", "mail").Return(providerStub, nil).Once()
		_, err := s.sendFunc(context.Background(), payload)
		require.Nil(t, err)
		providerStub.AssertExpectations(t)
	})

	t.Run("malformed smtp cc or bcc fails closed", func(t *testing.T) {
		stub.blocked = map[string]struct{}{"blocked@example.com": {}}
		for _, meta := range []dto.MessageMeta{{"cc": []any{"blocked@example.com", nil}}, {"bcc": []any{"blocked@example.com", 1}}} {
			payload, _ := sonic.Marshal(&dto.Message{Transport: "mail", Recipients: dto.Recipients{"allowed@example.com"}, Meta: meta})
			templates.On("GenerateMessage", mock.Anything).Return("", "", nil).Once()
			providerStub := &recordingProvider{}
			pm.On("GetProvider", "mail").Return(providerStub, nil).Once()
			_, err := s.sendFunc(context.Background(), payload)
			require.NotNil(t, err)
			assert.Equal(t, retrier.CriticalState, err.State)
			assert.Zero(t, providerStub.calls)
		}
	})

	t.Run("untagged custom transport remains compatible", func(t *testing.T) {
		custom := NewSender(context.Background(), &Config{Providers: map[string]*ProviderConfig{"custom": {Provider: "custom"}}}, pm, wm, templates, stoplist.New(stub))
		providerStub := &recordingProvider{}
		pm.On("GetProvider", "custom").Return(providerStub, nil).Once()
		templates.On("GenerateMessage", mock.Anything).Return("", "", nil).Once()
		payload, _ := sonic.Marshal(&dto.Message{Transport: "custom", Recipients: dto.Recipients{"opaque-recipient"}})
		_, err := custom.sendFunc(context.Background(), payload)
		require.Nil(t, err)
		assert.Equal(t, 1, providerStub.calls)
	})

	t.Run("tagged custom transport is terminal", func(t *testing.T) {
		custom := NewSender(context.Background(), &Config{Providers: map[string]*ProviderConfig{"custom": {Provider: "custom"}}}, pm, wm, templates, stoplist.New(stub))
		payload, _ := sonic.Marshal(&dto.Message{Transport: "custom", Recipients: dto.Recipients{"opaque-recipient"}, Tag: "order"})
		_, err := custom.sendFunc(context.Background(), payload)
		require.NotNil(t, err)
		assert.Equal(t, retrier.CriticalState, err.State)
		assert.ErrorIs(t, err.Err, ErrUnknownPreferenceChannel)
	})

	t.Run("routed messenger recipient is checked by original phone", func(t *testing.T) {
		stub.blocked = map[string]struct{}{"+79991234567": {}}
		providerStub := &recordingProvider{}
		pm.On("GetProvider", "tg").Return(providerStub, nil).Once()
		templates.On("GenerateMessage", mock.Anything).Return("", "", nil).Once()
		routed := &dto.Message{
			Transport:         "tg",
			Recipients:        dto.Recipients{"123456789"},
			RoutingRecipients: dto.Recipients{"+79991234567"},
		}
		payload, _ := sonic.Marshal(routed)
		_, err := NewSender(context.Background(), &Config{Providers: map[string]*ProviderConfig{"tg": {Provider: "telegram"}}}, pm, wm, templates, stoplist.New(stub)).sendFunc(context.Background(), payload)
		require.NotNil(t, err)
		assert.Equal(t, retrier.CriticalState, err.State)
		assert.Zero(t, providerStub.calls)
	})

	t.Run("tagged invitation uses SMS subscriptions", func(t *testing.T) {
		phone := "+79991234567"
		stub.blocked = map[string]struct{}{}
		stub.preferences = map[string]dto.StopListEntry{
			phone: {Recipient: phone, Subscriptions: dto.SubscriptionList{"sms.order"}},
		}
		providerStub := &recordingProvider{}
		pm.On("GetProvider", "beeline").Return(providerStub, nil).Once()
		templates.On("GenerateMessage", mock.Anything).Return("", "", nil).Once()
		store := onboardingTestStore(t)
		invitation := store.invitationPayload(&onboardingInvitation{Phone: phone, SMSTransport: "beeline", ConnectURL: "https://example.test/connect"}, &dto.Message{Tag: "promotion"})
		payload, marshalErr := sonic.Marshal(invitation)
		require.NoError(t, marshalErr)
		_, sendErr := NewSender(context.Background(), &Config{Providers: map[string]*ProviderConfig{"beeline": {Provider: "beeline"}}}, pm, wm, templates, stoplist.New(stub)).sendFunc(context.Background(), payload)
		require.NotNil(t, sendErr)
		assert.Equal(t, retrier.CriticalState, sendErr.State)
		assert.Zero(t, providerStub.calls)
		stub.preferences = nil
	})

	t.Run("invalid tagged recipient is terminal and not sent", func(t *testing.T) {
		providerStub := &recordingProvider{}
		pm.On("GetProvider", "mail").Return(providerStub, nil).Once()
		templates.On("GenerateMessage", mock.Anything).Return("", "", nil).Once()
		payload, _ := sonic.Marshal(&dto.Message{Transport: "mail", Recipients: dto.Recipients{"local-mailbox"}, Tag: "order"})
		_, err := s.sendFunc(context.Background(), payload)
		require.NotNil(t, err)
		assert.Equal(t, retrier.CriticalState, err.State)
		assert.ErrorIs(t, err.Err, stoplist.ErrInvalidRecipient)
		assert.Zero(t, providerStub.calls)
	})
}

func assertRecipientSet(t *testing.T, to, cc, bcc []string) interface{} {
	return mock.MatchedBy(func(message *provider.Message) bool {
		assert.Equal(t, to, []string(message.To))
		var meta dto.MessageMeta
		require.NoError(t, sonic.Unmarshal(message.Params, &meta))
		if len(cc) == 0 {
			assert.Empty(t, metaStrings(meta["cc"]))
		} else {
			assert.Equal(t, cc, metaStrings(meta["cc"]))
		}
		if len(bcc) == 0 {
			assert.Empty(t, metaStrings(meta["bcc"]))
		} else {
			assert.Equal(t, bcc, metaStrings(meta["bcc"]))
		}
		return true
	})
}
func metaStrings(value any) []string {
	values, _ := value.([]any)
	result := make([]string, len(values))
	for i := range values {
		result[i], _ = values[i].(string)
	}
	return result
}
