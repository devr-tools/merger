# Changelog

## Unreleased

### Features

* add the user-facing `merger` CLI (`cmd/merger`) with `scan`, `validate`,
  `init`, and `version` commands; `scan` runs the analysis pipeline offline
  against a diff and assigns a merge lane, with `-format text|json` and a
  `-fail-on-lane` CI gate. Config is auto-discovered from `.merger/`.
* add `internal/scan`, an offline pipeline that reuses the mutations, runtime
  graph, risk, policy, and lane engines without the ingest/service dependencies
* add `merger mcp`, a Model Context Protocol server (stdio) exposing
  `merger_scan` and `merger_validate` as agent tools
* scaffold the Phase 1 control-plane slice: GitHub webhook ingest, PR diff
  parsing, Change Packet generation, rule-based semantic mutation detection,
  risk scoring, policy evaluation, and merge-lane assignment
  (`GREEN`/`YELLOW`/`RED`/`BLACK`)
* publish public extension seams (`pkg/extensions`) for SCM, topology, event,
  analyzer, and persistence adapters
* add first-party GitHub, NATS JetStream, and PostgreSQL implementations

### Ecosystem

* add the `github.com/devr-tools/merger/pkg/merger` SDK (`Scan`, `LoadPolicy`,
  `DefaultLanes`) wrapping the offline pipeline
* rename the Go module to `github.com/devr-tools/merger` to match the GitHub
  home so `go install` and Homebrew resolve correctly (**breaking**: import
  paths change from `github.com/mergerhq/merger/...`)
* publish a composite GitHub Action (`action.yml`) that installs the CLI and
  runs `merger scan` with an optional `fail-on-lane` gate
* wire Homebrew distribution: `sync-homebrew-formula` in the release workflow
  and `homebrew-validation.yml` for PR validation
* adopt the devr-tools tool-family conventions: Apache-2.0 `LICENSE`,
  `internal/version` package, `.golangci.yml` lint config, `SECURITY.md`,
  `CONTRIBUTING.md`, and this changelog
