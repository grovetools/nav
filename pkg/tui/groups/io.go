package groups

import (
	"strings"

	core_theme "github.com/grovetools/core/tui/theme"
)

// resolveIcon converts a configured icon reference (a name token like
// "tree" or a literal glyph) into the rendered glyph. Mirrors the
// helper in cmd/nav/key_manage.go and pkg/tui/sessionizer/render.go.
func resolveIcon(iconRef string) string {
	switch iconRef {
	case "IconTree", "tree":
		return core_theme.IconTree
	case "IconProject", "project":
		return core_theme.IconProject
	case "IconRepo", "repo":
		return core_theme.IconRepo
	case "IconWorktree", "worktree":
		return core_theme.IconWorktree
	case "IconEcosystem", "ecosystem":
		return core_theme.IconEcosystem
	case "IconFolder", "folder":
		return core_theme.IconFolder
	case "IconFolderStar", "folder-star", "star":
		return core_theme.IconFolderStar
	case "IconHome", "home":
		return core_theme.IconHome
	case "IconCloud", "cloud":
		return "󰅧"
	case "IconCode", "code":
		return core_theme.IconCode
	case "IconBriefcase", "briefcase", "work":
		return "󰃖"
	case "IconKeyboard", "keyboard":
		return core_theme.IconKeyboard
	case "IconNote", "note":
		return core_theme.IconNote
	case "IconPlan", "plan":
		return core_theme.IconPlan
	default:
		return iconRef
	}
}

// resolvePrefixDisplay converts prefix placeholders to actual keys for
// display in the table.
func resolvePrefixDisplay(prefix string) string {
	switch prefix {
	case "<prefix>":
		return "C-b"
	case "<grove>":
		return "C-g"
	case "":
		return "(none)"
	default:
		if strings.HasPrefix(prefix, "<prefix> ") {
			return "C-b " + strings.TrimPrefix(prefix, "<prefix> ")
		}
		if strings.HasPrefix(prefix, "<grove> ") {
			return "C-g " + strings.TrimPrefix(prefix, "<grove> ")
		}
		return prefix
	}
}
