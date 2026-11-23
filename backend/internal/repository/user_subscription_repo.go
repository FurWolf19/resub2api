package repository

import (
	"context"
	"database/sql"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type userSubscriptionRepository struct {
	client *dbent.Client
}

func NewUserSubscriptionRepository(client *dbent.Client) service.UserSubscriptionRepository {
	return &userSubscriptionRepository{client: client}
}

func (r *userSubscriptionRepository) Create(ctx context.Context, sub *service.UserSubscription) error {
	if sub == nil {
		return service.ErrSubscriptionNilInput
	}

	client := clientFromContext(ctx, r.client)
	builder := client.UserSubscription.Create().
		SetUserID(sub.UserID).
		SetGroupID(sub.GroupID).
		SetExpiresAt(sub.ExpiresAt).
		SetNillableDailyWindowStart(sub.DailyWindowStart).
		SetNillableWeeklyWindowStart(sub.WeeklyWindowStart).
		SetNillableMonthlyWindowStart(sub.MonthlyWindowStart).
		SetNillableFiveHourWindowStart(sub.FiveHourWindowStart).
		SetDailyUsageUsd(sub.DailyUsageUSD).
		SetWeeklyUsageUsd(sub.WeeklyUsageUSD).
		SetMonthlyUsageUsd(sub.MonthlyUsageUSD).
		SetFiveHourUsageUsd(sub.FiveHourUsageUSD).
		SetQuotaMultiplier(sub.QuotaMultiplier).
		SetNillableAssignedBy(sub.AssignedBy)

	if sub.StartsAt.IsZero() {
		builder.SetStartsAt(time.Now())
	} else {
		builder.SetStartsAt(sub.StartsAt)
	}
	if sub.Status != "" {
		builder.SetStatus(sub.Status)
	}
	if !sub.AssignedAt.IsZero() {
		builder.SetAssignedAt(sub.AssignedAt)
	}
	// Keep compatibility with historical behavior: always store notes as a string value.
	builder.SetNotes(sub.Notes)

	created, err := builder.Save(ctx)
	if err == nil {
		applyUserSubscriptionEntityToService(sub, created)
	}
	return translatePersistenceError(err, nil, service.ErrSubscriptionAlreadyExists)
}

func (r *userSubscriptionRepository) GetByID(ctx context.Context, id int64) (*service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.UserSubscription.Query().
		Where(usersubscription.IDEQ(id)).
		WithUser().
		WithGroup().
		WithAssignedByUser().
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	return userSubscriptionEntityToService(m), nil
}

func (r *userSubscriptionRepository) GetByIDForUpdate(ctx context.Context, id int64) (*service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.UserSubscription.Query().
		Where(usersubscription.IDEQ(id)).
		ForUpdate().
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	return userSubscriptionEntityToService(m), nil
}

func (r *userSubscriptionRepository) GetByIDIncludeDeleted(ctx context.Context, id int64) (*service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	queryCtx := mixins.SkipSoftDelete(ctx)
	m, err := client.UserSubscription.Query().
		Where(usersubscription.IDEQ(id)).
		WithUser().
		WithGroup().
		WithAssignedByUser().
		Only(queryCtx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	return userSubscriptionEntityToServicePreserveStatus(m), nil
}

func (r *userSubscriptionRepository) GetByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.UserSubscription.Query().
		Where(usersubscription.UserIDEQ(userID), usersubscription.GroupIDEQ(groupID)).
		WithGroup().
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	return userSubscriptionEntityToService(m), nil
}

func (r *userSubscriptionRepository) GetActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.UserSubscription.Query().
		Where(
			usersubscription.UserIDEQ(userID),
			usersubscription.GroupIDEQ(groupID),
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			usersubscription.ExpiresAtGT(time.Now()),
		).
		WithGroup().
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	return userSubscriptionEntityToService(m), nil
}

