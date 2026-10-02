package admin

import (
	"context"
	"testing"

	"github.com/devian2011/msgchute/internal/service/sender"
	"github.com/stretchr/testify/require"
)

type messageDictionaryGetterStub struct {
	transportsCalled bool
}

func (s *messageDictionaryGetterStub) GetSenders(context.Context) ([]string, error) {
	return []string{"sender"}, nil
}

func (s *messageDictionaryGetterStub) GetTransports(context.Context) ([]string, error) {
	s.transportsCalled = true
	return []string{"ya-mail", "yamail"}, nil
}

func (s *messageDictionaryGetterStub) GetTemplates(context.Context) ([]string, error) {
	return []string{"template"}, nil
}

func TestMessageDictionaryHandlerUsesConfiguredTransports(t *testing.T) {
	getter := &messageDictionaryGetterStub{}
	handler := NewMessageDictionaryHandler(getter, []sender.Transport{
		{Code: "beeline"},
		{Code: "ya_mail"},
	})

	result, err := handler.Handle(context.Background())

	require.NoError(t, err)
	require.Equal(t, []string{"sender"}, result.SenderIDs)
	require.Equal(t, []string{"beeline", "ya_mail"}, result.Transports)
	require.Equal(t, []string{"template"}, result.Templates)
	require.False(t, getter.transportsCalled)
}
