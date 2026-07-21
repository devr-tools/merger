package action_test

import (
	"os"
	"strings"
	"testing"
)

func TestMergeQueueGateExampleUsesGeneratedMergeCommitAndStableCheckName(t *testing.T) {
	contents, err := os.ReadFile("../../docs/examples/github-merge-queue-gate.yml")
	if err != nil {
		t.Fatalf("read merge queue example: %v", err)
	}
	workflow := string(contents)
	for _, expected := range []string{
		"merge_group:",
		"name: Merger change control",
		"github.event.merge_group.head_sha",
		"github.event.merge_group.base_sha",
		"fetch-depth: 0",
		"fail-on-lane: RED",
	} {
		if !strings.Contains(workflow, expected) {
			t.Fatalf("merge queue example must contain %q", expected)
		}
	}
}
