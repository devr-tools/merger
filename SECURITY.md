# Security Policy

## Reporting a vulnerability

We take the security of merger seriously. If you believe you have found a
security vulnerability, please report it privately — **do not open a public
issue, pull request, or discussion for security reports.**

Preferred channel: use GitHub's private vulnerability reporting for this
repository. Go to the **Security** tab → **Report a vulnerability**
(<https://github.com/devr-tools/merger/security/advisories/new>). This opens a
private advisory visible only to you and the maintainers.

If private reporting is unavailable to you, contact a maintainer directly rather
than disclosing publicly, and we will open a private advisory on your behalf.

Please include, where possible:

- a description of the vulnerability and its impact;
- the affected version and platform;
- step-by-step reproduction, ideally with a minimal repository or config; and
- any proof-of-concept, logs, or suggested remediation.

## Our commitment

- **Acknowledgement** within **3 business days** of your report.
- **Triage and severity assessment** (CVSS-based) within **10 business days**,
  including whether we accept the report and an initial remediation plan.
- **Progress updates** at least every **10 business days** until resolution.
- **Coordinated disclosure**: we aim to ship a fix and publish an advisory
  within **90 days** of triage, and will credit reporters who wish to be named.

Please give us a reasonable opportunity to remediate before any public
disclosure.

## Supported versions

merger is pre-1.0 and released from the latest tag. Security fixes are applied
to the most recent release only; please upgrade to the latest version before
reporting.

| Version | Supported |
| --- | --- |
| Latest release | ✅ |
| Older releases | ❌ |

## Scope

In scope: the merger ingest and control-plane services, the GitHub App webhook
verification path, the policy and lane-assignment engines, released container
images, and the release/supply-chain pipeline in this repository.

Because merger ingests untrusted pull-request content (diffs, file contents,
webhook payloads) and evaluates operator-supplied policy, reports that merger
executes attacker-controlled input, mishandles webhook signature verification,
reaches non-allowlisted network endpoints, leaks credentials, or escalates a
change out of its assigned merge lane are in scope and valued.

Out of scope: vulnerabilities in third-party dependencies that have no impact on
merger as shipped, and findings that require the operator to have explicitly
disabled a documented safety control.
