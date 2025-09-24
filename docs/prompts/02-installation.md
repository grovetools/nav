# gmux Installation Guide

## Task
Write the installation instructions for `gmux`. The `README.md` shows "Todo" for installation, so you must infer the process from the CI/CD pipeline and Makefile.

1.  **Primary Method: GitHub Releases**: Explain that users can download pre-compiled binaries from the project's GitHub Releases page. Analyze `.github/workflows/release.yml` and the `build-all` target in `Makefile` to list the available platforms (OS/architecture).
2.  **Building from Source**: Provide instructions for building from source.
    - Mention Go is a prerequisite (check `.github/workflows/ci.yml` for the version).
    - Provide the `git clone` and `make build` commands.

## Output Format
Provide clean Markdown with an H1 title: `# Installation`. Use subheadings for "From GitHub Releases" and "From Source".