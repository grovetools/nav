package manager

import (
	"github.com/grovetools/nav/pkg/api"
)

// SessionizeProject is the in-process alias for api.Project. The canonical
// type now lives in nav/pkg/api so it can be imported by other modules
// (notably the terminal embedding of the sessionizer TUI).
type SessionizeProject = api.Project

// Legacy type alias for backward compatibility.
// Deprecated: Use SessionizeProject (or api.Project directly) instead.
type DiscoveredProject = api.Project
