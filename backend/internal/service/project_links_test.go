//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProjectLinks(t *testing.T) {
	require.Equal(t, "FurWolf19/resub2api", ProjectRepository)
	require.Equal(t, "https://github.com/FurWolf19/resub2api", ProjectURL)
	require.Equal(t, "https://github.com/FurWolf19/resub2api/blob/main/docs", ProjectDocsURL)
	require.Equal(t, "Wei-Shaw/sub2api", UpstreamRepository)
	require.Equal(t, "https://github.com/Wei-Shaw/sub2api", UpstreamURL)
	require.True(t, strings.HasPrefix(AdminComplianceDocumentURLZH, ProjectDocsURL+"/legal/"))
	require.True(t, strings.HasPrefix(AdminComplianceDocumentURLEN, ProjectDocsURL+"/legal/"))
}
