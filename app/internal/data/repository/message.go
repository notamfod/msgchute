package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/internal/io/storage"
)

const messagesTable = "messages"

var messageColumns = []string{
	"id", "sender_id", "transport", "template_code AS code",
	"recipients", "params", "retry", "schedule",
	"deadline", "subject", "body", "status", "meta", "tag",
}

type MessageRepository struct {
	db      DBContext
	builder squirrel.StatementBuilderType
}

func NewMessageRepository(db *sqlx.DB) *MessageRepository {
	return &MessageRepository{
		db:      db,
		builder: squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar),
	}
}

// getDB returns the appropriate DBContext (transaction from context or main db).
func (r *MessageRepository) getDB(ctx context.Context) DBContext {
	if tx := storage.ExtractTx(ctx); tx != nil {
		return tx
	}
	return r.db
}

// Create inserts a new message record.
func (r *MessageRepository) Create(ctx context.Context, m *dto.Message) error {
	status := m.Status
	if status == "" {
		status = dto.MessageStatusRunning
	}

	query, args, err := r.builder.Insert(messagesTable).
		Columns(
			"id", "sender_id", "transport", "template_code",
			"recipients", "params", "retry", "schedule",
			"deadline", "subject", "body", "status", "meta", "tag",
		).
		Values(
			m.ID, m.SenderID, m.Transport, m.Code,
			m.Recipients, m.Params, m.Retry, m.Schedule,
			m.Deadline, m.Subject, m.Body, status, m.Meta, m.Tag,
		).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert query: %w", err)
	}

	db := r.getDB(ctx)
	_, err = db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("execute insert: %w", err)
	}
	return nil
}

// GetByID retrieves a single message by its UUID.
func (r *MessageRepository) GetByID(ctx context.Context, ID uuid.UUID) (*dto.Message, error) {
	selectBuilder := r.builder.Select(messageColumns...).
		From(messagesTable).
		Where(squirrel.Eq{"id": ID}).
		Suffix("FOR UPDATE SKIP LOCKED")

	query, args, err := selectBuilder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get by id query: %w", err)
	}

	var message dto.Message
	db := r.getDB(ctx)
	if err := db.Get(&message, query, args...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get message %s: %w", ID, err)
	}
	return &message, nil
}

// GetByIDs retrieves multiple messages by their UUIDs.
func (r *MessageRepository) GetByIDs(ctx context.Context, IDs []uuid.UUID) ([]*dto.Message, error) {
	if len(IDs) == 0 {
		return []*dto.Message{}, nil
	}

	selectBuilder := r.builder.Select(messageColumns...).
		From(messagesTable).
		Where(squirrel.Eq{"id": IDs})

	query, args, err := selectBuilder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get by ids query: %w", err)
	}

	var messages []*dto.Message
	db := r.getDB(ctx)
	if err := db.Select(&messages, query, args...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []*dto.Message{}, nil
		}
		return nil, fmt.Errorf("get messages by IDs: %w", err)
	}
	return messages, nil
}

