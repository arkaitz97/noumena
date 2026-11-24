# Security policy

## Reporting a vulnerability

Report vulnerabilities privately through GitHub Security Advisories:

https://github.com/arkaitz97/noumena/security/advisories/new

Do not open a public issue, pull request, or discussion for a vulnerability.
Do not include exploit details in a public commit message.

Include:

- The affected version or commit.
- The endpoint, file, or component.
- Reproduction steps and the impact you observed.
- Whether the issue is already public.

## What to expect

This is a personal project with no on-call rotation. Reports are handled on a
best-effort basis. You will get an acknowledgement when the report is read,
and a resolution or a reasoned rejection after triage. There is no bounty.

## Supported versions

Only the `main` branch is supported. There are no released version branches.

## Scope

In scope:

- Authentication and session handling (`internal/auth`).
- Tenant isolation: reading comments or channels without access
  (`internal/db/comments_feed.go`, `internal/api/workspace_middleware.go`).
- Stripe webhook signature verification and replay
  (`internal/api/billing_handlers.go`).
- OAuth token storage and encryption (`internal/auth/encryption.go`).
- Account deletion and data export (`internal/lifecycle`, `internal/export`).
- Injection in SQL, prompts, or templates.
- Secret leakage in the repository or its history.

Out of scope:

- Findings that require a compromised host, a stolen database file, or
  physical access.
- Denial of service by volume against a self-hosted instance.
- Missing hardening headers on a deployment that terminates TLS incorrectly.
- Reports produced only by a scanner, without a reproduction.

## Safe harbour

Good-faith research on your own instance is welcome. Do not test against
another person's deployment. Do not access, modify, or exfiltrate data that is
not yours. Stop and report if you reach data that belongs to another user.

## Secret handling

The repository must never contain a live credential. If you find one:

1. Report it privately as above.
2. Do not paste the value in the report. Name the file and the commit.
3. Assume the credential is compromised and rotate it.
