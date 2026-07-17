package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/resolve"
	"github.com/devr-tools/merger/internal/scan"
)

var laneRank = map[domain.MergeLane]int{
	domain.MergeLaneGreen:  1,
	domain.MergeLaneYellow: 2,
	domain.MergeLaneRed:    3,
	domain.MergeLaneBlack:  4,
}

func runScan(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	root := fs.String("repo-root", ".", "repository root for content lookups and relative paths")
	configPath := fs.String("config", "", "path to a merger config file or directory (default: auto-discover)")
	policyPath := fs.String("policy", "", "path to a policy file (default: config's policy.path)")
	diffPath := fs.String("diff", "", "unified diff file to scan, or '-' for stdin")
	baseRef := fs.String("base-ref", "", "git base ref; scans `git diff <base-ref>...HEAD` when -diff is unset")
	format := fs.String("format", "text", "output format: text or json")
	explain := fs.Bool("explain", false, "include risk contributors, policy rationale, runtime details, and mitigations in text output")
	githubOutput := fs.String("github-output", "", "append lane, risk-score, and change-packet-id outputs to a GitHub Actions output file")
	failOn := fs.String("fail-on-lane", "", "exit non-zero when the assigned lane is at or above this lane (GREEN|YELLOW|RED|BLACK)")
	repo := fs.String("repo", "", "repository identifier as owner/name (optional)")
	ref := fs.String("ref", "", "revision the diff targets (optional)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if err := validateScanFormat(*format); err != nil {
		return err
	}
	failLane, err := parseFailLane(*failOn)
	if err != nil {
		return ExitError{Code: 2, Message: err.Error()}
	}

	rawDiff, err := readDiff(*root, *diffPath, *baseRef)
	if err != nil {
		return err
	}

	options, policyFound, err := resolve.ScanOptions(*root, *configPath, *policyPath, *repo, *ref, rawDiff)
	if err != nil {
		return err
	}
	if !policyFound && *format == "text" {
		fmt.Fprintln(os.Stderr, "merger: no policy file found — scanning with an empty rule set")
	}

	packet, err := scan.Run(ctx, options)
	if err != nil {
		return err
	}

	if err := writeScanOutput(*format, *explain, *githubOutput, packet); err != nil {
		return err
	}
	return enforceFailLane(failLane, packet)
}

func validateScanFormat(format string) error {
	if format != "text" && format != "json" {
		return ExitError{Code: 2, Message: fmt.Sprintf("unknown format %q (want text or json)", format)}
	}
	return nil
}

func writeScanOutput(format string, explain bool, githubOutput string, packet *domain.ChangePacket) error {
	if format == "json" {
		if err := writeJSON(os.Stdout, packet); err != nil {
			return err
		}
	} else {
		writeTextReport(os.Stdout, packet, explain)
	}
	if githubOutput != "" {
		return writeGitHubOutput(githubOutput, packet)
	}
	return nil
}

func enforceFailLane(failLane domain.MergeLane, packet *domain.ChangePacket) error {
	if failLane != "" && laneRank[packet.MergeLane] >= laneRank[failLane] {
		return ExitError{Code: 2, Message: fmt.Sprintf("merge lane %s is at or above the -fail-on-lane threshold %s", packet.MergeLane, failLane)}
	}
	return nil
}

func writeGitHubOutput(path string, packet *domain.ChangePacket) error {
	output, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open GitHub output file: %w", err)
	}
	defer output.Close()

	if _, err := fmt.Fprintf(output, "lane=%s\nrisk-score=%d\nchange-packet-id=%s\n", packet.MergeLane, packet.RiskSummary.Score, packet.ID); err != nil {
		return fmt.Errorf("write GitHub outputs: %w", err)
	}
	return nil
}

func parseFailLane(raw string) (domain.MergeLane, error) {
	if raw == "" {
		return "", nil
	}
	lane := domain.MergeLane(strings.ToUpper(raw))
	if _, ok := laneRank[lane]; !ok {
		return "", fmt.Errorf("invalid -fail-on-lane %q (want GREEN, YELLOW, RED, or BLACK)", raw)
	}
	return lane, nil
}

