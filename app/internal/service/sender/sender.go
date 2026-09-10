package sender

import (
	"context"
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/devian2011/retrier"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/pkg/shared/provider"
)

type TemplateGenerator interface {
	GenerateMessage(*dto.Message) (subject string, body string, err error)
}

type pm interface {
	GetProvider(code string) (provider.Provider, error)
	BuildPlugin(name string, code string, params []byte) error
	Close()
}

type wm interface {
	RegisterWorker(name string, w retrier.ManagerWorker, b retrier.Breaker) error
	Start()
	Stop()
}

type Sender struct {
	ctx           context.Context
	cfg           *Config
	pm            pm
	wm            wm
	tmplGenerator TemplateGenerator
}

func NewSender(
	ctx context.Context,
	cfg *Config,
	pm pm,
	wm wm,
	tmplGenerator TemplateGenerator,
) *Sender {
	return &Sender{
		ctx:           ctx,
		cfg:           cfg,
		pm:            pm,
		wm:            wm,
		tmplGenerator: tmplGenerator,
	}
}

func (s *Sender) Init() error {
	for pName, pCfg := range s.cfg.Providers {
		pluginParams, _ := sonic.Marshal(pCfg.Params)
		bPluginErr := s.pm.BuildPlugin(pName, pCfg.Provider, pluginParams)
		if bPluginErr != nil {
			return bPluginErr
		}

		wConfig, wConfigErr := retrier.NewWorkerCfg(
			int32(pCfg.RetrierSettings.Workers.Min),
			int32(pCfg.RetrierSettings.Workers.Max),
			5*time.Second)

		if wConfigErr != nil {
			return fmt.Errorf("worker config error: %w", wConfigErr)
		}

		w, wInitErr := retrier.NewWorker(s.ctx, wConfig, s.sendFunc)
		if wInitErr != nil {
			return fmt.Errorf("worker init error: %w", wInitErr)
		}

		regErr := s.wm.RegisterWorker(
			pName,
			w,
			retrier.NewSlidingWindowCircuitBreaker(
				pCfg.RetrierSettings.Breaker.WindowSize,
				pCfg.RetrierSettings.Breaker.FailureThreshold,
				pCfg.RetrierSettings.Breaker.MinRequests,
				pCfg.RetrierSettings.Breaker.Timeout,
			),
		)
		if regErr != nil {
			return regErr
		}
	}

	return nil
}

func (s *Sender) sendFunc(_ context.Context, payload []byte) (string, *retrier.ExecutionError) {
	var msg dto.Message
	getMsgErr := sonic.Unmarshal(payload, &msg)
	if getMsgErr != nil {
		return "", &retrier.ExecutionError{
			Err: fmt.Errorf("error unmarshal message task payload: %s, err: %v",
				string(payload), getMsgErr),
			State: retrier.CriticalState,
		}
	}

	prv, getProviderErr := s.pm.GetProvider(msg.Transport)
	if getProviderErr != nil {
		return "", &retrier.ExecutionError{
			Err: fmt.Errorf("unknown provider: %s payload: %s, err: %v",
				msg.Transport, string(payload), getProviderErr),
			State: retrier.CriticalState,
		}
	}

	subject, body, generateErr := s.tmplGenerator.GenerateMessage(&msg)
	if generateErr != nil {
		return "", &retrier.ExecutionError{
			Err: fmt.Errorf("error generate task message payload: %s, err: %v",
				string(payload), generateErr),
			State: retrier.CriticalState,
		}
	}

	msgParams, _ := sonic.Marshal(msg.Meta)

	result := prv.Send(&provider.Message{
		To:      msg.Recipients,
		Params:  msgParams,
		Subject: subject,
		Body:    body,
	})

	if result.Err != nil {
		responseErr := &retrier.ExecutionError{
			Err: fmt.Errorf("error on message send: %s, err: %v",
				string(payload), result.Err),
			State: retrier.UsualState,
		}
		if result.IsCritical {
			responseErr.State = retrier.CriticalState
		}
		return "", responseErr
	}

	return result.Response, nil
}

func (s *Sender) Run() {
	s.wm.Start()
}

func (s *Sender) Shutdown() {
	s.wm.Stop()
	s.pm.Close()
}
