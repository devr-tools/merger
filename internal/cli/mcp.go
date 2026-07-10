package cli

import (
	"context"
	"flag"

	"github.com/devr-tools/merger/internal/mcpserver"
)

func runMCP(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	return mcpserver.Run(ctx)
}
