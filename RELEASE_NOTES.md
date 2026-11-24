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

## v0.32.0: Takes in upstream pqcota v0.20.0 (in progress)

**Goal**: use upstream pqcota v0.20.0 (which includes v0.17.0, v0.18.0 and v0.19.0). The ruleset (`pqcota-enrich/v4`), the contract and
the database tables and columns are the same as v0.16.0, so the pqcota history and the sessions and plans of v0.31.x are used as they are.

### What changes for the receiving side

- **pqcota: v0.20.0** ([version table](docs/compatibility.md)). Keep the five upstream repositories together at v0.20.0.
  No ruleset change round is needed.
- **With the v0.20.0 collectors on the nodes, accepted connections are observed too.** From upstream v0.18.0 `pqcota-netcap` records
  the connections a node accepts, not only the ones it opens. The first observation after replacing the collectors can add communication
  edges for nodes that only accept connections. The assets do not change.
- **04 reads the accepting node's record as "opener → acceptor" and merges it with the opener's record into one edge.** A connection
  recorded by both nodes is one line in the reconciliation table. When the sending node was not observed but the accepting node saw the
  connection, the edge is in the table and is marked "observed on the accepting node only". A connection accepted from an unlisted peer
  gets "needs enrollment decision" on the From side.
- **Connections whose direction was not confirmed are kept out of the reconciliation table and shown separately under it.** A record
  for which the collector could not confirm which side opened the connection (for example SSH whose start was not seen) has no port and
  no negotiated group. It is not reconciled with the declaration and is not counted in "negotiated connections". The topology draws it
  as a dotted line without an arrow. Observing again may confirm it.
- **"Negotiated connections" counts edges.** It used to count records. A connection recorded by both nodes is one.
- **The collector signing keys are as in v0.31.0.** 03 uses one key per collector ID. The screen of this version does not handle the
  per-node key file of upstream v0.17.0 (`PQCOTA_VERIFY_KEY_FILE`). `PQCOTA_VERIFY_KEY` in the collector-bound form works unchanged with
  upstream v0.20.0. The limitation that a key taken from one node can sign results for another node remains.
- The order of the upgrade is in [operator guide, section 9, "v0.31.1 to v0.32.0"](docs/operator-guide.md#v0311-to-v0320).

### What was fixed

- The `pqcota-netcap` before upstream v0.18.0 could write the accepting role on a client record that missed its own ClientHello;
  upstream fixed that. When result files of that version are still in the workspace, 04 reads such a record as the opener's record (it
  is told apart by the port in the peer address).

### What was built

- New cases IC-E5, IC-E6 and IC-UI75 measure turning the accepting node's record around and merging it, keeping records with an
  unconfirmed direction out of the reconciliation and collecting them separately, and 04 showing the two apart. IC-R8 also measures
  linking an IPv4 peer written with `::ffff:` to its node.

---

## v0.31.1: Fixes the 01 declaration saving an input mistake as a deleted node (2026-11-23)

**Apache-2.0 conversion: 2030-11-23** (four years from the release date)

**Goal**: fix a defect in saving the 01 declaration. No feature is added. The state format, the session and plan formats, the ruleset
(`pqcota-enrich/v4`) and the pqcota used with it are the same as v0.31.0, so the workspace opens as it is.

### What changes for the receiving side

- **pqcota: v0.16.0** ([version table](docs/compatibility.md)). The same as v0.31.0.
- **01 does not save when a field is missing or a value is malformed.** The response is 422 and the declaration file is byte-for-byte
  unchanged. "Not saved" stands at the top of the screen with a link to each field, and what was typed stays on the screen. Fix the
  fields and save again from that screen. These are refused:
  - a node row whose name or IP is empty, whether it was emptied on an existing node or only one of the two was typed on a new row;
  - an asset row whose component is empty (a new row only when a runtime was picked);
  - an edge row whose sending or receiving side is empty;
  - an IP that is not an address (a hostname, an address with a port), and a port that is not a number from 0 to 65535.
- **A new row with nothing typed is still dropped.** To delete a node, an asset or an edge, use Remove (it asks first).
- **A node without an IP is refused instead of being left out.** Since v0.15.0 a row without an IP was left out of the save with a
  notice. If the declaration file already has a node without an IP (a hand-edited file), 01 shows that row, and saving needs its IP
  filled in or the row removed with Remove.
