# Changelog

## [1.2.0](https://github.com/devr-tools/merger/compare/v1.1.0...v1.2.0) (2026-07-21)


### Features

* **action:** support GitHub Merge Queue gates ([8012830](https://github.com/devr-tools/merger/commit/80128307192c485e3201d7aa98cfea9077970bb1))
* add conflict routing and agent readiness tools ([4cf8b52](https://github.com/devr-tools/merger/commit/4cf8b5203130690741dd7624c6dade814e72d39d))
* **evidence:** retain immutable audit history ([ad664b2](https://github.com/devr-tools/merger/commit/ad664b2b2eb1edfd91ab0c9592f6f03a481eef30))
* **extensions:** add GitLab and Bitbucket SCM clients ([ef91e69](https://github.com/devr-tools/merger/commit/ef91e697a3ce80e3052dc027d2687c96570b1638))
* gate unresolved decisions from green lanes ([d11b461](https://github.com/devr-tools/merger/commit/d11b461028d876291631cc82bb4d4d337210be46))
* **ingest:** reconcile trusted GitHub checks as evidence ([5e6d2d9](https://github.com/devr-tools/merger/commit/5e6d2d9d702ba7fd44e294be6d25a9b042a9f4a7))
* **mutations:** support safe external analyzers ([712b598](https://github.com/devr-tools/merger/commit/712b59835083fec26bfd71d37a60ddb77931622a))
* **policy:** harden GitHub check bindings ([57c1332](https://github.com/devr-tools/merger/commit/57c13320d2e2a76ba16a32456ee24170fa89cf2d))
* **risk:** calibrate recommendations from deployment outcomes ([da13346](https://github.com/devr-tools/merger/commit/da13346fb2a981becb4a078e12638d9395268edd))
* **runtime:** resolve bounded transitive graph impact ([6316335](https://github.com/devr-tools/merger/commit/6316335d0e8c15dd00d7af50b66f228ba6f34a99))


### Bug Fixes

* **lanes:** split conflict override routing from threshold assignment ([5d5f221](https://github.com/devr-tools/merger/commit/5d5f221cf975c3bfd5c8fbacfd6f0c8f657ea5c0))
* **security:** document reviewed external analyzer execution ([52e1249](https://github.com/devr-tools/merger/commit/52e1249008b354865e85175061f6ac6e21198908))
* **tests:** make audit ordering and analyzer fixtures portable ([c31e296](https://github.com/devr-tools/merger/commit/c31e2967564d997fee2b9b8d6c33052596d4ce71))

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
