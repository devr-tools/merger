package version

// Number is the merger version. It must be a var (not a const) so the release
// build can override it via the linker: GoReleaser injects the git tag with
// `-X github.com/devr-tools/merger/internal/version.Number=v{{.Version}}` (see
// .goreleaser.yaml). The linker's -X flag only sets string vars, so a const
// would silently leave released binaries reporting this default. Release Please
// keeps the default below in sync with the manifest via the marker comment.
var Number = "1.1.0" // x-release-please-version
