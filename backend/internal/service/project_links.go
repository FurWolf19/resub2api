package service

const (
	// ProjectRepository is the canonical repository for this fork.
	ProjectRepository = "FurWolf19/resub2api"
	ProjectURL        = "https://github.com/" + ProjectRepository
	ProjectDocsURL    = ProjectURL + "/blob/main/docs"

	// UpstreamRepository is the read-only upstream project this fork is based on.
	// It is used only for attribution and update notifications, never for updates.
	UpstreamRepository = "Wei-Shaw/sub2api"
	UpstreamURL        = "https://github.com/" + UpstreamRepository
)
