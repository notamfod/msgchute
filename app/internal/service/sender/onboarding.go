package sender

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/devian2011/retrier"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/internal/io/storage"
	"github.com/devian2011/msgchute/internal/service/stoplist"
	"github.com/devian2011/msgchute/pkg/generate"
)

const (
	onboardingWait         = time.Hour
	onboardingPollInterval = 15 * time.Second
	invitationMaxRetries   = 3
	invitationLifetime     = 10 * time.Minute
)

var (
	ErrInvalidOnboardingConfig      = errors.New("invalid messenger onboarding configuration")
	ErrUnsupportedOnboardingContent = errors.New("messenger onboarding SMS fallback supports plain text without attachments")
	markupPattern                   = regexp.MustCompile(`(?i)<[/!]?[a-z][^>]*>`)
)

type onboardingDB interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	GetContext(context.Context, any, string, ...any) error
	SelectContext(context.Context, any, string, ...any) error
}

type onboardingDelivery struct {
	TaskID            uuid.UUID  `db:"task_id"`
	MessageID         uuid.UUID  `db:"message_id"`
	Transport         string     `db:"transport"`
	BotID             string     `db:"bot_id"`
	Recipient         string     `db:"recipient"`
	Phone             *string    `db:"phone"`
	SelectedPath      string     `db:"selected_path"`
	ResolvedRecipient *string    `db:"resolved_recipient"`
	InvitationID      *uuid.UUID `db:"invitation_id"`
	WaitUntil         *time.Time `db:"wait_until"`
}

type onboardingInvitation struct {
	ID           uuid.UUID  `db:"id"`
	TaskID       uuid.UUID  `db:"task_id"`
	MessageID    uuid.UUID  `db:"message_id"`
	Transport    string     `db:"transport"`
	BotID        string     `db:"bot_id"`
	Phone        string     `db:"phone"`
	SMSTransport string     `db:"sms_transport"`
	ConnectURL   string     `db:"connect_url"`
	SentAt       *time.Time `db:"sent_at"`
	FinishedAt   *time.Time `db:"finished_at"`
	TaskStatus   string     `db:"task_status"`
}

type OnboardingStore struct {
	db        *sqlx.DB
	providers map[string]*ProviderConfig
	now       func() time.Time
	resolver  func(context.Context, string, string, string, time.Time) (string, time.Time, error)
}

func NewOnboardingStore(db *sqlx.DB, providers map[string]*ProviderConfig) (*OnboardingStore, error) {
	store := &OnboardingStore{db: db, providers: providers, now: time.Now}
	for code, providerConfig := range providers {
		if !onboardingEnabled(providerConfig) {
			continue
		}
		botID, botErr := strconv.ParseUint(providerConfig.Onboarding.BotID, 10, 64)
		connectURL, urlErr := url.Parse(providerConfig.Onboarding.ConnectURL)
		if botErr != nil || botID == 0 || urlErr != nil || connectURL.Scheme != "https" || connectURL.Host == "" || providerConfig.Onboarding.SMSTransport == "" {
			return nil, fmt.Errorf("%w for %s", ErrInvalidOnboardingConfig, code)
		}
		if providerConfig.Onboarding.SMSTransport == code {
			return nil, fmt.Errorf("%w for %s: SMS transport must use another profile", ErrInvalidOnboardingConfig, code)
		}
		smsConfig := providers[providerConfig.Onboarding.SMSTransport]
		channel, channelErr := ResolveChannel(smsConfig)
		if channelErr != nil || channel != "sms" {
			return nil, fmt.Errorf("%w for %s: unknown SMS transport", ErrInvalidOnboardingConfig, code)
		}
		if bindingTable(code, providerConfig) == "" {
			return nil, fmt.Errorf("%w for %s: unsupported messenger", ErrInvalidOnboardingConfig, code)
		}
	}
	return store, nil
}

func onboardingEnabled(config *ProviderConfig) bool {
	return config != nil && config.Onboarding != nil && config.Onboarding.Enabled
}

func (s *OnboardingStore) Enabled(transport string) bool {
	return s != nil && onboardingEnabled(s.providers[transport])
}

func (s *OnboardingStore) database(ctx context.Context) onboardingDB {
	if tx := storage.ExtractTx(ctx); tx != nil {
		return tx
	}
	return s.db
}

