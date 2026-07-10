# Testing Patterns

- Tests are **black-box, kept out of source packages**: they live under `tests/` (e.g. `tests/mutations`, `tests/policy`, `tests/lanes`, `tests/ingest`, `tests/github`, `tests/controlplane`), mirroring `internal/` package names.
- `make test` runs only packages that contain `*_test.go`; `make test-all` runs `./...`. `make smoke` runs a fast subset (`tests/controlplane`, `tests/github`, `tests/ingest`).
- Coverage is gated on **internal packages only**: `make coverage` runs the `tests/...` tree with `-coverpkg=./internal/...` and fails below `MIN_INTERNAL_COVERAGE` (default 60%).
- CI additionally enforces: `make gocyclo` (cyclomatic complexity ≤ 15 over `cmd`/`internal`/`pkg`), `make security` (govulncheck), a test-presence gate on changed code paths (`scripts/ci/check_test_presence.sh`), semgrep, and a DCO `Signed-off-by` check. Sign commits with `git commit -s`.
- The 3-OS matrix (ubuntu/macos/windows) runs `go test ./tests/...`, so keep tests OS-portable (no hardcoded `/tmp`, path separators, etc.).
