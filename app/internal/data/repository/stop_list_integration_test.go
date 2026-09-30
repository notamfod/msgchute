package repository

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/internal/service/stoplist"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

// Run against a disposable PostgreSQL: STOP_LIST_TEST_DSN=postgres://... go test ./internal/data/repository -run TestStopListPostgres
func TestStopListPostgres(t *testing.T) {
	dsn := os.Getenv("STOP_LIST_TEST_DSN")
	if dsn == "" {
		t.Skip("STOP_LIST_TEST_DSN not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := sqlx.ConnectContext(ctx, "pgx", dsn)
	require.NoError(t, err)
	defer admin.Close()
	schema := fmt.Sprintf("stop_list_test_%d", time.Now().UnixNano())
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
	migration, err := os.ReadFile("../../../../migrations/20260930000000_notification_stop_list.sql")
	require.NoError(t, err)
	parts := strings.Split(string(migration), "-- migrate:down")
	up := parts[0]
	_, err = db.ExecContext(ctx, up)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "CREATE TABLE messages (id UUID PRIMARY KEY)")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "INSERT INTO notification_stop_list (id, recipient, kind) VALUES ('00000000-0000-0000-0000-000000000001', 'old@example.com', 'email')")
	require.NoError(t, err)
	preferencesMigration, err := os.ReadFile("../../../../migrations/20260930010000_subscription_preferences.sql")
	require.NoError(t, err)
	preferencesParts := strings.Split(string(preferencesMigration), "-- migrate:down")
	_, err = db.ExecContext(ctx, preferencesParts[0])
	require.NoError(t, err)
	var migrated dto.StopListEntry
	require.NoError(t, db.GetContext(ctx, &migrated, "SELECT id, recipient, kind, reason, subscriptions, blocked_all, created_at FROM notification_stop_list WHERE recipient = 'old@example.com'"))
	require.True(t, migrated.BlockedAll)
	require.Equal(t, dto.SubscriptionList{"email.order"}, migrated.Subscriptions)
	repo := NewStopListRepository(db)
	service := stoplist.New(repo)
	entry, err := service.Create(ctx, &dto.StopListEntry{Kind: "phone", Recipient: "+85221234567", Reason: "requested"})
	require.NoError(t, err)
	require.False(t, entry.CreatedAt.IsZero())
	_, err = service.Create(ctx, &dto.StopListEntry{Kind: "phone", Recipient: "+85221234567"})
	require.ErrorIs(t, err, stoplist.ErrAlreadyExists)
	blocked, err := repo.Blocked(ctx, "phone", []string{"+85221234567", "+12025550123"})
	require.NoError(t, err)
	require.Contains(t, blocked, "+85221234567")
	items, total, err := repo.Find(ctx, dto.StopListFilter{Limit: 20})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, items, 2)
	require.NoError(t, repo.Delete(ctx, entry.ID.String()))
	blocked, err = repo.Blocked(ctx, "phone", []string{"+85221234567"})
	require.NoError(t, err)
	require.Empty(t, blocked)
	all, total, err := repo.List(ctx, dto.StopListFilter{Limit: 20})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	for _, item := range all {
		if item.Recipient == "+85221234567" {
			require.False(t, item.BlockedAll)
		}
	}
	updated, err := service.Upsert(ctx, &dto.StopListEntry{Kind: "phone", Recipient: "+85221234567", Subscriptions: dto.SubscriptionList{"whatsapp.news"}, BlockedAll: false})
	require.NoError(t, err)
	require.Equal(t, entry.ID, updated.ID)
	require.Equal(t, dto.SubscriptionList{"whatsapp.news"}, updated.Subscriptions)
	reblocked, err := service.Create(ctx, &dto.StopListEntry{Kind: "phone", Recipient: "+85221234567", Reason: "again"})
	require.NoError(t, err)
	require.True(t, reblocked.BlockedAll)
	require.Equal(t, dto.SubscriptionList{"whatsapp.news"}, reblocked.Subscriptions)
	require.NoError(t, service.Delete(ctx, reblocked.ID.String()))
	all, total, err = repo.List(ctx, dto.StopListFilter{Limit: 20})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	for _, item := range all {
		if item.Recipient == "+85221234567" {
			require.False(t, item.BlockedAll)
			require.Equal(t, dto.SubscriptionList{"whatsapp.news"}, item.Subscriptions)
		}
	}
	_, err = db.ExecContext(ctx, preferencesParts[1])
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, parts[1])
	require.NoError(t, err)
	var tableName *string
	require.NoError(t, db.GetContext(ctx, &tableName, "SELECT to_regclass('notification_stop_list')"))
	require.Nil(t, tableName)
}