// 已软删除的订阅不可更新：服务层必须先经 GetByID/GetByIDForUpdate 读取
// （两者都过滤 deleted_at），读不到直接返回 ErrSubscriptionNotFound。
func (r *userSubscriptionRepository) Update(ctx context.Context, sub *service.UserSubscription) error {
	if sub == nil {
		return service.ErrSubscriptionNilInput
	}

	client := clientFromContext(ctx, r.client)
	builder := client.UserSubscription.UpdateOneID(sub.ID).
		SetUserID(sub.UserID).
		SetGroupID(sub.GroupID).
		SetStartsAt(sub.StartsAt).
		SetExpiresAt(sub.ExpiresAt).
		SetStatus(sub.Status).
		SetNillableDailyWindowStart(sub.DailyWindowStart).
		SetNillableWeeklyWindowStart(sub.WeeklyWindowStart).
		SetNillableMonthlyWindowStart(sub.MonthlyWindowStart).
		SetDailyUsageUsd(sub.DailyUsageUSD).
		SetWeeklyUsageUsd(sub.WeeklyUsageUSD).
		SetMonthlyUsageUsd(sub.MonthlyUsageUSD).
		SetFiveHourUsageUsd(sub.FiveHourUsageUSD).
		SetQuotaMultiplier(sub.QuotaMultiplier).
		SetNillableAssignedBy(sub.AssignedBy).
		SetAssignedAt(sub.AssignedAt).
		SetNotes(sub.Notes)

	// SetNillable 在 nil 时不会生成 SET 子句，无法把已有窗口清空；续费必须显式写 NULL。
	if sub.FiveHourWindowStart != nil {
		builder = builder.SetFiveHourWindowStart(*sub.FiveHourWindowStart)
	} else {
		builder = builder.ClearFiveHourWindowStart()
	}

	updated, err := builder.Save(ctx)
	if err == nil {
		applyUserSubscriptionEntityToService(sub, updated)
		return nil
	}
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, service.ErrSubscriptionAlreadyExists)
}

func (r *userSubscriptionRepository) Delete(ctx context.Context, id int64) error {
	// Match GORM semantics: deleting a missing row is not an error.
	client := clientFromContext(ctx, r.client)
	_, err := client.UserSubscription.Delete().Where(usersubscription.IDEQ(id)).Exec(ctx)
	return err
}

func (r *userSubscriptionRepository) Restore(ctx context.Context, subscriptionID int64, restoredStatus string) (*service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	queryCtx := mixins.SkipSoftDelete(ctx)
	_, err := client.UserSubscription.UpdateOneID(subscriptionID).
		SetStatus(restoredStatus).
		ClearDeletedAt().
		SetUpdatedAt(time.Now()).
		Save(queryCtx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, service.ErrSubscriptionRestoreConflict)
	}
	return r.GetByID(ctx, subscriptionID)
}

