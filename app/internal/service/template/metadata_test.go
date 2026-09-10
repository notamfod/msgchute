package template

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/devian2011/msgchute/internal/data/repository"
	"github.com/devian2011/msgchute/internal/dto"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMergeMetadataPreservesOtherClientsAndDeletesKeys(t *testing.T) {
	current := dto.TemplateMetadata{"1c": map[string]any{"document": "order"}, "bitrix24": map[string]any{"visible": true, "category": "old"}, "large": json.Number("9007199254740993")}
	patch := dto.TemplateMetadata{"bitrix24": map[string]any{"category": nil, "visible": false, "nested": map[string]any{"remove": nil, "keep": "yes"}}, "list": []any{"a"}}
	mergeMetadata(current, patch)
	require.Equal(t, map[string]any{"document": "order"}, current["1c"])
	require.Equal(t, map[string]any{"visible": false, "nested": map[string]any{"keep": "yes"}}, current["bitrix24"])
	require.Equal(t, json.Number("9007199254740993"), current["large"])
	mergeMetadata(current, dto.TemplateMetadata{"1c": nil})
	require.NotContains(t, current, "1c")
}

func TestPatchMetadataLocksRowAndWritesOnlyAnnotations(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()
	wrapped := sqlx.NewDb(db, "sqlmock")
	manager := NewManager(wrapped, nil, repository.NewMessageTemplateRepository(wrapped))
	const selectSQL = "SELECT code, name, description, params, subject, body, metadata, systems, channels FROM message_templates WHERE code = $1 FOR UPDATE"
	mock.ExpectBegin()
	mock.ExpectQuery(selectSQL).WithArgs("order").WillReturnRows(sqlmock.NewRows([]string{"code", "metadata"}).AddRow("order", []byte(`{"1c":{"id":9007199254740993},"bitrix24":{"visible":true}}`)))
	mock.ExpectExec("UPDATE message_templates SET metadata = $1 WHERE code = $2").WithArgs([]byte(`{"1c":{"id":9007199254740993},"bitrix24":{"visible":false}}`), "order").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	result, err := manager.PatchMetadata(context.Background(), "order", dto.TemplateMetadata{"bitrix24": map[string]any{"visible": false}})
	require.NoError(t, err)
	require.Equal(t, json.Number("9007199254740993"), result["1c"].(map[string]any)["id"])
	mock.ExpectBegin()
	mock.ExpectQuery(selectSQL).WithArgs("missing").WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()
	_, err = manager.PatchMetadata(context.Background(), "missing", dto.TemplateMetadata{})
	require.ErrorIs(t, err, ErrTemplateNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}
