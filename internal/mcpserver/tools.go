package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/resolve"
	"github.com/devr-tools/merger/internal/scan"
)

// ErrUnknownTool is returned by Call for a tool name the server does not expose.
var ErrUnknownTool = errors.New("unknown tool")

// Content is a single MCP tool-result content block.
type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Result is an MCP tool-call result.
type Result struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// Definition describes a tool for tools/list.
type Definition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Definitions returns the tools this server exposes.
func Definitions() []Definition {
	return []Definition{
		{
			Name:        "merger_scan",
			Description: "Analyze a unified diff and return the resulting Change Packet as JSON: detected mutations, runtime impact, risk score, policy decision, and assigned merge lane.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"diff":      map[string]any{"type": "string", "description": "Raw unified diff to analyze (as produced by `git diff`)."},
					"repo_root": map[string]any{"type": "string", "description": "Repository root for file-content lookups and relative config paths. Defaults to the current directory."},
					"repo":      map[string]any{"type": "string", "description": "Repository identifier as owner/name (optional)."},
					"ref":       map[string]any{"type": "string", "description": "Revision the diff targets (optional)."},
					"config":    map[string]any{"type": "string", "description": "Path to a merger config file or directory (optional; auto-discovered otherwise)."},
					"policy":    map[string]any{"type": "string", "description": "Path to a policy file (optional; defaults to the config's policy path)."},
				},
				"required": []string{"diff"},
			},
		},
		{
			Name:        "merger_validate",
			Description: "Validate a merger configuration and its policy file, reporting the resolved config path, policy rule count, and lane thresholds.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"repo_root": map[string]any{"type": "string", "description": "Repository root to resolve config and policy against. Defaults to the current directory."},
					"config":    map[string]any{"type": "string", "description": "Path to a merger config file or directory (optional; auto-discovered otherwise)."},
					"policy":    map[string]any{"type": "string", "description": "Path to a policy file (optional; defaults to the config's policy path)."},
				},
			},
		},
		{
			Name:        "merger_explain",
			Description: "Explain a Change Packet in agent-oriented terms: why its lane and decision were assigned, what changed, and the concrete risks and mitigations to address. Pass the Change Packet returned by merger_scan.",
			InputSchema: packetInputSchema(),
		},
		{
			Name:        "merger_plan_evidence",
			Description: "Turn a Change Packet's required evidence into a concrete, ordered checklist for an agent. Includes trusted GitHub check bindings when configured. Pass the Change Packet returned by merger_scan.",
			InputSchema: packetInputSchema(),
		},
		{
			Name:        "merger_check_readiness",
			Description: "Assess whether a Change Packet is ready to merge using the evidence and reviews an agent reports as complete. Reports exact blockers without claiming unreported checks or reviews passed. Pass the Change Packet returned by merger_scan.",
			InputSchema: readinessInputSchema(),
		},
	}
}

func packetInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"change_packet": map[string]any{"type": "object", "description": "Change Packet JSON returned by merger_scan."},
		},
		"required": []string{"change_packet"},
	}
}

func readinessInputSchema() map[string]any {
	schema := packetInputSchema()
	properties := schema["properties"].(map[string]any)
	properties["completed_evidence"] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Names of evidence requirements completed for this exact Change Packet. Only use verified results."}
	properties["completed_reviews"] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Teams that have supplied the required review. Only use completed approvals."}
	return schema
}

// Call dispatches a tool by name.
func Call(ctx context.Context, name string, args map[string]any) (Result, error) {
	switch name {
	case "merger_scan":
		return toolScan(ctx, args)
	case "merger_validate":
		return toolValidate(args)
	case "merger_explain":
		return toolExplain(args)
	case "merger_plan_evidence":
		return toolPlanEvidence(args)
	case "merger_check_readiness":
		return toolCheckReadiness(args)
	default:
		return Result{}, fmt.Errorf("%w: %s", ErrUnknownTool, name)
	}
}

