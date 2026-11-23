//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type updateServiceCacheStub struct {
	data string
}

func (s *updateServiceCacheStub) GetUpdateInfo(context.Context) (string, error) {
	if s.data == "" {
		return "", errors.New("cache miss")
	}
	return s.data, nil
}

func (s *updateServiceCacheStub) SetUpdateInfo(_ context.Context, data string, _ time.Duration) error {
	s.data = data
	return nil
}

type updateServiceGitHubClientStub struct {
	release         *GitHubRelease
	upstreamRelease *GitHubRelease
	releasesByRepo  map[string]*GitHubRelease
	errByRepo       map[string]error
	recentReleases  []*GitHubRelease
	recentErr       error
	latestCalls     []string
	recentCalls     []string
	downloadCalls   []string
}

func (s *updateServiceGitHubClientStub) FetchLatestRelease(_ context.Context, repo string) (*GitHubRelease, error) {
	s.latestCalls = append(s.latestCalls, repo)
	if err := s.errByRepo[repo]; err != nil {
		return nil, err
	}
	if release := s.releasesByRepo[repo]; release != nil {
		return release, nil
	}
	if repo == UpstreamRepository {
		if s.upstreamRelease != nil {
			return s.upstreamRelease, nil
		}
		return s.release, nil
	}
	return s.release, nil
}

func (s *updateServiceGitHubClientStub) FetchRecentReleases(_ context.Context, repo string, _ int) ([]*GitHubRelease, error) {
	s.recentCalls = append(s.recentCalls, repo)
	return s.recentReleases, s.recentErr
}

func (s *updateServiceGitHubClientStub) DownloadFile(_ context.Context, url, _ string, _ int64) error {
	s.downloadCalls = append(s.downloadCalls, url)
	return nil
}

func (s *updateServiceGitHubClientStub) FetchChecksumFile(context.Context, string) ([]byte, error) {
	panic("FetchChecksumFile should not be called when no update is available")
}

func TestUpdateServicePerformUpdateNoUpdateReturnsSentinel(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{
			release: &GitHubRelease{
				TagName: "v0.1.132",
				Name:    "v0.1.132",
			},
		},
		"0.1.132",
		"release",
	)

	err := svc.PerformUpdate(context.Background())

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoUpdateAvailable))
	require.ErrorIs(t, err, ErrNoUpdateAvailable)
}

func newRollbackTestService(current string, releases []*GitHubRelease) *UpdateService {
	return NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentReleases: releases},
		current,
		"release",
	)
}

func TestUpdateServiceListRollbackVersionsFiltersAndCaps(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.148", PublishedAt: "2026-07-09T00:00:00Z"},                       // newer than current: excluded
		{TagName: "v0.1.147", PublishedAt: "2026-07-08T00:00:00Z"},                       // current: excluded
		{TagName: "v0.1.146-rc1", PublishedAt: "2026-07-07T12:00:00Z", Prerelease: true}, // prerelease: excluded
		{TagName: "v0.1.146", PublishedAt: "2026-07-07T00:00:00Z"},
		{TagName: "v0.1.145", PublishedAt: "2026-07-06T00:00:00Z", Draft: true}, // draft: excluded
		{TagName: "v0.1.144", PublishedAt: "2026-07-05T00:00:00Z"},
		{TagName: "v0.1.144", PublishedAt: "2026-07-05T00:00:00Z"}, // duplicate: excluded
		{TagName: "v0.1.143", PublishedAt: "2026-07-04T00:00:00Z"},
		{TagName: "v0.1.142", PublishedAt: "2026-07-03T00:00:00Z"}, // beyond cap of 3: excluded
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "0.1.146", versions[0].Version)
	require.Equal(t, "0.1.144", versions[1].Version)
	require.Equal(t, "0.1.143", versions[2].Version)
}

func TestUpdateServiceListRollbackVersionsSortsUnorderedInput(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.144"},
		{TagName: "v0.1.146"},
		{TagName: "v0.1.145"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "0.1.146", versions[0].Version)
	require.Equal(t, "0.1.145", versions[1].Version)
	require.Equal(t, "0.1.144", versions[2].Version)
}

func TestUpdateServiceListRollbackVersionsEmptyWhenNoneOlder(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.147"},
		{TagName: "v0.1.148"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Empty(t, versions)
}

func TestUpdateServiceListRollbackVersionsPropagatesFetchError(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentErr: errors.New("github unavailable")},
		"0.1.147",
		"release",
	)

	_, err := svc.ListRollbackVersions(context.Background())

	require.Error(t, err)
	require.Contains(t, err.Error(), "github unavailable")
}

