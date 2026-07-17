# API Surface

The control plane exposes matching HTTP and gRPC APIs for querying Change
Packets and reporting evidence execution results.

## HTTP

- `GET /api/v1/change-packets?limit=50` lists the most recently updated packets.
- `GET /api/v1/change-packets/{id}` returns a packet and its evidence executions.
- `PUT /api/v1/change-packets/{id}/evidence/{name}` reports an evidence status.

List limits are capped at 200. Evidence request bodies are capped at 64 KiB and
must contain one JSON object.

An evidence update accepts `pending`, `running`, `satisfied`, `failed`, or
`waived`. The evidence name must already be required by the Change Packet;
`type` and `required` are derived from policy rather than trusted from the
caller. The authenticated principal becomes `updatedBy`, and invalid lifecycle
transitions return a conflict response.

After an update, the control plane reconciles outstanding required evidence and
mandatory reviewers into the packet decision, recomputes its lane, persists the
result, and refreshes the GitHub Check with current evidence statuses. Blocked
policy decisions remain blocked.

## gRPC

`proto/merger/v1/controlplane.proto` defines `GetChangePacket`,
`ListChangePackets`, and `UpdateEvidenceExecution` on
`ChangeControlService`. gRPC requests use the same validation and list limits
as HTTP.

## Authentication and roles

Configure either environment-backed bearer tokens or signed JWTs.

Static tokens keep token values out of YAML:

```yaml
access:
  mode: static_token
  tokens:
    - subject: dashboard
      token_env: MERGER_DASHBOARD_TOKEN
      roles: [reader]
    - subject: ci
      token_env: MERGER_CI_TOKEN
      roles: [evidence_writer]
    - subject: operator
      token_env: MERGER_OPERATOR_TOKEN
      roles: [admin]
```

HTTP clients send `Authorization: Bearer <token>`. gRPC clients send the same
value in `authorization` metadata. `reader` permits Change Packet queries,
`evidence_writer` permits evidence updates, and `admin` permits both. The
`/healthz` HTTP endpoint remains public.

JWT mode verifies signed bearer tokens from an upstream identity provider or
auth gateway. Merger validates issuer, audience, expiry, and signature, then
maps configured claim values to Merger roles:

```yaml
access:
  mode: jwt
  jwt:
    algorithm: RS256
    issuer: https://auth.example.com
    audience: merger-controlplane
    public_key_path: ./secrets/controlplane-jwt-public.pem
    roles_claim: groups
    role_bindings:
      - claim_value: merger.read
        roles: [reader]
      - claim_value: merger.write
        roles: [evidence_writer]
      - claim_value: merger.admin
        roles: [admin]
```

For HMAC-signed tokens, replace `public_key_path` with `secret_env`. If
`subject_claim` or `roles_claim` are omitted, Merger defaults them to `sub`
and `roles`.

`access.mode: disabled` supplies a local administrator for development. Service
startup rejects disabled access when `telemetry.environment` is `prod` or
`production`.

Future API work includes packet filtering and pagination, immutable evidence
audit history, runtime graph queries, and reviewer routing.