- **IPs are saved as typed.** IPv6 with an embedded IPv4 (`::ffff:10.0.0.1`) is accepted. A zone (`fe80::1%eth0`) is not.
- **An empty port and `0` behave as before.** They are saved as 0 and pointed at with "the port is 0". A shared IP, a shared observed
  name and a sending side that is not under management are saved and pointed at as before too.
- **The browser stops the same things first.** The `pattern` and `required` of the fields come from the same expressions as the
  server's check. A request that did not pass the browser's check is refused by the server.
- The upgrade order is in [section 9 of the operator guide, "v0.31.0 to v0.31.1"](docs/operator-guide.md#v0310-to-v0311).

### What was fixed

- **Emptying a node's name on 01 and saving removed the node and its assets from the declaration. The defect had been there since
  v0.6.0.** The form read a row with an empty name as a deleted row, and the help on the screen said so. Saving with the name field
  emptied by mistake left out the node and its assets with no question and no notice; the result of the save showed only the smaller
  counts. Emptying an asset's component or one end of an edge left that row out the same way. The assets of a node left out this way
  came up as UNDECLARED in the next reconciliation.
- **A port that is not a number was saved as 0 on 01. The defect had been there since v0.6.0.** A value that cannot be read, such as
  `xyz`, was replaced with 0 and saved, and only "the port is 0" was shown. What was typed did not stay on the screen. An edge with
  port 0 does not match an observed edge.
- **Removing an existing node's last IP on 01 and saving left the node and its assets out of the declaration. This had been so since
  v0.15.0.** Leaving a row without an IP out of management, with a notice, was intended then. But an existing node whose IP was removed
  by mistake took the same path, and its assets went with it.
- In all three cases what was left out affected only the declaration file. The judgment ledger and finalized plans were not changed.
  Reconciliations and judgments after that were computed from the smaller declaration, so check on 01 that the node and asset counts
  are what you expect.

### What was built

- New cases IC-UI61 to IC-UI64 and IC-U219 to IC-U221: an empty name, an empty IP, an empty component, an edge with one end, and a
  malformed IP or port are refused and no declaration is built; the refused screen gives back what was typed and the node's assets, and
  saving after fixing succeeds; the browser's `pattern` is the server's expression and does not disagree with `net.ParseIP`. IC-U12
  and IC-UI27 measured the old behaviour, so their meaning was changed.

## v0.31.0: Takes in upstream pqcota v0.16.0 (2026-11-19)

**Apache-2.0 conversion: 2030-11-19** (four years from the release date)

**Goal**: use upstream pqcota v0.16.0 (which includes v0.15.0). The ruleset (`pqcota-enrich/v4`), the contract and the database tables
and columns are the same as v0.14.0, so the pqcota history and the sessions and plans of v0.30.0 are used as they are. This version
follows two upstream changes: collector results are signed with keys bound to the collector ID (v0.15.0), and the database schema is
created by a dedicated command while the query commands only read (v0.16.0).

### What changes for the receiving side

- **pqcota: v0.16.0** ([version table](docs/compatibility.md)). Keep the five upstream repositories together at v0.16.0. No ruleset
  change round is needed. Upgrade the upstream tools that load into, prune or record in the same history to v0.16.0 as well, and replace
  the collectors with the v0.16.0 release assets. 03 refuses the v0.14.0 collectors as "does not match".
- **03 does not start an observation without the collector signing keys.** For each collector ID (`openssl-collector` ·
  `network-collector`, and `jvm-collector` when there is a `collector.jar`), keep a private key made with `pqcota-keygen -collector <ID>`
  and the trusted public keys (`verify.pub`) in the same bound form you give the ingest, outside the workspace (folder 0700 · keys 0600).
  03 shows the missing IDs, why, and how to prepare them. Every node shares the key of an ID (upstream does not yet bind keys to nodes).
- **03 takes only results whose signatures check as the current results.** If any result has no signature, was signed with the key of
  another ID (shown so only when that key verifies it), or does not match the registered keys, the run's results are not taken and the
  current results stay. The run log records "signature checked" and the key fingerprint for each result.
- **03 observes Linux nodes only.** When the targets include a Windows node, the run is refused before it starts and the node is shown.
  03 in this version does not pass signing keys to Windows nodes.
- **03 checks every node before it makes the keys.** A node is reached as root, or its account can get a root shell through sudo without
  a password, with sudo I/O logging (`log_input` · `log_output`) off. A check "passes" only in the sense that no logging setting was found
  in the supported policy forms. If even one node is refused or cannot be confirmed, no key is made and the observation does not start;
  the run's node cleanup is recorded as "not needed".
- **03 does not read the operator's `ansible.cfg` or `ANSIBLE_*` environment variables.** Each run reads only the configuration the product
  makes (SSH for the connection, sudo for privilege escalation, pipelining on, plugin and collection paths cut, built-in modules only). The
  values to move and the settings no longer supported are in the table in section 9 of the operator guide. The versions measured are
  ansible-core 2.16.3 and 2.21.6.
- **03 makes temporary keys for each run and removes them.** After the checks pass, it makes the key files (0600) in a temporary folder
  (0700) outside the workspace and passes them to the nodes only through standard input. It removes them on a normal end, failure, stop
  and recovery at restart; when it cannot, the run is marked "needs reclaiming" and new observations are blocked. If the screen ends
  abnormally, the temporary keys can stay until the next recovery.
- **The pqcota re-ingest guidance of 05 has two steps.** Check without storing with `pqcota-ingest -verify-keys`, then ingest with the same
  `PQCOTA_VERIFY_KEY` (the value of `verify.pub`).
- **The pqcota history is split into three roles.** The schema role owns the tables and creates the schema with
  `pqcota-inventory -init-schema`, the ingest account runs with `PQCOTA_AUTO_DDL=0`, and the screen account may only read. From upstream
  v0.16.0 the query commands and the screen create no tables, so the screen tells "schema missing" from "no privilege" and does not start.
  For an existing history, move the tables' owner to the schema role, then run `-init-schema`.
- **06 does not take a collector signing key as the approval key.** A key made with `pqcota-keygen -collector` is refused in the approval
  key and verification key places with the state "collector".
- The upgrade steps are in [operator guide section 9, "v0.30.0 to v0.31.0"](docs/operator-guide.md#v0300-to-v0310).

### What was fixed

- **After an observation stopped or failed, the run's collectors kept running on the nodes.** The cleanup playbook removed only the
  staging folder on the node and did not end the collectors started from it. Remote commands started through pipelining and sudo have no
  tty, so they get no signal when the controller ends Ansible and ssh, and the collectors stayed as root until the end of their window.
  The screen recorded the node cleanup as successful. In this version the signing key is in that process's environment, so this was fixed
  before release. Cleanup now ends the processes whose command line starts with this run's staging folder, and records a cleanup failure
  when they do not end (IC-U218). Collectors of another run on the same node are left alone. That the defect was there from v0.24.0 is an
  estimate from reading the code; it was not reproduced with the old versions.
- **On machines whose default umask is 002, 06 refused the verification key (`approval.pub`) made as the operator guide said.** From
  v0.27.0 the command in the guide made a file the group could change, and 06 refuses such a public key file. No plan was signed or
  verified wrongly. The public key file is now created under umask 077 too, and the final `chmod` opens it to others for reading only. The
  new `verify.pub` command does the same.

### What was built

- Collector signing in 03: the key places and the checks before start (a missing ID, an ID other than the file name, a key without an ID,
  wide permissions, a key not in `verify.pub`, one public key under two IDs, a key equal to the approval key; IC-U210), refusing Windows
  nodes (IC-U211), the node check and reading `sudo -n -l` (18 output variants, IC-U209), the supervisor's order (check → keys → the
  observation-started marker → observation → reclaiming, IC-U212), the temporary key files (IC-U213), verifying result signatures (upstream
  `sign.VerifyBound`, IC-U214), reclaiming the temporary keys and "needs reclaiming" (IC-U215 · IC-U216), the product's Ansible configuration
  and the environment allow list (IC-U217), and ending collectors left behind (IC-U218).
- The screen's start check tells upstream's schema missing from no privilege (IC-S27, and IC-S28 on a real Postgres). `make test-pg` checks
  that IC-S28 ran too. 06 refuses collector signing keys (IC-U208), and the re-ingest step of 05 has `-verify-keys` (IC-UI60).
- Demo: at its very start (0/8), `extend.sh` moves the upstream demo's history to three roles (with the SQL of operator guide section 9 as
  written); loads and generation records are written by the ingest role with `PQCOTA_AUTO_DDL=0`, and the screen and the command reconcile
  through the read role. The passwords are made anew on every run and kept only in `~/.pgpass`. `ui.sh` makes the collector signing keys
  and `verify.pub` for 03 and gives them to the screen, and `apply-scope` checks the results of 03 with `-verify-keys` before ingesting.
