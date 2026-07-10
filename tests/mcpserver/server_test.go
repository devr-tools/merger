package mcpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devr-tools/merger/internal/mcpserver"
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

// drive feeds newline-delimited JSON-RPC requests through a fresh server and
// returns the emitted response lines.
func drive(t *testing.T, requests ...string) []string {
	t.Helper()
	in := bytes.NewBufferString(strings.Join(requests, "\n") + "\n")
	var out bytes.Buffer
	if err := mcpserver.New().Serve(context.Background(), in, &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	return lines
}

func TestInitializeAdvertisesServerInfo(t *testing.T) {
	lines := drive(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
	if len(lines) != 1 {
		t.Fatalf("expected 1 response, got %d: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], `"name":"merger"`) {
		t.Fatalf("expected serverInfo name merger, got %s", lines[0])
	}
}

func TestToolsListRequiresInitialize(t *testing.T) {
	lines := drive(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if !strings.Contains(lines[0], "not initialized") {
		t.Fatalf("expected not-initialized error before initialize, got %s", lines[0])
	}
}

func TestToolsListAndScanCall(t *testing.T) {
	args, err := json.Marshal(map[string]any{"diff": authDiff})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	lines := drive(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"merger_scan","arguments":`+string(args)+`}}`,
	)

	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "merger_scan") {
		t.Fatalf("expected merger_scan in tools/list, got:\n%s", joined)
	}
	if !strings.Contains(joined, "mergeLane") {
		t.Fatalf("expected a Change Packet with mergeLane in the scan result, got:\n%s", joined)
	}
}

func TestUnknownToolIsInvalidParams(t *testing.T) {
	lines := drive(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"does_not_exist","arguments":{}}}`,
	)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "unknown tool") {
		t.Fatalf("expected unknown tool error, got:\n%s", joined)
	}
}
