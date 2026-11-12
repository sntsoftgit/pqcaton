English · [한국어](compatibility.ko.md)

# The pqcota version to use with pqcaton

<!-- SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd. -->
<!-- SPDX-License-Identifier: BUSL-1.1 -->

[Overview](../README.md) · [Release notes](../RELEASE_NOTES.md) · [Operator guide](operator-guide.md)

For each pqcaton version, the version of upstream [pqcota](https://github.com/randyinthedev-hash/pqcota) to use with it. **The latest
upstream release is not automatically the right one.** pqcaton normalizes observation results with the ruleset of the upstream library
built into it, and carries the resulting snapshot fingerprints as the evidence of a plan. So upstream ingest (`pqcota-ingest`),
approval (`pqcota-approve`) and generation (`pqcota-provision`) must use the same ruleset, or the evidence of a plan is not found in the
upstream history.

## Version table

The newest version is on top. "Pinned" is the upstream version that version's `go.mod` requires. "Ruleset" is the normalization ruleset
of that upstream version (`normalize.RulesetVersion`); **when it differs, the same collection results give different asset IDs and
snapshot fingerprints**, so versions with different rulesets cannot be mixed. "End-to-end" is the upstream version the demo was run with
from start to finish (observe → reconcile → judge → approve → generate → apply → roll back). The sources are that version's section of
the [release notes](../RELEASE_NOTES.md) (v0.27.1 and earlier: the Korean edition [RELEASE_NOTES.ko.md](../RELEASE_NOTES.ko.md)) and the ruleset constant read from that upstream version's source.

| pqcaton | Pinned pqcota | Ruleset | End-to-end | Conditions |
|---|---|---|---|---|
| v0.29.0 | v0.13.0 | `pqcota-enrich/v4` | measured before release | Keep the five upstream repositories together at v0.13.0 (there is no need to pass through v0.12.0). The ruleset is `pqcota-enrich/v4` and is part of the snapshot fingerprint, so with an upstream history that v0.11.0 or earlier built under v3 neither the activation and baseline reconciliation nor the evidence of plans passes. Re-ingest the results with v0.13.0. Move the sessions and plans of v0.28.0 on through a ruleset change round (operator guide, section 9). Replace the three collectors on the nodes and `collector.jar` with v0.13.0 too. The normalization code is the same as v0.12.0, but it was not measured with v0.12.0 |
| v0.28.0 | v0.11.0 | `pqcota-enrich/v3` | v0.11.0 | Keep the five upstream repositories at v0.11.0. The screen does not start without `PQCOTA_DSN` (or `pqcota.env`), and activating a policy or adopting a baseline goes through the pqcota history reconciliation. The pqcota history's organization must be the organization of the judgments, so ingest under that organization too (`PQCOTA_ORG`). Going back to an earlier policy is blocked ([pqcota-inventory#1](https://github.com/randyinthedev-hash/pqcota-inventory/issues/1)) |
| v0.27.1 | v0.11.0 | `pqcota-enrich/v3` | v0.11.0 | Keep the five upstream repositories at v0.11.0. The ruleset and snapshot fingerprints are the same as v0.10.7, so evidence is found in that history too. Upgrade `pqcota-records`, which reads records, before v0.11.0 writes new records. 06 does not sign or generate plans that carry source evidence or a guide, so handle those with `pqcota-approve` and `pqcota-provision`. The collectors in the release assets are "cannot be verified" too, so start with `-allow-unverified-collectors` |
| v0.27.0 | v0.10.7 | `pqcota-enrich/v3` | v0.10.7 | Keep the five upstream repositories at v0.10.7. 03 checks the collectors' version; the current upstream builds use sibling-folder replaces for their dependencies and are "cannot be verified", so start with `-allow-unverified-collectors` (version information requested upstream: [pqcota-discovery#1](https://github.com/randyinthedev-hash/pqcota-discovery/issues/1)) |
| v0.26.2 ~ v0.26.3 | v0.10.7 | `pqcota-enrich/v3` | v0.10.7 | v0.26.2 cannot be taken with `go install`, so use v0.26.3 (the code behaves the same). Keep the five upstream repositories at v0.10.7. The ruleset and snapshot fingerprints are the same as v0.10.6, so evidence is found in a history v0.10.6 built, but keep `pqcota-approve` and `pqcota-provision` at v0.10.7. A v0.10.6 approval signature could pass for another plan when the plan's strings contained a NUL character (upstream v0.10.7's security fix). Replace `pqcota-jvmscan` on the nodes with v0.10.7 too |
| v0.25.0 ~ v0.26.1 | v0.10.6 | `pqcota-enrich/v3` | v0.10.6 | Keep the five upstream repositories at v0.10.6. The ruleset is `pqcota-enrich/v3`, so mixing with an upstream history of v0.10.5 or earlier loses the evidence of plans (confirmed with the v0.10.5 demo). Carry v0.24.x sessions and plans over with a ruleset round (operator guide section 9) |
| v0.21.0 ~ v0.24.1 | v0.10.4 | `pqcota-enrich/v2` | v0.10.4(v0.21.0) | Keep the five upstream repositories at the same tag, v0.10.4 or v0.10.5. v0.10.5 was confirmed to have the same ruleset and snapshot fingerprints as v0.10.4, without an end-to-end run. **It cannot be used with v0.10.6** (the ruleset differs). `pqcota-provisioning` is required from v0.24.0 |
| v0.19.0 ~ v0.20.0 | v0.10.0 | `pqcota-enrich/v2` | v0.10.0 | The first version where upstream split into five repositories. Keep the five at the same tag, v0.10.0. Compatibility with later versions was not measured |
| v0.18.0 | v0.9.0 | `pqcota-enrich/v2` | v0.9.0 | The minimum is v0.9.0 too. Plans carry evidence per action (`evidence_sources`), so `pqcota-approve` and `pqcota-provision` v0.8.0 or earlier, which do not know that field, refuse the plan |
| v0.17.0 | v0.8.0 | `pqcota-enrich/v1` | not recorded | Plans are issued as `IN_REVIEW`, so `pqcota-approve` v0.8.0 or later is needed. Below that only the signature is added, the status does not change, and the generator refuses |
| v0.16.0 | v0.7.6 | `pqcota-enrich/v1` | not recorded | not recorded |
| v0.15.0 | v0.6.3 | no constant | not recorded | Windows CNG observations (upstream v0.6.0 and later) come in as assets |
| v0.1.0 ~ v0.14.0 | v0.5.0 | no constant | not recorded | not recorded |

Until v0.19.0 upstream was a single repository (`pqcota`). From v0.19.0 upstream split into several repositories, and pqcaton pins
`pqcota-common`, `pqcota-inventory` and `pqcota-discovery` at the same tag. The `pqcota-provisioning` module dependency starts at
v0.24.0. The demo takes the five repositories, including `pqcota`, side by side at the same tag. "no constant" means that upstream version
had no ruleset constant yet (upstream has one from v0.7.3), and "not recorded" means that version's notes did not state the upstream
version of the end-to-end run or the conditions.

## When releasing

For a new version, add a row at the top of this table, or widen the pqcaton range of the top row if the upstream version stays the same.
The release notes of that version, in both languages, put the pqcota version used with it (`pqcota: vX` in English, `함께 쓰는 pqcota: vX`
in Korean) and a link to this table in the first item of "What changes for the receiving side" (`받는 쪽이 달라지는 것`), from the versions
after v0.25.0. A test checks that the top row's pqcaton
version is the version on top of the release notes, that the pinned pqcota version equals the version of every pqcota module in `go.mod`
(the modules must share one version), that the README (intro, prerequisites, matching versions) and the demo guide name that version,
and that the notes sections after v0.25.0 name the version used with it in the first item (IC-U144).
