package template

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/devian2011/msgchute/internal/data/repository"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func TestDeleteLocksTemplateAndCommitsOrRollsBack(t *testing.T) {
	for _, tc := range []struct {
		name         string
		missing      bool
		storageError bool
	}{
		{name: "success"},
		{name: "missing", missing: true},
		{name: "storage failure", storageError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			require.NoError(t, err)
			defer db.Close()
			wrapped := sqlx.NewDb(db, "sqlmock")
			manager := NewManager(wrapped, nil, repository.NewMessageTemplateRepository(wrapped))
			mock.ExpectBegin()
			query := mock.ExpectQuery("SELECT code, name, description, params, subject, body, metadata, systems, channels FROM message_templates WHERE code = $1 FOR UPDATE").WithArgs("sms_custom_test")
			if tc.missing {
				query.WillReturnError(sql.ErrNoRows)
			} else {
				query.WillReturnRows(sqlmock.NewRows([]string{"code"}).AddRow("sms_custom_test"))
				exec := mock.ExpectExec("DELETE FROM message_templates WHERE code = $1").WithArgs("sms_custom_test")
				if tc.storageError {
					exec.WillReturnError(errors.New("storage unavailable"))
				} else {
					exec.WillReturnResult(sqlmock.NewResult(0, 1))
				}
			}
			if tc.missing || tc.storageError {
				mock.ExpectRollback()
			} else {
				mock.ExpectCommit()
			}
			err = manager.Delete(context.Background(), "sms_custom_test")
			if tc.missing {
				require.ErrorIs(t, err, ErrTemplateNotFound)
			} else if tc.storageError {
				require.ErrorContains(t, err, "storage unavailable")
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
