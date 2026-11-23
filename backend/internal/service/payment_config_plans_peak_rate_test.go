//go:build unit

package service

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newPaymentConfigPlansTestService(t *testing.T) *PaymentConfigService {
	t.Helper()

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", t.Name()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	return &PaymentConfigService{entClient: client}
}

func TestGetGroupInfoMapNormalizesLegacyPeakWindows(t *testing.T) {
	ctx := context.Background()
	svc := newPaymentConfigPlansTestService(t)
	groupRow, err := svc.entClient.Group.Create().
		SetName("legacy-payment-plan-group").
		SetPlatform(PlatformOpenAI).
		SetPeakRateWindows([]domain.PeakRateWindow{
			{Start: "9:5", End: "18:5", Multiplier: 2, Days: []int{1, 2, 3, 4, 5}},
		}).
		Save(ctx)
	require.NoError(t, err)

	info := svc.GetGroupInfoMap(ctx, []*dbent.SubscriptionPlan{{GroupID: groupRow.ID}})
	require.Contains(t, info, groupRow.ID)
	require.Len(t, info[groupRow.ID].PeakRateWindows, 1)
	require.Equal(t, "09:05", info[groupRow.ID].PeakRateWindows[0].Start)
	require.Equal(t, "18:05", info[groupRow.ID].PeakRateWindows[0].End)
}

func TestGetGroupInfoMapKeepsInvalidPeakWindows(t *testing.T) {
	ctx := context.Background()
	svc := newPaymentConfigPlansTestService(t)
	groupRow, err := svc.entClient.Group.Create().
		SetName("invalid-payment-plan-group").
		SetPlatform(PlatformOpenAI).
		SetPeakRateWindows([]domain.PeakRateWindow{
			{Start: "25:00", End: "26:00", Multiplier: 2},
		}).
		Save(ctx)
	require.NoError(t, err)

	info := svc.GetGroupInfoMap(ctx, []*dbent.SubscriptionPlan{{GroupID: groupRow.ID}})
	require.Contains(t, info, groupRow.ID)
	require.Len(t, info[groupRow.ID].PeakRateWindows, 1)
	require.Equal(t, "25:00", info[groupRow.ID].PeakRateWindows[0].Start, "非法时间保持原样")
	require.Equal(t, "26:00", info[groupRow.ID].PeakRateWindows[0].End)
}