- Added to the demo end to end (IC-U152): the ownership and privileges of the three roles and the refusal of a load without
  `PQCOTA_AUTO_DDL=0`, that the screen which activates runs as the read role, the real path of 03 (check · temporary keys outside the
  workspace · signature check · replacement · reclaiming) and key exposure, and checking the results of 03 with `-verify-keys` and actually
  ingesting them.
- Both operator guides: in section 2, preparing pqcota (three roles · `-init-schema` · privileges), getting ready to run observations,
  preparing the collector signing keys, the Ansible configuration 03 uses and the node check; in section 6, the three re-ingest steps and
  the order of 03 and "needs reclaiming"; in section 9, "v0.30.0 to v0.31.0", "Moving an existing history to three roles" and the values to
  move from `ansible.cfg`. The environment variable document has `PQCOTA_VERIFY_KEY` · `PQCOTA_AUTO_DDL` and the environment 03 passes to
  Ansible.

### What was checked

- The demo end to end (IC-U152) passed, run alone with the five upstream repositories at v0.16.0. The upstream demo's history was moved to
  three roles. `pqcota-inventory -init-schema` created 4 of the 11 history tables and kept the existing 7. `pqcota-records -init-schema`
  kept the existing generation record table. All 12 tables the two tools cover are owned by the schema role, and the screen role was
  confirmed to read them. The screen role cannot write any table, and the ingest role cannot create tables and was refused without
  `PQCOTA_AUTO_DDL=0`. The default organization's approval and generation (the evidence resolved to real snapshots) and A → B → A in the
  judgment organization both passed, and the screen that activated ran as the read role. 03
  ended with node check → temporary keys outside the workspace → signatures of 7 results checked → replacement → keys reclaimed, and the
  results of 03 passed `-verify-keys` and were ingested into the judgment organization by the ingest role (no signature rejected). The demo
  nodes are reached as root.