func (r *userSubscriptionRepository) ListByUserID(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	subs, err := client.UserSubscription.Query().
		Where(usersubscription.UserIDEQ(userID)).
		WithGroup().
		Order(dbent.Desc(usersubscription.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return userSubscriptionEntitiesToService(subs), nil
}

func (r *userSubscriptionRepository) ListActiveByUserID(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	subs, err := client.UserSubscription.Query().
		Where(
			usersubscription.UserIDEQ(userID),
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			usersubscription.ExpiresAtGT(time.Now()),
		).
		WithGroup().
		Order(dbent.Desc(usersubscription.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return userSubscriptionEntitiesToService(subs), nil
}

func (r *userSubscriptionRepository) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	q := client.UserSubscription.Query().Where(usersubscription.GroupIDEQ(groupID))

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	subs, err := q.
		WithUser().
		WithGroup().
		Order(dbent.Desc(usersubscription.FieldCreatedAt)).
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	return userSubscriptionEntitiesToService(subs), paginationResultFromTotal(int64(total), params), nil
}

func (r *userSubscriptionRepository) List(ctx context.Context, params pagination.PaginationParams, userID, groupID *int64, status, platform, sortBy, sortOrder string) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	q := client.UserSubscription.Query()
	includeSoftDeleted := status == "" || status == service.SubscriptionStatusRevoked
	if userID != nil {
		q = q.Where(usersubscription.UserIDEQ(*userID))
	}
	if groupID != nil {
		q = q.Where(usersubscription.GroupIDEQ(*groupID))
	}
	if platform != "" {
		groupPredicates := []predicate.Group{group.PlatformEQ(platform)}
		if includeSoftDeleted {
			groupPredicates = append(groupPredicates, group.DeletedAtIsNil())
		}
		q = q.Where(usersubscription.HasGroupWith(groupPredicates...))
	}

	// Status filtering with real-time expiration check
	now := time.Now()
	switch status {
	case service.SubscriptionStatusActive:
		// Active: status is active AND not yet expired
		q = q.Where(
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			usersubscription.ExpiresAtGT(now),
		)
	case service.SubscriptionStatusExpired:
		// Expired: status is expired OR (status is active but already expired)
		q = q.Where(
			usersubscription.Or(
				usersubscription.StatusEQ(service.SubscriptionStatusExpired),
				usersubscription.And(
					usersubscription.StatusEQ(service.SubscriptionStatusActive),
					usersubscription.ExpiresAtLTE(now),
				),
			),
		)
	case service.SubscriptionStatusRevoked:
		// Revoked is a DTO/API display state backed by user_subscriptions.deleted_at.
		q = q.Where(usersubscription.DeletedAtNotNil())
	case "":
		// No filter. Use SkipSoftDelete below so admin "all status" includes revoked history.
	default:
		// Other persisted status.
		q = q.Where(usersubscription.StatusEQ(status))
	}

	queryCtx := ctx
	if includeSoftDeleted {
		queryCtx = mixins.SkipSoftDelete(ctx)
	}

	total, err := q.Clone().Count(queryCtx)
	if err != nil {
		return nil, nil, err
	}

	if !includeSoftDeleted {
		q = q.WithUser().WithGroup().WithAssignedByUser()
	}

	// Determine sort field
	var field string
	switch sortBy {
	case "expires_at":
		field = usersubscription.FieldExpiresAt
	case "status":
		field = usersubscription.FieldStatus
	default:
		field = usersubscription.FieldCreatedAt
	}

	// Determine sort order (default: desc)
	if sortOrder == "asc" && sortBy != "" {
		q = q.Order(dbent.Asc(field))
	} else {
		q = q.Order(dbent.Desc(field))
	}

	subs, err := q.
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(queryCtx)
	if err != nil {
		return nil, nil, err
	}

	result := userSubscriptionEntitiesToService(subs)
	if includeSoftDeleted {
		if err := r.attachUserSubscriptionRelations(ctx, result); err != nil {
			return nil, nil, err
		}
	}

	return result, paginationResultFromTotal(int64(total), params), nil
}

func (r *userSubscriptionRepository) ExistsByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error) {
	client := clientFromContext(ctx, r.client)
	return client.UserSubscription.Query().
		Where(usersubscription.UserIDEQ(userID), usersubscription.GroupIDEQ(groupID)).
		Exist(ctx)
}

func (r *userSubscriptionRepository) ExistsActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error) {
	return r.ExistsByUserIDAndGroupID(ctx, userID, groupID)
}

func (r *userSubscriptionRepository) ExtendExpiry(ctx context.Context, subscriptionID int64, newExpiresAt time.Time) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.UserSubscription.UpdateOneID(subscriptionID).
		SetExpiresAt(newExpiresAt).
		Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
}

// 已软删除的订阅不可更新：调用方须先经 GetByIDForUpdate 过滤 deleted_at。
func (r *userSubscriptionRepository) UpdateStatus(ctx context.Context, subscriptionID int64, status string) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.UserSubscription.UpdateOneID(subscriptionID).
		SetStatus(status).
		Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
}

// 已软删除的订阅不可更新：调用方须先经 GetByIDForUpdate 过滤 deleted_at。
func (r *userSubscriptionRepository) UpdateNotes(ctx context.Context, subscriptionID int64, notes string) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.UserSubscription.UpdateOneID(subscriptionID).
		SetNotes(notes).
		Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
}

func (r *userSubscriptionRepository) ActivateWindows(ctx context.Context, id int64, dailyStart, periodicStart time.Time) error {
	client := clientFromContext(ctx, r.client)
	n, err := client.UserSubscription.Update().
		Where(
			usersubscription.IDEQ(id),
			usersubscription.DailyWindowStartIsNil(),
			usersubscription.WeeklyWindowStartIsNil(),
			usersubscription.MonthlyWindowStartIsNil(),
		).
		SetDailyWindowStart(dailyStart).
		SetWeeklyWindowStart(periodicStart).
		SetMonthlyWindowStart(periodicStart).
		Save(ctx)
	return r.translateConditionalWindowReset(ctx, client, id, n, err)
}

