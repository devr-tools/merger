package cli

import (
	"flag"
	"fmt"
	"os"

	"github.com/devr-tools/merger/internal/config"
	"github.com/devr-tools/merger/internal/resolve"
)

func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	root := fs.String("repo-root", ".", "repository root to resolve relative paths against")
	configPath := fs.String("config", "", "path to a merger config file or directory (default: auto-discover)")
	policyPath := fs.String("policy", "", "path to a policy file (default: config's policy.path)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, resolvedConfig, err := resolve.Config(*root, *configPath)
	if err != nil {
		return err
	}

	if resolvedConfig == "" {
		fmt.Println("config:   (none found — validated defaults)")
	} else {
		fmt.Printf("config:   %s\n", resolvedConfig)
	}

	// A policy file is required for a configuration to be considered valid:
	// the whole point of merger is to evaluate policy against mutations.
	policyConfig, resolvedPolicy, found, err := resolve.Policy(*root, *policyPath, cfg)
	if err != nil {
		return err
	}
	if !found {
		wanted := *policyPath
		if wanted == "" {
			wanted = cfg.Policy.Path
		}
		return ExitError{Code: 1, Message: fmt.Sprintf("policy file not found (looked for %q); pass -policy or set policy.path", wanted)}
	}

	if err := validateLanes(cfg); err != nil {
		return ExitError{Code: 1, Message: err.Error()}
	}

	fmt.Printf("policy:   %s (%d rule(s))\n", resolvedPolicy, len(policyConfig.Policies))
	fmt.Printf("lanes:    green<=%d yellow<=%d red<=%d\n", cfg.Lanes.GreenMax, cfg.Lanes.YellowMax, cfg.Lanes.RedMax)
	fmt.Fprintln(os.Stdout, "ok")
	return nil
}

func validateLanes(cfg config.Config) error {
	l := cfg.Lanes
	if !(l.GreenMax < l.YellowMax && l.YellowMax < l.RedMax) {
		return fmt.Errorf("lane thresholds must be strictly increasing: green(%d) < yellow(%d) < red(%d)", l.GreenMax, l.YellowMax, l.RedMax)
	}
	return nil
}
