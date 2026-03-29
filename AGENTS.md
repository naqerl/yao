# Agent Instructions

## Build Verification

Use `make vet` instead of `go build` to verify code changes. This runs:
- `go fmt ./...` - Formats Go code
- `go vet ./...` - Reports suspicious constructs
- `staticcheck ./...` - Runs additional static analysis

Always run `make vet` after making code changes to ensure code quality and consistency.
