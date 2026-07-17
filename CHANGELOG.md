# Changelog

## [1.1.0](https://github.com/devr-tools/merger/compare/v1.0.0...v1.1.0) (2026-07-17)


### Features

* authenticate control plane and reconcile evidence ([f18b9e7](https://github.com/devr-tools/merger/commit/f18b9e722614bf528edde17e303dcf98bfcb49ee))
* harden analysis and improve change feedback ([17866b2](https://github.com/devr-tools/merger/commit/17866b2de37549336c5040acaa79e37c0ea42c2a))
* support jwt control-plane authentication ([b6d522f](https://github.com/devr-tools/merger/commit/b6d522fe71822985e0f295f6a195d0a95980e327))
* validate policies and guard evidence updates ([40163a1](https://github.com/devr-tools/merger/commit/40163a1a806da1053aa443b1e475b3cc9cb84cd7))


### Bug Fixes

* align ci with latest codeguard ([4d30fa3](https://github.com/devr-tools/merger/commit/4d30fa39d85bb13ca9ef5848d0bc2eac750e6654))

## [1.0.0](https://github.com/devr-tools/merger/compare/v0.1.0...v1.0.0) (2026-07-10)


### ⚠ BREAKING CHANGES

* import paths change from github.com/mergerhq/merger/... to github.com/devr-tools/merger/...

### Features

* add GitHub Action and Homebrew distribution for the CLI ([308672b](https://github.com/devr-tools/merger/commit/308672b36550e7e0936b247719fdd34391521115))
* add MCP server and extract shared config resolution ([351677d](https://github.com/devr-tools/merger/commit/351677d9028f02e28d5494c52ecf8e826d78d163))
* add pkg/merger SDK for the offline scan pipeline ([f633dea](https://github.com/devr-tools/merger/commit/f633deafd78352f3c13aee7957b7ac204f7c149a))
* add user-facing merger CLI with offline scan pipeline ([45d86b4](https://github.com/devr-tools/merger/commit/45d86b4db347b4a4a56e2c2ff522282556eb6779))


### Bug Fixes

* use /bin/bash in Makefile for CI portability ([62b1cc2](https://github.com/devr-tools/merger/commit/62b1cc22b4c1c1375dc8db70053a8a2ec4cf577f))


### Code Refactoring

* rename module to github.com/devr-tools/merger ([91f5f6b](https://github.com/devr-tools/merger/commit/91f5f6b79fbe7c611fd2239a0c93ec0c3a7580c3))

## Changelog

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
* dogfood the sibling tool `codeguard` as a CI quality/security gate
  (`.codeguard/codeguard.yaml` + baseline), running in `diff` mode on PRs
* adopt the devr-tools tool-family conventions: Apache-2.0 `LICENSE`,
  `internal/version` package, `.golangci.yml` lint config, `SECURITY.md`,
  `CONTRIBUTING.md`, and this changelog