func (r *userSubscriptionRepository) ResetUsageWindows(ctx context.Context, id int64, resetDaily, resetWeekly, resetMonthly bool, dailyStart, periodicStart time.Time) error {
	client := clientFromContext(ctx, r.client)
	update := client.UserSubscription.UpdateOneID(id)
	if resetDaily {
		update.SetDailyUsageUsd(0).SetDailyWindowStart(dailyStart)
	}
	if resetWeekly {
		update.SetWeeklyUsageUsd(0).SetWeeklyWindowStart(periodicStart)
	}
	if resetMonthly {
		update.SetMonthlyUsageUsd(0).SetMonthlyWindowStart(periodicStart)
	}
	_, err := update.Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
}

func (r *userSubscriptionRepository) ResetDailyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)
	query := client.UserSubscription.Update().Where(usersubscription.IDEQ(id))
	if expectedWindowStart == nil {
		query = query.Where(usersubscription.DailyWindowStartIsNil())
	} else {
		query = query.Where(usersubscription.DailyWindowStartEQ(*expectedWindowStart))
	}
	n, err := query.
		SetDailyUsageUsd(0).
		SetDailyWindowStart(newWindowStart).
		Save(ctx)
	return r.translateConditionalWindowReset(ctx, client, id, n, err)
}

func (r *userSubscriptionRepository) ResetWeeklyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)
	query := client.UserSubscription.Update().Where(usersubscription.IDEQ(id))
	if expectedWindowStart == nil {
		query = query.Where(usersubscription.WeeklyWindowStartIsNil())
	} else {
		query = query.Where(usersubscription.WeeklyWindowStartEQ(*expectedWindowStart))
	}
	n, err := query.
		SetWeeklyUsageUsd(0).
		SetWeeklyWindowStart(newWindowStart).
		Save(ctx)
	return r.translateConditionalWindowReset(ctx, client, id, n, err)
}

func (r *userSubscriptionRepository) ResetMonthlyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)
	query := client.UserSubscription.Update().Where(usersubscription.IDEQ(id))
	if expectedWindowStart == nil {
		query = query.Where(usersubscription.MonthlyWindowStartIsNil())
	} else {
		query = query.Where(usersubscription.MonthlyWindowStartEQ(*expectedWindowStart))
	}
	n, err := query.
		SetMonthlyUsageUsd(0).
		SetMonthlyWindowStart(newWindowStart).
		Save(ctx)
	return r.translateConditionalWindowReset(ctx, client, id, n, err)
}

// ResetFiveHourUsage 以 CAS 方式清零 5h 窗口，并把 window_start 置空。
// 窗口不会自动重新开始，下一笔 cost>0 消费会在 IncrementUsage 中开启新窗口。
func (r *userSubscriptionRepository) ResetFiveHourUsage(ctx context.Context, id int64, expectedWindowStart *time.Time) error {
	client := clientFromContext(ctx, r.client)
	query := client.UserSubscription.Update().Where(usersubscription.IDEQ(id))
	if expectedWindowStart == nil {
		query = query.Where(usersubscription.FiveHourWindowStartIsNil())
	} else {
		query = query.Where(usersubscription.FiveHourWindowStartEQ(*expectedWindowStart))
	}
	n, err := query.
		SetFiveHourUsageUsd(0).
		ClearFiveHourWindowStart().
		Save(ctx)
	return r.translateConditionalWindowReset(ctx, client, id, n, err)
}

