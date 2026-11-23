-- 订阅额度倍率：有效限额 = 分组限额 × quota_multiplier。
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS quota_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0;
