// Package cli implements the user-facing `merger` command. It is the local,
// installable face of the control plane: it discovers configuration, validates
// it, and runs the analysis pipeline offline against a diff (see internal/scan)
// without requiring the ingest/control-plane services or their dependencies.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/devr-tools/merger/internal/version"
)

// ExitError carries an explicit process exit code. Commands return it when the
// exit status is meaningful beyond success/failure (for example a scan that
// trips a merge-lane gate).
type ExitError struct {
	Code    int
	Message string
}

func (e ExitError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("exit code %d", e.Code)
}

// Run dispatches a merger subcommand. args excludes the program name.
func Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printUsage(os.Stderr)
		return ExitError{Code: 2}
	}

	command, rest := args[0], args[1:]
	switch command {
	case "version", "--version", "-v":
		return runVersion(rest)
	case "init":
		return runInit(rest)
	case "validate":
		return runValidate(rest)
	case "scan":
		return runScan(ctx, rest)
	case "mcp":
		return runMCP(ctx, rest)
	case "help", "--help", "-h":
		printUsage(os.Stdout)
		return nil
	default:
		printUsage(os.Stderr)
		return ExitError{Code: 2, Message: fmt.Sprintf("unknown command %q", command)}
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintf(w, `merger %s — mutation control plane CLI

Usage:
  merger <command> [flags]

Commands:
  scan        Analyze a diff and assign a merge lane (offline pipeline)
  validate    Validate a merger configuration and its policy file
  init        Scaffold a .merger/ configuration in the current repository
  mcp         Serve the analysis tools over MCP (stdio)
  version     Print the merger version
  help        Show this help

Run "merger <command> -h" for command-specific flags.
`, version.Number)
}
