-- 多时段高峰倍率：每个窗口独立配置 start/end/multiplier/days。
-- 旧单时段字段 peak_rate_enabled/peak_start/peak_end/peak_rate_multiplier 保留，
-- 新数组为空时代码回退旧字段；迁移把已启用的合法旧配置回填为单窗口（每天生效）。
ALTER TABLE groups
  ADD COLUMN IF NOT EXISTS peak_rate_windows JSONB NOT NULL DEFAULT '[]'::jsonb;

UPDATE groups
SET peak_rate_windows = jsonb_build_array(
  jsonb_build_object(
    'start', peak_start,
    'end', peak_end,
    'multiplier', peak_rate_multiplier,
    'days', jsonb_build_array(0,1,2,3,4,5,6)
  )
)
WHERE peak_rate_enabled = TRUE
  AND peak_start <> ''
  AND peak_end <> ''
  AND peak_start ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'
  AND peak_end ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'
  AND peak_start < peak_end
  AND peak_rate_windows = '[]'::jsonb;
