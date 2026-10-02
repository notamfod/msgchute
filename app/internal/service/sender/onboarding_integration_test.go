package sender

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/devian2011/retrier"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/devian2011/msgchute/internal/data/repository"
	"github.com/devian2011/msgchute/internal/dto"
)

// Run against a disposable PostgreSQL:
// MSGCHUTE_TEST_DSN=postgres://... go test ./internal/service/sender -run TestOnboardingPostgres
func TestOnboardingPostgres(t *testing.T) {
	dsn := os.Getenv("MSGCHUTE_TEST_DSN")
	if dsn == "" {
		t.Skip("MSGCHUTE_TEST_DSN not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	db := onboardingTestDB(t, ctx, dsn)
	defer db.Close()

	providers := map[string]*ProviderConfig{
		"tg": {
			Provider:   "telegram",
			Onboarding: &OnboardingConfig{Enabled: true, BotID: "8665735492", SMSTransport: "beeline", ConnectURL: "https://mircli.ru/notification/connect/tg"},
		},
		"beeline": {Provider: "beeline", Channel: "sms"},
	}
	onboarding, err := NewOnboardingStore(db, providers)
	require.NoError(t, err)
	messageRepo := repository.NewMessageRepository(db)
	taskRepo := repository.NewTaskRepository(db)
	resultRepo := repository.NewTaskResultRepository(db)
	queue := NewQueue(ctx, db, taskRepo, messageRepo, providers, onboardingSnapshotGenerator{}, onboarding)
	store := NewWorkerStore(ctx, db, resultRepo, taskRepo, messageRepo, onboarding)

	t.Run("deduplicates invitation and resumes through verified binding after restart", func(t *testing.T) {
		phone := "+79991234567"
		first, _, err := queue.Add(&dto.Message{SenderID: "integration", Transport: "tg", Recipients: dto.Recipients{phone}, Subject: "subject", Body: "body"})
		require.NoError(t, err)
		second, _, err := queue.Add(&dto.Message{SenderID: "integration", Transport: "tg", Recipients: dto.Recipients{phone}, Subject: "subject", Body: "body"})
		require.NoError(t, err)

		tasks, err := store.GetTasks()
		require.NoError(t, err)
		require.Empty(t, tasks, "unknown-phone originals must remain pending")
		var invitationCount int
		require.NoError(t, db.GetContext(ctx, &invitationCount, "SELECT count(*) FROM message_onboarding_invitations WHERE phone=$1", phone))
		require.Equal(t, 1, invitationCount)

		tasks, err = store.GetTasks()
		require.NoError(t, err)
		require.Len(t, tasks, 1)
		invitationTask := tasks[0]
		require.Equal(t, "beeline", invitationTask.Worker)
		var invitationPayload dto.Message
		require.NoError(t, sonic.Unmarshal(invitationTask.Payload, &invitationPayload))
		require.Equal(t, dto.Recipients{phone}, invitationPayload.Recipients)
		require.Contains(t, invitationPayload.Body, "/notification/connect/tg")

		invitationTask.Status = retrier.StatusSuccess
		require.NoError(t, store.SaveTask(&invitationTask, &retrier.TaskExecutionResult{
			ID: uuid.New(), TaskID: invitationTask.ID, Status: retrier.StatusSuccess, RunAt: time.Now(),
		}))
		var sentAt time.Time
		require.NoError(t, db.GetContext(ctx, &sentAt, "SELECT sent_at FROM message_onboarding_invitations WHERE task_id=$1", invitationTask.ID))
		_, err = db.ExecContext(ctx, `INSERT INTO telegram_contact_bindings(bot_id, chat_id, phone, phone_verified_at)
			VALUES ('8665735492', '424242', $1, clock_timestamp())`, phone)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, "UPDATE tasks SET next_run=CURRENT_TIMESTAMP WHERE message_id IN ($1,$2) AND status='pending'", first.ID, second.ID)
		require.NoError(t, err)

		// A new store instance proves that routing does not depend on process memory.
		restarted := NewWorkerStore(ctx, db, resultRepo, taskRepo, messageRepo, onboarding)
		tasks, err = restarted.GetTasks()
		require.NoError(t, err)
		require.Len(t, tasks, 2)
		for _, task := range tasks {
			require.Equal(t, "tg", task.Worker)
			var payload dto.Message
			require.NoError(t, sonic.Unmarshal(task.Payload, &payload))
			require.Equal(t, dto.Recipients{"424242"}, payload.Recipients)
			require.Equal(t, dto.Recipients{phone}, payload.RoutingRecipients)
		}
		var wg sync.WaitGroup
		errs := make(chan error, len(tasks))
		for i := range tasks {
			wg.Add(1)
			go func(task retrier.Task) {
				defer wg.Done()
				task.Status = retrier.StatusSuccess
				errs <- restarted.SaveTask(&task, &retrier.TaskExecutionResult{
					ID: uuid.New(), TaskID: task.ID, Status: retrier.StatusSuccess, RunAt: time.Now(),
				})
			}(tasks[i])
		}
		wg.Wait()
		close(errs)
		for saveErr := range errs {
			require.NoError(t, saveErr)
		}
		var terminalParents int
		require.NoError(t, db.GetContext(ctx, &terminalParents, "SELECT count(*) FROM messages WHERE id IN ($1,$2) AND status='succeeded'", first.ID, second.ID))
		require.Equal(t, 2, terminalParents, "terminal status must persist without an event bus callback")
		var finishedAt *time.Time
		require.NoError(t, db.GetContext(ctx, &finishedAt, "SELECT finished_at FROM message_onboarding_invitations WHERE phone=$1", phone))
		require.NotNil(t, finishedAt, "the shared invitation must close after concurrent parent completions")
	})

	t.Run("late binding cannot replace selected SMS fallback", func(t *testing.T) {
		phone := "+79997654321"
		message, _, err := queue.Add(&dto.Message{SenderID: "integration", Transport: "tg", Recipients: dto.Recipients{phone}, Subject: "subject", Body: "fallback body"})
		require.NoError(t, err)
		_, err = store.GetTasks()
		require.NoError(t, err)
		tasks, err := store.GetTasks()
		require.NoError(t, err)
		var invitationTaskID uuid.UUID
		require.NoError(t, db.GetContext(ctx, &invitationTaskID, "SELECT task_id FROM message_onboarding_invitations WHERE message_id=$1", message.ID))
		var invitationTask retrier.Task
		for _, task := range tasks {
			if task.ID == invitationTaskID {
				invitationTask = task
				break
			}
		}
		require.Equal(t, invitationTaskID, invitationTask.ID)
		invitationTask.Status = retrier.StatusSuccess
		require.NoError(t, store.SaveTask(&invitationTask, &retrier.TaskExecutionResult{
			ID: uuid.New(), TaskID: invitationTask.ID, Status: retrier.StatusSuccess, RunAt: time.Now(),
		}))
		sentAt := time.Now().Add(-2 * time.Hour).UTC()
		_, err = db.ExecContext(ctx, "UPDATE message_onboarding_invitations SET sent_at=$2 WHERE task_id=$1", invitationTask.ID, sentAt)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `INSERT INTO telegram_contact_bindings(bot_id, chat_id, phone, phone_verified_at)
			VALUES ('8665735492', '777777', $1, $2)`, phone, sentAt.Add(61*time.Minute))
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, "UPDATE tasks SET next_run=CURRENT_TIMESTAMP WHERE message_id=$1 AND status='pending'", message.ID)
		require.NoError(t, err)

		tasks, err = store.GetTasks()
		require.NoError(t, err)
		require.Len(t, tasks, 1)
		require.Equal(t, "beeline", tasks[0].Worker)
		var payload dto.Message
		require.NoError(t, sonic.Unmarshal(tasks[0].Payload, &payload))
		require.Equal(t, dto.Recipients{phone}, payload.Recipients)
		require.Empty(t, payload.Subject)
		require.Equal(t, "subject\nfallback body", payload.Body)

		_, err = db.ExecContext(ctx, "UPDATE tasks SET is_processed=false, lock_until=$2, next_run=CURRENT_TIMESTAMP WHERE id=$1", tasks[0].ID, time.Time{})
		require.NoError(t, err)
		tasks, err = NewWorkerStore(ctx, db, resultRepo, taskRepo, messageRepo, onboarding).GetTasks()
		require.NoError(t, err)
		require.Len(t, tasks, 1)
		require.Equal(t, "beeline", tasks[0].Worker, "persisted fallback must not switch after restart")
	})

	t.Run("scheduled original does not create invitation early", func(t *testing.T) {
		message, _, err := queue.Add(&dto.Message{
			SenderID: "integration", Transport: "tg", Recipients: dto.Recipients{"+79990000000"},
			Subject: "subject", Body: "body", Schedule: time.Now().Add(time.Hour),
		})
		require.NoError(t, err)
		tasks, err := store.GetTasks()
		require.NoError(t, err)
		for _, task := range tasks {
			require.NotEqual(t, message.ID, task.ID)
		}
		var count int
		require.NoError(t, db.GetContext(ctx, &count, "SELECT count(*) FROM message_onboarding_invitations WHERE message_id=$1", message.ID))
		require.Zero(t, count)
	})

	t.Run("partial retry preserves successful recipients", func(t *testing.T) {
		message, _, err := queue.Add(&dto.Message{
			SenderID: "integration", Transport: "tg", Recipients: dto.Recipients{"@already-sent", "@retry-me"},
			Subject: "subject", Body: "body",
		})
		require.NoError(t, err)
		var deliveries []struct {
			TaskID    uuid.UUID `db:"task_id"`
			Recipient string    `db:"recipient"`
		}
		require.NoError(t, db.SelectContext(ctx, &deliveries, `SELECT task_id, recipient FROM message_onboarding_deliveries WHERE message_id=$1 ORDER BY recipient`, message.ID))
		require.Len(t, deliveries, 2)
		for _, delivery := range deliveries {
			if delivery.Recipient == "@already-sent" {
				_, err = db.ExecContext(ctx, "UPDATE tasks SET status='success' WHERE id=$1", delivery.TaskID)
				require.NoError(t, err)
			}
		}
		status, found, err := onboarding.AggregateStatus(ctx, message.ID)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, dto.MessageStatusRunning, status, "one successful recipient must not complete the parent")
		for _, delivery := range deliveries {
			if delivery.Recipient == "@retry-me" {
				_, err = db.ExecContext(ctx, "UPDATE tasks SET status='failure' WHERE id=$1", delivery.TaskID)
				require.NoError(t, err)
			}
		}
		status, found, err = onboarding.AggregateStatus(ctx, message.ID)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, dto.MessageStatusFailed, status)
		_, err = db.ExecContext(ctx, "UPDATE messages SET status='failed' WHERE id=$1", message.ID)
		require.NoError(t, err)
		recipients, found, err := onboarding.RetryRecipients(ctx, message.ID)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, []string{"@retry-me"}, recipients)

		retryAt := time.Now().Add(20 * time.Minute)
		_, retryTask, err := queue.Retry(&dto.MessageRetryRequest{ID: message.ID, Schedule: retryAt})
		require.NoError(t, err)
		require.WithinDuration(t, retryAt, retryTask.NextRun, time.Millisecond)
		var parentStatus dto.MessageStatus
		require.NoError(t, db.GetContext(ctx, &parentStatus, "SELECT status FROM messages WHERE id=$1", message.ID))
		require.Equal(t, dto.MessageStatusRunning, parentStatus, "a delayed retry must reopen the parent atomically")
		var retriedRecipient string
		require.NoError(t, db.GetContext(ctx, &retriedRecipient, "SELECT recipient FROM message_onboarding_deliveries WHERE task_id=$1", retryTask.ID))
		require.Equal(t, "@retry-me", retriedRecipient)
	})

	t.Run("retry cannot skip an in-flight terminal task", func(t *testing.T) {
		message, queuedTask, err := queue.Add(&dto.Message{
			SenderID: "integration", Transport: "tg", Recipients: dto.Recipients{"@race-recipient"},
			Subject: "subject", Body: "body",
		})
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, "UPDATE messages SET status='failed' WHERE id=$1", message.ID)
		require.NoError(t, err)
		tasks, err := store.GetTasks()
		require.NoError(t, err)
		var terminalTask *retrier.Task
		for i := range tasks {
			if tasks[i].ID == queuedTask.ID {
				terminalTask = &tasks[i]
				break
			}
		}
		require.NotNil(t, terminalTask)
		terminalTask.Status = retrier.StatusSuccess
		_, err = db.ExecContext(ctx, `CREATE FUNCTION block_onboarding_task_update() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN PERFORM pg_advisory_xact_lock(20261002); RETURN NEW; END $$;
			CREATE TRIGGER block_onboarding_task_update BEFORE UPDATE ON tasks
			FOR EACH ROW WHEN (OLD.id = '`+queuedTask.ID.String()+`'::uuid) EXECUTE FUNCTION block_onboarding_task_update();`)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = db.ExecContext(context.Background(), "DROP TRIGGER IF EXISTS block_onboarding_task_update ON tasks; DROP FUNCTION IF EXISTS block_onboarding_task_update()")
		})

		blocker, err := db.BeginTxx(ctx, nil)
		require.NoError(t, err)
		_, err = blocker.ExecContext(ctx, "SELECT pg_advisory_xact_lock(20261002)")
		require.NoError(t, err)
		defer blocker.Rollback()

		saved := make(chan error, 1)
		go func() {
			saved <- store.SaveTask(terminalTask, &retrier.TaskExecutionResult{
				ID: uuid.New(), TaskID: terminalTask.ID, Status: retrier.StatusSuccess, RunAt: time.Now(),
			})
		}()

		// The task update trigger holds SaveTask after it has acquired the parent
		// and task locks. Give the goroutine time to reach that deterministic gate.
		time.Sleep(200 * time.Millisecond)

		_, _, retryErr := queue.Retry(&dto.MessageRetryRequest{ID: message.ID})
		require.Error(t, retryErr, "retry must not create work while terminal persistence owns the parent lock")
		require.NoError(t, blocker.Rollback())
		require.NoError(t, <-saved)
		var deliveryCount int
		require.NoError(t, db.GetContext(ctx, &deliveryCount, "SELECT count(*) FROM message_onboarding_deliveries WHERE message_id=$1", message.ID))
		require.Equal(t, 1, deliveryCount)
	})

	t.Run("known rich phone stays on legacy messenger route if binding disappears", func(t *testing.T) {
		phone := "+79993334455"
		_, err := db.ExecContext(ctx, `INSERT INTO telegram_contact_bindings(bot_id, chat_id, phone, phone_verified_at)
			VALUES ('8665735492', '343434', $1, clock_timestamp())`, phone)
		require.NoError(t, err)
		message, task, err := queue.Add(&dto.Message{
			SenderID: "integration", Transport: "tg", Recipients: dto.Recipients{phone},
			Subject: "<b>subject</b>", Body: "<b>body</b>", Meta: dto.MessageMeta{"Attachments": []string{"invoice.pdf"}},
		})
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, "DELETE FROM telegram_contact_bindings WHERE bot_id='8665735492' AND phone=$1", phone)
		require.NoError(t, err)

		var selectedPath string
		require.NoError(t, db.GetContext(ctx, &selectedPath, "SELECT selected_path FROM message_onboarding_deliveries WHERE task_id=$1", task.ID))
		require.Equal(t, "legacy", selectedPath)
		var invitations int
		require.NoError(t, db.GetContext(ctx, &invitations, "SELECT count(*) FROM message_onboarding_invitations WHERE message_id=$1", message.ID))
		require.Zero(t, invitations)

		tasks, err := store.GetTasks()
		require.NoError(t, err)
		var routed *retrier.Task
		for i := range tasks {
			if tasks[i].ID == task.ID {
				routed = &tasks[i]
				break
			}
		}
		require.NotNil(t, routed)
		require.Equal(t, "tg", routed.Worker)
		var payload dto.Message
		require.NoError(t, sonic.Unmarshal(routed.Payload, &payload))
		require.Equal(t, dto.Recipients{phone}, payload.Recipients)
		require.Equal(t, []any{"invoice.pdf"}, payload.Meta["Attachments"])
	})

	t.Run("failed invitation selects fallback without an hour wait", func(t *testing.T) {
		phone := "+79998887766"
		message, _, err := queue.Add(&dto.Message{SenderID: "integration", Transport: "tg", Recipients: dto.Recipients{phone}, Subject: "subject", Body: "body"})
		require.NoError(t, err)
		_, err = store.GetTasks()
		require.NoError(t, err)
		tasks, err := store.GetTasks()
		require.NoError(t, err)
		require.Len(t, tasks, 1)
		invitationTask := tasks[0]
		require.Equal(t, invitationMaxRetries, invitationTask.MaxRetries)
		invitationTask.Status = retrier.StatusFailure
		require.NoError(t, store.SaveTask(&invitationTask, &retrier.TaskExecutionResult{
			ID: uuid.New(), TaskID: invitationTask.ID, Status: retrier.StatusFailure, RunAt: time.Now(),
		}))
		_, err = db.ExecContext(ctx, "UPDATE tasks SET next_run=CURRENT_TIMESTAMP WHERE message_id=$1 AND status='pending'", message.ID)
		require.NoError(t, err)
		tasks, err = store.GetTasks()
		require.NoError(t, err)
		require.Len(t, tasks, 1)
		require.Equal(t, "beeline", tasks[0].Worker)
	})

	t.Run("canonical phone variants create one original delivery", func(t *testing.T) {
		message, _, err := queue.Add(&dto.Message{
			SenderID: "integration", Transport: "tg",
			Recipients: dto.Recipients{"+7 (900) 111-22-33", "79001112233", "89001112233", "@same", "@same"},
			Subject:    "subject", Body: "body",
		})
		require.NoError(t, err)
		var count int
		require.NoError(t, db.GetContext(ctx, &count, "SELECT count(*) FROM message_onboarding_deliveries WHERE message_id=$1", message.ID))
		require.Equal(t, 2, count)
		require.Len(t, message.Recipients, 5, "the audit record keeps the submitted recipient list")
	})

	t.Run("explicit deadline bounds the wait", func(t *testing.T) {
		deadline := time.Now().Add(30 * time.Minute).UTC()
		message, _, err := queue.Add(&dto.Message{
			SenderID: "integration", Transport: "tg", Recipients: dto.Recipients{"+79995554433"},
			Subject: "subject", Body: "body", Deadline: deadline,
		})
		require.NoError(t, err)
		_, err = store.GetTasks()
		require.NoError(t, err)
		tasks, err := store.GetTasks()
		require.NoError(t, err)
		var invitationTaskID uuid.UUID
		require.NoError(t, db.GetContext(ctx, &invitationTaskID, "SELECT task_id FROM message_onboarding_invitations WHERE message_id=$1", message.ID))
		var invitationTask retrier.Task
		for _, task := range tasks {
			if task.ID == invitationTaskID {
				invitationTask = task
				break
			}
		}
		require.Equal(t, invitationTaskID, invitationTask.ID)
		invitationTask.Status = retrier.StatusSuccess
		require.NoError(t, store.SaveTask(&invitationTask, &retrier.TaskExecutionResult{
			ID: uuid.New(), TaskID: invitationTask.ID, Status: retrier.StatusSuccess, RunAt: time.Now(),
		}))
		_, err = db.ExecContext(ctx, "UPDATE tasks SET next_run=CURRENT_TIMESTAMP WHERE message_id=$1 AND status='pending'", message.ID)
		require.NoError(t, err)
		tasks, err = store.GetTasks()
		require.NoError(t, err)
		require.Empty(t, tasks)
		var waitUntil time.Time
		require.NoError(t, db.GetContext(ctx, &waitUntil, "SELECT wait_until FROM message_onboarding_deliveries WHERE message_id=$1", message.ID))
		require.WithinDuration(t, deadline, waitUntil, time.Millisecond)
	})
}

