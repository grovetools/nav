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

