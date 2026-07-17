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
caller. Waivers require `updatedBy`, and invalid lifecycle transitions return a
conflict response.

## gRPC

`proto/merger/v1/controlplane.proto` defines `GetChangePacket`,
`ListChangePackets`, and `UpdateEvidenceExecution` on
`ChangeControlService`. gRPC requests use the same validation and list limits
as HTTP.

The services do not yet provide built-in API authentication. Deploy the control
plane behind an authenticated private ingress until transport authentication
and role-based authorization are configured.

Future API work includes packet filtering and pagination, immutable evidence
audit history, runtime graph queries, and reviewer routing.
