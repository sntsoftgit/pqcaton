English · [한국어](RELEASE_NOTES.ko.md)

# Release notes: pqcaton

[Overview](README.md) · **Release notes** · [Journey](docs/journey.md) · [Design](docs/developers.md) · [Acceptance criteria](docs/testcases.md) · [Demo](demo/README.md) · [Structure diagram](README.md#how-it-fits-together)

The **goal** and the **results** of each version, updated as versions advance, newest on top.

**Two languages.** From v0.28.0 this English edition is the default and the Korean edition [RELEASE_NOTES.ko.md](RELEASE_NOTES.ko.md)
is its pair. New sections are added to both. Sections of versions before v0.28.0 exist only in the Korean edition. The GitHub release
body is made from the English section, with a link at the top to the same version in the Korean edition, fixed to the tag.

Sections are split by the **kind** of content: **what was built**, **what was learned**, **what was fixed**. Defects are not dressed up as
features; they get their own section, which states three things: **what was wrong, since which version, and what came out wrong** (or that
nothing did). **Sections of published versions are not changed.** A defect is recorded in the version that fixed it, stating where it
started.

**Each version names the pqcota it is used with.** Sections after v0.25.0 put `pqcota: vX` and a link to the
[version table](docs/compatibility.md) in the first item of "What changes for the receiving side". The GitHub release body is made from
the section, so the same information goes out with it. Sections already published are not changed retroactively.

> These rules are the same as [pqcota's release notes](https://github.com/randyinthedev-hash/pqcota/blob/main/RELEASE_NOTES.md), so that
> someone reading both repositories finds the same thing in the same place.

**Wording is still fixed after publication.** "Published sections are not changed" protects **what was said to be built and what was
said to be wrong**; it is not a rule to leave awkward sentences. A sentence that does not read is noise, not a record. The claims stay word
for word and only the wording changes.

**Some sections were not published.** Sections are opened when work starts, so if the next version ships meanwhile, a section stays
without a tag. Such a section carries "not published" in its title and states at the top which release it went out with. **They are not
folded together**, because what was built and why stops reading once it becomes one lump.

**Each version's Apache-2.0 conversion date is written with it.** The license sets four years from the release date
([licensing.md](docs/licensing.md)), so each version opens on a different day. Whichever version you use, check here when it opens.

> **§ notation**: unless stated otherwise, section numbers of the pqcota
> [regulation (v0.9.1)](https://github.com/randyinthedev-hash/pqcota/blob/v0.9.1/docs/regulation.md).

---

## Roadmap: planned releases

A direction, not a commitment. Each version becomes a proper section under the rules above when it starts and finishes.

**The screen's boundary is set.** *What ends in files on one machine is pqcaton; what spans several machines, several people or time is
the control plane.* The license divides **only by the number of nodes** (§Additional Use Grant), so the free edition is not weakened by
removing features. The goal is that a 5-node user **can go around once without a terminal**, achieved in v0.12.0.

What goes on the screen **already exists as commands**; the work is attaching a screen, not building something new, starting where people
type, because writing CSV and JSON by hand is the most inconvenient place now.

**Not on the screen**

| | Where | Why |
|---|---|---|
| Runner registration · status · areas | Control plane | Several machines |
| Organizations · users · permissions | Control plane | Several people |
| Judgment history · drift · alerts | Control plane | Time |

**Connection information is on the screen from v0.24.0.** 03 Observation edits the workspace's `hosts.csv` (owner-only), and a password
written there stays in that file and its backups. The Ansible inventory for a run exists only while it runs. The upstream principle that the
pqcota inventory keeps no access secrets (§1.5) stays, and pqcaton puts no access information in that inventory.

### Not on the roadmap: things we do not build

Not a direction but **a boundary**, written so nobody waits for them.

| Not built | Instead |
|---|---|
| **The collectors themselves** | The operator takes pqcota-discovery's collectors. The screen only runs them with the operator's Ansible |
| **Applying changes** | The screen only makes playbooks with the upstream generator and does not apply them to production machines. The operator's Ansible applies them |
| **Automatic judgment**: "this is dangerous, change it" | It structures what is to be judged; **people finalize** ([§3.1](https://github.com/randyinthedev-hash/pqcota/blob/v0.9.1/docs/regulation.md)) |
| **Machine-finalizing UNOBSERVED** | Whether something exists but was not seen, or is already gone, only people know |

---

## v0.30.0: Takes in upstream pqcota v0.14.0 (in progress)

**Goal**: use upstream pqcota v0.14.0. The ruleset (`pqcota-enrich/v4`), the contract and the database tables are the same as
v0.13.0, so the pqcota history and the sessions and plans of v0.29.x are used as they are. This version holds only what the two
upstream changes make possible: a node's latest snapshot is now the state loaded last (v0.13.1), and the released collectors now
carry their version and content hash (v0.14.0).

### What changes for the receiving side

- **pqcota: v0.14.0** ([version table](docs/compatibility.md)). Keep the five upstream repositories together at v0.14.0. No ruleset
  change round is needed.
- **Upgrade the upstream tools that load into and prune the same pqcota history together, to the same version.** From v0.13.1 a
  load and a prune take a per-node lock together.
- **The v0.14.0 release collectors are "verified" on 03.** Replace the three collectors on the nodes and `collector.jar` with the
  v0.14.0 release assets.
- **A collector is "verified" only when its content hash (h1) matches too, not just its version.** The main module h1 in the
  collector's build information must equal the value pinned in this version's `go.sum`. The same version with another h1
  "does not match", and `-allow-unverified-collectors` does not let it through. A collector built yourself from a tag checkout
  has no h1 and "cannot be verified".
- **The `collector.jar` check is stricter.** It is verified only when the release version, a well-formed commit and no
  modification all hold, and 03's collector.jar row shows its version and the first 12 characters of its commit (the full
  commit in the row's description). A JAR with no version, one built locally (`devel`, a `git describe` form), one from a
  modified tree (`X-Pqcota-Dirty: true`) and one without a commit "cannot be verified" and **do not start without
  `-allow-unverified-collectors`**. Up to v0.29.x a JAR with no version only got a warning. The usual fix is the upstream
  v0.14.0 release `collector.jar`. Another release version still "does not match", and without a JAR nothing changes.
  Manifest values are cut at 64 characters on the screen.
- **An activation that goes back to an earlier policy can be done after re-ingesting the results under that policy.** Before
  that it is still 409 (`upstream-revert`), and the screen says "Re-ingest the results under this policy, then activate it". The
  reconciliation rule (compare with the node's latest) is unchanged. It became possible because from upstream v0.13.1 the state
  loaded last is the node's latest.

### What was built

- New case IC-S26: on a real pqcota Postgres history loaded A → B → A, the latest of each node is A, B and then the first A;
  loading A again does not create a new snapshot body; then A's reconciliation passes and B is `ErrRevert`; another
  organization's history does not find it.
- New cases IC-CV11, IC-CV12, IC-CV13 and IC-U206: the h1 comparison of a release-shaped build (a module whose content changed
  under the same version does not match), the JAR's commit and modified flag, the generated pins equal to `go.mod` and
  `go.sum`, and 03's JAR row with a dirty JAR refused or allowed. IC-CV10 became "a tag checkout build has no h1 and cannot be
  verified".
- A generator that makes the expected h1 from `go.mod` and `go.sum` (`tools/genpin`) and its generated code
  (`pkg/inventory/collectorver/pin_gen.go`). `make generate` makes it and `make verify-generated` refuses a stale one.
- A PostgreSQL 16 service and `make test-pg` in GitHub CI: the Postgres contract tests (IC-S26 included) run without skipping. A
  missing DSN, a skipped test or IC-S26 not running fails it. The release gate requires `ci` to succeed on the candidate commit, so
  these tests are part of the release gate too.

---

## v0.29.1: Fixes two defects in the reap and the finish after an observation (2026-11-13)

**Apache-2.0 conversion: 2030-11-13** (four years from the release date)

**Goal**: fix a defect in the cleanup after an observation. No feature is added. The state format, the session and plan formats, the
ruleset (`pqcota-enrich/v4`) and the pqcota used with it are the same as v0.29.0, so the workspace opens as it is.

### What changes for the receiving side

- **pqcota: v0.13.0** ([version table](docs/compatibility.md)). The same as v0.29.0.
- **The reap after an observation does not end on one empty search.** The screen ends the run's processes and then treats the
  cleanup as finished only when two searches in a row, a wait apart, are both empty. A process that appears late is ended and checked
  again, and one that is still there or keeps appearing after a fixed number of rounds is not counted as a success: the run closes as
  "Reclaim needed". The end of an observation can take about 0.3 seconds longer.
- **An observation ends only when its cleanup, reap and last record are done.** Meanwhile 03 shows "The observation has ended and the
  node cleanup and process reclaim are finishing" and hides the start form, and a new observation request is 409 (`run-busy`). When it
  is done, the result and the start form come back together. An unwritten last record does not count as finished: the run shows
  "Reclaim needed" with a class of reason (such as no space left on the disk), and a new observation is 409 (`run-not-ready`).
  Restarting the screen lets the start-up recovery finish it, and a recovery that cannot write either is blocked the same way.
- **The raw text of a failed record write (paths and such) is not shown on the screen, in the status or in the run log; only its class
  is.** This holds for the last record and also for the record written at the start, the record written just before the results swap
  and the record the supervisor writes about itself. When the supervisor cannot add the processes left behind to its record, that is no
  longer dropped either: it goes to the run log with its class. The error wording of work other than record writes (starting the node
  cleanup, removing the secret files, reading the results) is not changed in this version.

### What was fixed

- **The reap after an observation could miss a process that had just started. The defect had been there since v0.27.0.** When a
  cleanup command sent a child off in its own session and ended at once, the child's environment could read empty in the middle of
  `exec`, so its run marker did not show. The reap ended on that one search as "nothing left", and the process could stay on the
  controller. It came to light when IC-U135 failed now and then on GitHub CI (the v0.29.0 release commit too).
- **A new observation could be accepted on top of one that was still finishing. The defect had been there since v0.24.0.** When the
  observation process ended, the screen at once changed the state to "done" or "failed" and offered the start form again, while the
  node cleanup, the reap, the swap of the results and the last record came after that. A new observation sent in that gap was accepted,
  so two runs could overlap, and the run record still said "running" meanwhile. It came to light while building this version's demo end
  to end check.

### What was built

- New case IC-U203: holding the reap open makes the finishing gap; meanwhile there is no start form, a new observation is 409 and the
  record says running, and only after the record is written with its final state do the start form and new observations come back.
- New case IC-U204: an injected failure of the last record write (the raw error carries a path and a secret); the run ends but shows
  "Reclaim needed" with only a safe reason, there is no start form, a new observation is 409 and the record on disk stays running; a
  restart whose recovery cannot write is blocked the same way, and a restart after the disk is writable again finishes the run.
- New case IC-U205: an injected failure in each of the record written at the start, the record written just before the swap and the
  supervisor's record (the raw error carries a path and a fake secret); the screen, the status and the run log carry no raw text but
  the class, the run ends failed with the current results unchanged, and the next observation is accepted.
- New case IC-U202: injected search results measure, deterministically, empty → late → ended, nothing there, a process that appears
  again every time it is ended, and a search error.
- The demo end to end (IC-U152) runs two real observations from 03 (one failing with a node stopped, one succeeding with it back).
  After both, no process with the run marker may be left.
- A `github-checks` step in the release gate (`make release-check`): the required workflows (ci and site) of the candidate commit must
  all have succeeded. Missing, in progress or failed stops the gate. The site workflow runs on every push to main.
- Documentation site tools (`tools/site`, not deployed): main builds the site, checks its links and keeps only a preview
  ([site guide](docs/site.md)).

### What was checked

- IC-U202 failed in all four cases against the reap code of v0.29.0 and passed in all four with this version. IC-U203 failed against
  the old check (by the "running" state only) and passed with this version, also 20 times under race. IC-U135 passed 100 times
  in a row on Linux and 20 times under race.
- The full vet (`go vet ./...`) passed on Linux, and a cross vet for macOS and Windows confirmed that the tests compile for them too.
  Before, the tests did not compile for Windows (one observation process test called a Unix-only helper). That test moved to a
  Unix-only file; the product code is unchanged.
- The check results are reported with the summary of the release gate (`make release-check`) when asking to publish.

---

## v0.29.0: Takes in upstream pqcota v0.13.0 and normalizes with ruleset v4 (2026-11-12)

**Apache-2.0 conversion: 2030-11-12** (four years from the release date)

**Goal**: use upstream pqcota v0.13.0. Upstream v0.12.0 moved the ruleset to `pqcota-enrich/v4` (JCA deployment identity), so this
version cannot be mixed with v0.28.0. Judgments that rest on evidence whose meaning changed are made again in a new round, and earlier
conclusions stay only as reference values in the change report. No compatibility rule links old and new ids. The state format and the
session and plan formats are the same as v0.28.0.

### What changes for the receiving side

- **pqcota: v0.13.0** ([version table](docs/compatibility.md)). Keep the five upstream repositories together at v0.13.0. There is
  no need to pass through v0.12.0. Upgrade the upstream tools that read the same database first, and replace the three collectors on
  the nodes and `collector.jar` with v0.13.0 too.
- **The ruleset is `pqcota-enrich/v4`.** The ruleset is an input of the snapshot fingerprint, so with a pqcota history that v0.11.0 or
  earlier built under v3 the activation and baseline reconciliation on 05 do not pass. Re-ingest the results with v0.13.0. 06 does not
  sign or generate a v3 plan.
- **This upgrade carries no conclusions.** The ruleset goes from v3 to v4, so a v0.28.0 session moves on through a ruleset change
  round: the current conclusions are empty and earlier ones stay only as reference values. Judge each item again or press "Adopt again",
  then finalize and approve on the new evidence.
- **The JCA item keeps its id; its evidence changes.** pqcaton counts JCA as one item per node (`jca-provider-chain`). The new
  `pqcota-jvmscan` reports JVMs as per-deployment findings, so that item's evidence (finding ids and fingerprints) changes and it shows
  as "changed" in the change report.
- **Edge splitting and the change of `observed_count` do not directly affect pqcaton's judgment items.** Upstream v0.13.0 splits
  edges per application that opened the connection and counts distinct connections in `observed_count`. pqcaton counts an edge by
  source, destination, port and protocol, and edges do not enter the judgment queue, so reconciliation states and judgments stay the
  same. The "observed edges" count at the top of the report counts upstream edges as they are and can grow (4 → 5 in the demo), and
  `observed_count` is not compared across the upgrade.
- **If an asset scope rule has `user@<uid>.service` in its `app_key`, fix it.** The v0.13.0 collectors report a user service as
  `user@<uid>.service/<unit>`. Fix it on 02, finalize, and activate on 05 (operator guide, section 9).
- **Evidence that changes within the same ruleset works as before.** When the item id and the ruleset are the same, the conclusion
  moves over and the signature is cleared; finalize and approve again on the new evidence (IC-D20). A change of meaning within the same
  ruleset is announced by upstream raising the ruleset.

### What was built

- Tests fix the property that the ruleset is part of the snapshot fingerprint: the same ruleset gives the same fingerprint, v3 and v4
  give different ones, and a v3 history fails the v4 check and passes after a v4 re-ingest (IC-S25).
- New cases: IC-P51 (refusing v3 sessions and plans), IC-P52 (a ruleset change round that does not carry the conclusion of a JCA item
  whose evidence became per-deployment) and IC-P53 (a v4 round opened from a v3 session file).
- The demo's expected results (`demo/expected-output`) were made again from a run of the basic v0.13.0 demo.

### What was checked

- The demo end to end (`make e2e-demo`, upstream v0.13.0): policy activation ran to the end both on the screen and by the command,
  and 06 signed and generated. The collectors showed as "cannot be verified", like the release assets, so the screen started with
  `-allow-unverified-collectors`.
- Upstream v0.13.0 release assets: the downloaded files matched `SHA256SUMS`; the collectors are still replace builds
  (`pqcota-discovery v0.13.0 => ../pqcota-discovery (devel)`), so "cannot be verified", and `collector.jar` has no version. The three
  upstream issues (pqcota-inventory#1, pqcota-discovery#1, pqcota#8) are open.
- The check results are reported with the summary of the release gate (`make release-check`) when asking to publish.

---

## v0.28.0: Scope rounds always go through the pqcota history reconciliation, and the screen does not start without the pqcota history (2026-11-12)

**Apache-2.0 conversion: 2030-11-12** (four years from the release date)

**Goal**: pqcota is not treated as an optional dependency. pqcaton runs only where pqcota is installed, connected and working, and policy
activation and baseline adoption in 05 go ahead only after reconciling the pqcota history with pqcota results re-ingested under the new
policy. An operator statement does not replace that reconciliation. Moving the reconciliation into the screen removes the step of stopping
the screen and running a command to activate. The state format, the session and plan formats and the ruleset (`pqcota-enrich/v3`) are the
same as v0.27.1, so the workspace opens as it is.

### What changes for the receiving side

- **pqcota: v0.11.0** ([version table](docs/compatibility.md)). The upstream version is the same as v0.27.1.
- **The screen does not start without `PQCOTA_DSN`.** With the environment variable `PQCOTA_DSN` set it uses that; when it is empty it uses
  `pqcota.env` in the same user configuration folder as the approval key. If connecting and the required lookup (one read of the history
  to check the schema and read permission) do not pass within 10 seconds, it stops before opening the port or writing the address file.
  `pqcota.env` must be a regular file, not a link, owned by this user, 0600, with one hard link and outside the workspace, and its content
  is not evaluated by a shell: only one line `PQCOTA_DSN=<value>` is read as data. The connection is not left in the workspace, processing
  records, logs, error messages or the screen, and it is not passed to the observation processes the screen starts. **Operators who
  started it as a service or launcher must create `pqcota.env` before upgrading.**
- **The way to activate on an operator statement is gone.** The confirmation boxes in 05 ("I confirmed the re-ingest was done", "I
  confirmed it is the policy the upstream ingest used") are removed, and `-confirm-ingest` of `pqcaton-decide activate-scope` · `baseline`
  stops with an end-of-support error without writing anything. `-verify-upstream` is accepted but changes nothing (the reconciliation
  always runs).
- **The pqcota history's organization must be the organization of the judgments.** The screen and the command open only the pqcota
  history of the organization of the judgments. `-upstream-org` accepts only the same value; another organization stops before the history
  is opened. If the connected history, the organization of the judgments and the review session's organization differ, it refuses without
  looking anything up (409 `upstream-org`). **So ingest pqcota under the organization of the judgments too**
  (`PQCOTA_ORG=<organization> pqcota-ingest …`). If you ingested under another organization (such as the default one), ingest again under
  the organization of the judgments before activating.
- **Reconciling the pqcota re-ingest is required before activation and baseline adoption.** One activation request takes the input
  snapshot → reconciles outside the lock (10 seconds in all) → takes the lock right away → re-checks the local inputs inside the lock →
  writes in one transaction. For every node the latest pqcota ingest must match the results under the new policy; if any is missing,
  differs or cannot be looked up, nothing is written (`upstream-missing` · `upstream-empty` · 503 `upstream-unreachable`). **An
  activation that goes back to an earlier policy is blocked** (`upstream-revert`), because pqcota does not make a re-ingest of the same
  content the latest and it cannot be reconciled
  ([pqcota-inventory#1](https://github.com/randyinthedev-hash/pqcota-inventory/issues/1)). Changes to the pqcota history after the
  reconciliation and before the activation finishes are not detected, so the screen and the records say only that it matched the pqcota
  history at the time of checking.
- **The reconciliation is recorded in the processing record and the report.** The basis of new activation and baseline processing records
  is `upstream-verified` only, and the time of checking, the pqcota organization and the per-node fingerprints reconciled go into
  `upstream_check` of the change report (no secrets). The report format stays 1, and readers ignore unknown fields, so v0.27.x reads new
  reports. Rounds that v0.27.x or earlier opened on an operator statement are read as past records and show in 05 as "It was not
  reconciled with the pqcota history".
- **The demo's `ui.sh apply-scope` only re-ingests pqcota** (under the organization of the judgments). Activation is done in 05.
- **User documents and the screen call "영수증" a "processing record" (처리 기록).** `Receipt` in the code stays.

### What was built

- The reconciliation function `upstream.Reconcile` shared by the screen and the command (one 10-second limit for the whole
  reconciliation, error classification without connection details, telling a revert to an earlier policy apart).
- The three steps of the 05 activation box: re-ingest pqcota (a command with the organization of the judgments and the paths filled in) →
  reconcile the pqcota re-ingest → activate the policy. The confirmation page reconciles ahead and shows "At the time of checking, the
  pqcota history matched" with the number of nodes and the time of checking, and activation reconciles again from the start.
- Five new refusal kinds: `upstream-unreachable` (503) · `upstream-missing` · `upstream-empty` · `upstream-revert` · `upstream-org`.
- The screen start flag `-upstream-org` (only the same value as the organization of the judgments).
- New cases: IC-S22 (reconciliation verdicts) · IC-S23 (one time limit) · IC-S24 (no connection details in errors) · IC-P49 (scope rounds
  without a reconciliation record are refused) · IC-P50 (the compatibility contract of report format 1) · IC-D37 (input snapshot before
  the reconciliation) · IC-D38 (the command's organization match) · IC-U196 (does not start without pqcota) · IC-U197 (05 activation
  reconciles) · IC-U198 (reconciliation record and the re-check inside the lock) · IC-U199 (reverting to an earlier policy is blocked) ·
  IC-U200 (pqcota.env checks and the child environment) · IC-U201 (the screen's organization match). The demo end to end (IC-U152) runs
  both the screen path and the command path.

### What was fixed

- **The command's upstream check took the input snapshot after checking.** From v0.26.0, `pqcaton-decide activate-scope
  -verify-upstream` · `baseline -verify-upstream` took the snapshot of the workspace inputs **after** checking the upstream history. If
  the results folder or the policy file changed during the check, a `upstream-verified` record could be written for inputs that were not
  checked. The command ran the check and the write back to back, so the window was narrow, and no record actually written that way was
  found. This version takes the snapshot before the reconciliation and re-checks local input changes inside the lock (IC-D37 · IC-U198).
  Changes to the pqcota history after the reconciliation remain the limit above.

### What was checked

- The demo end to end (`make e2e-demo`, upstream v0.11.0): activation on the screen and by the command before the re-ingest, and
  `-confirm-ingest` · no `PQCOTA_DSN` · a hand-edited candidate were all refused, with the files and the number of pqcota snapshots
  unchanged. After the re-ingest, both the command path (on a copy of the workspace) and the screen path activated; the basis was
  `upstream-verified`, the report's organization was the organization of the judgments (demo-corp), and the DSN was not in the records.
  The Docker, temporary file and process state of the host was the same before and after the run.
- The results of the checks are reported as the release gate (`make release-check`) summary when the release is approved.

---

## Earlier versions

Sections of v0.27.1 and earlier are in the Korean edition, [RELEASE_NOTES.ko.md](RELEASE_NOTES.ko.md).
