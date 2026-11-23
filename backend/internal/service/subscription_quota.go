package service

import "context"

// GroupQuotaMultiplierEntry 分组额度倍率弹窗条目。
// 来源是“当前分组的有效订阅者”与“已有价格倍率用户”的并集。
// QuotaMultiplier/DefaultQuotaMultiplier 使用指针以区分“没有值”和合法的 0。
type GroupQuotaMultiplierEntry struct {
	UserID                 int64    `json:"user_id"`
	Username               string   `json:"username"`
	Email                  string   `json:"email"`
	Notes                  string   `json:"notes"`
	Status                 string   `json:"status"`
	SubscriptionID         *int64   `json:"subscription_id,omitempty"`
	QuotaMultiplier        *float64 `json:"quota_multiplier"`
	DefaultQuotaMultiplier *float64 `json:"default_quota_multiplier"`
	RateMultiplier         *float64 `json:"rate_multiplier"`
}

// GroupQuotaMultiplierInput 批量保存分组额度倍率的输入条目。
// QuotaMultiplier 为 nil 表示清空：有订阅时置 1.0，无订阅时删除默认值。
type GroupQuotaMultiplierInput struct {
	UserID          int64    `json:"user_id"`
	QuotaMultiplier *float64 `json:"quota_multiplier"`
}

// SubscriptionQuotaRepository 订阅额度倍率相关仓储。
// 作为 UserSubscriptionRepository 的可选扩展，便于旧测试桩保持兼容。
type SubscriptionQuotaRepository interface {
	ListGroupQuotaMultipliers(ctx context.Context, groupID int64) ([]GroupQuotaMultiplierEntry, error)
	SetSubscriptionQuotaMultiplier(ctx context.Context, subscriptionID int64, multiplier float64) error
	UpsertUserGroupQuotaDefault(ctx context.Context, userID, groupID int64, multiplier float64) error
	DeleteUserGroupQuotaDefault(ctx context.Context, userID, groupID int64) error
}
