package mutations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/devr-tools/merger/internal/domain"
	"os/exec"
	"path/filepath"
	"time"
)

// ExternalAnalyzerConfig opts into a single JSON-over-stdio analyzer. The
// executable must exactly match an allowlisted absolute path; no shell or
// caller-provided arguments are ever used.
type ExternalAnalyzerConfig struct {
	Name       string
	Executable string
	Allowlist  []string
	Timeout    time.Duration
	Paths      []string
}
type externalAnalyzer struct{ config ExternalAnalyzerConfig }

func NewExternalAnalyzer(config ExternalAnalyzerConfig) (Analyzer, error) {
	if !filepath.IsAbs(config.Executable) {
		return nil, fmt.Errorf("external analyzer executable must be absolute")
	}
	allowed := false
	for _, path := range config.Allowlist {
		if path == config.Executable {
			allowed = true
		}
	}
	if !allowed {
		return nil, fmt.Errorf("external analyzer executable is not allowlisted")
	}
	if config.Timeout <= 0 {
		config.Timeout = 5 * time.Second
	}
	if config.Name == "" {
		config.Name = filepath.Base(config.Executable)
	}
	return externalAnalyzer{config}, nil
}
func (a externalAnalyzer) Name() string { return a.config.Name }
func (a externalAnalyzer) Supports(file domain.ChangedFile) bool {
	if len(a.config.Paths) == 0 {
		return true
	}
	for _, pattern := range a.config.Paths {
		if ok, _ := filepath.Match(pattern, file.Path); ok {
			return true
		}
	}
	return false
}
func (a externalAnalyzer) Analyze(ctx context.Context, input AnalysisInput) ([]domain.Mutation, error) {
	ctx, cancel := context.WithTimeout(ctx, a.config.Timeout)
	defer cancel()
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, a.config.Executable)
	cmd.Stdin = bytes.NewReader(payload)
	output, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("external analyzer %s timed out: %w", a.Name(), ctx.Err())
	}
	if err != nil {
		return nil, fmt.Errorf("external analyzer %s: %w", a.Name(), err)
	}
	var mutations []domain.Mutation
	if err := json.Unmarshal(output, &mutations); err != nil {
		return nil, fmt.Errorf("external analyzer %s returned invalid JSON: %w", a.Name(), err)
	}
	for i := range mutations {
		mutations[i].Detector = a.Name()
	}
	return mutations, nil
}
