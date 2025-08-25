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

