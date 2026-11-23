//go:build unit

package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration253AddsGroupPeakRateWindows(t *testing.T) {
	content, err := FS.ReadFile("256_add_group_peak_rate_windows.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS peak_rate_windows JSONB NOT NULL DEFAULT '[]'::jsonb")
	require.Contains(t, sql, "jsonb_build_array(")
	require.Contains(t, sql, "jsonb_build_object(")
	require.Contains(t, sql, "'start', peak_start")
	require.Contains(t, sql, "'end', peak_end")
	require.Contains(t, sql, "'multiplier', peak_rate_multiplier")
	require.Contains(t, sql, "'days', jsonb_build_array(0,1,2,3,4,5,6)")
	require.Contains(t, sql, "WHERE peak_rate_enabled = TRUE")
	require.Contains(t, sql, "peak_start < peak_end")
	require.Contains(t, sql, "peak_rate_windows = '[]'::jsonb")
}
