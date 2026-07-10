# Changelog

## Unreleased

### Features

* scaffold the Phase 1 control-plane slice: GitHub webhook ingest, PR diff
  parsing, Change Packet generation, rule-based semantic mutation detection,
  risk scoring, policy evaluation, and merge-lane assignment
  (`GREEN`/`YELLOW`/`RED`/`BLACK`)
* publish public extension seams (`pkg/extensions`) for SCM, topology, event,
  analyzer, and persistence adapters
* add first-party GitHub, NATS JetStream, and PostgreSQL implementations

### Ecosystem

* adopt the devr-tools tool-family conventions: Apache-2.0 `LICENSE`,
  `internal/version` package, `.golangci.yml` lint config, `SECURITY.md`,
  `CONTRIBUTING.md`, and this changelog