- The sudo path was measured with the product `pqcaton-ui` in an isolated lab (ansible-core 2.16.3 · 2.21.6, an ordinary account then
  NOPASSWD sudo). A normal observation, refusing a node with I/O logging on, "cannot be confirmed" for a node whose sudo asks for a
  password, and a stop during observation all ended as expected. The two refused cases made no key and their node cleanup was "not needed".
- Key exposure: the collector signing keys were not found in any command line of the controller and the nodes collected during the
  observation, the nodes' temporary folders, Ansible places, sudo logs and I/O logs, the whole workspace, or the screen log. This is a
  statement about the places checked. The trust premise stays: during a run the key is in the collector process's environment and that
  node's root can read it. For signature failures, the checks before start stopped runs that would use a wrong key, and the official
  collectors' error output for a wrong key did not contain the key.
- The DB procedures and the key preparation commands of the operator guide were run as written on an isolated PostgreSQL 16 and an empty home
  folder. For a new install, `-init-schema` created 11 tables and the screen started with a screen account that may only read. For moving an
  existing install, `-init-schema` before moving the ownership failed with "must be owner" and changed nothing, and after the move
  initializing, loading and querying worked. An ingest account loading without `PQCOTA_AUTO_DDL=0` failed with "permission denied for schema
  public".
- The official v0.16.0 linux-amd64 bundle passed all eight checks of the preparation tool, and 03 showed the collectors and `collector.jar`
  as "verified".
- The Postgres contract tests (IC-S26 · IC-S28 included) passed without skipping in the PostgreSQL job of GitHub CI.
- The verification results are reported with the summary of the release gate (`make release-check`) at the release approval.

---

## v0.30.0: Takes in upstream pqcota v0.14.0 (2026-11-18)

**Apache-2.0 conversion: 2030-11-18** (four years from the release date)

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
- An `upstream-assets` step in the release gate (`make release-check`): with the same preparation tool as the demo it fetches the
  official upstream assets of the pinned version and checks the checksums, the release manifest and the attestations, writing the
  upstream tag and the target OS and architecture to the summary. A failure stops the gate. `gh` logged in is now a prerequisite.
