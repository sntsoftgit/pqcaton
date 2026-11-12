English · [한국어](developers.ko.md)

# Design: inventory governance

[Overview](../README.md) · [Release notes](../RELEASE_NOTES.md) · [Journey](journey.md) · **Design** · [Acceptance criteria](testcases.md) · [Demo](../demo/README.md) · [Structure diagram](../README.md#how-it-fits-together)

pqcota **observes**, and this repository **carries those observations through to judgments.**

pqcota reports only the facts it observed. It does not judge what is dangerous or what to change first.
[What that repository explicitly leaves out](https://github.com/randyinthedev-hash/pqcota/blob/v0.9.1/docs/architecture.md) is
declaration (CMDB) reconciliation, confidence scoring, the review queue and finalize governance, and this document covers them.

> **§ notation**: unless stated otherwise, section numbers of the pqcota
> [regulation (v0.9.1)](https://github.com/randyinthedev-hash/pqcota/blob/v0.9.1/docs/regulation.md).

> **In which order to run the commands, and what goes in and comes out, is in the [journey](journey.md).** This document answers only
> **why it was built that way**.

---

## 0. Why the screen does this work

### Sessions come from input files: from commands and from the screen alike

Review sessions and scope sessions **are derived from input files**. If the input files exist, both the commands and the screen can
build them. Having to run a command once before the screen opens, when the input files already exist, contradicts the reason for having a
screen (v0.12.0). **Which tab comes from what is in the table in the [journey](journey.md).**

**The session file holds what people wrote**: judgments, approvals and signatures. When the input files change, the session is built
again, and the judgments written follow along by identity (`RuleID` for rules, `ID` for items).

> **But what was not seen is not covered by an approval.** When **an unseen change appears in a layer or policy, that batch conclusion
> is cleared.** A batch judgment is "a conclusion drawn from what was seen", and nobody has seen the new change. Left as it is, an
> UNDECLARED that just appeared or an `exclude` just added would pass finalizing **carrying evidence nobody approved**. When the policy or
> the queue changes, **the signature is cleared too** (the approver's name stays; that is a person, not a confirmation).

Sessions can also be built with commands (`pqcaton-scope open` · `pqcaton-decide open`). The building code lives in one shared package
(`review.FromResults` · `scope.NewSession`), so the commands and the screen see the same queue. Computing it separately would make the
UNDECLARED seen on the screen differ from what is judged.

Once finalized, what remains is the output (`asset-scope.csv`, `plan.json`) and the judgment ledger.

### Asset-scope rules are edited on the screen

The five columns of a layer CSV (`action,runtime,lib,app_key,note`) are **pqcota's format as it is**. Asking people to write it by hand
means teaching the columns' meaning, glob and layer precedence in a document, and making that document costs more than making an editing
screen. **So the explanation is folded into the "How to write rules" help, and the table is edited on the screen.**

- **It edits the layer files, not the merged result.** Writing to the merged result loses whether a rule came from the organization or a
  node group, and at the next review nobody knows whom to ask.
- **`action` is a choice.** A typo cannot bend a rule.
- **A row with all three columns empty does not become a rule.** pqcota reads an empty column as `*`, so making it would give
  `exclude,*,*,*` and **empty the whole inventory.** To mean "everything", write `*`.

The layer **composition** (which files stack in which order) is organization structure, so it is set on the command line. The order of
`-layers` is the inheritance order.


---

## 1. The inventory engine: reconciliation, judgment, governance

pqcota gathers observations up to a **read-only view** and the `plan` and `decision` schemas. The engine that decides what to finalize on
top of that is here.

### 1.1 The reconciliation engine: three-state reconciliation (regulation §3.3①)
Each communication edge is classified by declared ∩ observed. The deterministic part is AUTO, and the ambiguous part is demoted to MANUAL.

| State | Definition | Grade | Meaning |
|---|---|---|---|
| **CONFIRMED** | declared ∩ observed | AUTO | Highest confidence |
| **UNDECLARED** | observed only | AUTO | **Communication the organization does not know about.** Security looks here first |
| **UNOBSERVED** | declared only | **MANUAL** | It must be told whether it exists but was not seen (something that runs now and then, such as disaster recovery or batches), is already gone, or cannot be seen in principle. The machine cannot decide |

- The key to UNOBSERVED: pqcota's **completeness map** (not observed ≠ absent) is decisive. It separates "not observed" meaning "really
  absent" from "cannot be seen in principle (a gap)". A gap means collect again; otherwise a person judges.
- Input: observed edges (network-collector TLS grades + CBOM) and declared edges (declaration-importer). evidence_strength propagates
  into confidence.

**Reconciliation is bound to an organization.** The engine opens for one organization (`reconcile.For`), and if even one asset or edge of
another organization is mixed into the input, it **stops without reconciling.** Neither pqcota snapshots nor the contract's `Envelope`
have an organization field, so the engine cannot find it out itself; the caller asserts the organization, and the engine catches the
moment that assertion goes wrong.

The reason this check is needed is that mixed input left as it is gives **not an error but a plausible result**. The organization is part
of identity, so identifiers do not match, the same asset splits into an UNDECLARED and UNOBSERVED pair, and it shows up in the review
queue as a newly appeared UNDECLARED. The subject id in the judgment ledger does not carry the organization: the ledger is already split per
organization, so it would be redundant, and adding it would misalign the subjects of every judgment stored before.

### 1.2 Confidence scoring (§3.5)
```
confidence = f(observation frequency, observation period, declaration freshness, source agreement, evidence_strength)
```
AUTO and deterministic (a derived view). It is the input for review queue priority and selecting auto-pass candidates.
`evidence_strength` (confirmed/inferred-*) is a first-class weight. inferred-low lowers the confidence ceiling.

### 1.3 The review queue (§3.3②): PROPOSE
The reconciliation engine structures what to judge, not the answer.
- **Auto-pass candidates**: CONFIRMED + high confidence + low risk → a batch approval bundle (people approve).
- **Mandatory individual review**: UNDECLARED, low-confidence UNOBSERVED, anything that touches legacy, edges whose change would break the
  other side.
- **Priority = risk × blast radius × data sensitivity.**

### 1.4 Review-finalize state machine (§3.3③): MANUAL
```
draft ──▶ in-review ──▶ finalized
                         └ every mandatory item judged + the judge's signature (precondition)
```
- **No plan is made before finalized** (§5, a gate that must be passed). Partial finalizing per ring or domain is allowed.
- **This repository's `finalized` is not the contract's `FINALIZED`.** Here finalized means **the judgment session is closed**. The
  contract's `FINALIZED` means **execution approval is done too**, which happens only when an upstream approver signs with their key
  (`pqcota-approve`). Judging and approving execution are different steps, so this repository issues the judged plan as the contract's
  **`IN_REVIEW`** and **leaves the approval field (`approval_signatures`) and the finalize time (`finalized_at`) empty.** Both belong to
  approving execution.
- **The judge's signature is not verified.** It is a free string the reviewer writes by hand, so it means only **"who is recorded as having
  judged"**, not "who judged". The weight of that record comes from the judgment ledger being append-only. Earlier this string was put in
  the contract's approval field, which made a value that proves nothing **look like it satisfies the upstream structural gate's "an approval
  exists"**. It is no longer put there.

| | What it answers | What it rests on |
|---|---|---|
| This repository's judge signature | Who is recorded as having judged | Not verified. The ledger being append-only is the basis |
| The upstream approval signature | Who approved execution | The approver's public key registered with the controller |

- **The session id joins the plan and the ledger.** A UUID is made when a session opens and saved in the session file; reopening keeps it
  (a new session differs even with the same contents). The id of the plan going out in the contract is `pqcaton:<scope>:<session id>`,
  and judgment ledger rows carry the same value, traced back with `BySessionID`. It answers "which plan event", so it is not a content
  hash: the same contents can be approved and run twice, and their audit records must not collide then. A session without a session id
  (opened by an old build) is not finalized and is asked to be reopened; minting one here would make the plan point to a session the
  ledger does not have.
- **Review unit (§3.4, hybrid)**: per policy by default (version × link mode and JDK × provider catalogs are the policy templates under
  review, so assets of a kind are seen in batches). Individual isolation is the exception; only policy exceptions, high risk and UNDECLARED
  edges are seen per edge.

### 1.5 Persisting judgments (§3.6)
- A judgment is "a person's conclusion", not an edge state, so it stays attached across re-collection (evidence is tracked with
  `Decision.BasisHash`).
- **Invalidation trigger**: when the underlying evidence really changes, only that judgment gets a re-review flag. That is the **delta
  review** (not a full re-review).
- Old judgments expire: confidence is reduced and they are confirmed again periodically. This judgment history is **the human branch**
  of the provenance chain (§1.3).

> The schemas (`Decision`, `FinalizedPlan`, `ReconState`) have their SSOT in
> [pqcota-common's contracts](https://github.com/randyinthedev-hash/pqcota-common/tree/v0.10.4/contracts).
> This repository uses that vocabulary as it is and builds only the engine above.

---

### 1.6 Asset-scope governance

pqcota has the **mechanism**: `scope.AssetPolicy` (CSV rules, glob). Even with a node registered, the user declares **what inside it to keep
managing**, and `pqcota-ingest -scope-assets` enforces it before ingest. The excluded part is counted in `Snapshot.ExcludedByScope` and
reported together (excluded ≠ absent, §2.6). If noise cannot be filtered, the inventory itself becomes useless, so the observation tool
must have this itself. It does not mean nothing can be done with this repository alone without pqcota; it is the opposite: **pqcota alone
can finish the whole process.**

What this repository adds on top is **governance and scale**:

| Axis | What this repository adds | Why only an organization needs it |
|---|---|---|
| **Review-finalize** | Puts scope changes (an exclude added, above all) through [the review-finalize state machine of §1.4 in this document](#14-review-finalize-state-machine-33-manual): propose → review → approve | The decision "we do not look at this asset" **is audited**. Alone it is your own responsibility, but an organization must leave the evidence and the approver |
| **Audit trail** | Records who excluded what, when and why, and what fell out then, in [persisting judgments of §1.5 in this document](#15-persisting-judgments-36) | After an incident one must answer "why was this not in the inventory" |
| **Policy inheritance and batches** | Layered policies organization → environment (prod/dev) → node group, applied to thousands of machines at once, finding what is off | One CSV works up to 20 machines, but at 5000 it cannot be managed |
| **Re-review of exclusions** | Sweeps the excluded assets again periodically and puts "what became dangerous while left out" in the review queue | An exclusion is not a permanent exemption. This judgment is the extension of §1.1 reconciliation in this document |

**Boundary in short**: **defining and enforcing** rules is pqcota; **approving, auditing and operating rule changes at scale** is here. This
repository does not reimplement pqcota's `AssetPolicy`; it **generates and distributes** it. Policies governance finalized go down as CSV
and contracts, and pqcota's enforcer uses them as they are.

**Implementation status**: in [`pkg/inventory/scope`](../pkg/inventory/scope) and [`pqcaton-scope`](../inventory/cmd/pqcaton-scope),
and the screen's "② Cryptographic asset scope" tab uses the same.

Inheritance is **appending rules**. pqcota already decided that "the last matching rule decides", so putting the upper layers first and
the lower ones after gives lower-layer precedence as it is. **The judging rule lives in one place only.** If we added locks of our own,
the result pqcota enforces from the CSV we send down would differ from our screen. It also means a lower layer can turn an upper exclusion
back with `include`; the place that stops that is not the inheritance rule but **the review-finalize gate**. Lower policy changes go
through approval too.

**Only adding an `exclude` requires a reason.** Withdrawing an exclusion widens the inventory, so it weighs differently. And pqcota counts
the excluded part **as a number** (`ExcludedByScope`). That is enough to filter noise, but answering "why was this not in the inventory"
after an incident needs **names**. So `review` calls pqcota's `Managed` as it is and reports what fell out by name.

---

## 2. Organization isolation

The inventory is **a map of one organization's cryptographic assets**. Another organization's assets mixed in is an incident in itself.
Whether splitting affiliates or handling several customers together, the boundary **must be explicit in the data, not in the memory of the
person writing queries.**

### 2.1 Making isolation impossible to turn off

**The organization type is pqcota-common's `pkg/org` as it is.** The rule stays the same: **a store cannot be opened with an empty
organization.**

This repository used to have its own package of the same name, but when pqcota v0.2.0 added an organization axis to the inventory there
were **two different types of the same name**. The control plane holds both stores (judgments here, the inventory in pqcota), so every
boundary needed a conversion and left places where it was unclear which validation rule applied. pqcota's was the superset, so we moved
to it.

**But `Resolve` is not used.** pqcota's `Resolve` turns an empty value into the default organization, and in this repository an empty
organization is not a situation to accept but a bug. Only `Parse` is used.

With the types the same, the stores satisfy pqcota's `org.Scoped` as they are. When the control plane starts it can check by **reading the
organization from the handle itself**. Environment variables differ per process, so reading it from the real handle is safer. A test keeps
that property at compile time.

The store handle is bound to an organization. Every query of the handle `NewPgJudgmentStore(ctx, dsn, org)` returns already has
`WHERE org = $1`, and **there is no way to drop it.** There is nothing to remember per query, so nothing to forget.

| | |
|---|---|
| Index | `(org, subject, seq)`. org leads, so every lookup filters by organization first |
| In-memory store | **The same rules** as the Pg one. If tests took a path without isolation, they would differ from reality |
| Identifier shape | Lowercase letters, digits and hyphens. Mixing case would make `Acme` and `acme` different organizations and split the data in two without an error |

### 2.2 Row-level security: one layer the DB enforces

Adding `org` to every query rests on **the premise that we make no mistakes**. In a deployment where several organizations share one
database, everything cannot hang on that one premise; one day a query missing the condition comes in. So the judgment ledger has
Postgres RLS.

```sql
ALTER TABLE pqcota_judgments ENABLE ROW LEVEL SECURITY;
ALTER TABLE pqcota_judgments FORCE ROW LEVEL SECURITY;
CREATE POLICY pqcaton_org_isolation ON pqcota_judgments
    USING      (org = current_setting('pqcaton.org', true))
    WITH CHECK (org = current_setting('pqcaton.org', true));
```

The organization is set **as a session variable per connection**. One handle is one organization, so per session is enough, and there is
nothing to remember per query.

There are three traps, recorded here.

| Trap | How it was handled |
|---|---|
| **Without `FORCE`, the table owner is exempt** | Applications usually connect as the owner, so without that one line the policy is in place and nothing happens |
| **Superusers and `BYPASSRLS` roles skip the policy entirely** | Measured once on open and kept in `RLSActive()`. With `PQCATON_REQUIRE_RLS=1` it **does not open.** The same shape as pqcota's `PQCOTA_REQUIRE_SIGNATURE` |
| **Without `WITH CHECK`, only reads are blocked** | Rows written under another organization's name get in, and those rows are invisible to us. It is the hardest form to deal with, so writes are blocked too |

**The application does not connect as the table owner; that is the premise.** Then that connection normally has no DDL rights, so the
store moves on if the schema is already in place, and otherwise **says what to run as the owner and stops.** In a deployment, run the
following once with owner rights.

```sql
CREATE ROLE pqcaton_app LOGIN PASSWORD '...' NOSUPERUSER NOBYPASSRLS;
GRANT USAGE ON SCHEMA public TO pqcaton_app;
GRANT SELECT, INSERT ON pqcota_judgments TO pqcaton_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO pqcaton_app;
```

### 2.3 What is not done yet

- **Authentication and permissions**: this layer does not decide who belongs to which organization. It receives an organization the
  caller has already checked.
- **Authorization (who may approve what)**: this layer does not decide who within an organization is entitled to finalize what. It takes
  and keeps the signature but does not ask whether that person is entitled.

---

## 3. Handing the plan to pqcota

This section used to be in the README; it explains what the finalized plan carries and where execution approval happens.

**Judging per policy.** Writing one conclusion per policy in the session file's `policy_decisions` judges all items of that policy at
once. A review that looks at thousands of machines one by one never ends. An individual `conclusion` is the place for exceptions.

**Finalizing is the gate this repository must pass.** If anything is empty it does not finalize and **tells you what is left.**

```
❌ cannot finalize: mandatory items are still unjudged — every mandatory item must be judged
   · the signature is not filled in
   · no record of why this was decided: local/openssl/libssl-e2f2d68a (UNDECLARED)
```

> **Command output is in English.** Choose Korean or English for the screen. See
> [CONTRIBUTING.md, "Which language to use"](../CONTRIBUTING.md#which-language-to-use).

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

**One judgment goes to every installation under its asset key.** The asset key is `node/runtime/component`, so the same OpenSSL seen at
another path becomes two installations under one item. The judgment screen shows all paths and asks to confirm "this action applies to
every installation below" before finalizing. Different actions per installation cannot be expressed in this version.

**The plans this version issues carry evidence per action (`evidenceSources`), so `pqcota-approve` and `pqcota-provision` v0.8.0 or
earlier, which do not know that field, cannot read the plan and refuse it.** The `IN_REVIEW` status is accepted from v0.8.0; below that
only the signature is added, the plan stays `IN_REVIEW`, and the generator refuses it as "not finalized". Besides, approval verification
closed by default and exit status 3 come from v0.7.0, the controller CLI attached to releases from v0.7.2, and the hardening ruleset as a
constant from v0.7.3 (end to end through receiving external CBOM from v0.7.4).
