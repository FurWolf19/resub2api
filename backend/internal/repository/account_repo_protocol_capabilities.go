package repository

import (
	"context"
	"encoding/json"
	"errors"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// MergeOpenAIProtocolCapabilities 在事务内锁定账号行，读取最新 extra，按
// probed_at 合并协议能力后再写回。不同实例并发写不同协议不会互相覆盖。
//
// 锁顺序：本方法只锁 accounts 单行；持有该行锁时不得再获取订阅/分组/API Key
// 行锁，避免与其它写路径形成反向锁序。
func (r *accountRepository) MergeOpenAIProtocolCapabilities(
	ctx context.Context,
	accountID int64,
	patches []service.OpenAIProtocolCapabilityPatch,
) error {
	if len(patches) == 0 {
		return nil
	}
	if scoped := r.accountTransactionScope(ctx); scoped != r {
		return scoped.MergeOpenAIProtocolCapabilities(ctx, accountID, patches)
	}

	baseCtx := ctx
	tx := dbent.TxFromContext(ctx)
	ownTx := false
	if tx == nil {
		var err error
		tx, err = r.client.Tx(ctx)
		if err != nil {
			if errors.Is(err, dbent.ErrTxStarted) {
				return r.mergeOpenAIProtocolCapabilitiesInTx(ctx, accountID, patches)
			}
			return err
		}
		defer func() { _ = tx.Rollback() }()
		ctx = dbent.NewTxContext(ctx, tx)
		ownTx = true
	}

	if err := r.mergeOpenAIProtocolCapabilitiesInTx(ctx, accountID, patches); err != nil {
		return err
	}
	if ownTx {
		if err := tx.Commit(); err != nil {
			return err
		}
		// 与 UpdateExtra 的观测型字段行为一致：提交后同步单账号调度快照。
		r.syncSchedulerAccountSnapshot(baseCtx, accountID)
	}
	return nil
}

func (r *accountRepository) mergeOpenAIProtocolCapabilitiesInTx(
	ctx context.Context,
	accountID int64,
	patches []service.OpenAIProtocolCapabilityPatch,
) error {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx,
		`SELECT COALESCE(extra, '{}'::jsonb) FROM accounts WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`,
		accountID,
	)
	if err != nil {
		return translatePersistenceError(err, service.ErrAccountNotFound, nil)
	}
	if !rows.Next() {
		closeErr := rows.Close()
		if err := rows.Err(); err != nil {
			return translatePersistenceError(err, service.ErrAccountNotFound, nil)
		}
		if closeErr != nil {
			return closeErr
		}
		return service.ErrAccountNotFound
	}
	var raw []byte
	if err := rows.Scan(&raw); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	extra := make(map[string]json.RawMessage)
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &extra); err != nil {
			return err
		}
	}
	if extra == nil {
		extra = make(map[string]json.RawMessage)
	}

	payload, changed, err := mergeOpenAIProtocolCapabilitiesExtra(extra, patches)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}

	result, err := client.ExecContext(ctx,
		`UPDATE accounts SET extra = $1::jsonb, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL`,
		string(payload), accountID,
	)
	if err != nil {
		return translatePersistenceError(err, service.ErrAccountNotFound, nil)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrAccountNotFound
	}
	return nil
}

// mergeOpenAIProtocolCapabilitiesExtra 只解析/改写目标键，其它键保持原始 JSON
// 字节，避免把大整数等值解码成 float64 后丢精度。
func mergeOpenAIProtocolCapabilitiesExtra(
	extra map[string]json.RawMessage,
	patches []service.OpenAIProtocolCapabilityPatch,
) ([]byte, bool, error) {
	currentExtra := make(map[string]any, 2)
	if raw, ok := extra[openai_compat.ExtraKeyProtocolCapabilities]; ok {
		currentExtra[openai_compat.ExtraKeyProtocolCapabilities] = raw
	}
	if raw, ok := extra[openai_compat.ExtraKeyResponsesSupported]; ok {
		var supported *bool
		// null 解码到 *bool 是 nil；缺失/null 都视为 unknown，不能合成 false 结论。
		if json.Unmarshal(raw, &supported) == nil && supported != nil {
			currentExtra[openai_compat.ExtraKeyResponsesSupported] = *supported
		}
	}
	merged, legacyResponsesSupported, changed, err := service.MergeOpenAIProtocolCapabilities(
		openai_compat.ResolveProtocolCapabilities(currentExtra), patches,
	)
	if err != nil {
		return nil, false, err
	}

	extraChanged := false
	if changed {
		encoded, err := json.Marshal(merged.ToExtra())
		if err != nil {
			return nil, false, err
		}
		extra[openai_compat.ExtraKeyProtocolCapabilities] = encoded
		extraChanged = true
	}
	if legacyResponsesSupported != nil {
		var existing *bool
		existingValid := false
		if raw, ok := extra[openai_compat.ExtraKeyResponsesSupported]; ok {
			// null 是 Unknown，不算已有值；不能把它当 false 跳过同步或写成 false。
			existingValid = json.Unmarshal(raw, &existing) == nil && existing != nil
		}
		if !existingValid || *existing != *legacyResponsesSupported {
			encoded, err := json.Marshal(*legacyResponsesSupported)
			if err != nil {
				return nil, false, err
			}
			extra[openai_compat.ExtraKeyResponsesSupported] = encoded
			extraChanged = true
		}
	}
	if !extraChanged {
		return nil, false, nil
	}
	payload, err := json.Marshal(extra)
	if err != nil {
		return nil, false, err
	}
	return payload, true, nil
}

var _ service.OpenAIProtocolCapabilityRepository = (*accountRepository)(nil)
