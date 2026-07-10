# Local Development Setup

- Requires Go `1.25.10` (pinned as part of the security baseline, not a preference). Bootstrap with `eval "$(./scripts/dev/use-go-1.25.10.sh)"` — it installs the toolchain into `$HOME/sdk/go1.25.10` and exports `GOROOT`/`PATH`/`GO`.
- After the one-time install, `make` targets auto-prefer `$HOME/sdk/go1.25.10/bin/go` when present, so `make ci` does not depend on the shell default. `make print-go` shows which Go binary the Makefile resolved.
- Build cache is redirected to `.build/go-cache` (via `GOCACHE` in the Makefile); `make clean` removes `.build`.
- Run the stack locally: `make compose-up` (Postgres/Redis/NATS via `deployments/local/docker-compose.yml`), then `make run-ingest` and `make run-controlplane`. Services read config from `MERGER_CONFIG_PATH` (default `config/merger.yaml`).
- Default ports: ingest HTTP `:8080`, control-plane HTTP `:8081`, control-plane gRPC `:9091`, Postgres `:5432`, Redis `:6379`, NATS `:4222`.
- Regenerate protobufs with `make proto` (needs `protoc` plus the Go plugins on PATH).