func bindingTable(_ string, config *ProviderConfig) string {
	providerName := ""
	if config != nil {
		providerName = strings.ToLower(config.Provider)
	}
	switch providerName {
	case "telegram":
		return "telegram_contact_bindings"
	case "max":
		return "max_contact_bindings"
	default:
		return ""
	}
}

func explicitPhone(recipient string) (string, bool) {
	trimmed := strings.TrimSpace(recipient)
	phone, err := stoplist.Normalize(stoplist.KindPhone, trimmed)
	if err != nil {
		return "", false
	}
	if strings.HasPrefix(trimmed, "+") {
		return phone, true
	}
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, trimmed)
	if len(digits) == 11 && (digits[0] == '7' || digits[0] == '8') {
		return phone, true
	}
	return "", false
}

func (s *OnboardingStore) CreateDelivery(ctx context.Context, task *dto.Task, message *dto.Message, recipient string) error {
	config := s.providers[message.Transport]
	phone, ok := explicitPhone(recipient)
	var phoneValue any
	path := "legacy"
	if ok && fallbackSafeContent(message.Subject, message.Body, message.Meta) {
		phoneValue = phone
		path = "pending"
	}
	_, err := s.database(ctx).ExecContext(ctx, `
		INSERT INTO message_onboarding_deliveries
		(task_id, message_id, transport, bot_id, recipient, phone, selected_path, selected_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7::varchar, CASE WHEN $7::varchar = 'legacy' THEN CURRENT_TIMESTAMP ELSE NULL END)`,
		task.ID, message.ID, message.Transport, config.Onboarding.BotID, recipient, phoneValue, path)
	return err
}

func (s *OnboardingStore) ValidateContent(ctx context.Context, message *dto.Message, subject, body string) error {
	if !s.Enabled(message.Transport) {
		return nil
	}
	for _, recipient := range message.Recipients {
		phone, ok := explicitPhone(recipient)
		if !ok {
			continue
		}
		resolved, _, err := s.resolveBinding(ctx, message.Transport, s.providers[message.Transport].Onboarding.BotID, phone, s.now())
		if err != nil {
			return err
		}
		if resolved != "" {
			continue
		}
		if !fallbackSafeContent(subject, body, message.Meta) {
			return ErrUnsupportedOnboardingContent
		}
	}
	return nil
}

func fallbackSafeContent(subject, body string, meta dto.MessageMeta) bool {
	for key, value := range meta {
		switch {
		case strings.EqualFold(key, "attachment"), strings.EqualFold(key, "attachments"), strings.EqualFold(key, "file"), strings.EqualFold(key, "files"):
			return false
		case strings.EqualFold(key, "format"):
			if !plainFormat(value) {
				return false
			}
		case strings.EqualFold(key, "parse_mode"):
			if !emptyString(value) {
				return false
			}
		}
	}
	return !markupPattern.MatchString(subject) && !markupPattern.MatchString(body)
}

func plainFormat(value any) bool {
	text, ok := value.(string)
	return ok && (strings.TrimSpace(text) == "" || strings.EqualFold(strings.TrimSpace(text), "plain"))
}

func emptyString(value any) bool {
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) == ""
}

func (s *OnboardingStore) resolveBinding(ctx context.Context, transport, botID, phone string, cutoff time.Time) (string, time.Time, error) {
	if s.resolver != nil {
		return s.resolver(ctx, transport, botID, phone, cutoff)
	}
	table := bindingTable(transport, s.providers[transport])
	idColumn := "user_id"
	if table == "telegram_contact_bindings" {
		idColumn = "chat_id"
	}
	query := fmt.Sprintf(`SELECT %s::text AS recipient, phone_verified_at
		FROM %s
		WHERE bot_id = $1::bigint AND phone = $2 AND phone_verified_at IS NOT NULL AND phone_verified_at <= $3
		ORDER BY phone_verified_at DESC LIMIT 1`, idColumn, table)
	var row struct {
		Recipient string    `db:"recipient"`
		Verified  time.Time `db:"phone_verified_at"`
	}
	err := s.database(ctx).GetContext(ctx, &row, query, botID, phone, cutoff)
	if errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, nil
	}
	return row.Recipient, row.Verified, err
}

