# Contributing to merger

Thanks for contributing to `merger`.

This file covers the fast path for contributors. Keep the repository root
[README](README.md) focused on the architecture and usage, and use the
[docs/](docs/) tree for deeper detail.

## Development Setup

merger requires Go `1.25.10` for local development and CI. Bootstrap the
expected toolchain into your shell before running repo commands:

```bash
eval "$(./scripts/dev/use-go-1.25.10.sh)"
go version
```

After that one-time install, plain `make` targets automatically prefer
`$HOME/sdk/go1.25.10/bin/go` when it exists.

Verify the local toolchain works:

```bash
make build
make test
```

## Common Commands

Run these before opening or updating a pull request:

```bash
make fmt
make lint
make test
make verify
make ci
```

`make verify` runs the full test tree and builds the commands. `make ci` is the
closest local match to GitHub Actions and additionally enforces the cyclomatic
complexity budget, the internal coverage gate, and the vulnerability scan.

## Running Locally

merger is a control plane made of two services plus platform dependencies
(PostgreSQL, Redis, NATS). Use the provided compose stack:

```bash
make compose-up
make run-ingest
make run-controlplane
```

See the [README](README.md#local-development) for default ports.

## Pull Requests

- keep changes scoped and update docs when behavior changes;
- add or update tests with code changes (CI enforces test presence on changed
  code paths);
- sign off your commits — CI verifies a `Signed-off-by` trailer (`git commit -s`);
- run `make ci` for CI-parity validation before pushing.

If your change touches install, release, or packaging behavior, also update
[README.md](README.md) and the relevant files in [docs/](docs/).

## Release and Packaging

Release automation (Release Please, tagged releases, GHCR publishing, and
Homebrew sync) is documented in [docs/release-automation.md](docs/release-automation.md).
