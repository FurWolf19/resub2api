//go:build unit

package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserGroupQuotaDefaultsMigrationAddsGroupIndexAndCheck(t *testing.T) {
	content, err := FS.ReadFile("255_add_user_group_quota_defaults.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_user_group_quota_defaults_group_id ON user_group_quota_defaults(group_id)")
	require.Contains(t, sql, "DROP CONSTRAINT IF EXISTS user_group_quota_defaults_quota_multiplier_check")
	require.Contains(t, sql, "CHECK (quota_multiplier >= 0 AND quota_multiplier <= 1)")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS user_group_quota_defaults")
}
