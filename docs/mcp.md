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

## Example session

```jsonl
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"merger_scan","arguments":{"diff":"<unified diff>"}}}
```

Ordinary failures (bad config path, unparseable diff) come back as `isError`
tool results the model can read and self-correct from; only an unknown tool or a
handler panic is surfaced as a JSON-RPC protocol error.