// Find fetches a paginated collection of messages.
func (r *MessageRepository) Find(ctx context.Context, filter *dto.MessageFilter) ([]*dto.Message, uint64, error) {
	selectBuilder := r.builder.Select(messageColumns...).From(messagesTable)
	countBuilder := r.builder.Select("COUNT(*)").From(messagesTable)

	if len(filter.IDs) > 0 {
		selectBuilder = selectBuilder.Where(squirrel.Eq{"id": filter.IDs})
		countBuilder = countBuilder.Where(squirrel.Eq{"id": filter.IDs})
	}
	if len(filter.SenderIDs) > 0 {
		selectBuilder = selectBuilder.Where(squirrel.Eq{"sender_id": filter.SenderIDs})
		countBuilder = countBuilder.Where(squirrel.Eq{"sender_id": filter.SenderIDs})
	}
	if len(filter.Code) > 0 {
		selectBuilder = selectBuilder.Where(squirrel.Eq{"template_code": filter.Code})
		countBuilder = countBuilder.Where(squirrel.Eq{"template_code": filter.Code})
	}
	if len(filter.Transport) > 0 {
		selectBuilder = selectBuilder.Where(squirrel.Eq{"transport": filter.Transport})
		countBuilder = countBuilder.Where(squirrel.Eq{"transport": filter.Transport})
	}
	if len(filter.Status) > 0 {
		selectBuilder = selectBuilder.Where(squirrel.Eq{"status": filter.Status})
		countBuilder = countBuilder.Where(squirrel.Eq{"status": filter.Status})
	}
	if len(filter.Recipient) > 0 {
		recipientJSON := fmt.Sprintf(`["%s"]`, strings.Join(filter.Recipient, `","`))
		selectBuilder = selectBuilder.Where("recipients @> ?", recipientJSON)
		countBuilder = countBuilder.Where("recipients @> ?", recipientJSON)
	}

	countQuery, countArgs, err := countBuilder.ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("build count query: %w", err)
	}
	var total uint64
	db := r.getDB(ctx)
	if err := db.Get(&total, countQuery, countArgs...); err != nil {
		return nil, 0, fmt.Errorf("get total count: %w", err)
	}

	if filter.Limit > 0 {
		selectBuilder = selectBuilder.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		selectBuilder = selectBuilder.Offset(filter.Offset)
	}

	orderClause := "schedule DESC, id DESC"
	if filter.SortField != nil && *filter.SortField != "" {
		field := strings.ToLower(*filter.SortField)
		allowed := map[string]bool{
			"id": true, "sender_id": true, "status": true,
			"schedule": true, "deadline": true, "created_at": true,
		}
		if allowed[field] {
			order := "ASC"
			if filter.SortOrder != nil && strings.ToUpper(*filter.SortOrder) == "DESC" {
				order = "DESC"
			}
			orderClause = fmt.Sprintf("%s %s", field, order)
		}
	}
	selectBuilder = selectBuilder.OrderBy(orderClause).Suffix("FOR UPDATE SKIP LOCKED")

	query, args, err := selectBuilder.ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("build select query: %w", err)
	}

	var messages []*dto.Message
	if err := db.Select(&messages, query, args...); err != nil {
		return nil, 0, fmt.Errorf("select messages: %w", err)
	}
	return messages, total, nil
}

// GetSenders returns distinct sender IDs.
func (r *MessageRepository) GetSenders(ctx context.Context) ([]string, error) {
	query, _, err := r.builder.
		Select("sender_id").
		From(messagesTable).
		GroupBy("sender_id").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get senders query: %w", err)
	}

	var senders []string
	db := r.getDB(ctx)
	if err := db.Select(&senders, query); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("get senders: %w", err)
	}
	if senders == nil {
		return []string{}, nil
	}
	return senders, nil
}

// GetTransports returns distinct transport names.
func (r *MessageRepository) GetTransports(ctx context.Context) ([]string, error) {
	query, _, err := r.builder.
		Select("transport").
		From(messagesTable).
		GroupBy("transport").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get transports query: %w", err)
	}

	var transports []string
	db := r.getDB(ctx)
	if err := db.Select(&transports, query); err != nil {
		return nil, fmt.Errorf("get transports: %w", err)
	}
	if transports == nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []string{}, nil
		}
		return []string{}, nil
	}
	return transports, nil
}

// GetTemplateCodes returns distinct transport names.
func (r *MessageRepository) GetTemplateCodes(ctx context.Context) ([]string, error) {
	query, _, err := r.builder.
		Select("template_code").
		From(messagesTable).
		Where("template_code IS NOT NULL").
		GroupBy("template_code").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get transports query: %w", err)
	}

	var transports []string
	db := r.getDB(ctx)
	if err := db.Select(&transports, query); err != nil {
		return nil, fmt.Errorf("get transports: %w", err)
	}
	if transports == nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []string{}, nil
		}
		return []string{}, nil
	}
	return transports, nil
}

// UpdateStatus updates the status of a message.
func (r *MessageRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status dto.MessageStatus) error {
	query, args, err := r.builder.
		Update(messagesTable).
		Set("status", status).
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build update status query: %w", err)
	}

	db := r.getDB(ctx)
	_, err = db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	return nil
}

// GetRecipients returns distinct recipient strings matching a search pattern.
// If search is empty, returns all distinct recipients from all messages.
// search is matched as a substring (case‑insensitive).
func (r *MessageRepository) GetRecipients(ctx context.Context, search string) ([]string, error) {
	queryBuilder := r.builder.
		Select("DISTINCT r.recipient").
		From(messagesTable + ", LATERAL jsonb_array_elements_text(recipients) AS r(recipient)")

	if search != "" {
		queryBuilder = queryBuilder.Where("r.recipient ILIKE ?", "%"+search+"%")
	}

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get recipients query: %w", err)
	}

	var recipients []string
	db := r.getDB(ctx)
	if err := db.Select(&recipients, query, args...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("get recipients: %w", err)
	}
	if recipients == nil {
		return []string{}, nil
	}
	return recipients, nil
}