// toolExplain deliberately returns structured JSON rather than prose alone so
// agents can turn the explanation into a patch or a review plan reliably.
func toolExplain(args map[string]any) (Result, error) {
	packet, err := changePacketArg(args)
	if err != nil {
		return Result{}, err
	}

	mutations := make([]map[string]any, 0, len(packet.Mutations))
	for _, mutation := range packet.Mutations {
		mutations = append(mutations, map[string]any{"kind": mutation.Kind, "severity": mutation.Severity, "title": mutation.Title, "files": mutation.Files})
	}
	risks := make([]map[string]any, 0, len(packet.Risks))
	for _, risk := range packet.Risks {
		risks = append(risks, map[string]any{"type": risk.Type, "severity": risk.Severity, "summary": risk.Summary, "reason": risk.Reason, "mitigations": risk.Mitigations})
	}

	return jsonResult(map[string]any{
		"changePacketId":  packet.ID,
		"mergeLane":       packet.MergeLane,
		"decision":        packet.Decision.Status,
		"decisionSummary": packet.Decision.Summary,
		"decisionReasons": packet.Decision.Reasons,
		"risk": map[string]any{
			"score": packet.RiskSummary.Score, "severity": packet.RiskSummary.Severity, "contributors": packet.RiskSummary.Contributors,
		},
		"mutations":        mutations,
		"risks":            risks,
		"runtimeImpact":    packet.Runtime,
		"nextAction":       nextAction(packet),
		"evidenceRequired": requiredEvidenceCount(packet),
	})
}

func toolPlanEvidence(args map[string]any) (Result, error) {
	packet, err := changePacketArg(args)
	if err != nil {
		return Result{}, err
	}

	steps := make([]map[string]any, 0, len(packet.Evidence))
	for _, evidence := range packet.Evidence {
		if !evidence.Required {
			continue
		}
		step := map[string]any{
			"name": evidence.Name, "type": evidence.Type, "reason": evidence.Reason,
			"producer": evidence.Producer, "action": "run, attach, or explicitly waive this evidence for the exact Change Packet",
		}
		if evidence.GitHubCheck != nil {
			step["trustedGitHubCheck"] = evidence.GitHubCheck
			step["action"] = "wait for the named trusted GitHub check on this Change Packet's head SHA"
		}
		steps = append(steps, step)
	}

	return jsonResult(map[string]any{
		"changePacketId":  packet.ID,
		"mergeLane":       packet.MergeLane,
		"decision":        packet.Decision.Status,
		"steps":           steps,
		"deployment":      packet.Deployment,
		"requiredReviews": requiredReviews(packet),
		"note":            "This plan describes requirements; use merger_check_readiness only with verified evidence and completed reviews.",
	})
}

func toolCheckReadiness(args map[string]any) (Result, error) {
	packet, err := changePacketArg(args)
	if err != nil {
		return Result{}, err
	}
	completedEvidence := stringSet(args, "completed_evidence")
	completedReviews := stringSet(args, "completed_reviews")
	blockers := make([]string, 0)
	if packet.Decision.Status != domain.DecisionApproved {
		blockers = append(blockers, fmt.Sprintf("policy decision is %q", packet.Decision.Status))
	}
	for _, evidence := range packet.Evidence {
		if evidence.Required && !completedEvidence[evidence.Name] {
			blockers = append(blockers, "required evidence not verified: "+evidence.Name)
		}
	}
	for _, review := range packet.Reviewers {
		if review.Mandatory && !completedReviews[review.Team] {
			blockers = append(blockers, "mandatory review not verified: "+review.Team)
		}
	}
	if packet.MergeLane == domain.MergeLaneBlack {
		blockers = append(blockers, "BLACK lane changes require escalation before merge")
	}
	next := nextAction(packet)
	if len(blockers) == 0 {
		next = "the reported policy decision, required evidence, and mandatory reviews are ready for merge"
	}

	return jsonResult(map[string]any{
		"changePacketId": packet.ID,
		"ready":          len(blockers) == 0,
		"mergeLane":      packet.MergeLane,
		"decision":       packet.Decision.Status,
		"blockers":       blockers,
		"nextAction":     next,
		"safetyNote":     "Merger treats only explicitly reported completed evidence and reviews as verified in this offline MCP workflow.",
	})
}

