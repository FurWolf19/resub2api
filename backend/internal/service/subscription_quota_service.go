package service

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// UpdateQuotaMultiplier 修改单个订阅的额度倍率。
// 事务内 FOR UPDATE 锁定订阅行，提交后失效订阅 L1/Redis 与 API Key 认证缓存。
func (s *SubscriptionService) UpdateQuotaMultiplier(ctx context.Context, subscriptionID int64, multiplier float64) error {
	if multiplier < 0 || multiplier > 1 {
		return ErrInvalidQuotaMultiplier
	}
	var userID, groupID int64
	err := s.withSubscriptionUpdateTx(ctx, func(txCtx context.Context) error {
		sub, err := s.userSubRepo.GetByIDForUpdate(txCtx, subscriptionID)
		if err != nil {
			return err
		}
		userID, groupID = sub.UserID, sub.GroupID
		repo, ok := s.quotaRepository()
		if !ok {
			return fmt.Errorf("subscription quota repository is not available")
		}
		return repo.SetSubscriptionQuotaMultiplier(txCtx, subscriptionID, multiplier)
	})
	if err != nil {
		return err
	}
	return s.invalidateSubscriptionCaches(userID, groupID)
}

// GetGroupQuotaMultipliers 返回分组额度倍率弹窗所需的用户并集：
// 当前分组的有效订阅者 + 已有价格倍率的用户。
func (s *SubscriptionService) GetGroupQuotaMultipliers(ctx context.Context, groupID int64) ([]GroupQuotaMultiplierEntry, error) {
	repo, ok := s.quotaRepository()
	if !ok {
		return nil, fmt.Errorf("subscription quota repository is not available")
	}
	entries, err := repo.ListGroupQuotaMultipliers(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []GroupQuotaMultiplierEntry{}
	}
	return entries, nil
}

// BatchSetGroupQuotaMultipliers 批量保存分组额度倍率。
// 有有效订阅的用户更新订阅字段并失效缓存；无有效订阅的用户写入“下次分配默认值”。
// 值为 nil 表示清空：订阅置 1.0，默认值删除。
func (s *SubscriptionService) BatchSetGroupQuotaMultipliers(ctx context.Context, groupID int64, entries []GroupQuotaMultiplierInput) error {
	type changedSub struct {
		userID  int64
		groupID int64
	}
	changed := make([]changedSub, 0, len(entries))

	for i := range entries {
		entry := entries[i]
		if entry.UserID <= 0 {
			return fmt.Errorf("user_id must be positive (index=%d)", i)
		}
		if entry.QuotaMultiplier != nil && (*entry.QuotaMultiplier < 0 || *entry.QuotaMultiplier > 1) {
			return ErrInvalidQuotaMultiplier
		}
	}

	err := s.withSubscriptionUpdateTx(ctx, func(txCtx context.Context) error {
		repo, ok := s.quotaRepository()
		if !ok {
			return fmt.Errorf("subscription quota repository is not available")
		}
		for _, entry := range entries {
			sub, err := s.userSubRepo.GetActiveByUserIDAndGroupID(txCtx, entry.UserID, groupID)
			if err != nil {
				if errors.Is(err, ErrSubscriptionNotFound) {
					sub = nil
				} else {
					return err
				}
			}
			if sub != nil {
				multiplier := 1.0
				if entry.QuotaMultiplier != nil {
					multiplier = *entry.QuotaMultiplier
				}
				if err := repo.SetSubscriptionQuotaMultiplier(txCtx, sub.ID, multiplier); err != nil {
					return err
				}
				changed = append(changed, changedSub{userID: sub.UserID, groupID: sub.GroupID})
				continue
			}
			if entry.QuotaMultiplier == nil {
				if err := repo.DeleteUserGroupQuotaDefault(txCtx, entry.UserID, groupID); err != nil {
					return err
				}
				continue
			}
			if err := repo.UpsertUserGroupQuotaDefault(txCtx, entry.UserID, groupID, *entry.QuotaMultiplier); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	for _, item := range changed {
		if err := s.invalidateSubscriptionCaches(item.userID, item.groupID); err != nil {
			return err
		}
	}
	return nil
}

func (s *SubscriptionService) quotaRepository() (SubscriptionQuotaRepository, bool) {
	repo, ok := s.userSubRepo.(SubscriptionQuotaRepository)
	return repo, ok
}

// resetFiveHourUsage 通过可选仓储接口清零 5h 窗口；旧实现未提供时保持兼容（无操作）。
func (s *SubscriptionService) resetFiveHourUsage(ctx context.Context, id int64, expectedWindowStart *time.Time) error {
	repo, ok := s.userSubRepo.(fiveHourUsageResetter)
	if !ok {
		return nil
	}
	return repo.ResetFiveHourUsage(ctx, id, expectedWindowStart)
}
