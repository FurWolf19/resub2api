package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

// roundQuotaMultiplier 与数据库 numeric(10,4) 对齐，确保内存返回值与实际生效值一致。
func roundQuotaMultiplier(multiplier float64) float64 {
	return math.Round(multiplier*10000) / 10000
}

// UpdateQuotaMultiplier 修改单个订阅的额度倍率。
// 事务内 FOR UPDATE 锁定订阅行，提交后失效订阅 L1/Redis 与 API Key 认证缓存。
func (s *SubscriptionService) UpdateQuotaMultiplier(ctx context.Context, subscriptionID int64, multiplier float64) error {
	if multiplier < 0 || multiplier > 1 {
		return ErrInvalidQuotaMultiplier
	}
	multiplier = roundQuotaMultiplier(multiplier)
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
// 空数组表示清空整个分组（兼容前端“全部清空”后提交空列表）。
func (s *SubscriptionService) BatchSetGroupQuotaMultipliers(ctx context.Context, groupID int64, entries []GroupQuotaMultiplierInput) error {
	normalized, err := normalizeGroupQuotaMultiplierInputs(entries)
	if err != nil {
		return err
	}

	var changedUserIDs []int64
	err = s.withSubscriptionUpdateTx(ctx, func(txCtx context.Context) error {
		repo, ok := s.quotaRepository()
		if !ok {
			return fmt.Errorf("subscription quota repository is not available")
		}
		if batchRepo, ok := s.userSubRepo.(SubscriptionQuotaBatchRepository); ok {
			ids, err := batchRepo.BatchSetGroupQuotaMultipliers(txCtx, groupID, normalized)
			if err != nil {
				return err
			}
			changedUserIDs = ids
			return nil
		}
		if len(normalized) == 0 {
			// 旧仓储不支持批量清空，保持兼容；生产仓储走上面的批量路径。
			return nil
		}
		changedUserIDs = make([]int64, 0, len(normalized))
		for _, entry := range normalized {
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
				changedUserIDs = append(changedUserIDs, sub.UserID)
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

	// 提交后统一失效缓存，避免在事务内把未提交数据重新加载进缓存。
	var firstErr error
	for _, userID := range uniquePositiveInt64s(changedUserIDs) {
		if err := s.invalidateSubscriptionCaches(userID, groupID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// normalizeGroupQuotaMultiplierInputs 校验并去重（同一用户以最后一条为准），
// 同时保证空数组仍作为合法的“清空该分组”请求传给仓储。
func normalizeGroupQuotaMultiplierInputs(entries []GroupQuotaMultiplierInput) ([]GroupQuotaMultiplierInput, error) {
	normalized := make([]GroupQuotaMultiplierInput, 0, len(entries))
	indexByUser := make(map[int64]int, len(entries))
	for i := range entries {
		entry := entries[i]
		if entry.UserID <= 0 {
			return nil, fmt.Errorf("user_id must be positive (index=%d)", i)
		}
		if entry.QuotaMultiplier != nil && (*entry.QuotaMultiplier < 0 || *entry.QuotaMultiplier > 1) {
			return nil, ErrInvalidQuotaMultiplier
		}
		if idx, ok := indexByUser[entry.UserID]; ok {
			normalized[idx] = entry
			continue
		}
		indexByUser[entry.UserID] = len(normalized)
		normalized = append(normalized, entry)
	}
	return normalized, nil
}

func uniquePositiveInt64s(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	out := make([]int64, 0, len(values))
	for _, v := range values {
		if v <= 0 {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
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
