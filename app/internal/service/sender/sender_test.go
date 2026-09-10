package sender

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/devian2011/retrier"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/pkg/shared/provider"
)

type MockProviderManager struct {
	mock.Mock
}

func (m *MockProviderManager) GetProvider(code string) (provider.Provider, error) {
	args := m.Called(code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(provider.Provider), args.Error(1)
}

func (m *MockProviderManager) BuildPlugin(name string, code string, params []byte) error {
	args := m.Called(name, code, params)
	return args.Error(0)
}

func (m *MockProviderManager) Close() {
	m.Called()
}

type MockWorkerManager struct {
	mock.Mock
}

func (m *MockWorkerManager) RegisterWorker(name string, w retrier.ManagerWorker, b retrier.Breaker) error {
	args := m.Called(name, w, b)
	return args.Error(0)
}

func (m *MockWorkerManager) Start() {
	m.Called()
}

func (m *MockWorkerManager) Stop() {
	m.Called()
}

type MockTemplateGenerator struct {
	mock.Mock
}

func (m *MockTemplateGenerator) GenerateMessage(msg *dto.Message) (subject string, body string, err error) {
	args := m.Called(msg)
	return args.String(0), args.String(1), args.Error(2)
}

type MockProvider struct {
	mock.Mock
}

func (m *MockProvider) Configure(params []byte) error {
	args := m.Called(params)
	return args.Error(0)
}

func (m *MockProvider) Send(msg *provider.Message) *provider.MessageResponse {
	args := m.Called(msg)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*provider.MessageResponse)
}

func TestSender_Init(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Providers: map[string]*ProviderConfig{
			"email": {
				RetrierSettings: RetrierSettings{
					Breaker: struct {
						WindowSize       time.Duration `yaml:"windowSize"`
						MinRequests      int           `yaml:"minRequests"`
						FailureThreshold float64       `yaml:"failureThreshold"`
						Timeout          time.Duration `yaml:"timeout"`
					}{
						WindowSize:       10 * time.Second,
						FailureThreshold: 0.5,
						MinRequests:      5,
						Timeout:          30 * time.Second,
					},
				},
			},
			"sms": {
				RetrierSettings: RetrierSettings{
					Breaker: struct {
						WindowSize       time.Duration `yaml:"windowSize"`
						MinRequests      int           `yaml:"minRequests"`
						FailureThreshold float64       `yaml:"failureThreshold"`
						Timeout          time.Duration `yaml:"timeout"`
					}{
						WindowSize:       20 * time.Second,
						FailureThreshold: 0.3,
						MinRequests:      3,
						Timeout:          60 * time.Second,
					},
				},
			},
		},
	}

	pm := new(MockProviderManager)
	wm := new(MockWorkerManager)
	tmplGen := new(MockTemplateGenerator)

	pm.On("BuildPlugin", "email", "", mock.Anything).Return(nil).Once()
	pm.On("BuildPlugin", "sms", "", mock.Anything).Return(nil).Once()

	wm.On("RegisterWorker", "email", mock.Anything, mock.Anything).Return(nil).Once()
	wm.On("RegisterWorker", "sms", mock.Anything, mock.Anything).Return(nil).Once()

	sender := NewSender(ctx, cfg, pm, wm, tmplGen)
	err := sender.Init()
	assert.NoError(t, err)
	wm.AssertExpectations(t)
	pm.AssertExpectations(t)
}

func TestSender_Init_Error(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Providers: map[string]*ProviderConfig{
			"email": {
				RetrierSettings: RetrierSettings{
					Breaker: struct {
						WindowSize       time.Duration `yaml:"windowSize"`
						MinRequests      int           `yaml:"minRequests"`
						FailureThreshold float64       `yaml:"failureThreshold"`
						Timeout          time.Duration `yaml:"timeout"`
					}{
						WindowSize:       10 * time.Second,
						FailureThreshold: 0.5,
						MinRequests:      5,
						Timeout:          30 * time.Second,
					},
				},
			},
		},
	}

	pm := new(MockProviderManager)
	wm := new(MockWorkerManager)
	tmplGen := new(MockTemplateGenerator)

	pm.On("BuildPlugin", "email", "", mock.Anything).Return(nil).Once()
	wm.On("RegisterWorker", "email", mock.Anything, mock.Anything).Return(errors.New("registration failed")).Once()

	sender := NewSender(ctx, cfg, pm, wm, tmplGen)
	err := sender.Init()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "registration failed")
	wm.AssertExpectations(t)
	pm.AssertExpectations(t)
}

