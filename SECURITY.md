English · [한국어](SECURITY.ko.md)

# Security Policy

pqcaton is a governance tool that **reconciles an organization's cryptographic asset inventory with its declaration and applies only
the decisions people have finalized**. The runner works **inside the customer's servers with the customer's permissions**. What it
handles is which node uses what, which is the organization's attack surface itself. So we take security reports seriously.

## Supported versions

| Version | Security fixes |
|---|---|
| **Latest release** · `main` | ✅ |
| Earlier releases | **Not backported** |

**Fixes go into `main` and ship with the next release.** Older versions get no separate patches. This is a one-person project, and
keeping several lines alive means none of them gets fixed in time. **To receive a security fix, upgrade to the latest release.**

Each release converts to Apache-2.0 four years later (→ [LICENSING](LICENSING.md)), but that **opens the license; it does not mean
that version keeps being fixed.**

## Reporting a vulnerability

**Please do not post it in a public issue, PR or discussion.** Tell us through one of these private routes:

1. **GitHub private reporting (preferred)**: this repository's **Security tab → "Report a vulnerability"**.
2. If that is not possible, write to **kty@sntsoft.co.kr**. A subject starting with `[security]` is noticed sooner.

The affected component (runner / reconciliation engine / CLI), steps to reproduce, the impact (information disclosure, privilege
escalation, integrity damage) and a PoC if possible make triage faster.

> **Things that are not vulnerabilities belong in public issues.** Bugs, questions and suggestions have no reason to be hidden, and a
> public discussion stays for the next person. We take privately only **what could get users attacked if it became known before it
> is fixed**.

## Response

This is a one-person project, so it is **best-effort**. We acknowledge the report, reproduce and assess it, then agree on a fix plan with
the reporter. There is no SLA, but valid reports come first.

## What this project treats as a security issue

What the runner sends and does not send is in a table in [saas/runner/README.md](saas/runner/README.md). **Any path that breaks that
table is a vulnerability.**

- **Runner token leak**: the token leaves only in a header and is never put in a URL, an error message or a log. A leak anywhere is a
  vulnerability. One token is one organization, so a leak opens that whole organization.
- **Address leak**: registration turns a connection address into an HMAC token with the organization key and **discards the
  original.** If the original address or any value usable to connect leaves or stays somewhere, it is a vulnerability.
- **Breaking organization isolation**: a path that lets one organization's runner read or write another organization's data.
- **Bypassing the decision gate**: a decision that is not `finalized` going out in a plan, accepting forged or altered entries in the
  judgment ledger (append-only), or an old decision staying valid after its evidence changed.
- **Bypassing the screen's request protection**: the screen (`pqcaton-ui`) has no login; instead it opens only on loopback and checks
  the Host of every request and the Origin and CSRF token of changing requests. A path that lets another origin's page reach saving or
  finalizing, opens the screen beyond loopback, or reads the review queue under another name belongs here.
- **Injection into generated output**: a path through which observation or declaration input flows as malicious content into the
  playbooks or config of the base project pqcota.
- **Leaking the pqcota connection**: `PQCOTA_DSN` (or `pqcota.env`) appearing in the workspace, processing records, logs, error
  messages, the screen or the environment of the processes the screen starts.

## Out of scope

- Problems in **the base project pqcota** (collectors, CLI, contracts) follow
  [pqcota's security policy](https://github.com/randyinthedev-hash/pqcota/blob/main/SECURITY.md).
- The **execution environment** of the playbooks the operator runs (permissions, network configuration) is the responsibility of the
  side that runs them.

If the scope is unclear, do not decide for yourself; report it and we will discuss it together.
