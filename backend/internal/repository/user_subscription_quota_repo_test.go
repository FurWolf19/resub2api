//go:build unit

package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestBatchSetGroupQuotaMultipliers_BulkUpdatesSubscriptionsAndDefaults(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	repo := &userSubscriptionRepository{client: client}

	half := 0.5
	mock.ExpectQuery(`(?s)`+regexp.QuoteMeta("UPDATE user_subscriptions us")+`.*`+regexp.QuoteMeta("FROM UNNEST($2::bigint[], $3::double precision[])")+`.*`+regexp.QuoteMeta("RETURNING us.user_id")).
		WithArgs(int64(7), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(int64(1)))
	mock.ExpectExec(`(?s)`+regexp.QuoteMeta("INSERT INTO user_group_quota_defaults")+`.*`+regexp.QuoteMeta("WHERE i.multiplier IS NOT NULL")+`.*`+regexp.QuoteMeta("ON CONFLICT (user_id, group_id)")).
		WithArgs(int64(7), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)`+regexp.QuoteMeta("DELETE FROM user_group_quota_defaults d")+`.*`+regexp.QuoteMeta("i.multiplier IS NULL")).
		WithArgs(int64(7), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	ids, err := repo.BatchSetGroupQuotaMultipliers(ctx, 7, []service.GroupQuotaMultiplierInput{
		{UserID: 1, QuotaMultiplier: &half},
		{UserID: 2, QuotaMultiplier: nil},
	})
	require.NoError(t, err)
	require.Equal(t, []int64{1}, ids)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBatchSetGroupQuotaMultipliers_EmptyEntriesClearsGroup(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	repo := &userSubscriptionRepository{client: client}

	mock.ExpectQuery(`(?s)` + regexp.QuoteMeta("UPDATE user_subscriptions") + `.*` + regexp.QuoteMeta("SET quota_multiplier = 1.0") + `.*` + regexp.QuoteMeta("RETURNING user_id")).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(int64(1)).AddRow(int64(2)))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM user_group_quota_defaults WHERE group_id = $1")).
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 2))

	ids, err := repo.BatchSetGroupQuotaMultipliers(ctx, 7, nil)
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2}, ids)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestClearQuotaMultiplierNeverWritesNull(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	repo := &userSubscriptionRepository{client: client}

	mock.ExpectQuery(`(?s)`+regexp.QuoteMeta("UPDATE user_subscriptions us")+`.*`+regexp.QuoteMeta("SET quota_multiplier = COALESCE(i.multiplier, 1.0)::numeric(10,4)")+`.*`+regexp.QuoteMeta("RETURNING us.user_id")).
		WithArgs(int64(7), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(int64(1)))
	mock.ExpectExec(`(?s)`+regexp.QuoteMeta("INSERT INTO user_group_quota_defaults")+`.*`+regexp.QuoteMeta("WHERE i.multiplier IS NOT NULL")).
		WithArgs(int64(7), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)`+regexp.QuoteMeta("DELETE FROM user_group_quota_defaults d")+`.*`+regexp.QuoteMeta("i.multiplier IS NULL")).
		WithArgs(int64(7), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	_, err = repo.BatchSetGroupQuotaMultipliers(ctx, 7, []service.GroupQuotaMultiplierInput{{UserID: 1, QuotaMultiplier: nil}})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestClearQuotaMultiplierBulkPathUsesOne(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	repo := &userSubscriptionRepository{client: client}

	mock.ExpectQuery(`(?s)` + regexp.QuoteMeta("UPDATE user_subscriptions") + `.*` + regexp.QuoteMeta("SET quota_multiplier = 1.0") + `.*` + regexp.QuoteMeta("deleted_at IS NULL") + `.*` + regexp.QuoteMeta("expires_at > NOW()") + `.*` + regexp.QuoteMeta("RETURNING user_id")).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM user_group_quota_defaults WHERE group_id = $1")).
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	ids, err := repo.BatchSetGroupQuotaMultipliers(ctx, 7, []service.GroupQuotaMultiplierInput{})
	require.NoError(t, err)
	require.Empty(t, ids)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListGroupQuotaMultipliers_IncludesDefaultOnlyUsers(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	repo := &userSubscriptionRepository{client: client}

	mock.ExpectQuery(`(?s)` + regexp.QuoteMeta("SELECT user_id FROM defaults") + `.*` + regexp.QuoteMeta("LEFT JOIN defaults ON defaults.user_id = ids.user_id")).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{
			"user_id", "username", "email", "notes", "status",
			"subscription_id", "quota_multiplier", "default_quota_multiplier", "rate_multiplier",
		}).AddRow(int64(5), "u5", "u5@example.com", "", "active", nil, nil, 0.4, nil))

	entries, err := repo.ListGroupQuotaMultipliers(ctx, 7)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, int64(5), entries[0].UserID)
	require.Nil(t, entries[0].SubscriptionID)
	require.Nil(t, entries[0].QuotaMultiplier)
	require.NotNil(t, entries[0].DefaultQuotaMultiplier)
	require.Equal(t, 0.4, *entries[0].DefaultQuotaMultiplier)
	require.NoError(t, mock.ExpectationsWereMet())
}