func changePacketArg(args map[string]any) (domain.ChangePacket, error) {
	raw, ok := args["change_packet"]
	if !ok || raw == nil {
		return domain.ChangePacket{}, errors.New("change_packet is required")
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return domain.ChangePacket{}, fmt.Errorf("encode change_packet: %w", err)
	}
	var packet domain.ChangePacket
	if err := json.Unmarshal(data, &packet); err != nil {
		return domain.ChangePacket{}, fmt.Errorf("decode change_packet: %w", err)
	}
	return packet, nil
}

func jsonResult(value any) (Result, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return Result{}, err
	}
	return Result{Content: []Content{{Type: "text", Text: string(data)}}}, nil
}

func stringSet(args map[string]any, key string) map[string]bool {
	values := map[string]bool{}
	raw, ok := args[key].([]any)
	if !ok {
		return values
	}
	for _, item := range raw {
		if value, ok := item.(string); ok && value != "" {
			values[value] = true
		}
	}
	return values
}

func requiredEvidenceCount(packet domain.ChangePacket) int {
	count := 0
	for _, evidence := range packet.Evidence {
		if evidence.Required {
			count++
		}
	}
	return count
}

func requiredReviews(packet domain.ChangePacket) []domain.ReviewerRequirement {
	reviews := make([]domain.ReviewerRequirement, 0, len(packet.Reviewers))
	for _, review := range packet.Reviewers {
		if review.Mandatory {
			reviews = append(reviews, review)
		}
	}
	return reviews
}

func nextAction(packet domain.ChangePacket) string {
	if packet.Decision.Status != domain.DecisionApproved {
		return "resolve the policy decision and its reasons before merge"
	}
	if requiredEvidenceCount(packet) > 0 {
		return "complete the required evidence plan and verify its results"
	}
	if len(requiredReviews(packet)) > 0 {
		return "obtain the mandatory reviews"
	}
	if packet.MergeLane == domain.MergeLaneBlack {
		return "escalate the BLACK-lane change for explicit approval"
	}
	return "the packet has no unreported policy, evidence, or review requirements"
}

func toolScan(ctx context.Context, args map[string]any) (Result, error) {
	root := stringArg(args, "repo_root")
	if root == "" {
		root = "."
	}

	options, _, err := resolve.ScanOptions(
		root,
		stringArg(args, "config"),
		stringArg(args, "policy"),
		stringArg(args, "repo"),
		stringArg(args, "ref"),
		stringArg(args, "diff"),
	)
	if err != nil {
		return Result{}, err
	}

	packet, err := scan.Run(ctx, options)
	if err != nil {
		return Result{}, err
	}

	data, err := json.MarshalIndent(packet, "", "  ")
	if err != nil {
		return Result{}, err
	}
	return Result{Content: []Content{{Type: "text", Text: string(data)}}}, nil
}

func toolValidate(args map[string]any) (Result, error) {
	root := stringArg(args, "repo_root")
	if root == "" {
		root = "."
	}

	cfg, cfgPath, err := resolve.Config(root, stringArg(args, "config"))
	if err != nil {
		return Result{}, err
	}
	policyConfig, policyPath, found, err := resolve.Policy(root, stringArg(args, "policy"), cfg)
	if err != nil {
		return Result{}, err
	}
	if !found {
		return Result{
			Content: []Content{{Type: "text", Text: "no policy file found; set policy.path in the config or pass a policy path"}},
			IsError: true,
		}, nil
	}

	var b strings.Builder
	if cfgPath == "" {
		b.WriteString("config: (none found — using defaults)\n")
	} else {
		fmt.Fprintf(&b, "config: %s\n", cfgPath)
	}
	fmt.Fprintf(&b, "policy: %s (%d rule(s))\n", policyPath, len(policyConfig.Policies))
	fmt.Fprintf(&b, "lanes:  green<=%d yellow<=%d red<=%d\n", cfg.Lanes.GreenMax, cfg.Lanes.YellowMax, cfg.Lanes.RedMax)
	b.WriteString("ok")

	return Result{Content: []Content{{Type: "text", Text: b.String()}}}, nil
}

func stringArg(args map[string]any, key string) string {
	if raw, ok := args[key]; ok {
		if s, ok := raw.(string); ok {
			return s
		}
	}
	return ""
}
