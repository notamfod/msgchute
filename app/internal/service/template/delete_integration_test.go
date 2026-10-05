package template

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/devian2011/msgchute/internal/data/repository"
	"github.com/devian2011/msgchute/internal/dto"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func TestDeletePostgresPreservesMessageHistory(t *testing.T) {
	dsn := os.Getenv("MSGCHUTE_TEST_DSN")
	if dsn == "" {
		t.Skip("MSGCHUTE_TEST_DSN not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	admin, err := sqlx.ConnectContext(ctx, "pgx", dsn)
	require.NoError(t, err)
	defer admin.Close()
	schema := fmt.Sprintf("sms_delete_test_%d", time.Now().UnixNano())
	_, err = admin.ExecContext(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	defer admin.Exec("DROP SCHEMA " + schema + " CASCADE")
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, err := sqlx.ConnectContext(ctx, "pgx", parsed.String())
	require.NoError(t, err)
	defer db.Close()
	_, err = db.ExecContext(ctx, `CREATE TABLE message_templates (
		code text PRIMARY KEY, name text NOT NULL, description text, params jsonb,
		subject text, body text NOT NULL, metadata jsonb, systems jsonb, channels jsonb
	); CREATE TABLE messages (
		id integer PRIMARY KEY, template_code text REFERENCES message_templates(code) ON DELETE SET NULL,
		body text NOT NULL
	);`)
	require.NoError(t, err)
	manager := NewManager(db, nil, repository.NewMessageTemplateRepository(db))
	_, err = manager.Create(&dto.Template{Code: "sms_custom_test", Name: "Test", Body: "Hello"})
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "INSERT INTO messages VALUES (1, 'sms_custom_test', 'Already sent')")
	require.NoError(t, err)
	require.NoError(t, manager.Delete(ctx, "sms_custom_test"))
	require.ErrorIs(t, manager.Delete(ctx, "sms_custom_test"), ErrTemplateNotFound)
	var count int
	require.NoError(t, db.GetContext(ctx, &count, "SELECT COUNT(*) FROM message_templates"))
	require.Zero(t, count)
	var body string
	require.NoError(t, db.GetContext(ctx, &body, "SELECT body FROM messages WHERE id=1 AND template_code IS NULL"))
	require.Equal(t, "Already sent", body)

	const writers = 12
	failures := make(chan error, writers)
	var workers sync.WaitGroup
	for range writers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, err := manager.Create(&dto.Template{Code: "sms_custom_concurrent", Name: "Concurrent", Body: "Hello"})
			failures <- err
		}()
	}
	workers.Wait()
	close(failures)
	created, conflicts := 0, 0
	for err := range failures {
		if err == nil {
			created++
		} else if errors.Is(err, ErrTemplateAlreadyExists) {
			conflicts++
		} else {
			t.Fatalf("unexpected creation error: %v", err)
		}
	}
	require.Equal(t, 1, created)
	require.Equal(t, writers-1, conflicts)
}
