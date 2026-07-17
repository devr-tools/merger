package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

const defaultConfigTemplate = `# merger configuration
# Docs: https://github.com/devr-tools/merger

service:
  ingest_address: ":8080"
  controlplane_address: ":8081"
  controlplane_grpc_address: ":9091"

access:
  mode: disabled

logging:
  level: info

policy:
  path: .merger/policies/default.yaml

lanes:
  green_max: 20
  yellow_max: 55
  red_max: 85

runtime_graph:
  enable_codeowners: true
`

const defaultPolicyTemplate = `# merger policy rules
# Policies are evaluated against detected mutations, risk, and ownership.
policies:
  - name: auth_requires_security_review
    when:
      mutations:
        - auth_behavior_change
    require:
      reviewers:
        - security
      evidence:
        - auth_integration_tests
      deployment:
        strategy: canary
        requires_canary: true
    action:
      minimum_lane: RED
`

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	dir := fs.String("dir", ".", "repository root to initialize")
	force := fs.Bool("force", false, "overwrite existing files")
	if err := fs.Parse(args); err != nil {
		return err
	}

	mergerDir := filepath.Join(*dir, ".merger")
	policiesDir := filepath.Join(mergerDir, "policies")
	if err := os.MkdirAll(policiesDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", policiesDir, err)
	}

	files := []struct {
		path    string
		content string
	}{
		{filepath.Join(mergerDir, "merger.yaml"), defaultConfigTemplate},
		{filepath.Join(policiesDir, "default.yaml"), defaultPolicyTemplate},
	}

	for _, file := range files {
		if fileExists(file.path) && !*force {
			return ExitError{Code: 1, Message: fmt.Sprintf("%s already exists (use -force to overwrite)", file.path)}
		}
	}

	for _, file := range files {
		if err := os.WriteFile(file.path, []byte(file.content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", file.path, err)
		}
		fmt.Printf("wrote %s\n", file.path)
	}

	fmt.Println("initialized .merger/ — run `merger validate` to check it")
	return nil
}
