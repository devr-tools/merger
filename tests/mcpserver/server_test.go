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

func TestAgentWorkflowToolsExplainPlanAndCheckReadiness(t *testing.T) {
	packet := map[string]any{
		"id": "cp_agent", "mergeLane": "RED",
		"decision":    map[string]any{"status": "approved", "summary": "policy requirements recorded"},
		"riskSummary": map[string]any{"score": 72, "severity": "high", "contributors": []string{"security"}},
		"mutations":   []map[string]any{{"kind": "auth_behavior_change", "severity": "high", "title": "Authentication behavior changed"}},
		"risks":       []map[string]any{{"type": "security", "severity": "high", "summary": "Authentication surface changed", "mitigations": []string{"Run authentication tests"}}},
		"evidence":    []map[string]any{{"name": "auth tests", "type": "auth_integration_tests", "required": true, "reason": "authentication changed", "githubCheck": map[string]any{"name": "CI / auth", "appId": 42}}},
		"reviewers":   []map[string]any{{"team": "security", "mandatory": true, "reason": "auth review"}},
	}
	packetJSON, err := json.Marshal(packet)
	if err != nil {
		t.Fatalf("marshal packet: %v", err)
	}
	lines := drive(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"merger_explain","arguments":{"change_packet":`+string(packetJSON)+`}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"merger_plan_evidence","arguments":{"change_packet":`+string(packetJSON)+`}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"merger_check_readiness","arguments":{"change_packet":`+string(packetJSON)+`}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"merger_check_readiness","arguments":{"change_packet":`+string(packetJSON)+`,"completed_evidence":["auth tests"],"completed_reviews":["security"]}}}`,
	)
	joined := strings.Join(lines, "\n")
	for _, tool := range []string{"merger_explain", "merger_plan_evidence", "merger_check_readiness"} {
		if !strings.Contains(joined, tool) {
			t.Fatalf("expected %s in tools/list, got:\n%s", tool, joined)
		}
	}
	if !strings.Contains(joined, "trustedGitHubCheck") {
		t.Fatalf("expected trusted check in evidence plan, got:\n%s", joined)
	}
	if !strings.Contains(joined, "required evidence not verified: auth tests") {
		t.Fatalf("expected unverified evidence blocker, got:\n%s", joined)
	}
	if !strings.Contains(joined, `\"ready\": true`) {
		t.Fatalf("expected readiness after verified inputs, got:\n%s", joined)
	}
}