func (r *userSubscriptionRepository) translateConditionalWindowReset(ctx context.Context, client *dbent.Client, id int64, affected int, err error) error {
	if err != nil {
		return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	if affected > 0 {
		return nil
	}

	// A stale reset is an expected no-op: another request already advanced the
	// window. Preserve not-found semantics for callers that target a missing row.
	exists, err := client.UserSubscription.Query().Where(usersubscription.IDEQ(id)).Exist(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	if !exists {
		return service.ErrSubscriptionNotFound
	}
	return nil
}

// IncrementUsage 原子性地累加订阅用量。
// 限额检查已在请求前由 BillingCacheService.CheckBillingEligibility 完成，
// 此处仅负责记录实际消费，确保消费数据的完整性。
func (r *userSubscriptionRepository) IncrementUsage(ctx context.Context, id int64, costUSD float64) error {
	const updateSQL = `
		UPDATE user_subscriptions us
		SET
			daily_usage_usd = us.daily_usage_usd + $1,
			weekly_usage_usd = us.weekly_usage_usd + $1,
			monthly_usage_usd = us.monthly_usage_usd + $1,
			five_hour_window_start = CASE
				WHEN $1 <= 0 THEN us.five_hour_window_start
				WHEN us.five_hour_window_start IS NULL
					OR us.five_hour_window_start <= NOW() - INTERVAL '5 hours'
				THEN NOW()
				ELSE us.five_hour_window_start
			END,
			five_hour_usage_usd = CASE
				WHEN $1 < 0 THEN GREATEST(us.five_hour_usage_usd + $1, 0)
				WHEN $1 = 0 THEN us.five_hour_usage_usd
				WHEN us.five_hour_window_start IS NULL
					OR us.five_hour_window_start <= NOW() - INTERVAL '5 hours'
				THEN $1
				ELSE us.five_hour_usage_usd + $1
			END,
			updated_at = NOW()
		FROM groups g
		WHERE us.id = $2
			AND us.deleted_at IS NULL
			AND us.group_id = g.id
			AND g.deleted_at IS NULL
	`

	client := clientFromContext(ctx, r.client)
	result, err := client.ExecContext(ctx, updateSQL, costUSD, id)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if affected > 0 {
		return nil
	}

	// affected == 0：订阅不存在或已删除
	return service.ErrSubscriptionNotFound
}

func (r *userSubscriptionRepository) BatchUpdateExpiredStatus(ctx context.Context) (int64, error) {
	client := clientFromContext(ctx, r.client)
	n, err := client.UserSubscription.Update().
		Where(
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			usersubscription.ExpiresAtLTE(time.Now()),
		).
		SetStatus(service.SubscriptionStatusExpired).
		Save(ctx)
	return int64(n), err
}

// ============================================
// 订阅额度倍率
// ============================================

// ListGroupQuotaMultipliers 返回分组下有效订阅者与已有价格倍率用户的并集。
// 有效订阅取 status=active 且未过期；同一用户若有多条（历史数据）取最新到期的一条。
func (r *userSubscriptionRepository) ListGroupQuotaMultipliers(ctx context.Context, groupID int64) ([]service.GroupQuotaMultiplierEntry, error) {
	const query = `
		WITH active_subs AS (
			SELECT DISTINCT ON (us.user_id)
				us.user_id,
				us.id AS subscription_id,
				us.quota_multiplier
			FROM user_subscriptions us
			WHERE us.group_id = $1
				AND us.deleted_at IS NULL
				AND us.status = 'active'
				AND us.expires_at > NOW()
			ORDER BY us.user_id, us.expires_at DESC, us.id DESC
		),
		rate_users AS (
			SELECT user_id, rate_multiplier
			FROM user_group_rate_multipliers
			WHERE group_id = $1 AND rate_multiplier IS NOT NULL
		),
		defaults AS (
			SELECT user_id, quota_multiplier
			FROM user_group_quota_defaults
			WHERE group_id = $1
		),
		ids AS (
			SELECT user_id FROM active_subs
			UNION
			SELECT user_id FROM rate_users
			UNION
			SELECT user_id FROM defaults
		)
		SELECT
			ids.user_id,
			u.username,
			u.email,
			COALESCE(u.notes, ''),
			u.status,
			active_subs.subscription_id,
			active_subs.quota_multiplier,
			defaults.quota_multiplier,
			rate_users.rate_multiplier
		FROM ids
		JOIN users u ON u.id = ids.user_id AND u.deleted_at IS NULL
		LEFT JOIN active_subs ON active_subs.user_id = ids.user_id
		LEFT JOIN defaults ON defaults.user_id = ids.user_id
		LEFT JOIN rate_users ON rate_users.user_id = ids.user_id
		ORDER BY ids.user_id
	`
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, query, groupID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []service.GroupQuotaMultiplierEntry
	for rows.Next() {
		var entry service.GroupQuotaMultiplierEntry
		var subscriptionID sql.NullInt64
		var quotaMultiplier sql.NullFloat64
		var defaultQuotaMultiplier sql.NullFloat64
		var rateMultiplier sql.NullFloat64
		if err := rows.Scan(
			&entry.UserID,
			&entry.Username,
			&entry.Email,
			&entry.Notes,
			&entry.Status,
			&subscriptionID,
			&quotaMultiplier,
			&defaultQuotaMultiplier,
			&rateMultiplier,
		); err != nil {
			return nil, err
		}
		if subscriptionID.Valid {
			v := subscriptionID.Int64
			entry.SubscriptionID = &v
		}
		if quotaMultiplier.Valid {
			v := quotaMultiplier.Float64
			entry.QuotaMultiplier = &v
		}
		if defaultQuotaMultiplier.Valid {
			v := defaultQuotaMultiplier.Float64
			entry.DefaultQuotaMultiplier = &v
		}
		if rateMultiplier.Valid {
			v := rateMultiplier.Float64
			entry.RateMultiplier = &v
		}
		result = append(result, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *userSubscriptionRepository) SetSubscriptionQuotaMultiplier(ctx context.Context, subscriptionID int64, multiplier float64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.ExecContext(ctx, `
		UPDATE user_subscriptions
		SET quota_multiplier = $1, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`, multiplier, subscriptionID)
	return err
}

// BatchSetGroupQuotaMultipliers 在调用方事务内批量保存分组额度倍率。
// entries 为空表示清空整个分组：有效订阅统一置 1.0，并删除全部默认值。
// 清空写 1.0：quota_multiplier 是 NOT NULL DEFAULT 1.0，不存 NULL。
// user_group_quota_defaults 只用于分配弹窗预填，不参与有效倍率。
// 返回实际更新过订阅的用户 ID，供提交后批量失效缓存。
func (r *userSubscriptionRepository) BatchSetGroupQuotaMultipliers(ctx context.Context, groupID int64, entries []service.GroupQuotaMultiplierInput) ([]int64, error) {
	client := clientFromContext(ctx, r.client)

	if len(entries) == 0 {
		rows, err := client.QueryContext(ctx, `
			UPDATE user_subscriptions
			SET quota_multiplier = 1.0, updated_at = NOW()
			WHERE group_id = $1
				AND deleted_at IS NULL
				AND status = 'active'
				AND expires_at > NOW()
			RETURNING user_id
		`, groupID)
		if err != nil {
			return nil, err
		}
		userIDs, err := scanInt64Rows(rows)
		if err != nil {
			return nil, err
		}
		if _, err := client.ExecContext(ctx, `
			DELETE FROM user_group_quota_defaults WHERE group_id = $1
		`, groupID); err != nil {
			return nil, err
		}
		return userIDs, nil
	}

	userIDs := make([]int64, 0, len(entries))
	multipliers := make([]*float64, 0, len(entries))
	for i := range entries {
		userIDs = append(userIDs, entries[i].UserID)
		multipliers = append(multipliers, entries[i].QuotaMultiplier)
	}

	rows, err := client.QueryContext(ctx, `
		UPDATE user_subscriptions us
		SET quota_multiplier = COALESCE(i.multiplier, 1.0)::numeric(10,4),
			updated_at = NOW()
		FROM UNNEST($2::bigint[], $3::double precision[]) AS i(user_id, multiplier)
		WHERE us.group_id = $1
			AND us.user_id = i.user_id
			AND us.deleted_at IS NULL
			AND us.status = 'active'
			AND us.expires_at > NOW()
		RETURNING us.user_id
	`, groupID, pq.Array(userIDs), pq.Array(multipliers))
	if err != nil {
		return nil, err
	}
	changedUserIDs, err := scanInt64Rows(rows)
	if err != nil {
		return nil, err
	}

	// 无有效订阅的用户写入“下次分配默认值”；有订阅的用户只更新订阅。
	if _, err := client.ExecContext(ctx, `
		INSERT INTO user_group_quota_defaults (user_id, group_id, quota_multiplier, created_at, updated_at)
		SELECT i.user_id, $1, i.multiplier::numeric(10,4), NOW(), NOW()
		FROM UNNEST($2::bigint[], $3::double precision[]) AS i(user_id, multiplier)
		WHERE i.multiplier IS NOT NULL
			AND NOT EXISTS (
				SELECT 1
				FROM user_subscriptions us
				WHERE us.user_id = i.user_id
					AND us.group_id = $1
					AND us.deleted_at IS NULL
					AND us.status = 'active'
					AND us.expires_at > NOW()
			)
		ON CONFLICT (user_id, group_id)
		DO UPDATE SET quota_multiplier = EXCLUDED.quota_multiplier, updated_at = NOW()
	`, groupID, pq.Array(userIDs), pq.Array(multipliers)); err != nil {
		return nil, err
	}

	// 显式传入 null 表示清除该用户的默认值。
	if _, err := client.ExecContext(ctx, `
		DELETE FROM user_group_quota_defaults d
		USING UNNEST($2::bigint[], $3::double precision[]) AS i(user_id, multiplier)
		WHERE d.group_id = $1
			AND d.user_id = i.user_id
			AND i.multiplier IS NULL
	`, groupID, pq.Array(userIDs), pq.Array(multipliers)); err != nil {
		return nil, err
	}

	return changedUserIDs, nil
}

func scanInt64Rows(rows *sql.Rows) ([]int64, error) {
	defer func() { _ = rows.Close() }()
	result := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

// UpsertUserGroupQuotaDefault 仅用于分配弹窗预填，不得在分配/续费/支付路径读取。
func (r *userSubscriptionRepository) UpsertUserGroupQuotaDefault(ctx context.Context, userID, groupID int64, multiplier float64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.ExecContext(ctx, `
		INSERT INTO user_group_quota_defaults (user_id, group_id, quota_multiplier, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (user_id, group_id)
		DO UPDATE SET quota_multiplier = EXCLUDED.quota_multiplier, updated_at = NOW()
	`, userID, groupID, multiplier)
	return err
}

func (r *userSubscriptionRepository) DeleteUserGroupQuotaDefault(ctx context.Context, userID, groupID int64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.ExecContext(ctx, `
		DELETE FROM user_group_quota_defaults WHERE user_id = $1 AND group_id = $2
	`, userID, groupID)
	return err
}

// Extra repository helpers (currently used only by integration tests).

func (r *userSubscriptionRepository) ListExpired(ctx context.Context) ([]service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	subs, err := client.UserSubscription.Query().
		Where(
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			usersubscription.ExpiresAtLTE(time.Now()),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return userSubscriptionEntitiesToService(subs), nil
}

func (r *userSubscriptionRepository) CountByGroupID(ctx context.Context, groupID int64) (int64, error) {
	client := clientFromContext(ctx, r.client)
	count, err := client.UserSubscription.Query().Where(usersubscription.GroupIDEQ(groupID)).Count(ctx)
	return int64(count), err
}

func (r *userSubscriptionRepository) CountActiveByGroupID(ctx context.Context, groupID int64) (int64, error) {
	client := clientFromContext(ctx, r.client)
	count, err := client.UserSubscription.Query().
		Where(
			usersubscription.GroupIDEQ(groupID),
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			usersubscription.ExpiresAtGT(time.Now()),
		).
		Count(ctx)
	return int64(count), err
}

func (r *userSubscriptionRepository) DeleteByGroupID(ctx context.Context, groupID int64) (int64, error) {
	client := clientFromContext(ctx, r.client)
	n, err := client.UserSubscription.Delete().Where(usersubscription.GroupIDEQ(groupID)).Exec(ctx)
	return int64(n), err
}

func (r *userSubscriptionRepository) attachUserSubscriptionRelations(ctx context.Context, subs []service.UserSubscription) error {
	if len(subs) == 0 {
		return nil
	}

	userIDs := make([]int64, 0, len(subs))
	groupIDs := make([]int64, 0, len(subs))
	assignedByIDs := make([]int64, 0, len(subs))
	for i := range subs {
		userIDs = append(userIDs, subs[i].UserID)
		groupIDs = append(groupIDs, subs[i].GroupID)
		if subs[i].AssignedBy != nil {
			assignedByIDs = append(assignedByIDs, *subs[i].AssignedBy)
		}
	}

	client := clientFromContext(ctx, r.client)
	users, err := client.User.Query().Where(user.IDIn(uniqueInt64s(userIDs)...)).All(ctx)
	if err != nil {
		return err
	}
	userByID := make(map[int64]*service.User, len(users))
	for _, u := range users {
		userByID[u.ID] = userEntityToService(u)
	}

	groups, err := client.Group.Query().Where(group.IDIn(uniqueInt64s(groupIDs)...)).All(ctx)
	if err != nil {
		return err
	}
	groupByID := make(map[int64]*service.Group, len(groups))
	for _, g := range groups {
		groupByID[g.ID] = groupEntityToService(g)
	}

	assignedByID := map[int64]*service.User{}
	if len(assignedByIDs) > 0 {
		assignedUsers, err := client.User.Query().Where(user.IDIn(uniqueInt64s(assignedByIDs)...)).All(ctx)
		if err != nil {
			return err
		}
		assignedByID = make(map[int64]*service.User, len(assignedUsers))
		for _, u := range assignedUsers {
			assignedByID[u.ID] = userEntityToService(u)
		}
	}

	for i := range subs {
		subs[i].User = userByID[subs[i].UserID]
		subs[i].Group = groupByID[subs[i].GroupID]
		if subs[i].AssignedBy != nil {
			subs[i].AssignedByUser = assignedByID[*subs[i].AssignedBy]
		}
	}
	return nil
}

func uniqueInt64s(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	out := make([]int64, 0, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func userSubscriptionEntityToService(m *dbent.UserSubscription) *service.UserSubscription {
	return userSubscriptionEntityToServiceWithStatusMapping(m, true)
}

func userSubscriptionEntityToServicePreserveStatus(m *dbent.UserSubscription) *service.UserSubscription {
	return userSubscriptionEntityToServiceWithStatusMapping(m, false)
}

func userSubscriptionEntityToServiceWithStatusMapping(m *dbent.UserSubscription, mapDeletedToRevoked bool) *service.UserSubscription {
	if m == nil {
		return nil
	}
	status := m.Status
	if mapDeletedToRevoked && m.DeletedAt != nil {
		status = service.SubscriptionStatusRevoked
	}
	out := &service.UserSubscription{
		ID:                  m.ID,
		UserID:              m.UserID,
		GroupID:             m.GroupID,
		StartsAt:            m.StartsAt,
		ExpiresAt:           m.ExpiresAt,
		Status:              status,
		DailyWindowStart:    m.DailyWindowStart,
		WeeklyWindowStart:   m.WeeklyWindowStart,
		MonthlyWindowStart:  m.MonthlyWindowStart,
		FiveHourWindowStart: m.FiveHourWindowStart,
		DailyUsageUSD:       m.DailyUsageUsd,
		WeeklyUsageUSD:      m.WeeklyUsageUsd,
		MonthlyUsageUSD:     m.MonthlyUsageUsd,
		FiveHourUsageUSD:    m.FiveHourUsageUsd,
		QuotaMultiplier:     m.QuotaMultiplier,
		AssignedBy:          m.AssignedBy,
		AssignedAt:          m.AssignedAt,
		Notes:               derefString(m.Notes),
		CreatedAt:           m.CreatedAt,
		UpdatedAt:           m.UpdatedAt,
		DeletedAt:           m.DeletedAt,
	}
	if m.Edges.User != nil {
		out.User = userEntityToService(m.Edges.User)
	}
	if m.Edges.Group != nil {
		out.Group = groupEntityToService(m.Edges.Group)
	}
	if m.Edges.AssignedByUser != nil {
		out.AssignedByUser = userEntityToService(m.Edges.AssignedByUser)
	}
	return out
}

func userSubscriptionEntitiesToService(models []*dbent.UserSubscription) []service.UserSubscription {
	out := make([]service.UserSubscription, 0, len(models))
	for i := range models {
		if s := userSubscriptionEntityToService(models[i]); s != nil {
			out = append(out, *s)
		}
	}
	return out
}

func applyUserSubscriptionEntityToService(dst *service.UserSubscription, src *dbent.UserSubscription) {
	if dst == nil || src == nil {
		return
	}
	dst.ID = src.ID
	dst.CreatedAt = src.CreatedAt
	dst.UpdatedAt = src.UpdatedAt
}
