# Installation

gmux is distributed as pre-built binaries for common platforms and can also be built from source.

## From GitHub Releases

Pre-compiled binaries are published on the GitHub Releases page. The release workflow builds native executables for the following platforms (from Makefile PLATFORMS and release workflow):

- macOS (darwin) amd64 (Intel) and arm64 (Apple Silicon)
- Linux amd64 (x86_64) and arm64

Release assets are uploaded as plain executables named like:
- gmux-darwin-amd64
- gmux-darwin-arm64
- gmux-linux-amd64
- gmux-linux-arm64
- checksums.txt (SHA‑256 sums for all assets)

Steps:
1) Download the binary that matches your OS/architecture from:
   https://github.com/mattsolo1/grove-tmux/releases

2) Verify the checksum (recommended):
   - Linux:
     - sha256sum gmux-linux-amd64
     - Compare the output to the corresponding line in checksums.txt
   - macOS:
     - shasum -a 256 gmux-darwin-arm64
     - Compare the output to the corresponding line in checksums.txt

3) Make it executable and place it on your PATH:
   - Linux (amd64 example):
     - chmod +x gmux-linux-amd64
     - sudo mv gmux-linux-amd64 /usr/local/bin/gmux
   - macOS (arm64 example):
     - chmod +x gmux-darwin-arm64
     - sudo mv gmux-darwin-arm64 /usr/local/bin/gmux

4) Test the installation:
   - gmux version

Note:
- If macOS reports the file as from an unidentified developer, you can allow it in System Settings (Privacy & Security) or remove the quarantine attribute:
  - xattr -d com.apple.quarantine /usr/local/bin/gmux

## From Source

You can build gmux locally using the provided Makefile.

Prerequisites:
- Go 1.24.4 (as used in CI)
- Git
- make
- tmux installed (required to use tmux-related commands)

Steps:
1) Clone the repository:
   - git clone https://github.com/mattsolo1/grove-tmux.git
   - cd grove-tmux

2) Build:
   - make build
   - The binary is produced at ./bin/gmux

3) Optionally place it on your PATH:
   - chmod +x ./bin/gmux
   - sudo cp ./bin/gmux /usr/local/bin/gmux

4) Verify:
   - gmux version

Cross-compilation:
- To build all supported platforms locally:
  - make build-all
  - Outputs are written to ./dist as gmux-<os>-<arch>

After installation, consult the Getting Started guide to configure tmux integration (adding the generated bindings to your tmux config) and to run the sessionizer.