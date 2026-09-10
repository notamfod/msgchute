package template

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devian2011/msgchute/internal/data/repository"
	"github.com/devian2011/msgchute/internal/dto"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

// Run against a disposable PostgreSQL: MSGCHUTE_TEST_DSN=postgres://... go test ./internal/service/template -run TestMetadataPostgres
func TestMetadataPostgres(t *testing.T) {
	dsn := os.Getenv("MSGCHUTE_TEST_DSN")
	if dsn == "" {
		t.Skip("MSGCHUTE_TEST_DSN not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := sqlx.ConnectContext(ctx, "pgx", dsn)
	require.NoError(t, err)
	defer admin.Close()
	schema := fmt.Sprintf("msgchute_test_%d", time.Now().UnixNano())
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
	_, err = db.ExecContext(ctx, `CREATE TABLE message_templates(code text PRIMARY KEY,name text NOT NULL,description text,params jsonb,subject text,body text NOT NULL); INSERT INTO message_templates VALUES ('order','Order','', '{}','', 'Order');`)
	require.NoError(t, err)
	migration, err := os.ReadFile("../../../../migrations/20260910120000_template_metadata.sql")
	require.NoError(t, err)
	parts := strings.Split(string(migration), "-- migrate:down")
	tx, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, parts[0])
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	repo := repository.NewMessageTemplateRepository(db)
	manager := NewManager(db, nil, repo)
	var wg sync.WaitGroup
	failures := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := manager.PatchMetadata(ctx, "order", dto.TemplateMetadata{fmt.Sprintf("client%d", i): map[string]any{"value": i}})
			failures <- err
		}(i)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	stored, err := repo.GetByCode(ctx, "order")
	require.NoError(t, err)
	require.Len(t, stored.Metadata, 12)
	stored.Name = "Renamed"
	stored.Systems = dto.TemplateLabels{"1c", "bitrix24"}
	stored.Channels = dto.TemplateLabels{"sms"}
	require.NoError(t, repo.Update(ctx, stored))
	_, err = manager.PatchMetadata(ctx, "order", dto.TemplateMetadata{"client0": nil})
	require.NoError(t, err)
	matches, total, err := repo.Find(ctx, &dto.MessageTemplateFilter{Systems: []string{"1c"}, Channels: []string{"sms"}})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, matches["order"].Metadata, 11)
	// A legacy update does not erase newer labels or metadata.
	require.NoError(t, repo.Update(ctx, &dto.Template{Code: "order", Name: "Legacy", Body: "Updated"}))
	stored, err = repo.GetByCode(ctx, "order")
	require.NoError(t, err)
	require.Len(t, stored.Metadata, 11)
	require.Equal(t, dto.TemplateLabels{"sms"}, stored.Channels)
	// Down migration restores the old schema only in this disposable test schema.
	tx, err = db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, parts[1])
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	var name string
	require.NoError(t, db.GetContext(ctx, &name, "SELECT name FROM message_templates WHERE code='order'"))
	require.Equal(t, "Legacy", name)
}
