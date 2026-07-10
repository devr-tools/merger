# Release Automation

merger follows the same devr-tools tool-family release shape as its siblings
(`codeguard`, `cleanr`, `szr`). Because merger is a control-plane **service**
rather than a single CLI, its primary released artifact is a multi-arch
container image; cross-platform binary archives are published alongside it.

## Workflows

### `.github/workflows/cd.yml`

Branch-driven CD entry point. It currently:

- runs on pushes to `develop`, `master`, and `main`;
- computes a prerelease tag automatically for `develop` (conventional-commit
  bump: `feat` → minor, `BREAKING CHANGE`/`!` → major, otherwise patch);
- reuses `.github/workflows/release.yml` for prerelease packaging;
- runs Release Please on `main`/`master` to open and land release PRs.

### `.github/workflows/release.yml`

Reusable publisher invoked by CD or manual dispatch. It currently:

- supports `workflow_dispatch` and `workflow_call`;
- normalizes and validates the tag before releasing;
- runs GoReleaser using `.goreleaser.yaml`;
- uploads release archives and `SHA256SUMS`;
- publishes multi-arch GHCR images (`linux/amd64`, `linux/arm64`) plus a
  combined manifest for the release tag.

## Release Please Files

Stable-branch release preparation is driven by:

- `.github/release-please-config.json`
- `.release-please-manifest.json`
- `CHANGELOG.md`
- `internal/version/version.go` (the `Number` var is updated in place via the
  `x-release-please-version` marker and injected into release binaries by
  GoReleaser)

## Required Secrets

- `GITHUB_TOKEN`: used by the release workflow for GitHub Releases and GHCR
  publishing.
- `RELEASE_PLEASE_TOKEN`: used for Release Please PRs (a PAT or GitHub App token
  so release PRs trigger required `pull_request` workflows).

## Published Outputs

Each tagged release currently publishes:

- `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64` archives, each
  containing the `merger-ingest` and `merger-controlplane` binaries;
- `SHA256SUMS`;
- `ghcr.io/devr-tools/merger:<tag>` (multi-arch: `-amd64`, `-arm64`).

The image bundles both services; select one at runtime by overriding the
command (default is `merger-controlplane`):

```bash
docker run ghcr.io/devr-tools/merger:<tag> merger-ingest
```

## Local Developer Commands

```bash
make release-check     # goreleaser check (validate .goreleaser.yaml)
make release-snapshot  # build a local snapshot into dist/ without publishing
make commit            # guided conventional-commit helper
```

## Not Yet Wired

- **Homebrew tap sync** and **npm/pip distribution** are deferred until the
  user-facing `merger` CLI binary lands (see the CLI phase of the ecosystem
  build-up); those channels distribute a CLI, not the daemons.