func TestUpdateServiceRollbackToVersionRejectsDisallowedTargets(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.148"},
		{TagName: "v0.1.147"},
		{TagName: "v0.1.146"},
		{TagName: "v0.1.145"},
		{TagName: "v0.1.144"},
		{TagName: "v0.1.143"},
		{TagName: "v0.1.142"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	for _, target := range []string{
		"",         // empty
		"0.1.147",  // current version
		"v0.1.147", // current version with prefix
		"0.1.148",  // newer than current
		"0.1.142",  // older than the 3 most recent
		"9.9.9",    // nonexistent
	} {
		err := svc.RollbackToVersion(context.Background(), target)
		require.ErrorIs(t, err, ErrRollbackVersionNotAllowed, "target %q should be rejected", target)
	}
}

func TestUpdateServiceRollbackToVersionAcceptsVPrefix(t *testing.T) {
	// No platform asset in the release: the target passes the allowlist check
	// and fails later at asset lookup, proving the version itself was accepted.
	releases := []*GitHubRelease{
		{TagName: "v0.1.147"},
		{TagName: "v0.1.146"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	err := svc.RollbackToVersion(context.Background(), "v0.1.146")

	require.Error(t, err)
	require.NotErrorIs(t, err, ErrRollbackVersionNotAllowed)
	require.Contains(t, err.Error(), "no compatible release found")
}

func newForkAndUpstreamTestService(current string, forkRelease, upstreamRelease *GitHubRelease) (*UpdateService, *updateServiceGitHubClientStub) {
	client := &updateServiceGitHubClientStub{
		releasesByRepo: map[string]*GitHubRelease{
			ProjectRepository:  forkRelease,
			UpstreamRepository: upstreamRelease,
		},
	}
	return NewUpdateService(&updateServiceCacheStub{}, client, current, "release"), client
}

func TestUpdateServiceCheckUpdateUsesForkAndUpstreamRepos(t *testing.T) {
	svc, client := newForkAndUpstreamTestService(
		"0.2.24",
		&GitHubRelease{
			TagName: "v0.2.24",
			HTMLURL: "https://github.com/FurWolf19/resub2api/releases/tag/v0.2.24",
		},
		&GitHubRelease{
			TagName:     "v0.2.25",
			HTMLURL:     "https://github.com/Wei-Shaw/sub2api/releases/tag/v0.2.25",
			PublishedAt: "2026-11-20T09:19:00Z",
		},
	)

	info, err := svc.CheckUpdate(context.Background(), true)

	require.NoError(t, err)
	require.Equal(t, []string{UpstreamRepository, ProjectRepository}, client.latestCalls)
	require.Equal(t, ProjectRepository, info.Repository)
	require.True(t, info.OwnReleaseAvailable)
	require.False(t, info.HasUpdate)
	require.Equal(t, "0.2.24", info.LatestVersion)
	require.NotNil(t, info.Upstream)
	require.Equal(t, UpstreamRepository, info.Upstream.Repository)
	require.Equal(t, "v0.2.25", info.Upstream.LatestTag)
	require.Equal(t, "0.2.25", info.Upstream.LatestVersion)
	require.True(t, info.Upstream.HasUpdate)
	require.Equal(t, "https://github.com/Wei-Shaw/sub2api/releases/tag/v0.2.25", info.Upstream.HTMLURL)
	require.NotEmpty(t, info.Upstream.CheckedAt)
}

func TestUpdateServiceOwnUpdateDoesNotUseUpstreamAssets(t *testing.T) {
	svc, client := newForkAndUpstreamTestService(
		"0.2.24",
		&GitHubRelease{
			TagName: "v0.2.25",
			HTMLURL: "https://github.com/FurWolf19/resub2api/releases/tag/v0.2.25",
			Assets: []GitHubAsset{{
				Name:               "sub2api_0.2.25_linux_amd64.tar.gz",
				BrowserDownloadURL: "https://github.com/FurWolf19/resub2api/releases/download/v0.2.25/sub2api_0.2.25_linux_amd64.tar.gz",
			}},
		},
		&GitHubRelease{
			TagName: "v0.2.26",
			Assets: []GitHubAsset{{
				Name:               "sub2api_0.2.26_linux_amd64.tar.gz",
				BrowserDownloadURL: "https://github.com/Wei-Shaw/sub2api/releases/download/v0.2.26/sub2api_0.2.26_linux_amd64.tar.gz",
			}},
		},
	)

	info, err := svc.CheckUpdate(context.Background(), true)

	require.NoError(t, err)
	require.True(t, info.HasUpdate)
	require.Equal(t, "https://github.com/FurWolf19/resub2api/releases/tag/v0.2.25", info.ReleaseInfo.HTMLURL)
	require.Len(t, info.ReleaseInfo.Assets, 1)
	require.Contains(t, info.ReleaseInfo.Assets[0].DownloadURL, "FurWolf19/resub2api")
	require.NotContains(t, info.ReleaseInfo.Assets[0].DownloadURL, "Wei-Shaw/sub2api")
	require.Empty(t, client.downloadCalls)
}

func TestUpdateServiceMissingOwnReleaseStillReturnsUpstream(t *testing.T) {
	svc, _ := newForkAndUpstreamTestService(
		"0.2.24",
		nil,
		&GitHubRelease{TagName: "v0.2.24", HTMLURL: "https://github.com/Wei-Shaw/sub2api/releases/tag/v0.2.24"},
	)
	svc.githubClient.(*updateServiceGitHubClientStub).errByRepo = map[string]error{
		ProjectRepository: errors.New("GitHub API returned 404"),
	}

	info, err := svc.CheckUpdate(context.Background(), true)

	require.NoError(t, err)
	require.False(t, info.OwnReleaseAvailable)
	require.False(t, info.HasUpdate)
	require.Equal(t, "0.2.24", info.LatestVersion)
	require.NotEmpty(t, info.Warning)
	require.NotNil(t, info.Upstream)
	require.Equal(t, "0.2.24", info.Upstream.LatestVersion)
}

func TestUpdateServiceUpstreamFailureIsNonFatal(t *testing.T) {
	svc, _ := newForkAndUpstreamTestService(
		"0.2.24",
		&GitHubRelease{TagName: "v0.2.25"},
		nil,
	)
	svc.githubClient.(*updateServiceGitHubClientStub).errByRepo = map[string]error{
		UpstreamRepository: errors.New("upstream unavailable"),
	}

	info, err := svc.CheckUpdate(context.Background(), true)

	require.NoError(t, err)
	require.True(t, info.OwnReleaseAvailable)
	require.True(t, info.HasUpdate)
	require.NotNil(t, info.Upstream)
	require.NotEmpty(t, info.Upstream.Warning)
}

func TestUpdateServiceCacheSchema(t *testing.T) {
	cache := &updateServiceCacheStub{
		data: `{"schema":2,"latest":"0.2.24","own_release_available":true,"upstream":{"repository":"Wei-Shaw/sub2api","latest_version":"0.2.25","has_update":true,"html_url":"https://github.com/Wei-Shaw/sub2api/releases/tag/v0.2.25"},"timestamp":` + fmt.Sprint(time.Now().Unix()) + `}`,
	}
	client := &updateServiceGitHubClientStub{}
	svc := NewUpdateService(cache, client, "0.2.24", "release")

	info, err := svc.CheckUpdate(context.Background(), false)

	require.NoError(t, err)
	require.Empty(t, client.latestCalls)
	require.True(t, info.Cached)
	require.True(t, info.OwnReleaseAvailable)
	require.NotNil(t, info.Upstream)
	require.True(t, info.Upstream.HasUpdate)

	cache.data = `{"latest":"0.1.1","timestamp":` + fmt.Sprint(time.Now().Unix()) + `}`
	client.release = &GitHubRelease{TagName: "v0.2.24"}

	info, err = svc.CheckUpdate(context.Background(), false)

	require.NoError(t, err)
	require.False(t, info.Cached)
	require.NotEmpty(t, client.latestCalls)
}

func TestUpdateServiceRollbackUsesForkRepo(t *testing.T) {
	client := &updateServiceGitHubClientStub{recentReleases: []*GitHubRelease{{TagName: "v0.2.23"}}}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "0.2.24", "release")

	_, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Equal(t, []string{ProjectRepository}, client.recentCalls)
}

func TestUpdateServicePerformUpdateWithoutOwnRelease(t *testing.T) {
	svc, client := newForkAndUpstreamTestService("0.2.24", nil, &GitHubRelease{TagName: "v0.2.25"})
	client.errByRepo = map[string]error{ProjectRepository: errors.New("GitHub API returned 404")}

	err := svc.PerformUpdate(context.Background())

	require.ErrorIs(t, err, ErrNoReleasePublished)
	require.Empty(t, client.downloadCalls)
}