func readDiff(root, diffPath, baseRef string) (string, error) {
	switch {
	case diffPath == "-":
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read diff from stdin: %w", err)
		}
		return string(data), nil
	case diffPath != "":
		data, err := os.ReadFile(diffPath)
		if err != nil {
			return "", fmt.Errorf("read diff file: %w", err)
		}
		return string(data), nil
	case baseRef != "":
		cmd := exec.Command("git", "-C", root, "diff", "--no-color", baseRef+"...HEAD")
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("git diff %s...HEAD: %w", baseRef, err)
		}
		return string(out), nil
	default:
		return "", ExitError{Code: 2, Message: "provide a diff with -diff <file|-> or a range with -base-ref <ref>"}
	}
}

func writeJSON(w io.Writer, payload any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}

func writeTextReport(w io.Writer, packet *domain.ChangePacket, explain bool) {
	repo := packet.Repo.FullName
	if repo == "" {
		repo = "-"
	}
	fmt.Fprintf(w, "merger scan\n")
	fmt.Fprintf(w, "repo:       %s\n", repo)
	fmt.Fprintf(w, "files:      %d changed\n", len(packet.Files))

	fmt.Fprintf(w, "mutations:  %d\n", len(packet.Mutations))
	for _, mutation := range packet.Mutations {
		fmt.Fprintf(w, "  - [%s] %s: %s (%s)\n", mutation.Severity, mutation.Kind, mutation.Title, mutation.Detector)
	}

	fmt.Fprintf(w, "risk:       score %d (%s)\n", packet.RiskSummary.Score, packet.RiskSummary.Severity)
	fmt.Fprintf(w, "runtime:    blast radius %s, criticality %s\n", packet.Runtime.BlastRadius, packet.Runtime.Criticality)
	fmt.Fprintf(w, "decision:   %s\n", packet.Decision.Status)

	if len(packet.Reviewers) > 0 {
		labels := make([]string, 0, len(packet.Reviewers))
		for _, reviewer := range packet.Reviewers {
			label := reviewer.Team
			if reviewer.Mandatory {
				label += " (mandatory)"
			}
			labels = append(labels, label)
		}
		fmt.Fprintf(w, "reviewers:  %s\n", strings.Join(labels, ", "))
	}

	if len(packet.Evidence) > 0 {
		labels := make([]string, 0, len(packet.Evidence))
		for _, evidence := range packet.Evidence {
			label := string(evidence.Type)
			if evidence.Required {
				label += " (required)"
			}
			labels = append(labels, label)
		}
		fmt.Fprintf(w, "evidence:   %s\n", strings.Join(labels, ", "))
	}

	deployment := string(packet.Deployment.Strategy)
	if packet.Deployment.RequiresCanary {
		deployment += " (canary required)"
	}
	fmt.Fprintf(w, "deployment: %s\n", deployment)
	fmt.Fprintf(w, "merge lane: %s\n", packet.MergeLane)

	if explain {
		writeExplanation(w, packet)
	}
}

func writeExplanation(w io.Writer, packet *domain.ChangePacket) {
	fmt.Fprintln(w, "\nexplanation:")
	if packet.Decision.Summary != "" {
		fmt.Fprintf(w, "  policy: %s\n", packet.Decision.Summary)
	}
	for _, reason := range packet.Decision.Reasons {
		fmt.Fprintf(w, "  - policy reason: %s\n", reason)
	}
	for _, violation := range packet.Decision.Violations {
		fmt.Fprintf(w, "  - violation [%s] %s: %s\n", violation.Severity, violation.Policy, violation.Reason)
	}

	if len(packet.Risks) == 0 {
		fmt.Fprintln(w, "  risks: no scored risk contributors")
	} else {
		fmt.Fprintln(w, "  risks:")
		for _, risk := range packet.Risks {
			fmt.Fprintf(w, "    - [%s] %s +%d: %s\n", risk.Severity, risk.Type, risk.Score, risk.Summary)
			if risk.Reason != "" {
				fmt.Fprintf(w, "      reason: %s\n", risk.Reason)
			}
			for _, mitigation := range risk.Mitigations {
				fmt.Fprintf(w, "      mitigate: %s\n", mitigation)
			}
		}
	}

	if len(packet.Runtime.Services) > 0 {
		fmt.Fprintln(w, "  affected services:")
		for _, service := range packet.Runtime.Services {
			fmt.Fprintf(w, "    - %s (%s, criticality=%s)\n", service.Name, service.Kind, service.Criticality)
		}
	}
	for _, note := range packet.Runtime.Notes {
		fmt.Fprintf(w, "  - runtime note: %s\n", note)
	}
}
