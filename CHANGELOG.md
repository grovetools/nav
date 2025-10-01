## v0.1.0 (2025-10-01)

This release introduces significant improvements to the Terminal User Interfaces (TUIs) in `gmux`. The sessionizer (`gmux sz`) and key manager (`gmux key manage`) now use the centralized Kanagawa theme for a consistent look and feel across the Grove ecosystem (4224b82). A standardized help system has been implemented in both interfaces, providing more detailed and structured guidance (ca2cc26). Navigation in the sessionizer is now more intuitive with the addition of vim-style keys (`j/k`, `ctrl+u/d`, `g/G`) and a clearer separation between navigation and filtering modes. The key binding for closing a session has been changed from `ctrl+d` to `X` to avoid conflicts (ca2cc26).

A comprehensive documentation suite has been added, covering all aspects of `gmux` from getting started guides to a full command reference (88e78ad, d04d8be). The project's README now features an automatically generated Table of Contents to improve navigability (4078852).

Internal architecture has been refactored to use the centralized tmux client from `grove-core`, removing duplicated code and standardizing interactions with tmux (d6822e2, 873ebc2). The CI/CD pipeline has also seen improvements, with the release process now sourcing its notes directly from the CHANGELOG file for better consistency (e3bf988).

### Features

*   Implement standardized help components and improved vim-style navigation in TUIs (ca2cc26)
*   Migrate TUI components to use the centralized Kanagawa theme for visual consistency (4224b82)
*   Add comprehensive project documentation and generation configuration (d04d8be)
*   Add Table of Contents generation for README and update docgen configuration (4078852)
*   Update release workflow to extract release notes from CHANGELOG.md (e3bf988)
*   Improve documentation generation configuration (b26e4b8)

### Bug Fixes

*   Update CI workflow to correctly disable execution using 'branches: [ none ]' (3658b82)

### Code Refactoring

*   Migrate from local implementation to the centralized tmux client from grove-core (d6822e2)
*   Update internal manager to use the centralized grove-core tmux client (873ebc2)
*   Standardize docgen.config.yml key order and settings (582aa2f)

### Documentation

*   Add a comprehensive documentation structure and generation prompts (88e78ad)
*   Update docgen configuration and add generated documentation files (70db90d)
*   Simplify documentation structure to four main sections (50f3c79)
*   Update docgen configuration and README templates for TOC generation (9d99352)
*   Simplify installation instructions to point to the main Grove ecosystem guide (be380ce)

### Chores

*   Temporarily disable CI workflow (e24dc3d)
*   Update .gitignore to track CLAUDE.md and ignore go.work files (ac2be9f)

### Continuous Integration

*   Remove redundant test execution from the release workflow (02c4606)

### File Changes

```
 .github/workflows/ci.yml             |    4 +-
 .github/workflows/release.yml        |   24 +-
 .gitignore                           |    3 +
 CLAUDE.md                            |   30 +
 README.md                            |  152 +--
 cmd/gmux/key.go                      |   25 +-
 cmd/gmux/key_manage.go               |   93 +-
 cmd/gmux/launch.go                   |   10 +-
 cmd/gmux/main.go                     |    6 +-
 cmd/gmux/session.go                  |    8 +-
 cmd/gmux/sessionize.go               |  496 ++++++----
 cmd/gmux/start.go                    |    5 +-
 cmd/gmux/wait.go                     |    4 +-
 docs/01-introduction.md              |   66 ++
 docs/01-overview.md                  |   45 +
 docs/02-examples.md                  |  108 ++
 docs/02-installation.md              |   27 +
 docs/03-configuration.md             |  115 +++
 docs/03-getting-started.md           |  110 +++
 docs/04-command-reference.md         |  396 ++++++++
 docs/04-configuration.md             |  184 ++++
 docs/05-live-sessionizer.md          |  160 +++
 docs/06-session-hotkeys.md           |  180 ++++
 docs/07-command-reference.md         |  177 ++++
 docs/08-advanced-topics.md           |  194 ++++
 docs/09-contributing.md              |  132 +++
 docs/README.md.tpl                   |    6 +
 docs/docgen.config.yml               |   40 +
 docs/docs.rules                      |    1 +
 docs/images/grove-tmux-readme.svg    | 1791 ++++++++++++++++++++++++++++++++++
 docs/prompts/01-overview.md          |   31 +
 docs/prompts/02-examples.md          |   20 +
 docs/prompts/03-configuration.md     |   20 +
 docs/prompts/04-command-reference.md |   21 +
 internal/manager/manager.go          |   56 +-
 pkg/docs/docs.json                   |  174 ++++
 pkg/tmux/client.go                   |   39 -
 pkg/tmux/doc.go                      |   30 -
 pkg/tmux/example_test.go             |   63 --
 pkg/tmux/launch.go                   |   79 --
 pkg/tmux/launch_test.go              |  159 ---
 pkg/tmux/monitor.go                  |   26 -
 pkg/tmux/monitor_test.go             |  137 ---
 pkg/tmux/session.go                  |   89 --
 pkg/tmux/session_test.go             |  127 ---
 pkg/tmux/types.go                    |   14 -
 46 files changed, 4527 insertions(+), 1150 deletions(-)
```

