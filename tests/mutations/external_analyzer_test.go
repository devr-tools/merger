package mutations_test

import (
	"context"
	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/mutations"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestExternalAnalyzerSafetyAndFailureHandling(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external analyzer fixture uses POSIX shell scripts")
	}
	script := helperPath(t, "analyzer")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho '[]'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := mutations.NewExternalAnalyzer(mutations.ExternalAnalyzerConfig{Executable: script}); err == nil {
		t.Fatal("expected allowlist rejection")
	}
	analyzer, err := mutations.NewExternalAnalyzer(mutations.ExternalAnalyzerConfig{Name: "external", Executable: script, Allowlist: []string{script}, Paths: []string{"*.go"}})
	if err != nil {
		t.Fatal(err)
	}
	if analyzer.Supports(domain.ChangedFile{Path: "x.yaml"}) {
		t.Fatal("path filter should reject yaml")
	}
	result, err := analyzer.Analyze(context.Background(), mutations.AnalysisInput{File: domain.ChangedFile{Path: "x.go"}})
	if err != nil || len(result) != 0 {
		t.Fatalf("unexpected result %v %v", result, err)
	}
	bad := helperPath(t, "bad")
	_ = os.WriteFile(bad, []byte("#!/bin/sh\necho invalid\n"), 0700)
	invalid, err := mutations.NewExternalAnalyzer(mutations.ExternalAnalyzerConfig{Executable: bad, Allowlist: []string{bad}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = invalid.Analyze(context.Background(), mutations.AnalysisInput{}); err == nil {
		t.Fatal("expected invalid JSON error")
	}
	slow := helperPath(t, "slow")
	_ = os.WriteFile(slow, []byte("#!/bin/sh\nsleep 1\n"), 0700)
	timed, err := mutations.NewExternalAnalyzer(mutations.ExternalAnalyzerConfig{Executable: slow, Allowlist: []string{slow}, Timeout: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = timed.Analyze(context.Background(), mutations.AnalysisInput{}); err == nil {
		t.Fatal("expected timeout")
	}
}

func helperPath(t *testing.T, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return filepath.Join(t.TempDir(), name+".exe")
	}
	return filepath.Join(t.TempDir(), name)
}
