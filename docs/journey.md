English · [한국어](journey.ko.md)

# Journey

> **This English page is a summary, not a full translation.** The journey is a historical record, and its original is the Korean
> [journey.ko.md](journey.ko.md), which keeps every step, command and table. This page gives its purpose and the main turning points.

[Overview](../README.md) · [Release notes](../RELEASE_NOTES.md) · **Journey** · [Design](developers.md) · [Acceptance criteria](testcases.md) · [Demo](../demo/README.md) · [Structure diagram](../README.md#how-it-fits-together)

## What the journey is for

The journey follows one installation **from observation to finalizing, from start to end**: in which order the commands run, what goes
in and what comes out. Why it was built that way is in the [design](developers.md). It follows installing it yourself: observation,
reconciliation and judgment all finish inside the customer's infrastructure, and nothing is sent out or received from outside. Hosted use
differs, and what leaves then is in [`saas/runner`](../saas/runner/README.md).

The numbers in its overview diagram are those of pqcota's own journey; pqcaton adds **one box**, reconciliation and review, between
pqcota's observation and pqcota's generation.

## The main steps and turning points

| Step | What happens | The turning point |
|---|---|---|
| 0. Before starting | Everything runs inside the customer's infrastructure. Free for production up to 5 nodes | Nothing stays on a target node after a normal run; what was not observed is written down as not observed |
| 1. Preparation | pqcota, Go and Ansible | **This repository alone can do nothing**, because without observation there is nothing to reconcile. The reverse holds: pqcota alone can finish the whole process |
| 2. Declaration | The organization writes down "this is what we know" (from a CMDB or by hand) | Reconciliation needs two sides; the declaration is the one only the organization can write. "What to look at" has two layers: managed nodes and the cryptographic asset scope |
| 3. Observation and ingest | pqcota observes and ingests | There is a main path (pqcota's results) and a shortcut (scanning this machine, Linux only) |
| 4. Reconciliation | Three states: CONFIRMED, UNDECLARED, UNOBSERVED, with confidence and the governance topology | **UNDECLARED is the first use**; **UNOBSERVED is not decided by the machine** |
| 5. Review and finalize | Judgment, approval and finalize are three different things | The reconciliation engine does not give the answer; it structures what people judge |
| 6. Generate, apply, roll back | Back to pqcota with one `FinalizedPlan` | **Nothing is applied before finalizing**, and applying is the one step that must not run twice |
| 7. Continuing | Re-observation with delta review, expiring old judgments, re-reviewing exclusions | An exclusion is not a permanent exemption; the judgment history stays append-only |

## Running it once with commands

Without the screen and without pqcota, the shortcut scans **this machine itself** (`/proc`, Linux) and goes through open → judge →
finalize → delta with `pqcaton-decide`. It is for seeing the steps; real observation goes through pqcota's `results/`
(`pqcaton-decide open -results`). Run it in the repository after `go build -o bin/ ./inventory/cmd/...`.

```bash
umask 022
printf 'node,runtime,component\nlocal,openssl,libssl\nlocal,jca,jca-provider-chain\n' > decl.csv
./bin/pqcaton-decide open decl.csv local > session.json
```

`umask 022` is needed because `close` refuses a session file other users can write (Ubuntu's default umask 002 makes it 0664).
Between `open` and `close` a person fills in `session.json`: one conclusion per policy under `policy_decisions`, and `reviewer` and
`signature`. Until every mandatory item is judged, `close` refuses and lists what is left. For a trial run you can fill them like this:

```bash
python3 - <<'PY'
import json
s = json.load(open("session.json"))
s["reviewer"] = s["signature"] = "me"
for p in s["policy_decisions"]:
    s["policy_decisions"][p] = "trial run: keep as is"
json.dump(s, open("session.json", "w"), indent=2)
PY
./bin/pqcaton-decide close session.json -o plan.json -judgments judgments.jsonl -org acme
./bin/pqcaton-decide delta judgments.jsonl decl.csv local -org acme
```

`close` appends the judgments to `judgments.jsonl` and writes `plan.json` (`IN_REVIEW`). `delta` shows only what changed since then.
Approving and generating a plan need pqcota (`pqcota-approve`, `pqcota-provision`), which the demo goes through.

## Where it stands

The engine and the screen both work: reconciliation, the review–finalize state machine and persisted judgments, asset-scope governance,
the commands (`pqcaton-report`, `pqcaton-decide`, `pqcaton-scope`, `pqcaton-ui`) and the screens. Observation and applying changes stay in
pqcota. The [demo](../demo/README.md) runs on top of pqcota's discovery demo.

## What it does not decide

What to change and in which order is **not decided by this tool**. It structures what is to be judged, and people finalize. The screen's
structure and texts are in the [design](developers.md) and the release notes; the inside of observation is in pqcota's
[discovery design](https://github.com/randyinthedev-hash/pqcota/blob/v0.9.1/discovery/design.md).

For the full record of every step, command and decision, read [journey.ko.md](journey.ko.md).
