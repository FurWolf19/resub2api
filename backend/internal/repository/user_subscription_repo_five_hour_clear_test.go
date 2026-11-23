//go:build unit

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newUserSubscriptionSQLiteRepo(t *testing.T) (*userSubscriptionRepository, *dbent.Client) {
	t.Helper()

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", t.Name()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(10)

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	return NewUserSubscriptionRepository(client).(*userSubscriptionRepository), client
}

func createUserSubscriptionFixture(t *testing.T, ctx context.Context, client *dbent.Client, windowStart *time.Time, usage float64) int64 {
	t.Helper()

	userRow, err := client.User.Create().
		SetEmail("five-hour-clear@example.com").
		SetPasswordHash("test-password-hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	groupRow, err := client.Group.Create().
		SetName("five-hour-clear-group").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	now := time.Now()
	create := client.UserSubscription.Create().
		SetUserID(userRow.ID).
		SetGroupID(groupRow.ID).
		SetStartsAt(now.Add(-time.Hour)).
		SetExpiresAt(now.Add(24 * time.Hour)).
		SetStatus(service.SubscriptionStatusActive).
		SetAssignedAt(now).
		SetNotes("").
		SetFiveHourUsageUsd(usage).
		SetQuotaMultiplier(1)
	if windowStart != nil {
		create.SetFiveHourWindowStart(*windowStart)
	}
	sub, err := create.Save(ctx)
	require.NoError(t, err)
	return sub.ID
}

func TestUserSubscriptionUpdateClearsFiveHourWindowWhenNil(t *testing.T) {
	repo, client := newUserSubscriptionSQLiteRepo(t)
	ctx := context.Background()
	windowStart := time.Now().Add(-2 * time.Hour)
	subID := createUserSubscriptionFixture(t, ctx, client, &windowStart, 3)

	sub, err := repo.GetByID(ctx, subID)
	require.NoError(t, err)
	require.NotNil(t, sub.FiveHourWindowStart)

	sub.FiveHourWindowStart = nil
	sub.FiveHourUsageUSD = 0
	require.NoError(t, repo.Update(ctx, sub))

	row, err := client.UserSubscription.Query().
		Where(usersubscription.IDEQ(subID)).
		Only(ctx)
	require.NoError(t, err)
	require.Nil(t, row.FiveHourWindowStart, "续费清窗必须写 NULL，SetNillable 在 nil 时不会生成 SET")
	require.Zero(t, row.FiveHourUsageUsd)
	require.Equal(t, 1.0, row.QuotaMultiplier, "清窗不得改动额度倍率")
}

func TestUserSubscriptionUpdateKeepsFiveHourWindowWhenSet(t *testing.T) {
	repo, client := newUserSubscriptionSQLiteRepo(t)
	ctx := context.Background()
	windowStart := time.Now().Add(-2 * time.Hour)
	subID := createUserSubscriptionFixture(t, ctx, client, nil, 0)

	sub, err := repo.GetByID(ctx, subID)
	require.NoError(t, err)
	sub.FiveHourWindowStart = &windowStart
	sub.FiveHourUsageUSD = 1.25
	require.NoError(t, repo.Update(ctx, sub))

	row, err := client.UserSubscription.Query().
		Where(usersubscription.IDEQ(subID)).
		Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, row.FiveHourWindowStart)
	require.WithinDuration(t, windowStart, *row.FiveHourWindowStart, time.Second)
	require.InDelta(t, 1.25, row.FiveHourUsageUsd, 1e-9)
}
