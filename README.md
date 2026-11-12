English · [한국어](README.ko.md)

# pqcaton

**Overview** · [Release notes](RELEASE_NOTES.md) · [Journey](docs/journey.md) · [Design](docs/developers.md) · [Acceptance criteria](docs/testcases.md) · [Demo](demo/README.md) · [Structure diagram](https://www.sntsoft.co.kr/pqcaton/)

**The place to decide what to change in a PQC migration.** [pqcota](https://github.com/randyinthedev-hash/pqcota) observes, and this
repository **reconciles those observations with the declaration, puts them in a review queue, and finalizes them.** This version is
used with pqcota **v0.11.0**. The pqcota version to use with each pqcaton version is in the [version table](docs/compatibility.md).

**There are two ways to use it.** This document describes **installing it yourself**. Observation results stay in your own
infrastructure and reconciliation, judging and finalizing happen in the same place, so nothing leaves. **Hosted**, observation results
leave the customer network for the control plane, and the part that sends them is [`saas/runner`](saas/runner/README.md). What it sends
and does not send is in a table in that document.

**Where to start reading.**

| What you want to do | Where to start |
|---|---|
| Run it once around | [Prerequisites](#prerequisites) → [Try it](#try-it) below |
| Follow the procedure from start to end | [Journey](docs/journey.md) |
| See why it is built this way | [Design](docs/developers.md) · [Acceptance criteria](docs/testcases.md) |
| Build from source, or look up an environment variable | [Build](docs/build.md) · [Environment variables](docs/environment-variables.md) |
| See what leaves when hosted | [`saas/runner`](saas/runner/README.md) |
| Send a fix | [CONTRIBUTING](CONTRIBUTING.md) · [CLA](CLA.md) ([한국어](CLA.ko.md)) |
| Report a vulnerability | [SECURITY](SECURITY.md) |

**Name**: *pqcaton* (pronounced **P-caton**) = **PQC** + **baton**. If pqcota is a *member* of the orchestra, this is the stick the
conductor holds. **The conductor is the operator, and pqcaton is the tool they use. People decide, not the tool.**

> **Source-available, not open source.** It is [BUSL-1.1](LICENSE). Free up to 5 managed nodes, a contract beyond that. **Each
> release converts to Apache-2.0 four years later.** → [License guide](docs/licensing.md)

---

## Why it is separate

pqcota **reports only what it observed.** 🔴 is not a verdict that something "is vulnerable" but an observation that it "negotiated a
classical algorithm", and that tool does not decide what to change or when. To keep that line, it **deliberately does not build**
declaration reconciliation, a review queue or finalize governance.

But in an organization someone has to make that judgment, and the judgment **must remain**: who decided what, when, on which evidence
is what gets audited. This repository takes that place.

```
pqcota                          pqcaton
  observe ─────► contracts/ ────►  reconcile with the declaration (three states)
  normalize                         confidence scoring
  approve run ◄── judged plan ◄──   review queue → judgment
    └─► generate changes
```

The dependency goes **one way**. pqcaton uses pqcota's public modules, and pqcota does not depend on pqcaton. pqcota is complete on its
own without this repository and can really be used that way.

## What is inside

| Module | What it does |
|---|---|
| [`pkg/inventory/reconcile`](pkg/inventory/reconcile) | **Three-state reconciliation**: CONFIRMED (declared ∩ observed) · UNDECLARED (observed only) · UNOBSERVED (declared only) |
| [`pkg/inventory/decision`](pkg/inventory/decision) | **Review–finalize state machine**: draft → in-review → finalized. Provisioning does not run before finalizing |
| [`pkg/inventory/review`](pkg/inventory/review) | **The session file format and the finalize gate**: the commands and the screen share this one format |
| [`pkg/inventory/decl`](pkg/inventory/decl) | **The declaration format and its self-check**: if node↔IP is wrong, the reconciliation result is wrong **without any error**. It is caught before saving |
| [`pkg/inventory/report`](pkg/inventory/report) | **Multi-node reconciliation**: lane separation, joining IPs, completeness. The commands and the screen use this one computation |
| [`pkg/inventory/ui`](pkg/inventory/ui) | **The screen**: it only draws and reads forms. Where it reads from and who connects is decided by the caller |
| [`pkg/inventory/scope`](pkg/inventory/scope) | **Asset-scope governance**: layer inheritance, change approval, re-review of exclusions. The rule format and enforcement are pqcota's as they are |
| [`pkg/inventory/localscan`](pkg/inventory/localscan) | **A shortcut that scans this machine**: a place to run the whole procedure once without observation. What it could not see is not reported as "absent" |
| [`inventory/cmd/pqcaton-decide`](inventory/cmd/pqcaton-decide) | **Judging and closing the review queue**: it makes the judged plan in the contract format, with the status `IN_REVIEW` and the approval field empty. Approving execution is not this repository's job |
| [`inventory/cmd/pqcaton-scope`](inventory/cmd/pqcaton-scope) | **Approving and distributing "do not look at this asset"**: the finalized policy becomes the input of pqcota's enforcer |
| [`inventory/cmd/pqcaton-ui`](inventory/cmd/pqcaton-ui) | **Seven screens people use**: declaration · cryptographic asset scope · observation · reconciliation · judgment · approval and playbook generation · inventory lookup. **It opens only on this machine's loopback**, and styles and scripts are inside the single binary, so it starts on a machine without a network too |
| [`inventory/cmd/pqcaton-report`](inventory/cmd/pqcaton-report) | Governance report and topology |

**UNDECLARED is the first use of this tool.** An edge that is not in the CMDB but is really communicating, that is, a connection the
organization does not know about. That is where security should look first.

**UNOBSERVED is not decided by the machine.** Whether something declared but not observed *exists but was not seen* or *is already gone*
only people know. pqcota's completeness map tells "unobservable in principle" from "really absent", and people decide on top of it.

The **structure diagram** is at [www.sntsoft.co.kr/pqcaton](https://www.sntsoft.co.kr/pqcaton/) ([source](docs/index.html)).

## Prerequisites

| | Needed | Why |
|---|---|---|
| **pqcota** | For observation, approval and generation | This version is used with pqcota **v0.11.0** (all upstream repositories at the same tag). With another version the evidence of plans is not found in the upstream history. The combinations for each pqcaton version are in the [version table](docs/compatibility.md) |
| **Go** | **Required** | For building. The version needed is in [`go.mod`](go.mod) (1.26.11 now). No other runtime is needed |
| **Graphviz** (`dot`) | **Optional** | Only to see the governance topology as a picture. Without it the screen and the commands show the **DOT source**, which can be drawn anywhere later |
| Postgres | Optional | Only to keep the judgment ledger in a DB. The default is a file (JSONL), so the whole procedure works without it |
| Docker · the pqcota repository | Only for the demo | See [demo/README.md](demo/README.md) |

Installing `dot` is described at [graphviz.org/download](https://graphviz.org/download/):
`apt install graphviz` · `brew install graphviz` · `winget install graphviz`.

**Where it runs.** This repository does the work of the **ctl node** (where observation results are gathered, reconciled and judged).
The ctl node **runs on any OS.** Observation itself is done on the target nodes by pqcota's collectors, which handle Linux and Windows
(Windows observation from upstream v0.6.0).

> There is one exception. `pqcaton-decide open` without `-results` **scans the machine running the command** (`/proc`). It is a
> shortcut for "around once with only a checkout" and works **only on Linux**. To handle several nodes properly, use
> [`pqcaton-report`](inventory/cmd/pqcaton-report), which reads the results pqcota gathered.

## Try it

```bash
make            # license · text · style · docs · case · format gates → build → test
```

**You can go from start to end with this repository alone.** The target to observe is this machine. **This shortcut reads `/proc`, so
it works only on Linux.** On macOS and Windows, read the `results/` pqcota gathered, as the comment in ② below shows.

```bash
go build -o bin/ ./inventory/cmd/...

# ① declaration — write down "this is what we know". Extract it from a CMDB or write it by hand
printf 'node,runtime,component\nlocal,openssl,libssl\nlocal,jca,jca-provider-chain\n' > decl.csv

# ② reconciliation — scan this machine, match it with the declaration, and make the review queue as a session file
bin/pqcaton-decide open decl.csv local > session.json

#    several nodes are handled like this — reconcile with the observations pqcota gathered
#    bin/pqcaton-decide open declaration.json -results results/ -scope-assets scope-assets.csv -org acme > session.json
#    (-scope-assets: the same file given to pqcota-ingest. The policy must be the same for the fingerprints to match upstream)

# ③ judgment — the place for people. Open session.json and
#    fill in the conclusion of the mandatory items, and reviewer · signature
#    set `include_in_plan` to true for items to put in the finalized plan

# ④ finalize — passes only with every mandatory judgment and the approval signature,
#    and judgments stay append-only (the audit record)
#    the session, plan and judgment ledger are written at once. Finalizing the same session again writes nothing again
bin/pqcaton-decide close session.json -o plan.json -judgments judgments.jsonl -org acme

# ⑤ after observing again — look again only at judgments whose evidence changed (not a full re-review)
bin/pqcaton-decide delta judgments.jsonl decl.csv local -org acme
```

**If filling JSON by hand is tedious, fill it on the screen.** The same files, the same gates.

**Give only the files you have and the screen builds the session too.** No need to run a command first. **The screen does not start
without the pqcota history.** There must be `PQCOTA_DSN` in the shell that starts it or `pqcota.env` in the user configuration folder
(the connection of the pqcota history; read permission is enough), and connecting and one lookup must pass within 10 seconds. The
connection is left nowhere (operator guide section 2, "pqcota connection"). Activating a policy and adopting a baseline in 05 always go
through the pqcota history reconciliation.

```bash
bin/pqcaton-ui session.json \
  -decl declaration.json -results results/ \
  -layers corp.csv,prod.csv -base asset-scope.csv \
  -judgments judgments.jsonl -org acme
# → http://127.0.0.1:8765 — the six step cards at the top are the order of the procedure
#   01 declaration (managed nodes and their cryptographic assets and edges) → 02 cryptographic asset scope
#   → 03 observation (access information, running observations) → 04 reconciliation (three states, grades, topology)
#   → 05 judgment (review queue — judging and finalizing) → 06 approval and generation (approval signature, playbooks)
```

**Each card shows the current state of its step.** The two steps that finalize (scope · judgment) also show the finalize state
(unfinalized · finalized · superseded · recovery needed). The inventory lookup is next to the cards. The screen works at 320px wide and
with the keyboard too. The old screens of v0.21 and earlier went away in v0.23.0, and the old addresses (`/classic/…` · `-next`) lead
to the current screens.

**To use it without a terminal, start it with no arguments** (`pqcaton-ui`). It uses the `pqcaton` workspace in your home folder and
opens a browser. The declaration and layers are made on the screen too. Installation, service setup, backup, recovery and the error
table are in the [operator guide](docs/operator-guide.md) ([한국어](docs/operator-guide.ko.md)).

**The screen opens only inside this machine.** The screen has no login, so it does not start if `-addr` is not loopback. To see it from
another machine, forward the port over SSH and open it in that machine's browser.

```bash
ssh -L 8765:127.0.0.1:8765 <machine running the screen>
# → http://127.0.0.1:8765 on your machine
```

Without `session.json`, **the screen builds the session from the declaration and the observation results.** So does the asset scope when
layer CSVs are given. And **rules are edited on the screen.** What the five columns mean is in the "How to write rules" help, and
`action` is a choice, so a typo cannot bend a rule.

You can also make the session with a command first. The same files and the same gates.

```bash
bin/pqcaton-decide open declaration.json -results results/ -org acme > session.json
bin/pqcaton-scope  open corp.csv prod.csv -base asset-scope.csv -org acme > scope-session.json
```

**Choose Korean or English for the screen.** The toggle at the right end of the navigation changes only the language, keeping your
place, and the choice is remembered. At first it follows the browser setting.

> **Command output and logs are in English only.** They get pasted into searches and issues; if the languages differed, one problem
> would be left as two sentences. The detailed rules are in [CONTRIBUTING.md](CONTRIBUTING.md#which-language-to-use).

`-decl`, `-layers` and `-results` **open only the tabs for what is given.** With only a declaration file, only "Declaration" and
"Judgment" show. It does not offer what is not there.

**What to keep watching also goes through approval.** An asset taken out of the inventory must later answer "why was this not looked
at", so judging and finalizing are handled together in the same procedure.

Layers stack in the order given: organization, environment, node group. **When several rules apply to the same asset, the later layer's
rule applies.**

```bash
# stack the layers and put only the changed rules up for review (give the current policy with -base for the delta only)
bin/pqcaton-scope open corp.csv prod.csv pay.csv -org acme > scope-session.json

# approve — adding an exclude does not finalize without a conclusion
bin/pqcaton-scope close scope-session.json -o asset-scope.csv -judgments judgments.jsonl -org acme

# the CSV that comes out is the input of pqcota's enforcer as it is
pqcota-ingest -scope-assets asset-scope.csv results/

# an exclusion is not a permanent exemption — only those without approval or with an old one come up again
bin/pqcaton-scope review asset-scope.csv results/ -judgments judgments.jsonl -org acme
```

**③ judges per policy.** Writing one conclusion per policy in the session file's `policy_decisions` judges all items of that policy at
once. A review that looks at thousands of machines one by one never ends. An individual `conclusion` is the place for exceptions.

**④ is the gate this repository must pass.** If anything is empty it does not finalize and **tells you what is left.**

```
❌ cannot finalize: mandatory items are still unjudged — every mandatory item must be judged
   · the signature is not filled in
   · no record of why this was decided: local/openssl/libssl-e2f2d68a (UNDECLARED)
```

> **Command output is in English.** Choose Korean or English for the screen. See ["Which language to use" in
> CONTRIBUTING.md](CONTRIBUTING.md#which-language-to-use).

The `plan.json` that comes out is **in the contract format as it is.** We have no format of our own, so pqcota reads it as it is.

**The plan carries its evidence.** Each action states in `evidenceSources` which finding came from which snapshot state. The reference
holds the snapshot's **content fingerprint** (`pqcota-snapshot-content/v1`) and the **source node** written in the envelope, and the
source node may differ from the declared name. With `--dsn`, upstream really looks it up in the history and records it, and if it is not
found the exit status is 3. **For that, this repository must build the same snapshots as the upstream ingest.** It gathers the results per
source node, normalizes them with the same function as upstream, and **gives `-scope-assets` the same file given to `pqcota-ingest`**.
Normalizing with another policy (or none) changes the fingerprints and the trace fails. Whether there is a policy is not guessed.

**Judging and approving execution are different steps.** What this repository issues is **a plan whose judging is done**, and in the
contract's status it is `IN_REVIEW`. The approval field (`approvalSignatures`) and the finalize time go out empty. Both belong to
approving execution, and upstream's `pqcota-approve` fills them when it signs with the approver's key and raises the plan to
`FINALIZED`. The signature the reviewer writes here records **"who is recorded as having judged"**; it is not execution approval. So one
approval step sits in between. That is not a gap left unjoined but **the boundary between the person who judges and the person who
approves execution**. Joining them would not remove the step: upstream always verifies with the key it registered, so only who holds the
private key moves.

**Approving on the 06 screen makes this screen hold that private key.** The screen reads the approval key file (`pqcaton/approval.key` in
the user configuration folder, or the file given with `-approval-key`) and signs with the same upstream function as the commands above, so
judging and approval happen on one machine by one user. An organization that must separate the person who judges from the person who
approves does not use 06 and runs the commands below on the approver's machine.

```bash
# all three below are commands of the pqcota repository.
eval "$(pqcota-keygen | grep '^PQCOTA_')"
PQCOTA_APPROVAL_KEY="$PQCOTA_SIGN_KEY" \
  pqcota-approve --approver reviewer-1 plan.json > plan.signed.json
PQCOTA_APPROVAL_KEYS="reviewer-1=$PQCOTA_VERIFY_KEY" \
  pqcota-provision --level l2 plan.signed.json > provision.yml
```

If a plan does not fill every execution field, pqcota still issues the output but **does not end in success** (exit status 3), and the
warning names what is empty. But it does not get that far: the delegation level and action kind, the target algorithm and the L3
activation hook are **chosen on our screen, and finalizing is blocked while they are empty.** Stopping it here, before the approval
signature is added, is better than passing empty values upstream to be caught there.

**Matching versions.** The pqcota version to use with each pqcaton version is in the [version table](docs/compatibility.md). This
repository is built with pqcota **v0.11.0** (the four `pqcota-common`, `pqcota-inventory`, `pqcota-discovery` and
`pqcota-provisioning` at the same tag), and `go.mod` pins that version. The end-to-end (demo) check was done with v0.11.0 (the five
repositories taken side by side at the same tag). The commands above are for v0.11.0 too. **The upstream version must match.** This
repository normalizes observation results with the v0.11.0 ruleset built into it (`pqcota-enrich/v3`, the same as v0.10.6 and v0.10.7)
and carries those snapshot fingerprints as the evidence of plans. So upstream's `pqcota-ingest` and `pqcota-provision` must use the same
ruleset for the evidence to be found in the history.

| pqcota | Status | Why |
|---|---|---|
| v0.11.0 | **Supported · checked end to end** | The demo was run to the end with the five repositories at this version |
| v0.10.7 | **History shared, upstream tools at v0.11.0** | The ruleset and snapshot fingerprints are the same, so evidence is found in the upstream history. But records v0.11.0 writes cannot be read by the v0.10.7 `pqcota-records`, and plans carrying source evidence or a guide are refused by the v0.10.7 `pqcota-approve` and `pqcota-provision`, so keep the upstream tools at v0.11.0 |
| v0.10.6 | **History shared, approval and generation at v0.11.0** | The ruleset and snapshot fingerprints are the same as v0.10.7, so evidence is found in the upstream history. But a v0.10.6 approval signature could pass for another plan when the plan's strings contained a NUL (upstream v0.10.7's security fix), so keep `pqcota-approve` and `pqcota-provision` at v0.11.0 |
| v0.10.5 and earlier | **Cannot be used with the upstream history, approval and generation** | The ruleset is `pqcota-enrich/v2`, so the same collection results give different asset IDs and snapshot fingerprints, and the evidence of plans this repository issues is not found in that history. If you run upstream at v0.10.5 or earlier, use pqcaton [v0.24.1](https://github.com/sntsoftgit/pqcaton/tree/v0.24.1) |

How far versions must match differs per layer.

- **Collection result format**: reading v0.10.5 collection results was confirmed in the mixing check (the v0.10.5 demo). Older versions
  were not in scope this time. `pqcota-jvmscan` and `collector.jar` before v0.10.6 do not carry the list of PQC services per provider
  (upstream v0.10.6's note), so JCA readiness becomes `unknown`. The 03 and 04 screens report such nodes as "the JCA results cannot be
  confirmed as v0.10.6 collection results" (without asserting the version).
- **Joining the upstream history, approval and generation**: keep `pqcota-ingest`, `pqcota-approve`, `pqcota-provision` and
  `pqcota-records` at v0.11.0.
- **Demo**: pinned to the checked combination, the five repositories at v0.11.0 ([demo guide](demo/README.md)).
- **Operating advice**: if you run the upstream programs too, keep the five repositories at the same tag, v0.11.0, and replace the
  collectors and `collector.jar` on the nodes with them. Upgrade `pqcota-records`, which reads records, before v0.11.0 writes new
  records. `pqcota-jvmscan` on the nodes must be v0.10.7 or later; earlier versions wrote the result in several lines on a node with two
  or more JVMs, and the whole ingest request holding that node was refused (upstream v0.10.7's fix).

**One judgment goes to every installation under its asset key.** The asset key is `node/runtime/component`, so the same OpenSSL seen at
another path becomes two installations under one item. The judgment screen shows all paths and asks to confirm "this action applies to
every installation below" before finalizing. Different actions per installation cannot be expressed in this version.

**The plans this version issues carry evidence per action (`evidenceSources`), so `pqcota-approve` and `pqcota-provision` v0.8.0 or
earlier, which do not know that field, cannot read the plan and refuse it.** The `IN_REVIEW` status is accepted from v0.8.0; below that
only the signature is added, the plan stays `IN_REVIEW`, and the generator refuses it as "not finalized". Besides, approval verification
closed by default and exit status 3 come from v0.7.0, the controller CLI attached to releases from v0.7.2, and the hardening ruleset as a
constant from v0.7.3 (end to end through receiving external CBOM from v0.7.4).

How to sweep several nodes and the governance topology are in the [journey](docs/journey.md).

The demo goes on top of pqcota's discovery demo. See [demo/README.md](demo/README.md).

## License

Copyright 2026 SNT Soft Co., Ltd. The copyright holders are listed in [AUTHORS](AUTHORS).

[**BUSL-1.1**](LICENSE) · **each release converts to Apache-2.0 four years after its publication.** The conversion date of each version
is in the [release notes](RELEASE_NOTES.md) (v0.1.0 to v0.5.0: 2030-08-11).

- Evaluation, development and testing are free without a size limit
- Production is free up to 5 managed nodes
- Beyond that, a contract: **kty@sntsoft.co.kr**

Details are in [licensing.md](docs/licensing.md). Before contributing, please read [CONTRIBUTING.md](CONTRIBUTING.md) (the [CLA](CLA.md) is
required). Report security vulnerabilities through the routes in [SECURITY.md](SECURITY.md), not in issues.

> **Languages of the documents**: documents are in English by default with Korean pairs (`*.ko.md`), and the first lines of the two
> editions point to each other. Code, command output and file formats are in English. For the CLA, the Korean original
> ([CLA.ko.md](CLA.ko.md)) governs.

The base project pqcota is Apache-2.0, and the attribution notice is in [NOTICE](NOTICE).

---

SNT Soft Co., Ltd.
