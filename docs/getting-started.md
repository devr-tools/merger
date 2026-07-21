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

For an actionable explanation of the result, including policy rationale, risk
contributors, mitigations, affected services, and runtime notes, add
`-explain`:

```bash
merger scan -base-ref origin/main -explain
```

`merger validate` uses the same strict validators as the SDK, MCP server, and
long-running services. It rejects unknown YAML fields, invalid lane thresholds
or enum values, duplicate policy names, and rules with no condition or effect.
This catches misspelled safety controls before a scan or service starts.

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
      github_checks:
        - evidence: auth_integration_tests
          name: CI / auth integration
          app_id: 12345
      deployment:
        strategy: canary
        requires_canary: true
    action:
      minimum_lane: RED
```

`github_checks` optionally authorizes automatic evidence reconciliation from
GitHub `check_run` webhooks. A binding must reference an evidence item declared
in the same rule and includes both the exact check name and the numeric GitHub
App ID. Merger rejects a check/App pair bound to different evidence items and
rejects conflicting bindings for the same evidence across policies. Scalar
`evidence` entries remain manual unless explicitly bound.

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

The Action exports `lane`, `risk-score`, and `change-packet-id` outputs for use
by later workflow steps.

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

### Control-plane authentication

Local development defaults to `access.mode: disabled`. For a deployed control
plane, either configure environment-backed bearer tokens or validate signed
JWTs from your auth gateway or identity provider.

Static token mode:

```yaml
access:
  mode: static_token
  tokens:
    - subject: ci
      token_env: MERGER_CI_TOKEN
      roles: [evidence_writer]
    - subject: dashboard
      token_env: MERGER_DASHBOARD_TOKEN
      roles: [reader]
```

Set the referenced environment variables on the control-plane process. Merger
stores only token digests in memory. Production configuration validation rejects
disabled access.

JWT mode:

```yaml
access:
  mode: jwt
  jwt:
    algorithm: HS256
    issuer: https://auth.example.test
    audience: merger-controlplane
    secret_env: MERGER_CONTROLPLANE_JWT_SECRET
    roles_claim: scope
    role_bindings:
      - claim_value: merger.read
        roles: [reader]
      - claim_value: merger.write
        roles: [evidence_writer]
```

For asymmetric signing, use `algorithm: RS256` and `public_key_path` instead of
`secret_env`. Merger validates issuer, audience, expiry, and signature, then
maps claim values to Merger roles. `subject_claim` defaults to `sub`, and
`roles_claim` defaults to `roles` when omitted.

### Evidence audit history

Every accepted evidence update retains the current execution snapshot and
appends an immutable audit record containing the previous and new status,
actor, timestamp, summary, details URL, and provenance metadata. Read a
packet's history with:

```bash
curl -H "Authorization: Bearer $MERGER_DASHBOARD_TOKEN" \
  'http://localhost:8081/api/v1/change-packets/cp_example/evidence/audit?limit=50'
```

Audit records are append-only. GitHub check reconciliation records its trusted
check-run, app, and commit provenance in the entry metadata.

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
