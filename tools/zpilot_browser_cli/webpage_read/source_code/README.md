# webpage_read source

This directory is a self-contained Go module. No neighboring tool source or base agent is required.

Build here with Go 1.24+: go build -trimpath -o webpage_read .
Test with: go test ./...
The internal/browsercli package is included in full and compiled into the executable.
Keep config.json beside the executable. See the release root README.md and CLI-USAGE.md for setup and flags.
