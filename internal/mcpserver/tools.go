package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/devr-tools/merger/internal/resolve"
	"github.com/devr-tools/merger/internal/scan"
)

// ErrUnknownTool is returned by Call for a tool name the server does not expose.
var ErrUnknownTool = errors.New("unknown tool")

// Content is a single MCP tool-result content block.
type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Result is an MCP tool-call result.
type Result struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// Definition describes a tool for tools/list.
type Definition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Definitions returns the tools this server exposes.
func Definitions() []Definition {
	return []Definition{
		{
			Name:        "merger_scan",
			Description: "Analyze a unified diff and return the resulting Change Packet as JSON: detected mutations, runtime impact, risk score, policy decision, and assigned merge lane.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"diff":      map[string]any{"type": "string", "description": "Raw unified diff to analyze (as produced by `git diff`)."},
					"repo_root": map[string]any{"type": "string", "description": "Repository root for file-content lookups and relative config paths. Defaults to the current directory."},
					"repo":      map[string]any{"type": "string", "description": "Repository identifier as owner/name (optional)."},
					"ref":       map[string]any{"type": "string", "description": "Revision the diff targets (optional)."},
					"config":    map[string]any{"type": "string", "description": "Path to a merger config file or directory (optional; auto-discovered otherwise)."},
					"policy":    map[string]any{"type": "string", "description": "Path to a policy file (optional; defaults to the config's policy path)."},
				},
				"required": []string{"diff"},
			},
		},
		{
			Name:        "merger_validate",
			Description: "Validate a merger configuration and its policy file, reporting the resolved config path, policy rule count, and lane thresholds.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"repo_root": map[string]any{"type": "string", "description": "Repository root to resolve config and policy against. Defaults to the current directory."},
					"config":    map[string]any{"type": "string", "description": "Path to a merger config file or directory (optional; auto-discovered otherwise)."},
					"policy":    map[string]any{"type": "string", "description": "Path to a policy file (optional; defaults to the config's policy path)."},
				},
			},
		},
	}
}

// Call dispatches a tool by name.
func Call(ctx context.Context, name string, args map[string]any) (Result, error) {
	switch name {
	case "merger_scan":
		return toolScan(ctx, args)
	case "merger_validate":
		return toolValidate(args)
	default:
		return Result{}, fmt.Errorf("%w: %s", ErrUnknownTool, name)
	}
}

func toolScan(ctx context.Context, args map[string]any) (Result, error) {
	root := stringArg(args, "repo_root")
	if root == "" {
		root = "."
	}

	options, _, err := resolve.ScanOptions(
		root,
		stringArg(args, "config"),
		stringArg(args, "policy"),
		stringArg(args, "repo"),
		stringArg(args, "ref"),
		stringArg(args, "diff"),
	)
	if err != nil {
		return Result{}, err
	}

	packet, err := scan.Run(ctx, options)
	if err != nil {
		return Result{}, err
	}

	data, err := json.MarshalIndent(packet, "", "  ")
	if err != nil {
		return Result{}, err
	}
	return Result{Content: []Content{{Type: "text", Text: string(data)}}}, nil
}

func toolValidate(args map[string]any) (Result, error) {
	root := stringArg(args, "repo_root")
	if root == "" {
		root = "."
	}

	cfg, cfgPath, err := resolve.Config(root, stringArg(args, "config"))
	if err != nil {
		return Result{}, err
	}
	policyConfig, policyPath, found, err := resolve.Policy(root, stringArg(args, "policy"), cfg)
	if err != nil {
		return Result{}, err
	}
	if !found {
		return Result{
			Content: []Content{{Type: "text", Text: "no policy file found; set policy.path in the config or pass a policy path"}},
			IsError: true,
		}, nil
	}

	var b strings.Builder
	if cfgPath == "" {
		b.WriteString("config: (none found — using defaults)\n")
	} else {
		fmt.Fprintf(&b, "config: %s\n", cfgPath)
	}
	fmt.Fprintf(&b, "policy: %s (%d rule(s))\n", policyPath, len(policyConfig.Policies))
	fmt.Fprintf(&b, "lanes:  green<=%d yellow<=%d red<=%d\n", cfg.Lanes.GreenMax, cfg.Lanes.YellowMax, cfg.Lanes.RedMax)
	b.WriteString("ok")

	return Result{Content: []Content{{Type: "text", Text: b.String()}}}, nil
}

func stringArg(args map[string]any, key string) string {
	if raw, ok := args[key]; ok {
		if s, ok := raw.(string); ok {
			return s
		}
	}
	return ""
}
