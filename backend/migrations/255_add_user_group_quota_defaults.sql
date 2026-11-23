-- 用户 × 分组“无订阅时的下次分配默认额度倍率”。
-- 仅用于分配弹窗预填，不自动应用到支付/兑换码/续费创建的新订阅。
CREATE TABLE IF NOT EXISTS user_group_quota_defaults (
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  quota_multiplier DECIMAL(10,4) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (user_id, group_id)
);
