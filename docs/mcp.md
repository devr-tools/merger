# MCP Server

merger ships a minimal [Model Context Protocol](https://modelcontextprotocol.io)
server over stdio, so an agent can run the offline analysis pipeline as tools.

```bash
merger mcp
```

The server speaks newline-delimited JSON-RPC 2.0 on stdin/stdout. Point an MCP
client at the `merger mcp` command as a stdio server.

## Tools

### `merger_scan`

Analyze a unified diff and return the resulting Change Packet as JSON —
mutations, runtime impact, risk score, policy decision, and assigned merge lane.

| Argument | Required | Description |
| --- | --- | --- |
| `diff` | yes | Raw unified diff (as produced by `git diff`) |
| `repo_root` | no | Root for content lookups and relative config paths (default `.`) |
| `repo` | no | Repository identifier as `owner/name` |
| `ref` | no | Revision the diff targets |
| `config` | no | Config file or directory (auto-discovered otherwise) |
| `policy` | no | Policy file (defaults to the config's policy path) |

### `merger_validate`

Validate a repository's merger config and policy, reporting the resolved config
path, policy rule count, and lane thresholds.

| Argument | Required | Description |
| --- | --- | --- |
| `repo_root` | no | Root to resolve config and policy against (default `.`) |
| `config` | no | Config file or directory (auto-discovered otherwise) |
| `policy` | no | Policy file (defaults to the config's policy path) |

## Agent workflow tools

First call `merger_scan`, then pass the returned Change Packet as
`change_packet` to the following tools. This keeps an agent's explanation and
readiness decision tied to the same analyzed change rather than asking it to
reconstruct policy from prose.

### `merger_explain`

Explain why a Change Packet received its decision and lane. The structured
result includes detected mutations, risk contributors and mitigations, runtime
impact, outstanding evidence count, and the most useful next action.

| Argument | Required | Description |
| --- | --- | --- |
| `change_packet` | yes | Change Packet JSON returned by `merger_scan` |

### `merger_plan_evidence`

Produce an ordered, agent-actionable checklist of the packet's required
evidence and mandatory reviews. When a policy binds evidence to a GitHub check,
the result identifies that exact trusted check and App ID.

| Argument | Required | Description |
| --- | --- | --- |
| `change_packet` | yes | Change Packet JSON returned by `merger_scan` |

### `merger_check_readiness`

Determine whether an agent may represent a packet as ready to merge. Pass only
evidence and reviews that have been verified for this exact Change Packet; the
tool never infers successful checks or approvals. It returns `ready: false`
with the precise blockers until the policy decision, required evidence,
mandatory reviews, and any BLACK-lane escalation are resolved.

| Argument | Required | Description |
| --- | --- | --- |
| `change_packet` | yes | Change Packet JSON returned by `merger_scan` |
| `completed_evidence` | no | Verified evidence requirement names |
| `completed_reviews` | no | Teams that supplied verified mandatory review |

## Example session

```jsonl
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"merger_scan","arguments":{"diff":"<unified diff>"}}}
{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"merger_explain","arguments":{"change_packet":{"id":"cp_..."}}}}
```

Ordinary failures (bad config path, unparseable diff) come back as `isError`
tool results the model can read and self-correct from; only an unknown tool or a
handler panic is surfaced as a JSON-RPC protocol error.
