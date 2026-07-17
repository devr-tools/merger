package cli_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devr-tools/merger/internal/cli"
)

const authDiff = `diff --git a/internal/auth/session.go b/internal/auth/session.go
index 3333333..4444444 100644
--- a/internal/auth/session.go
+++ b/internal/auth/session.go
@@ -10,6 +10,9 @@ func Authenticate(token string) bool {
-	return legacyCheck(token)
+	if token == "" {
+		return false
+	}
+	return verify(token)
 }
`

// captureStdout redirects os.Stdout for the duration of fn and returns what was
// written. Tests here run sequentially, so the swap is safe.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()
	_ = w.Close()
	out, _ := io.ReadAll(r)
	return string(out)
}

func TestRunVersionPrintsVersion(t *testing.T) {
	var err error
	out := captureStdout(t, func() { err = cli.Run(context.Background(), []string{"version"}) })
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if !strings.Contains(out, "merger v") {
		t.Fatalf("expected version output, got %q", out)
	}
}

func TestRunUnknownCommandExitsTwo(t *testing.T) {
	err := cli.Run(context.Background(), []string{"nope"})
	assertExitCode(t, err, 2)
}

func TestInitThenValidate(t *testing.T) {
	dir := t.TempDir()

	var err error
	captureStdout(t, func() { err = cli.Run(context.Background(), []string{"init", "-dir", dir}) })
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".merger", "merger.yaml")); statErr != nil {
		t.Fatalf("expected .merger/merger.yaml: %v", statErr)
	}

	captureStdout(t, func() { err = cli.Run(context.Background(), []string{"validate", "-repo-root", dir}) })
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestInitTwiceWithoutForceFails(t *testing.T) {
	dir := t.TempDir()
	captureStdout(t, func() { _ = cli.Run(context.Background(), []string{"init", "-dir", dir}) })

	err := cli.Run(context.Background(), []string{"init", "-dir", dir})
	assertExitCode(t, err, 1)
}

func TestScanFailOnLaneTripsExitCode(t *testing.T) {
	dir := t.TempDir()
	captureStdout(t, func() { _ = cli.Run(context.Background(), []string{"init", "-dir", dir}) })

	diffPath := filepath.Join(dir, "change.diff")
	if err := os.WriteFile(diffPath, []byte(authDiff), 0o644); err != nil {
		t.Fatalf("write diff: %v", err)
	}

	var err error
	captureStdout(t, func() {
		err = cli.Run(context.Background(), []string{
			"scan", "-repo-root", dir, "-diff", diffPath, "-fail-on-lane", "RED",
		})
	})
	assertExitCode(t, err, 2)
}

func TestScanWithoutDiffSourceFails(t *testing.T) {
	err := cli.Run(context.Background(), []string{"scan", "-repo-root", t.TempDir()})
	assertExitCode(t, err, 2)
}

func TestScanExplainIncludesDecisionDetails(t *testing.T) {
	dir := t.TempDir()
	captureStdout(t, func() { _ = cli.Run(context.Background(), []string{"init", "-dir", dir}) })

	diffPath := filepath.Join(dir, "change.diff")
	if err := os.WriteFile(diffPath, []byte(authDiff), 0o644); err != nil {
		t.Fatalf("write diff: %v", err)
	}

	var err error
	out := captureStdout(t, func() {
		err = cli.Run(context.Background(), []string{
			"scan", "-repo-root", dir, "-diff", diffPath, "-explain",
		})
	})
	if err != nil {
		t.Fatalf("scan --explain: %v", err)
	}
	for _, want := range []string{"explanation:", "policy:", "risks:", "mitigate:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected explanation output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestScanWritesGitHubActionOutputs(t *testing.T) {
	dir := t.TempDir()
	captureStdout(t, func() { _ = cli.Run(context.Background(), []string{"init", "-dir", dir}) })

	diffPath := filepath.Join(dir, "change.diff")
	if err := os.WriteFile(diffPath, []byte(authDiff), 0o644); err != nil {
		t.Fatalf("write diff: %v", err)
	}
	outputPath := filepath.Join(dir, "github-output")

	var err error
	captureStdout(t, func() {
		err = cli.Run(context.Background(), []string{
			"scan", "-repo-root", dir, "-diff", diffPath, "-github-output", outputPath,
		})
	})
	if err != nil {
		t.Fatalf("scan with GitHub output: %v", err)
	}

	raw, readErr := os.ReadFile(outputPath)
	if readErr != nil {
		t.Fatalf("read GitHub output: %v", readErr)
	}
	output := string(raw)
	for _, want := range []string{"lane=RED", "risk-score=", "change-packet-id=cp_"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected GitHub output to contain %q, got:\n%s", want, output)
		}
	}
}

func assertExitCode(t *testing.T, err error, want int) {
	t.Helper()
	var exit cli.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected cli.ExitError, got %v", err)
	}
	if exit.Code != want {
		t.Fatalf("expected exit code %d, got %d (%s)", want, exit.Code, exit.Message)
	}
}