type onboardingSnapshotGenerator struct{}

func (onboardingSnapshotGenerator) GenerateMessage(message *dto.Message) (string, string, error) {
	return message.Subject, message.Body, nil
}

func onboardingTestDB(t *testing.T, ctx context.Context, dsn string) *sqlx.DB {
	t.Helper()
	admin, err := sqlx.ConnectContext(ctx, "pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { admin.Close() })
	schema := fmt.Sprintf("onboarding_test_%d", time.Now().UnixNano())
	_, err = admin.ExecContext(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE") })
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, err := sqlx.ConnectContext(ctx, "pgx", parsed.String())
	require.NoError(t, err)

	migrations, err := filepath.Glob("../../../../migrations/*.sql")
	require.NoError(t, err)
	sort.Strings(migrations)
	for _, migrationPath := range migrations {
		migration, readErr := os.ReadFile(migrationPath)
		require.NoError(t, readErr)
		up := strings.Split(string(migration), "-- migrate:down")[0]
		_, err = db.ExecContext(ctx, up)
		require.NoErrorf(t, err, "migration %s", filepath.Base(migrationPath))
	}
	_, err = db.ExecContext(ctx, `
		CREATE TABLE telegram_contact_bindings(
			bot_id bigint NOT NULL, chat_id bigint NOT NULL, phone text NOT NULL, phone_verified_at timestamptz
		);
		CREATE TABLE max_contact_bindings(
			bot_id bigint NOT NULL, user_id bigint NOT NULL, phone text NOT NULL, phone_verified_at timestamptz
		);`)
	require.NoError(t, err)
	return db
}