func (s *OnboardingStore) Delivery(ctx context.Context, taskID uuid.UUID) (*onboardingDelivery, error) {
	var delivery onboardingDelivery
	err := s.database(ctx).GetContext(ctx, &delivery, `SELECT task_id, message_id, transport, bot_id, recipient, phone,
		selected_path, resolved_recipient, invitation_id, wait_until
		FROM message_onboarding_deliveries WHERE task_id = $1 FOR UPDATE`, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &delivery, err
}

func (s *OnboardingStore) Invitation(ctx context.Context, taskID uuid.UUID) (*onboardingInvitation, error) {
	var invitation onboardingInvitation
	err := s.database(ctx).GetContext(ctx, &invitation, `SELECT i.id, i.task_id, i.message_id, i.transport, i.bot_id, i.phone,
		i.sms_transport, i.connect_url, i.sent_at, i.finished_at, t.status::text AS task_status
		FROM message_onboarding_invitations i JOIN tasks t ON t.id = i.task_id
		WHERE i.task_id = $1 FOR UPDATE OF i`, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &invitation, err
}

func (s *OnboardingStore) selectPath(ctx context.Context, delivery *onboardingDelivery, path, recipient string, waitUntil *time.Time) error {
	_, err := s.database(ctx).ExecContext(ctx, `UPDATE message_onboarding_deliveries
		SET selected_path = $2, resolved_recipient = NULLIF($3, ''), wait_until = $4,
			selected_at = COALESCE(selected_at, CURRENT_TIMESTAMP)
		WHERE task_id = $1 AND selected_path = 'pending'`, delivery.TaskID, path, recipient, waitUntil)
	if err == nil {
		delivery.SelectedPath = path
		if recipient != "" {
			delivery.ResolvedRecipient = &recipient
		}
		delivery.WaitUntil = waitUntil
	}
	return err
}

func (s *OnboardingStore) hold(ctx context.Context, task *dto.Task, until time.Time) error {
	task.NextRun = until
	task.IsProcessed = false
	task.LockUntil = time.Time{}
	_, err := s.database(ctx).ExecContext(ctx, `UPDATE tasks SET next_run = $2, is_processed = false, lock_until = $3 WHERE id = $1`, task.ID, until, time.Time{})
	return err
}

func (s *OnboardingStore) ensureInvitation(ctx context.Context, delivery *onboardingDelivery, task *dto.Task) (*onboardingInvitation, error) {
	var invitation onboardingInvitation
	err := s.database(ctx).GetContext(ctx, &invitation, `SELECT i.id, i.task_id, i.message_id, i.transport, i.bot_id, i.phone,
		i.sms_transport, i.connect_url, i.sent_at, i.finished_at, t.status::text AS task_status
		FROM message_onboarding_invitations i JOIN tasks t ON t.id = i.task_id
		WHERE i.transport = $1 AND i.bot_id = $2 AND i.phone = $3 AND i.finished_at IS NULL
		FOR UPDATE OF i`, delivery.Transport, delivery.BotID, *delivery.Phone)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		_, err = s.database(ctx).ExecContext(ctx, `UPDATE message_onboarding_deliveries SET invitation_id = $2 WHERE task_id = $1`, delivery.TaskID, invitation.ID)
		delivery.InvitationID = &invitation.ID
		return &invitation, err
	}

	config := s.providers[delivery.Transport].Onboarding
	invitationID := generate.ID()
	invitationTaskID := generate.ID()
	deadline := s.now().Add(invitationLifetime)
	if !task.Deadline.IsZero() && task.Deadline.Before(deadline) {
		deadline = task.Deadline
	}
	_, err = s.database(ctx).ExecContext(ctx, `INSERT INTO tasks
		(id, message_id, worker, status, retries, max_retries, backoff_code, backoff_params, deadline, is_processed, lock_until, last_run, next_run)
		VALUES ($1, $2, $3, 'pending', 0, $4, $5, $6, $7, false, $8, $9, $10)`,
		invitationTaskID, task.MessageID, config.SMSTransport, invitationMaxRetries,
		retrier.JitterLinearBackOff, dto.BackOffParams{retrier.DurationKey: time.Second}, deadline,
		time.Time{}, time.Time{}, s.now())
	if err != nil {
		return nil, err
	}
	_, err = s.database(ctx).ExecContext(ctx, `INSERT INTO message_onboarding_invitations
		(id, task_id, message_id, transport, bot_id, phone, sms_transport, connect_url)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, invitationID, invitationTaskID, task.MessageID,
		delivery.Transport, delivery.BotID, *delivery.Phone, config.SMSTransport, config.ConnectURL)
	if err != nil {
		return nil, err
	}
	_, err = s.database(ctx).ExecContext(ctx, `UPDATE message_onboarding_deliveries SET invitation_id = $2 WHERE task_id = $1`, delivery.TaskID, invitationID)
	if err != nil {
		return nil, err
	}
	return &onboardingInvitation{ID: invitationID, TaskID: invitationTaskID, MessageID: task.MessageID,
		Transport: delivery.Transport, BotID: delivery.BotID, Phone: *delivery.Phone,
		SMSTransport: config.SMSTransport, ConnectURL: config.ConnectURL, TaskStatus: string(retrier.StatusPending)}, nil
}

func (s *OnboardingStore) invitationPayload(invitation *onboardingInvitation, base *dto.Message) *dto.Message {
	copy := *base
	copy.Transport = invitation.SMSTransport
	copy.Recipients = dto.Recipients{invitation.Phone}
	copy.RoutingRecipients = dto.Recipients{invitation.Phone}
	copy.Code = nil
	copy.Params = nil
	copy.Meta = nil
	copy.Subject = ""
	copy.Body = "Подключите бота для получения уведомлений: " + invitation.ConnectURL
	return &copy
}

func (s *OnboardingStore) routePayload(delivery *onboardingDelivery, base *dto.Message) (*dto.Message, string) {
	copy := *base
	copy.Code = base.Code
	copy.Recipients = dto.Recipients{delivery.Recipient}
	copy.RoutingRecipients = nil
	worker := delivery.Transport
	switch delivery.SelectedPath {
	case "messenger":
		copy.Code = nil
		copy.Params = nil
		copy.Recipients = dto.Recipients{*delivery.ResolvedRecipient}
		copy.RoutingRecipients = dto.Recipients{*delivery.Phone}
	case "sms_fallback":
		copy.Code = nil
		copy.Params = nil
		worker = s.providers[delivery.Transport].Onboarding.SMSTransport
		copy.Transport = worker
		copy.Recipients = dto.Recipients{*delivery.Phone}
		copy.RoutingRecipients = dto.Recipients{*delivery.Phone}
		copy.Body = smsFallbackText(copy.Subject, copy.Body)
		copy.Subject = ""
		copy.Meta = nil
	}
	return &copy, worker
}

func smsFallbackText(subject, body string) string {
	switch {
	case subject == "":
		return body
	case body == "":
		return subject
	default:
		return subject + "\n" + body
	}
}

// Prepare returns a task-specific payload and worker. A false ready value means
// the original task was durably postponed without consuming a retry.
func (s *OnboardingStore) Prepare(ctx context.Context, task *dto.Task, message *dto.Message, now time.Time) (*dto.Message, string, bool, error) {
	invitation, err := s.Invitation(ctx, task.ID)
	if err != nil {
		return nil, "", false, err
	}
	if invitation != nil {
		return s.invitationPayload(invitation, message), invitation.SMSTransport, true, nil
	}
	delivery, err := s.Delivery(ctx, task.ID)
	if err != nil {
		return nil, "", false, err
	}
	if delivery == nil {
		return message, task.Worker, true, nil
	}
	if delivery.SelectedPath != "pending" {
		payload, worker := s.routePayload(delivery, message)
		return payload, worker, true, nil
	}
	if !task.Deadline.IsZero() && !now.Before(task.Deadline) {
		if err := s.selectPath(ctx, delivery, "expired", "", nil); err != nil {
			return nil, "", false, err
		}
		return message, task.Worker, true, nil
	}
	resolved := ""
	if delivery.InvitationID == nil {
		resolved, _, err = s.resolveBinding(ctx, delivery.Transport, delivery.BotID, *delivery.Phone, now)
		if err != nil {
			return nil, "", false, err
		}
		if resolved != "" {
			if err := s.selectPath(ctx, delivery, "messenger", resolved, nil); err != nil {
				return nil, "", false, err
			}
			payload, worker := s.routePayload(delivery, message)
			return payload, worker, true, nil
		}
	}
	invitation, err = s.ensureInvitation(ctx, delivery, task)
	if err != nil {
		return nil, "", false, err
	}
	if invitation.SentAt == nil {
		resolved, _, err = s.resolveBinding(ctx, delivery.Transport, delivery.BotID, *delivery.Phone, now)
		if err != nil {
			return nil, "", false, err
		}
		if resolved != "" {
			if err := s.selectPath(ctx, delivery, "messenger", resolved, nil); err != nil {
				return nil, "", false, err
			}
			payload, worker := s.routePayload(delivery, message)
			return payload, worker, true, nil
		}
		if invitation.TaskStatus == string(retrier.StatusFailure) {
			if err := s.selectPath(ctx, delivery, "sms_fallback", "", nil); err != nil {
				return nil, "", false, err
			}
			payload, worker := s.routePayload(delivery, message)
			return payload, worker, true, nil
		}
		return nil, "", false, s.hold(ctx, task, now.Add(onboardingPollInterval))
	}
	waitUntil := invitation.SentAt.Add(onboardingWait)
	if !task.Deadline.IsZero() && task.Deadline.Before(waitUntil) {
		waitUntil = task.Deadline
	}
	if delivery.WaitUntil == nil {
		_, err = s.database(ctx).ExecContext(ctx, `UPDATE message_onboarding_deliveries SET wait_until = $2 WHERE task_id = $1`, task.ID, waitUntil)
		if err != nil {
			return nil, "", false, err
		}
		delivery.WaitUntil = &waitUntil
	} else {
		waitUntil = *delivery.WaitUntil
	}
	if now.Before(waitUntil) {
		resolved, _, err = s.resolveBinding(ctx, delivery.Transport, delivery.BotID, *delivery.Phone, waitUntil)
		if err != nil {
			return nil, "", false, err
		}
		if resolved != "" {
			if err := s.selectPath(ctx, delivery, "messenger", resolved, &waitUntil); err != nil {
				return nil, "", false, err
			}
			payload, worker := s.routePayload(delivery, message)
			return payload, worker, true, nil
		}
		next := now.Add(onboardingPollInterval)
		if waitUntil.Before(next) {
			next = waitUntil
		}
		return nil, "", false, s.hold(ctx, task, next)
	}
	resolved, _, err = s.resolveBinding(ctx, delivery.Transport, delivery.BotID, *delivery.Phone, waitUntil)
	if err != nil {
		return nil, "", false, err
	}
	if resolved != "" {
		err = s.selectPath(ctx, delivery, "messenger", resolved, &waitUntil)
	} else if !task.Deadline.IsZero() && !waitUntil.Before(task.Deadline) {
		err = s.selectPath(ctx, delivery, "expired", "", &waitUntil)
	} else {
		err = s.selectPath(ctx, delivery, "sms_fallback", "", &waitUntil)
	}
	if err != nil {
		return nil, "", false, err
	}
	payload, worker := s.routePayload(delivery, message)
	return payload, worker, true, nil
}

func (s *OnboardingStore) TaskCompleted(ctx context.Context, taskID uuid.UUID, status retrier.TaskStatus) error {
	if status == retrier.StatusSuccess {
		if result, err := s.database(ctx).ExecContext(ctx, `UPDATE message_onboarding_invitations
			SET sent_at = COALESCE(sent_at, clock_timestamp()) WHERE task_id = $1`, taskID); err != nil {
			return err
		} else if affected, _ := result.RowsAffected(); affected > 0 {
			return nil
		}
	}
	if status == retrier.StatusSuccess || status == retrier.StatusFailure {
		_, err := s.database(ctx).ExecContext(ctx, `UPDATE message_onboarding_deliveries SET completed_at = clock_timestamp() WHERE task_id = $1`, taskID)
		return err
	}
	return nil
}

func (s *OnboardingStore) IsOnboardingTask(ctx context.Context, taskID uuid.UUID) (bool, error) {
	var found bool
	err := s.database(ctx).GetContext(ctx, &found, `SELECT
		EXISTS(SELECT 1 FROM message_onboarding_invitations WHERE task_id = $1)
		OR EXISTS(SELECT 1 FROM message_onboarding_deliveries WHERE task_id = $1)`, taskID)
	return found, err
}

// LockTaskParent establishes the same parent-before-task lock order used by
// retries. This prevents a retry from skipping an in-flight terminal task and
// creating duplicate recipient work.
func (s *OnboardingStore) LockTaskParent(ctx context.Context, taskID uuid.UUID) (bool, error) {
	var messageID uuid.UUID
	err := s.database(ctx).GetContext(ctx, &messageID, `SELECT message_id FROM (
		SELECT message_id FROM message_onboarding_deliveries WHERE task_id = $1
		UNION ALL
		SELECT message_id FROM message_onboarding_invitations WHERE task_id = $1
	) onboarding_task LIMIT 1`, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var locked uuid.UUID
	if err := s.database(ctx).GetContext(ctx, &locked, `SELECT id FROM messages WHERE id = $1 FOR UPDATE`, messageID); err != nil {
		return true, err
	}
	return true, nil
}

// FinalizeTask aggregates an onboarding parent in the same transaction that
// persisted the terminal recipient task. Locking the parent row serializes
// concurrent recipient completions and makes the terminal status restart-safe.
func (s *OnboardingStore) FinalizeTask(ctx context.Context, taskID uuid.UUID) (bool, error) {
	var messageID uuid.UUID
	err := s.database(ctx).GetContext(ctx, &messageID, `SELECT message_id FROM message_onboarding_deliveries WHERE task_id = $1`, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return s.IsOnboardingTask(ctx, taskID)
	}
	if err != nil {
		return false, err
	}
	var current dto.MessageStatus
	if err := s.database(ctx).GetContext(ctx, &current, `SELECT status FROM messages WHERE id = $1 FOR UPDATE`, messageID); err != nil {
		return true, err
	}
	aggregated, found, err := s.AggregateStatus(ctx, messageID)
	if err != nil || !found {
		return true, err
	}
	if current != dto.MessageStatusSucceeded && current != aggregated {
		if _, err := s.database(ctx).ExecContext(ctx, `UPDATE messages SET status = $2 WHERE id = $1`, messageID, aggregated); err != nil {
			return true, err
		}
	}
	return true, s.closeSharedInvitation(ctx, taskID)
}

func (s *OnboardingStore) closeSharedInvitation(ctx context.Context, taskID uuid.UUID) error {
	var invitationID *uuid.UUID
	if err := s.database(ctx).GetContext(ctx, &invitationID, `SELECT invitation_id FROM message_onboarding_deliveries WHERE task_id = $1`, taskID); err != nil || invitationID == nil {
		return err
	}
	var locked uuid.UUID
	if err := s.database(ctx).GetContext(ctx, &locked, `SELECT id FROM message_onboarding_invitations WHERE id = $1 FOR UPDATE`, *invitationID); err != nil {
		return err
	}
	_, err := s.database(ctx).ExecContext(ctx, `UPDATE message_onboarding_invitations i
		SET finished_at = COALESCE(i.finished_at, clock_timestamp())
		WHERE i.id = $1 AND NOT EXISTS (
			SELECT 1 FROM message_onboarding_deliveries d JOIN tasks t ON t.id = d.task_id
			WHERE d.invitation_id = i.id AND t.status = 'pending'
		)`, *invitationID)
	return err
}

func (s *OnboardingStore) AggregateStatus(ctx context.Context, messageID uuid.UUID) (dto.MessageStatus, bool, error) {
	var rows []struct {
		Recipient string `db:"recipient"`
		Succeeded bool   `db:"succeeded"`
		Pending   bool   `db:"pending"`
	}
	err := s.database(ctx).SelectContext(ctx, &rows, `SELECT d.recipient,
		bool_or(t.status = 'success') AS succeeded,
		bool_or(t.status = 'pending') AS pending
		FROM message_onboarding_deliveries d JOIN tasks t ON t.id = d.task_id
		WHERE d.message_id = $1 GROUP BY d.recipient`, messageID)
	if err != nil || len(rows) == 0 {
		return "", false, err
	}
	allSuccess := true
	allTerminal := true
	for _, row := range rows {
		allSuccess = allSuccess && row.Succeeded
		allTerminal = allTerminal && (row.Succeeded || !row.Pending)
	}
	if allSuccess {
		return dto.MessageStatusSucceeded, true, nil
	}
	if allTerminal {
		return dto.MessageStatusFailed, true, nil
	}
	return dto.MessageStatusRunning, true, nil
}

func (s *OnboardingStore) RetryRecipients(ctx context.Context, messageID uuid.UUID) ([]string, bool, error) {
	var count int
	if err := s.database(ctx).GetContext(ctx, &count, `SELECT count(*) FROM message_onboarding_deliveries WHERE message_id = $1`, messageID); err != nil || count == 0 {
		return nil, false, err
	}
	var recipients []string
	err := s.database(ctx).SelectContext(ctx, &recipients, `SELECT DISTINCT d.recipient
		FROM message_onboarding_deliveries d
		WHERE d.message_id = $1 AND NOT EXISTS (
			SELECT 1 FROM message_onboarding_deliveries succeeded
			JOIN tasks t ON t.id = succeeded.task_id
			WHERE succeeded.message_id = d.message_id AND succeeded.recipient = d.recipient AND t.status = 'success'
		) ORDER BY d.recipient`, messageID)
	return recipients, true, err
}