- "Checking the collectors you download" in section 2 of the operator guide: the preparation and re-check commands, the `gh` and
  network conditions, the target architecture, and what the release check and the version check on 03 each look at.
- An official collector preparation tool (`inventory/cmd/pqcaton-collectors`, `go install github.com/sntsoftgit/pqcaton/inventory/cmd/pqcaton-collectors@v0.30.0`): it downloads the upstream release assets of the version go.mod pins,
  checks the attestations (pinned to the repository, the signing workflow and the tag), `SHA256SUMS`, a safe unpacking, each file's
  hash, version and commit against the release manifest, the attestations of the unpacked programs and that only the expected files
  are there, then moves the folder into place at once (`fetch`). An existing target is never overwritten. It checks again right
  before use (`verify`). The demo (`ui.sh`) uses only the official assets prepared with it and starts the screen without
  `-allow-unverified-collectors`; the way that used collectors built from upstream source is gone. The target (linux-amd64 or
  linux-arm64) is always given and written into the prepared folder, and the OS and architecture in the programs' build
  information must match it. `ui.sh` refuses a prepared folder whose target differs from the `pqcota-ctl` container before checking
  it. Putting it into the workspace is done by the same tool inside the container (`recover`, `install`): the copy in a temporary
  place on the same file system is checked against the hash list again and brought in with `rename`, the existing one is restored
  on failure, and what an interrupted run left is tidied by its state. It holds the same workspace run lock as the screen while putting them in, so nothing changes while the screen runs (the same
  collectors reuse the screen; others are refused, asking to stop the screen first) and two installs never overlap. New cases
  IC-CF1 to IC-CF14. `ui.sh` only puts the verified collectors in a temporary place outside the workspace in the container; the
  copy into the workspace, the check and the install are done by the tool in one go under the lock (that place is checked by its
  real path and device/inode before and inside the lock).
- The demo's `extend.sh` also loads the first policy into the pqcota history of the judgment organization (`demo-corp`). The
  upstream demo's `demo.sh` loads into the default organization, and the generation step still uses that load. The demo follows the
  operator guide's contract ("load pqcota under the judgment organization too") from the start.
- A → B → A in the demo end to end (IC-U152): after moving to policy B, going back to A before the re-ingest is 409
  (`upstream-revert`) and changes nothing. Re-ingesting A adds no snapshot bodies and puts each node's latest back on the first A,
  and activating A passes; B is then refused as a past result. All of it runs in the one judgment organization, on real Postgres
  with real collection results.
- The demo end to end (IC-U152): it fetches the official assets and checks that `ui.sh` refuses a prepared folder with one byte
  changed, that 03 shows every collector and the JAR as "verified" with no allowance or unverified warning, and that the screen's
  command line and log carry no override.
- A generator that makes the expected h1 from `go.mod` and `go.sum` (`tools/genpin`) and its generated code
  (`pkg/inventory/collectorver/pin_gen.go`). `make generate` makes it and `make verify-generated` refuses a stale one.
- A PostgreSQL 16 service and `make test-pg` in GitHub CI: the Postgres contract tests (IC-S26 included) run without skipping. A
  missing DSN, a skipped test or IC-S26 not running fails it. The release gate requires `ci` to succeed on the candidate commit, so
  these tests are part of the release gate too.

### What was checked

- IC-S26 failed on a copy with only `go.mod` and `go.sum` taken back to those of v0.29.1 (upstream v0.13.0) and passed with this
  version. After A → B → A the latest of the two nodes stayed at B, so the check of A was `ErrRevert`. It also passed, without
  skipping, in the PostgreSQL job of the GitHub CI.
- The demo end-to-end check (IC-U152) ran with the five upstream repositories at v0.14.0. It fetched the official assets
  (linux/amd64) and checked them again, 03 showed the collectors and `collector.jar` all as "verified", and the screen's command
  line and log had no override. A → B → A in the one judgment organization passed too.
- The demo expected output (`plan.json` and `report.txt` in `demo/expected-output`) was rewritten by the v0.14.0 default demo and
  was byte for byte the same as that of v0.29.1. That agrees with the ruleset and the contract being the same as v0.13.0.
- The full vet (`go vet ./...`) passed on Linux, and a cross vet for macOS and Windows confirmed that the tests compile for them too.
- The check results are reported with the summary of the release gate (`make release-check`) when asking to publish.

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
