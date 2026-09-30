package sender

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/devian2011/retrier"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/internal/service/stoplist"
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
	stopList      *stoplist.Service
}

var errMalformedSMTPRecipients = errors.New("invalid SMTP recipients")

func NewSender(
	ctx context.Context,
	cfg *Config,
	pm pm,
	wm wm,
	tmplGenerator TemplateGenerator,
	stopList *stoplist.Service,
) *Sender {
	return &Sender{
		ctx:           ctx,
		cfg:           cfg,
		pm:            pm,
		wm:            wm,
		tmplGenerator: tmplGenerator,
		stopList:      stopList,
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

func (s *Sender) sendFunc(ctx context.Context, payload []byte) (string, *retrier.ExecutionError) {
	var msg dto.Message
	getMsgErr := sonic.Unmarshal(payload, &msg)
	if getMsgErr != nil {
		return "", &retrier.ExecutionError{
			Err: fmt.Errorf("error unmarshal message task payload: %s, err: %v",
				string(payload), getMsgErr),
			State: retrier.CriticalState,
		}
	}
	if msg.Tag != "" && !ValidTag(msg.Tag) {
		return "", &retrier.ExecutionError{Err: ErrInvalidTag, State: retrier.CriticalState}
	}
	if msg.Tag != "" {
		if _, err := s.channel(msg.Transport); err != nil {
			return "", &retrier.ExecutionError{Err: err, State: retrier.CriticalState}
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

	recipients, meta, allowedCount, filterErr := s.filterRecipients(ctx, &msg)
	if filterErr != nil {
		state := retrier.UsualState
		if errors.Is(filterErr, errMalformedSMTPRecipients) || errors.Is(filterErr, stoplist.ErrInvalidRecipient) || errors.Is(filterErr, stoplist.ErrInvalidEntry) {
			state = retrier.CriticalState
		}
		return "", &retrier.ExecutionError{Err: filterErr, State: state}
	}
	if allowedCount == 0 {
		return "", &retrier.ExecutionError{Err: fmt.Errorf("all recipients blocked by stop list or subscription preferences"), State: retrier.CriticalState}
	}

	msgParams, _ := sonic.Marshal(meta)

	result := prv.Send(&provider.Message{
		To:      recipients,
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

func (s *Sender) filterRecipients(ctx context.Context, msg *dto.Message) ([]string, dto.MessageMeta, int, error) {
	if s.stopList == nil {
		return msg.Recipients, msg.Meta, len(msg.Recipients), nil
	}
	recipients := append([]string(nil), msg.Recipients...)
	meta := msg.Meta
	isSMTP := s.cfg.Providers[msg.Transport] != nil && s.cfg.Providers[msg.Transport].Provider == "smtp"
	if isSMTP {
		meta = copyMeta(msg.Meta)
		for key, value := range meta {
			if isSMTPRecipientKey(key) {
				values, err := smtpRecipientSlice(value)
				if err != nil {
					return nil, nil, 0, fmt.Errorf("%w in %s", errMalformedSMTPRecipients, key)
				}
				recipients = append(recipients, values...)
			}
		}
	}
	channel := ""
	if msg.Tag != "" {
		var err error
		channel, err = s.channel(msg.Transport)
		if err != nil {
			return nil, nil, 0, err
		}
	}
	filtered, err := s.stopList.Filter(ctx, recipients, channel, msg.Tag)
	if err != nil {
		return nil, nil, 0, err
	}
	allowed := make(map[string]struct{}, len(filtered))
	for _, recipient := range filtered {
		allowed[recipient] = struct{}{}
	}
	to := filterList(msg.Recipients, allowed)
	if isSMTP && meta != nil {
		for key, value := range meta {
			if isSMTPRecipientKey(key) {
				values, err := smtpRecipientSlice(value)
				if err != nil {
					return nil, nil, 0, fmt.Errorf("%w in %s", errMalformedSMTPRecipients, key)
				}
				meta[key] = filterList(values, allowed)
			}
		}
	}
	if len(filtered) != len(recipients) {
		slog.Info("filtered stop-listed recipients", "count", len(recipients)-len(filtered))
	}
	return to, meta, len(filtered), nil
}

func (s *Sender) channel(transport string) (string, error) {
	return ResolveChannel(s.cfg.Providers[transport])
}

func isSMTPRecipientKey(key string) bool {
	return strings.EqualFold(key, "cc") || strings.EqualFold(key, "bcc")
}

func copyMeta(meta dto.MessageMeta) dto.MessageMeta {
	if meta == nil {
		return dto.MessageMeta{}
	}
	copy := make(dto.MessageMeta, len(meta))
	for key, value := range meta {
		copy[key] = value
	}
	return copy
}

func smtpRecipientSlice(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	values, ok := value.([]string)
	if ok {
		return values, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("not an array")
	}
	values = make([]string, 0, len(items))
	for _, item := range items {
		value, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("not a string")
		}
		values = append(values, value)
	}
	return values, nil
}

func filterList(values []string, allowed map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := allowed[value]; ok {
			result = append(result, value)
		}
	}
	return result
}

func (s *Sender) Run() {
	s.wm.Start()
}

func (s *Sender) Shutdown() {
	s.wm.Stop()
	s.pm.Close()
}