## v0.0.15 (2025-09-17)

### Chores

* bump dependencies

## v0.0.14 (2025-09-13)

### Chores

* update Grove dependencies to latest versions

## v0.0.12 (2025-08-27)

### Bug Fixes

* add version cmd
* address code review feedback

### Chores

* **deps:** bump dependencies

### Features

* improve first-run UX for gmux sessionizer

## Unreleased

### Features

* **sessionizer:** Add interactive first-run setup for `gmux sz` to guide new users through configuration

### Bug Fixes

* **sessionizer:** Improve error messages for malformed `project-search-paths.yaml` files by including the file path
* **makefile:** Fix dev target to use correct cmd/gmux directory
* **launch:** Update help text to use gmux instead of gtmux

## v0.0.11 (2025-08-26)

### Code Refactoring

* rename command directory from gtmux to gmux

## v0.0.10 (2025-08-26)

### Chores

* update readme (#1)

## v0.0.9 (2025-08-26)

### Bug Fixes

* improve worktree ordering and grouping in sessionizer

### Features

* prioritize open tmux sessions in gmux sz search

## v0.0.8 (2025-08-25)

### Chores

* **deps:** sync Grove dependencies to latest versions

## v0.0.7 (2025-08-25)

### Bug Fixes

* disbale unit tests
* disbale lfs

## v0.0.6 (2025-08-25)

### Continuous Integration

* disable linting in workflow

### Chores

* **deps:** bump dependencies

## v0.0.5 (2025-08-25)

### Bug Fixes

* prevent flashing in live sessionizer by only updating on actual changes

### Features

* implement live sessionizer with automatic refresh

### Chores

* **deps:** bump dependencies
* bump dependencies

## v0.0.4 (2025-08-15)

### Features

* improve sessionizer key mapping UX and Claude session filtering
* enhance Claude session integration in sessionizer
* add Claude session status indicator to sessionizer
* key to copy paths from session list
* add session management features to gmux sz
* implement automatic Git worktree discovery and hierarchical display
* add session status indicator to gmux sz command
* add sessionize command with smart features and key management
* add compact style for gmux list and improve key management UX
* add comprehensive key management commands with interactive UI
* **tmux:** add window management methods

### Bug Fixes

* use configured repository name in session list and update tests to use gmux
* change binary to gmux

### Chores

* bump deps
* fix linting issues in sessionizer

### Tests

* fix E2E tests to match current list command output

### Continuous Integration

* disable Git LFS to fix dependency download issues

## v0.0.3 (2025-08-13)

### Features

* add new tmux package and CLI commands

### Code Refactoring

* standardize E2E binary naming and use grove.yml for binary discovery

### Continuous Integration

* switch to Linux runners to reduce costs
* consolidate to single test job on macOS
* reduce test matrix to macOS with Go 1.24.4 only

### Bug Fixes

* remove tmux pane numbering test that varies between versions

## v0.0.2 (2025-08-12)

### Bug Fixes

* makefile

