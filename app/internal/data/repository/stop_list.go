package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/internal/io/storage"
	"github.com/devian2011/msgchute/internal/service/stoplist"
)

const stopListTable = "notification_stop_list"

type StopListRepository struct {
	db      DBContext
	builder squirrel.StatementBuilderType
}

func NewStopListRepository(db *sqlx.DB) *StopListRepository {
	return &StopListRepository{db: db, builder: squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)}
}

func (r *StopListRepository) getDB(ctx context.Context) DBContext {
	if tx := storage.ExtractTx(ctx); tx != nil {
		return tx
	}
	return r.db
}

func (r *StopListRepository) Create(ctx context.Context, entry *dto.StopListEntry) (*dto.StopListEntry, error) {
	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}
	query, args, err := r.builder.Insert(stopListTable).Columns("id", "recipient", "kind", "reason", "subscriptions", "blocked_all").Values(entry.ID, entry.Recipient, entry.Kind, entry.Reason, entry.Subscriptions, entry.BlockedAll).Suffix("RETURNING created_at").ToSql()
	if err != nil {
		return nil, fmt.Errorf("build stop list insert: %w", err)
	}
	if err := sqlx.GetContext(ctx, r.getDB(ctx), &entry.CreatedAt, query, args...); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, stoplist.ErrAlreadyExists
		}
		return nil, fmt.Errorf("create stop list entry: %w", err)
	}
	return entry, nil
}

func (r *StopListRepository) Delete(ctx context.Context, id string) error {
	return r.Unblock(ctx, id)
}

func (r *StopListRepository) Unblock(ctx context.Context, id string) error {
	entryID, err := uuid.Parse(id)
	if err != nil {
		return stoplist.ErrNotFound
	}
	query, args, err := r.builder.Update(stopListTable).Set("blocked_all", false).Where(squirrel.Eq{"id": entryID, "blocked_all": true}).ToSql()
	if err != nil {
		return fmt.Errorf("build stop list unblock: %w", err)
	}
	result, err := r.getDB(ctx).ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("unblock stop list entry: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get unblocked stop list count: %w", err)
	}
	if count == 0 {
		return stoplist.ErrNotFound
	}
	return nil
}

func (r *StopListRepository) Block(ctx context.Context, entry *dto.StopListEntry) (*dto.StopListEntry, error) {
	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}
	query, args, err := r.builder.Update(stopListTable).Set("blocked_all", true).Set("reason", entry.Reason).Where(squirrel.Eq{"kind": entry.Kind, "recipient": entry.Recipient, "blocked_all": false}).Suffix("RETURNING id, created_at, subscriptions").ToSql()
	if err != nil {
		return nil, fmt.Errorf("build stop list block: %w", err)
	}
	var existing struct {
		ID            uuid.UUID            `db:"id"`
		CreatedAt     time.Time            `db:"created_at"`
		Subscriptions dto.SubscriptionList `db:"subscriptions"`
	}
	if err := sqlx.GetContext(ctx, r.getDB(ctx), &existing, query, args...); err == nil {
		entry.ID, entry.CreatedAt, entry.Subscriptions, entry.BlockedAll = existing.ID, existing.CreatedAt, existing.Subscriptions, true
		return entry, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("block stop list entry: %w", err)
	}
	created, err := r.Create(ctx, entry)
	if err == nil {
		return created, nil
	}
	if !errors.Is(err, stoplist.ErrAlreadyExists) {
		return nil, err
	}
	return nil, stoplist.ErrAlreadyExists
}

