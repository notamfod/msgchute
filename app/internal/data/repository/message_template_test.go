package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/internal/io/storage"
)

func TestMessageTemplateRepository_Create(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	repo := NewMessageTemplateRepository(db)
	ctx := context.Background()

	tmpl := &dto.Template{
		Code:        "welcome_email",
		Name:        "Welcome Template",
		Description: "Sent on registration",
		Params:      nil,
		Subject:     "Welcome!",
		Body:        "Hello, {{name}}",
	}

	expectedSQL := "INSERT INTO message_templates (code,name,description,params,subject,body,metadata,systems,channels) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)"

	mock.ExpectExec(expectedSQL).
		WithArgs(tmpl.Code, tmpl.Name, tmpl.Description, tmpl.Params, tmpl.Subject, tmpl.Body, sqlmock.AnyArg(), tmpl.Systems, tmpl.Channels).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.Create(ctx, tmpl)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMessageTemplateRepository_GetByCode(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	repo := NewMessageTemplateRepository(db)
	ctx := context.Background()
	code := "welcome_email"

	expectedSQL := "SELECT code, name, description, params, subject, body, metadata, systems, channels FROM message_templates WHERE code = $1 FOR UPDATE"

	t.Run("success", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"code", "name", "description", "params", "subject", "body"}).
			AddRow("welcome_email", "Welcome", "Desc", []byte("{}"), "Subj", "Body")

		mock.ExpectQuery(expectedSQL).
			WithArgs(code).
			WillReturnRows(rows)

		res, err := repo.GetByCode(ctx, code)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		assert.Equal(t, "welcome_email", res.Code)
	})

	t.Run("not found", func(t *testing.T) {
		mock.ExpectQuery(expectedSQL).
			WithArgs(code).
			WillReturnError(sql.ErrNoRows)

		res, err := repo.GetByCode(ctx, code)
		assert.Nil(t, err)
		assert.Nil(t, res)
	})

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMessageTemplateRepository_Update(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	repo := NewMessageTemplateRepository(db)
	ctx := context.Background()

	tmpl := &dto.Template{
		Code: "welcome_email",
		Name: "Updated Name",
	}

	expectedSQL := "UPDATE message_templates SET name = $1, description = $2, params = $3, subject = $4, body = $5 WHERE code = $6"

	t.Run("success", func(t *testing.T) {
		mock.ExpectExec(expectedSQL).
			WithArgs(tmpl.Name, tmpl.Description, tmpl.Params, tmpl.Subject, tmpl.Body, tmpl.Code).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.Update(ctx, tmpl)
		assert.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		mock.ExpectExec(expectedSQL).
			WithArgs(tmpl.Name, tmpl.Description, tmpl.Params, tmpl.Subject, tmpl.Body, tmpl.Code).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.Update(ctx, tmpl)
		assert.Error(t, err)
		assert.Equal(t, fmt.Sprintf("template with code %s not found", tmpl.Code), err.Error())
	})

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMessageTemplateRepository_Delete(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	repo := NewMessageTemplateRepository(db)
	ctx := context.Background()
	code := "to_delete"

	expectedSQL := "DELETE FROM message_templates WHERE code = $1"

	t.Run("success", func(t *testing.T) {
		mock.ExpectExec(expectedSQL).
			WithArgs(code).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.Delete(ctx, code)
		assert.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		mock.ExpectExec(expectedSQL).
			WithArgs(code).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.Delete(ctx, code)
		assert.Error(t, err)
	})

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMessageTemplateRepository_Find(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	repo := NewMessageTemplateRepository(db)
	ctx := context.Background()

	t.Run("full filters with correct sorting", func(t *testing.T) {
		codeFilter := "welcome_email"
		searchFilter := "test"
		sortField := "name"
		sortOrder := "DESC"

		filter := &dto.MessageTemplateFilter{
			Limit:     10,
			Offset:    0,
			Code:      []string{codeFilter},
			Search:    &searchFilter,
			SortField: &sortField,
			SortOrder: &sortOrder,
		}

		expectedCountSQL := "SELECT COUNT(*) FROM message_templates WHERE code IN ($1) AND (name ILIKE $2 OR subject ILIKE $3 OR body ILIKE $4)"
		mock.ExpectQuery(expectedCountSQL).
			WithArgs("welcome_email", "%test%", "%test%", "%test%").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		expectedSelectSQL := "SELECT code, name, description, params, subject, body, metadata, systems, channels FROM message_templates WHERE code IN ($1) AND (name ILIKE $2 OR subject ILIKE $3 OR body ILIKE $4) ORDER BY name DESC LIMIT 10"
		rows := sqlmock.NewRows([]string{"code", "name", "description", "params", "subject", "body"}).
			AddRow("welcome_email", "Welcome", "Desc", nil, "Subj", "Body")

		mock.ExpectQuery(expectedSelectSQL).
			WithArgs("welcome_email", "%test%", "%test%", "%test%").
			WillReturnRows(rows)

		templates, total, err := repo.Find(ctx, filter)
		assert.NoError(t, err)
		assert.Equal(t, uint64(1), total)
		assert.Len(t, templates, 1)
		assert.Equal(t, "welcome_email", templates["welcome_email"].Code)
	})

	t.Run("fallback sorting on invalid sort field", func(t *testing.T) {
		invalidField := "body; DROP TABLE message_templates;--"
		filter := &dto.MessageTemplateFilter{
			Limit:     5,
			SortField: &invalidField,
		}

		mock.ExpectQuery("SELECT COUNT(*) FROM message_templates").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

		expectedSelectSQL := "SELECT code, name, description, params, subject, body, metadata, systems, channels FROM message_templates ORDER BY code ASC LIMIT 5"
		mock.ExpectQuery(expectedSelectSQL).
			WillReturnRows(sqlmock.NewRows([]string{"code", "name", "description", "params", "subject", "body"}))

		_, _, err := repo.Find(ctx, filter)
		assert.NoError(t, err)
	})

	t.Run("multiple codes in filter", func(t *testing.T) {
		codes := []string{"code1", "code2"}
		filter := &dto.MessageTemplateFilter{
			Code: codes,
		}

		expectedCountSQL := "SELECT COUNT(*) FROM message_templates WHERE code IN ($1,$2)"
		mock.ExpectQuery(expectedCountSQL).
			WithArgs("code1", "code2").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

		expectedSelectSQL := "SELECT code, name, description, params, subject, body, metadata, systems, channels FROM message_templates WHERE code IN ($1,$2) ORDER BY code ASC"
		rows := sqlmock.NewRows([]string{"code", "name", "description", "params", "subject", "body"}).
			AddRow("code1", "Name1", "Desc1", nil, "Subj1", "Body1").
			AddRow("code2", "Name2", "Desc2", nil, "Subj2", "Body2")

		mock.ExpectQuery(expectedSelectSQL).
			WithArgs("code1", "code2").
			WillReturnRows(rows)

		templates, total, err := repo.Find(ctx, filter)
		assert.NoError(t, err)
		assert.Equal(t, uint64(2), total)
		assert.Len(t, templates, 2)
		assert.Contains(t, templates, "code1")
		assert.Contains(t, templates, "code2")
	})

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMessageTemplateRepository_CreateWithTransaction(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	repo := NewMessageTemplateRepository(db)
	ctx := context.Background()

	mock.ExpectBegin()
	tx, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)

	ctxWithTx := storage.WithTx(ctx, tx)

	tmpl := &dto.Template{
		Code:        "tx_code",
		Name:        "Tx Name",
		Description: "Tx Desc",
		Params:      nil,
		Subject:     "Tx Subj",
		Body:        "Tx Body",
	}

	expectedSQL := "INSERT INTO message_templates (code,name,description,params,subject,body,metadata,systems,channels) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)"

	mock.ExpectExec(expectedSQL).
		WithArgs(tmpl.Code, tmpl.Name, tmpl.Description, tmpl.Params, tmpl.Subject, tmpl.Body, sqlmock.AnyArg(), tmpl.Systems, tmpl.Channels).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = repo.Create(ctxWithTx, tmpl)
	assert.NoError(t, err)

	mock.ExpectCommit()
	assert.NoError(t, tx.Commit())

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTemplateSystemAndChannelFilters(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()
	repo := NewMessageTemplateRepository(db)
	where := " FROM message_templates WHERE (systems @> $1::jsonb OR systems @> $2::jsonb) AND (channels @> $3::jsonb)"
	mock.ExpectQuery("SELECT COUNT(*)"+where).
		WithArgs([]byte(`["1c"]`), []byte(`["bitrix24"]`), []byte(`["sms"]`)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT code, name, description, params, subject, body, metadata, systems, channels"+where+" ORDER BY code ASC").
		WithArgs([]byte(`["1c"]`), []byte(`["bitrix24"]`), []byte(`["sms"]`)).
		WillReturnRows(sqlmock.NewRows([]string{"code"}))
	_, _, err := repo.Find(context.Background(), &dto.MessageTemplateFilter{Systems: []string{"1c", "bitrix24"}, Channels: []string{"sms"}})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