func TestSender_sendFunc(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{}
	pm := new(MockProviderManager)
	wm := new(MockWorkerManager)
	tmplGen := new(MockTemplateGenerator)

	sender := NewSender(ctx, cfg, pm, wm, tmplGen)
	sendFunc := sender.sendFunc

	t.Run("success", func(t *testing.T) {
		msg := &dto.Message{
			Transport:  "email",
			Recipients: dto.Recipients{"test@example.com"},
			Meta:       dto.MessageMeta{"key": "value"},
			Subject:    "Hello",
			Body:       "World",
		}
		payload, _ := sonic.Marshal(msg)

		tmplGen.On("GenerateMessage", msg).Return("Hello", "World", nil).Once()
		prv := new(MockProvider)
		prv.On("Send", mock.Anything).Return(&provider.MessageResponse{
			Response: "OK",
			Err:      nil,
		}).Once()
		pm.On("GetProvider", "email").Return(prv, nil).Once()

		resp, execErr := sendFunc(ctx, payload)
		assert.Equal(t, "OK", resp)
		assert.Nil(t, execErr)
		tmplGen.AssertExpectations(t)
		pm.AssertExpectations(t)
		prv.AssertExpectations(t)
	})

	t.Run("task unmarshal error", func(t *testing.T) {
		payload := []byte("invalid json")
		resp, execErr := sendFunc(ctx, payload)
		assert.Empty(t, resp)
		require.NotNil(t, execErr)
		assert.Equal(t, retrier.CriticalState, execErr.State)
		assert.Contains(t, execErr.Err.Error(), "error unmarshal message task payload")
	})

	t.Run("get provider error", func(t *testing.T) {
		msg := &dto.Message{
			Transport: "unknown",
		}
		payload, _ := sonic.Marshal(msg)

		pm.On("GetProvider", "unknown").Return(nil, errors.New("provider not found")).Once()

		resp, execErr := sendFunc(ctx, payload)
		assert.Empty(t, resp)
		require.NotNil(t, execErr)
		assert.Equal(t, retrier.CriticalState, execErr.State)
		assert.Contains(t, execErr.Err.Error(), "unknown provider")
		pm.AssertExpectations(t)
	})

	t.Run("message unmarshal error", func(t *testing.T) {
		// уже есть тест на invalid json, этот можно убрать
	})

	t.Run("template generation error", func(t *testing.T) {
		msg := &dto.Message{
			Transport: "email",
		}
		payload, _ := sonic.Marshal(msg)

		tmplGen.On("GenerateMessage", msg).Return("", "", errors.New("template error")).Once()
		prv := new(MockProvider)
		pm.On("GetProvider", "email").Return(prv, nil).Once()

		resp, execErr := sendFunc(ctx, payload)
		assert.Empty(t, resp)
		require.NotNil(t, execErr)
		assert.Equal(t, retrier.CriticalState, execErr.State)
		assert.Contains(t, execErr.Err.Error(), "error generate task message payload")
		assert.ErrorContains(t, execErr.Err, "template error")
		tmplGen.AssertExpectations(t)
		pm.AssertExpectations(t)
	})

	t.Run("provider send error (critical)", func(t *testing.T) {
		msg := &dto.Message{
			Transport:  "email",
			Recipients: dto.Recipients{"test@example.com"},
			Meta:       dto.MessageMeta{},
		}
		payload, _ := sonic.Marshal(msg)

		tmplGen.On("GenerateMessage", msg).Return("", "", nil).Once()
		prv := new(MockProvider)
		prv.On("Send", mock.Anything).Return(&provider.MessageResponse{
			Err:        errors.New("send failed"),
			IsCritical: true,
		}).Once()
		pm.On("GetProvider", "email").Return(prv, nil).Once()

		resp, execErr := sendFunc(ctx, payload)
		assert.Empty(t, resp)
		require.NotNil(t, execErr)
		assert.Equal(t, retrier.CriticalState, execErr.State)
		assert.Contains(t, execErr.Err.Error(), "error on message send")
		tmplGen.AssertExpectations(t)
		pm.AssertExpectations(t)
		prv.AssertExpectations(t)
	})

	t.Run("provider send error (usual)", func(t *testing.T) {
		msg := &dto.Message{
			Transport:  "email",
			Recipients: dto.Recipients{"test@example.com"},
		}
		payload, _ := sonic.Marshal(msg)

		tmplGen.On("GenerateMessage", msg).Return("", "", nil).Once()
		prv := new(MockProvider)
		prv.On("Send", mock.Anything).Return(&provider.MessageResponse{
			Err:        errors.New("send failed"),
			IsCritical: false,
		}).Once()
		pm.On("GetProvider", "email").Return(prv, nil).Once()

		resp, execErr := sendFunc(ctx, payload)
		assert.Empty(t, resp)
		require.NotNil(t, execErr)
		assert.Equal(t, retrier.UsualState, execErr.State)
		assert.Contains(t, execErr.Err.Error(), "error on message send")
		tmplGen.AssertExpectations(t)
		pm.AssertExpectations(t)
		prv.AssertExpectations(t)
	})
}

func TestSender_Run(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{}
	pm := new(MockProviderManager)
	wm := new(MockWorkerManager)
	tmplGen := new(MockTemplateGenerator)

	wm.On("Start").Once()

	sender := NewSender(ctx, cfg, pm, wm, tmplGen)
	sender.Run()
	wm.AssertExpectations(t)
}

func TestSender_Shutdown(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{}
	pm := new(MockProviderManager)
	wm := new(MockWorkerManager)
	tmplGen := new(MockTemplateGenerator)

	wm.On("Stop").Once()
	pm.On("Close").Once()

	sender := NewSender(ctx, cfg, pm, wm, tmplGen)
	sender.Shutdown()
	wm.AssertExpectations(t)
	pm.AssertExpectations(t)
}
