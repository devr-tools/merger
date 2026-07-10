# Getting Started

This guide covers installing `merger`, running your first scan, and standing up
the full control plane locally. If you only want to classify a diff and preview
its merge lane, you can stop after [Install](#install) and [First scan](#first-scan)
— no services, database, or event bus required.

## Prerequisites

| Use case | Requirements |
| --- | --- |
| CLI / SDK | Go `1.25+` (only to build from source or `go install`; prebuilt binaries need nothing) |
| Local control plane | Docker + Docker Compose, Go `1.25.10` |
| GitHub Action | None — the action installs `merger` for you |

## Install

Recommended (Go):

```bash
go install github.com/devr-tools/merger/cmd/merger@latest
merger version
```

Other install paths:

- **GitHub Releases** — tagged archives for direct download
- **Homebrew** — `brew install devr-tools/tap/merger`
- **GitHub Marketplace Action** — `Devr Merger` (see [GitHub Action](#github-action))
- **Source build** — `make build` produces `./bin/merger`

## First scan

`merger` runs the same analysis pipeline the control plane uses, entirely
offline. Scaffold a config, validate it, then scan a diff:

```bash
merger init                       # scaffold .merger/ config + policy
merger validate                   # check config and policy resolve
merger scan -base-ref origin/main # analyze the diff vs a base ref
```

You can also feed a diff file (or stdin) directly and choose an output format:

```bash
merger scan -diff change.diff -format json
git diff main...HEAD | merger scan -diff - -format text
```

Use `merger scan` as a CI gate by failing on a lane threshold:

```bash
merger scan -base-ref origin/main -fail-on-lane RED
```

This exits non-zero when the assigned lane is at or above `RED`.

### Configuration discovery

`merger` auto-discovers configuration from, in order:

1. `merger.yaml` in the repository root
2. `.merger/merger.yaml`

Point `-config` at an explicit file or a directory (such as `.merger`) to
override discovery. Policies default to the config's policy path; override with
`-policy <file>`.

A minimal, composable policy looks like:

```yaml
policies:
  - name: auth_requires_security_review
    when:
      mutations:
        - auth_behavior_change
    require:
      reviewers:
        - security
      evidence:
        - auth_integration_tests
      deployment:
        strategy: canary
        requires_canary: true
    action:
      minimum_lane: RED
```

## MCP server

Serve the same offline analysis as agent tools over the Model Context Protocol
(stdio):

```bash
merger mcp
```

Point an MCP client at the `merger mcp` command as a stdio server. See
[docs/mcp.md](mcp.md) for the tool contract.

## GitHub Action

Add the `Devr Merger` action to gate pull requests on their assigned lane:

```yaml
- uses: devr-tools/merger@v1
  with:
    base-ref: ${{ github.base_ref }}
    fail-on-lane: RED
```

| Input | Default | Description |
| --- | --- | --- |
| `base-ref` | `main` | Base ref to diff against (`git diff <base-ref>...HEAD`) |
| `config` | auto-discover | Path to a config file or directory |
| `repo-root` | `.` | Repository root for content lookups and relative paths |
| `format` | `text` | Output format (`text` or `json`) |
| `fail-on-lane` | report only | Fail when the assigned lane is at or above this lane |
| `version` | `latest` | `merger` version to install |

## Run the control plane locally

The full control plane (webhook ingest, persistence, event bus) needs the local
toolchain and the Compose stack for platform dependencies.

### 1. Bootstrap the toolchain

Merger requires Go `1.25.10` for local development and CI — this is part of the
security baseline, not an optional preference. Bootstrap it into your shell:

```bash
eval "$(./scripts/dev/use-go-1.25.10.sh)"
go version
```

The helper installs the `go1.25.10` launcher if needed, downloads the toolchain
into `$HOME/sdk/go1.25.10`, and exports `GOROOT`, `PATH`, and `GO` for the
current shell. After that one-time install, plain `make` targets prefer
`$HOME/sdk/go1.25.10/bin/go` when it exists, so `make ci` does not depend on your
shell defaulting to the right Go version.

### 2. Start dependencies and services

```bash
make compose-up        # Postgres, Redis, NATS via deployments/local/docker-compose.yml
make run-ingest        # HTTP ingress for GitHub pull_request webhooks
make run-controlplane  # downstream orchestration and subscriptions
```

Default ports:

| Service | Port |
| --- | --- |
| Ingest HTTP | `:8080` |
| Control-plane HTTP | `:8081` |
| Control-plane gRPC | `:9091` |
| PostgreSQL | `:5432` |
| Redis | `:6379` |
| NATS | `:4222` |

Tear the stack down with `make compose-down`.

### 3. Verify

```bash
make verify   # test-all + build
make ci       # full local CI (fmt, vet, tests, coverage, lint, security)
```

## Next steps

- [SDK guide](sdk.md) — embed the offline pipeline as a Go library
- [MCP server](mcp.md) — agent tool contract
- [Extending merger](extending-merger.md) — plug in your own SCM, topology, event, and persistence adapters
- [GitHub webhook flow](flows/github-webhook-flow.md) — detailed end-to-end flow
- [Release automation](release-automation.md) — how releases are cut
