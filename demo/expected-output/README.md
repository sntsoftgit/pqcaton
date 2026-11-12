English · [한국어](README.ko.md)

# Expected output: the extension sample

**Representative results** of running `../scripts/extend.sh` (on top of a running pqcota discovery demo). On top of the observation
grades (pqcota) come **three-state reconciliation against the declaration, the review queue and the governance topology**.

| File | Contents |
|---|---|
| [report.txt](report.txt) | Console output. ① observation (what was seen and **what was not**) → ② three-state assets and the review queue → ③ edge reconciliation |
| [topology-governance.svg](topology-governance.svg) | Governance topology (color = grade, line style = state: solid CONFIRMED / bold UNDECLARED / dotted UNOBSERVED) |
| [plan.json](plan.json) | **A judged plan**: a plan in the `IN_REVIEW` state after judging, the input of `pqcota-approve`; once approved and `FINALIZED` it goes on to `pqcota-provision` |

**It does not stop at reconciliation.** The demo goes through judging all the way to `plan.json`. If anything between them were broken,
it should show here. Before v0.9.0 the demo stopped at reconciliation, and **nobody knew there was no way to judge the observed nodes.**

> **This `plan.json` has two actions, both for auto-passed assets.** The demo reconciles under **the same asset-scope policy** as the
> upstream ingest, because without it the snapshot fingerprints differ from the central history and the evidence cannot be traced
> back. With the policy, no observed asset stays in the review queue, but the assets that match the declaration and are confirmed
> (auto-passed) go into the plan. **"No one needs to look at it" is not "nothing needs to change."** Before v0.18.0 auto-pass was a
> list of identifiers with no place in the plan, and this file had no actions.
>
> **② shows two mismatch warnings.** The declaration lists `openssl/libcrypto` of `web-gw` and `pay-db` as managed, and the policy
> excludes the applications that were observed using it (`sshd`, `python`). Before v0.18.0 these two came out as "declared but not
> seen" (UNOBSERVED). **They were seen, and chosen not to be managed.** Now the reconciliation axis stays CONFIRMED and the management
> axis is excluded, so the screen reports the mismatch with every application identifier. Which side to fix is not the tool's decision.
>
> **Read the application list of the warning.** `libcrypto` on `pay-db` is loaded not only by `sshd` and `python` but also by
> `/opt/apps/api-gw` and `/opt/apps/payment-gw`. The policy aimed at `sshd`, but one shared `.so` is used by several applications, so
> **the whole list was excluded with it.** That is exactly the trap the upstream `AssetPolicy` comment warns about, and you do not see
> it if you write only one application identifier.

The trace chain (plan → evidence → snapshot → record) is measured with a dedicated fixture. Running with `PQCATON_E2E_TRACE=1` removes
the assets from the declaration and renames one node to an alias, then goes around once. **Its results are not written here.** The names
the fixture makes are not in the default topology, so placing them here would make readers take them as the default run's results.

**These files are not made by hand.** `extend.sh` copies them as it runs. Only the **session UUID in `plan.json` is replaced with a
fixed value.** It is drawn anew on every run, so left as it is the file would change on every run and the day the contents really
changed could not be told. The real run's id stays in the console and the judgment ledger. Only `report.txt` was hand-made for a while,
so it stayed Korean even after the command output became English (fixed on 2026-08-19).

**Why ① comes first**: someone who reads only this report without going through the pqcota demo should see what came before
reconciliation. Above all, without "the layers not seen", readers cannot tell whether an UNOBSERVED in ② means "absent" or "could not be
seen in principle."

The core story (against the declaration). **No asset-scope policy applies to edge reconciliation, so it stands as it is.**
- 🟢 `web-gw→pay-app` MLKEM **CONFIRMED** · 🔴 `web-gw→pay-db` classical **CONFIRMED**
- 🟢 `web-gw→pay-app` SSH sntrup761 · 🔴 `web-gw→pay-db` SSH curve25519. **Both UNDECLARED.** Managed communication nobody declared is
  actually running
- ⚪ `pay-app→web-gw` **UNOBSERVED**: declared but not observed (≠ absent). The register lists both directions while traffic flows one
  way, a common shape

> **These results come from the default topology of the pqcota demo** (`demo/topology/topology.example.yaml`). **The declaration is
> made from that file too** ([`declare.py`](../scripts/declare.py)). Adding nodes or renaming them, our side follows without changes.
> We confirmed it by adding one node: nodes 3→4, CONFIRMED 4→6.

## What may differ in a real run (and why)

- **Edge reconciliation**: it reconciles the edge set core observed as it is. core `demo.sh` re-collects with **retry-until-complete**
  up to the target edges (complete even on the first run), so the extension's edges are consistent too. Depending on the capture window,
  one more UNDECLARED edge may appear.
- **Versions and IPs**: the same as the [core expected-output](https://github.com/randyinthedev-hash/pqcota/blob/v0.11.0/demo/expected-output/README.md).
  Base image digests pin the version strings; only the IPs change on every run (unrelated to the story).
- **Declaration rules**: changing `DECLARED_PROTOS` in [`../scripts/declare.py`](../scripts/declare.py) changes the distribution. Adding
  `ssh` makes UNDECLARED disappear, the picture of an organization with a perfect declaration.

## Determinism

Given the same observations and declaration, the three-state reconciliation, confidence and review queue are always identical. The
"differences" above are differences in what core observed (capture timing), not nondeterminism in the reconciliation logic.
