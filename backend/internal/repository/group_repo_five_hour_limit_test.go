//go:build unit

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newGroupSQLiteRepo(t *testing.T) (*groupRepository, *dbent.Client) {
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

	return newGroupRepositoryWithSQL(client, db), client
}

func float64Ptr(v float64) *float64 { return &v }

func TestGroupRepositoryCreateNormalizesNonPositiveFiveHourLimitToNull(t *testing.T) {
	repo, client := newGroupSQLiteRepo(t)
	ctx := context.Background()

	g := &service.Group{
		Name:             "five-hour-null-create",
		Platform:         service.PlatformOpenAI,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
		RateMultiplier:   1,
		FiveHourLimitUSD: float64Ptr(0),
	}
	require.NoError(t, repo.Create(ctx, g))

	row, err := client.Group.Query().Where(group.IDEQ(g.ID)).Only(ctx)
	require.NoError(t, err)
	require.Nil(t, row.FiveHourLimitUsd, "5h 限额 <=0 写入必须为 NULL")
}

func TestGroupRepositoryUpdateNormalizesNonPositiveFiveHourLimitToNull(t *testing.T) {
	repo, client := newGroupSQLiteRepo(t)
	ctx := context.Background()

	g := &service.Group{
		Name:             "five-hour-null-update",
		Platform:         service.PlatformOpenAI,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
		RateMultiplier:   1,
		FiveHourLimitUSD: float64Ptr(5),
	}
	require.NoError(t, repo.Create(ctx, g))

	g.FiveHourLimitUSD = float64Ptr(0)
	require.NoError(t, repo.Update(ctx, g))

	row, err := client.Group.Query().Where(group.IDEQ(g.ID)).Only(ctx)
	require.NoError(t, err)
	require.Nil(t, row.FiveHourLimitUsd)

	g.FiveHourLimitUSD = float64Ptr(2.5)
	require.NoError(t, repo.Update(ctx, g))
	row, err = client.Group.Query().Where(group.IDEQ(g.ID)).Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, row.FiveHourLimitUsd)
	require.Equal(t, 2.5, *row.FiveHourLimitUsd)
}

func TestGroupRepositoryKeepsHistoricalZeroFiveHourLimit(t *testing.T) {
	_, client := newGroupSQLiteRepo(t)
	ctx := context.Background()

	row, err := client.Group.Create().
		SetName("historical-zero").
		SetFiveHourLimitUsd(0).
		Save(ctx)
	require.NoError(t, err)

	reloaded, err := client.Group.Query().Where(group.IDEQ(row.ID)).Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, reloaded.FiveHourLimitUsd, "历史 0 不迁移，读取侧仍保留原值")
	require.Zero(t, *reloaded.FiveHourLimitUsd)
}