func (r *StopListRepository) Upsert(ctx context.Context, entry *dto.StopListEntry) (*dto.StopListEntry, error) {
	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}
	query, args, err := r.builder.Insert(stopListTable).Columns("id", "recipient", "kind", "reason", "subscriptions", "blocked_all").Values(entry.ID, entry.Recipient, entry.Kind, entry.Reason, entry.Subscriptions, entry.BlockedAll).Suffix("ON CONFLICT (kind, recipient) DO UPDATE SET reason = EXCLUDED.reason, subscriptions = EXCLUDED.subscriptions, blocked_all = EXCLUDED.blocked_all RETURNING id, created_at").ToSql()
	if err != nil {
		return nil, fmt.Errorf("build subscription upsert: %w", err)
	}
	if err := sqlx.GetContext(ctx, r.getDB(ctx), entry, query, args...); err != nil {
		return nil, fmt.Errorf("upsert subscriptions: %w", err)
	}
	return entry, nil
}

func (r *StopListRepository) Find(ctx context.Context, filter dto.StopListFilter) ([]dto.StopListEntry, uint64, error) {
	return r.find(ctx, filter, true)
}

func (r *StopListRepository) List(ctx context.Context, filter dto.StopListFilter) ([]dto.StopListEntry, uint64, error) {
	return r.find(ctx, filter, false)
}

func (r *StopListRepository) find(ctx context.Context, filter dto.StopListFilter, blockedOnly bool) ([]dto.StopListEntry, uint64, error) {
	base := r.builder.Select("id", "recipient", "kind", "reason", "subscriptions", "blocked_all", "created_at").From(stopListTable)
	count := r.builder.Select("COUNT(*)").From(stopListTable)
	if blockedOnly {
		base, count = base.Where(squirrel.Eq{"blocked_all": true}), count.Where(squirrel.Eq{"blocked_all": true})
	}
	if filter.Search != "" {
		condition := squirrel.ILike{"recipient": "%" + filter.Search + "%"}
		base, count = base.Where(condition), count.Where(condition)
	}
	countQuery, countArgs, err := count.ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("build stop list count: %w", err)
	}
	var total uint64
	if err := sqlx.GetContext(ctx, r.getDB(ctx), &total, countQuery, countArgs...); err != nil {
		return nil, 0, fmt.Errorf("count stop list entries: %w", err)
	}
	query, args, err := base.OrderBy("created_at DESC").Limit(filter.Limit).Offset(filter.Offset).ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("build stop list find: %w", err)
	}
	var entries []dto.StopListEntry
	if err := sqlx.SelectContext(ctx, r.getDB(ctx), &entries, query, args...); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, 0, fmt.Errorf("find stop list entries: %w", err)
	}
	if entries == nil {
		entries = []dto.StopListEntry{}
	}
	return entries, total, nil
}

func (r *StopListRepository) Blocked(ctx context.Context, kind string, recipients []string) (map[string]struct{}, error) {
	if len(recipients) == 0 {
		return map[string]struct{}{}, nil
	}
	query, args, err := r.builder.Select("recipient").From(stopListTable).Where(squirrel.Eq{"kind": kind, "recipient": recipients, "blocked_all": true}).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build stop list lookup: %w", err)
	}
	var values []string
	if err := sqlx.SelectContext(ctx, r.getDB(ctx), &values, query, args...); err != nil {
		return nil, fmt.Errorf("lookup stop list entries: %w", err)
	}
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result, nil
}

func (r *StopListRepository) Preferences(ctx context.Context, kind string, recipients []string) (map[string]dto.StopListEntry, error) {
	if len(recipients) == 0 {
		return map[string]dto.StopListEntry{}, nil
	}
	query, args, err := r.builder.Select("id", "recipient", "kind", "reason", "subscriptions", "blocked_all", "created_at").From(stopListTable).Where(squirrel.Eq{"kind": kind, "recipient": recipients}).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build preferences lookup: %w", err)
	}
	var entries []dto.StopListEntry
	if err := sqlx.SelectContext(ctx, r.getDB(ctx), &entries, query, args...); err != nil {
		return nil, fmt.Errorf("lookup preferences: %w", err)
	}
	result := make(map[string]dto.StopListEntry, len(entries))
	for _, entry := range entries {
		result[entry.Recipient] = entry
	}
	return result, nil
}
