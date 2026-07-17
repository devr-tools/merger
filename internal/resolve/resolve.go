// Package resolve turns a repository root and optional config/policy paths into
// a ready-to-run scan.Options. It centralizes merger's config auto-discovery so
// the CLI and the MCP server resolve configuration identically.
package resolve

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/devr-tools/merger/internal/config"
	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/lanes"
	"github.com/devr-tools/merger/internal/policy"
	"github.com/devr-tools/merger/internal/scan"
)

// configCandidates lists the config locations merger auto-discovers, in
// priority order, relative to the repository root. This mirrors the devr-tools
// convention (a root file, then a tool-named dot directory).
var configCandidates = []string{
	"merger.yaml",
	"merger.yml",
	"merger.json",
	".merger/merger.yaml",
	".merger/merger.yml",
	".merger/merger.json",
	".merger/config.yaml",
	".merger/config.yml",
	".merger/config.json",
}

var configDirCandidates = []string{
	"merger.yaml", "merger.yml", "merger.json",
	"config.yaml", "config.yml", "config.json",
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// discoverConfigPath resolves the config file to use. An explicit path is
// honored (and, if it is a directory, searched for a merger config). Otherwise
// the standard candidates under root are tried. An empty return with a nil
// error means no config was found and defaults should be used.
func discoverConfigPath(root, explicit string) (string, error) {
	if explicit != "" {
		info, err := os.Stat(explicit)
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return explicit, nil
		}
		for _, name := range configDirCandidates {
			candidate := filepath.Join(explicit, name)
			if fileExists(candidate) {
				return candidate, nil
			}
		}
		return "", fmt.Errorf("no merger config (merger.* or config.*) found in %s", explicit)
	}

	for _, candidate := range configCandidates {
		path := filepath.Join(root, candidate)
		if fileExists(path) {
			return path, nil
		}
	}
	return "", nil
}

// Config discovers and loads configuration, falling back to defaults when none
// is found. It returns the resolved path ("" when defaults were used).
func Config(root, explicit string) (config.Config, string, error) {
	path, err := discoverConfigPath(root, explicit)
	if err != nil {
		return config.Config{}, "", err
	}
	if path == "" {
		cfg := config.Defaults()
		if err := config.Validate(cfg); err != nil {
			return config.Config{}, "", err
		}
		return cfg, "", nil
	}
	cfg, err := config.Load(path)
	if err != nil {
		return config.Config{}, "", fmt.Errorf("load config %s: %w", path, err)
	}
	return cfg, path, nil
}

// PolicyPath determines the policy file for a run. An explicit flag wins;
// otherwise the config's policy path is resolved relative to root. The returned
// path is empty when no policy file could be located.
func PolicyPath(root, explicit string, cfg config.Config) string {
	if explicit != "" {
		return explicit
	}
	if cfg.Policy.Path == "" {
		return ""
	}
	path := cfg.Policy.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	if fileExists(path) {
		return path
	}
	return ""
}

// Policy loads a policy file when one is resolvable. A missing policy is not an
// error: it yields an empty rule set and found=false.
func Policy(root, explicit string, cfg config.Config) (policy.Config, string, bool, error) {
	path := PolicyPath(root, explicit, cfg)
	if path == "" {
		return policy.Config{}, "", false, nil
	}
	policyConfig, err := policy.LoadConfig(path)
	if err != nil {
		return policy.Config{}, path, false, fmt.Errorf("load policy %s: %w", path, err)
	}
	return policyConfig, path, true, nil
}

// RepoRef parses an "owner/name" identifier into a domain.RepoRef.
func RepoRef(raw string) domain.RepoRef {
	if raw == "" {
		return domain.RepoRef{}
	}
	owner, name, found := strings.Cut(raw, "/")
	if !found {
		return domain.RepoRef{Name: raw, FullName: raw}
	}
	return domain.RepoRef{Owner: owner, Name: name, FullName: raw}
}

// ScanOptions discovers configuration and policy and assembles the scan
// options for a diff. The bool return reports whether a policy file was found.
func ScanOptions(root, configPath, policyPath, repo, ref, diff string) (scan.Options, bool, error) {
	cfg, _, err := Config(root, configPath)
	if err != nil {
		return scan.Options{}, false, err
	}
	policyConfig, _, policyFound, err := Policy(root, policyPath, cfg)
	if err != nil {
		return scan.Options{}, false, err
	}

	return scan.Options{
		Diff:     diff,
		RepoRoot: root,
		Repo:     RepoRef(repo),
		Ref:      ref,
		Policy:   policyConfig,
		Lanes: lanes.Config{
			GreenMax:  cfg.Lanes.GreenMax,
			YellowMax: cfg.Lanes.YellowMax,
			RedMax:    cfg.Lanes.RedMax,
		},
		EnableCodeOwners: cfg.RuntimeGraph.EnableCodeOwners,
	}, policyFound, nil
}
