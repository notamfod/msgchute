package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/internal/io/storage"
)

const messageTemplatesTable = "message_templates"

type MessageTemplateRepository struct {
	db      DBContext
	builder squirrel.StatementBuilderType
}

func NewMessageTemplateRepository(db *sqlx.DB) *MessageTemplateRepository {
	return &MessageTemplateRepository{
		db:      db,
		builder: squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar),
	}
}

// getDB returns the appropriate DBContext (transaction from context or main db).
func (r *MessageTemplateRepository) getDB(ctx context.Context) DBContext {
	if tx := storage.ExtractTx(ctx); tx != nil {
		return tx
	}
	return r.db
}

func (r *MessageTemplateRepository) Create(ctx context.Context, t *dto.Template) error {
	if t.Metadata == nil {
		t.Metadata = dto.TemplateMetadata{}
	}
	query, args, err := r.builder.Insert(messageTemplatesTable).
		Columns("code", "name", "description", "params", "subject", "body", "metadata", "systems", "channels").
		Values(t.Code, t.Name, t.Description, t.Params, t.Subject, t.Body, t.Metadata, t.Systems, t.Channels).
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

func (r *MessageTemplateRepository) GetByCode(ctx context.Context, code string) (*dto.Template, error) {
	query, args, err := r.builder.Select("code", "name", "description", "params", "subject", "body", "metadata", "systems", "channels").
		From(messageTemplatesTable).
		Where(squirrel.Eq{"code": code}).
		Suffix("FOR UPDATE").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get by code query: %w", err)
	}

	var t dto.Template
	db := r.getDB(ctx)
	if err := sqlx.GetContext(ctx, db, &t, query, args...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get template by code: %w", err)
	}
	return &t, nil
}

func (r *MessageTemplateRepository) Update(ctx context.Context, t *dto.Template) error {
	builder := r.builder.Update(messageTemplatesTable).
		Set("name", t.Name).
		Set("description", t.Description).
		Set("params", t.Params).
		Set("subject", t.Subject).
		Set("body", t.Body).
		Where(squirrel.Eq{"code": t.Code})
	if t.Systems != nil {
		builder = builder.Set("systems", t.Systems)
	}
	if t.Channels != nil {
		builder = builder.Set("channels", t.Channels)
	}
	query, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("build update query: %w", err)
	}

	db := r.getDB(ctx)
	res, err := db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("execute update: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("template with code %s not found", t.Code)
	}
	return nil
}

func (r *MessageTemplateRepository) Delete(ctx context.Context, code string) error {
	query, args, err := r.builder.Delete(messageTemplatesTable).
		Where(squirrel.Eq{"code": code}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build delete query: %w", err)
	}

	db := r.getDB(ctx)
	res, err := db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("execute delete: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("template with code %s not found", code)
	}
	return nil
}

func (r *MessageTemplateRepository) Find(
	ctx context.Context,
	filter *dto.MessageTemplateFilter,
) (map[string]*dto.Template, uint64, error) {
	selectBuilder := r.builder.Select("code", "name", "description", "params", "subject", "body", "metadata", "systems", "channels").
		From(messageTemplatesTable)
	countBuilder := r.builder.Select("COUNT(*)").
		From(messageTemplatesTable)
	for _, f := range []struct {
		column string
		values []string
	}{{"systems", filter.Systems}, {"channels", filter.Channels}} {
		if len(f.values) == 0 {
			continue
		}
		conditions := squirrel.Or{}
		for _, value := range f.values {
			conditions = append(conditions, squirrel.Expr(f.column+" @> ?::jsonb", dto.TemplateLabels{value}))
		}
		selectBuilder = selectBuilder.Where(conditions)
		countBuilder = countBuilder.Where(conditions)
	}

	if len(filter.Code) > 0 {
		selectBuilder = selectBuilder.Where(squirrel.Eq{"code": filter.Code})
		countBuilder = countBuilder.Where(squirrel.Eq{"code": filter.Code})
	}
	if filter.Search != nil && *filter.Search != "" {
		searchPattern := "%" + *filter.Search + "%"
		orCondition := squirrel.Or{
			squirrel.ILike{"name": searchPattern},
			squirrel.ILike{"subject": searchPattern},
			squirrel.ILike{"body": searchPattern},
		}
		selectBuilder = selectBuilder.Where(orCondition)
		countBuilder = countBuilder.Where(orCondition)
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

	orderClause := "code ASC"
	if filter.SortField != nil && *filter.SortField != "" {
		field := strings.ToLower(*filter.SortField)
		allowed := map[string]bool{
			"code": true, "name": true, "subject": true,
		}
		if allowed[field] {
			order := "ASC"
			if filter.SortOrder != nil && strings.ToUpper(*filter.SortOrder) == "DESC" {
				order = "DESC"
			}
			orderClause = fmt.Sprintf("%s %s", field, order)
		}
	}
	selectBuilder = selectBuilder.OrderBy(orderClause)

	query, args, err := selectBuilder.ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("build select query: %w", err)
	}

	var templates []*dto.Template
	if err := db.Select(&templates, query, args...); err != nil {
		return nil, 0, fmt.Errorf("select templates: %w", err)
	}

	result := make(map[string]*dto.Template, len(templates))
	for _, t := range templates {
		result[t.Code] = t
	}
	return result, total, nil
}

func (r *MessageTemplateRepository) UpdateMetadata(ctx context.Context, code string, metadata dto.TemplateMetadata) error {
	_, err := r.getDB(ctx).ExecContext(ctx, "UPDATE message_templates SET metadata = $1 WHERE code = $2", metadata, code)
	return err
}
