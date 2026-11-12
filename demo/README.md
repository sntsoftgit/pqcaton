English · [한국어](README.ko.md)

# Demo: governance on top of pqcota

[Overview](../README.md) · [Release notes](../RELEASE_NOTES.md) · [Journey](../docs/journey.md) · [Design](../docs/design.md) · [Acceptance criteria](../docs/testcases.md) · **Demo** · [Structure diagram](https://www.sntsoft.co.kr/pqcaton/)

📊 **The expected results before you run it** are in [`expected-output/`](expected-output/): a sample of the extension report and the
governance topology, with an explanation of what may differ.

## The environment this demo builds

Written for someone seeing the pqcota demo for the first time. **Three nodes imitating a payment service** come up in Docker, and
observation happens on them.

| Node | What runs | Why it is set up that way |
|---|---|---|
| `web-gw` | Recent OpenSSL 3 · client | The side that sends traffic. The client decides the SSH grade |
| `pay-app` | JVM (JCA providers registered at run time) | **Not visible to a static scan**; caught only by attaching |
| `pay-db` | Legacy OpenSSL 1.1.1 · server · two applications share one libssl | How far quantum vulnerability spreads (blast radius) |

The network is split in two: `corp` (where web, application and DB meet) and `db` (an isolated tier). So some communication is
observed and some is invisible in principle. **That difference separates "absent" from "not seen."**

The environment itself is defined by one file, pqcota's `demo/topology/topology.yaml`. Changing it changes the nodes and the network,
and **the declaration is made from it, so there is nothing to change on our side** (below).

## What it adds

This demo is **an extension, not a separate stack**. It starts [pqcota's discovery demo](https://github.com/randyinthedev-hash/pqcota/tree/v0.11.0/demo)
as it is, and on top of it adds this repository's features: **three-state reconciliation against the declaration
(CONFIRMED/UNDECLARED/UNOBSERVED) + the review queue + the governance topology**.

```
Observation grades (pqcota)      →   + reconciliation against the declaration (pqcaton)
🟢 web-gw→pay-app MLKEM                🟢 web-gw→pay-app  TLS  CONFIRMED
🔴 web-gw→pay-db  classical            🔴 web-gw→pay-db   TLS  CONFIRMED
🟢 web-gw→pay-app SSH sntrup761        🟢 web-gw→pay-app  SSH  UNDECLARED ← undeclared communication!
🔴 web-gw→pay-db  SSH curve25519       🔴 web-gw→pay-db   SSH  UNDECLARED
                                       ⚪ pay-db→pay-app  TLS  UNOBSERVED (declared but not observed ≠ absent)
```

## Running it

```bash
# 1) start the OSS discovery demo first (environment + collection)
../../pqcota/demo/scripts/up.sh
../../pqcota/demo/scripts/demo.sh

# 2) extend it with pqcaton's reconciliation and topology
./scripts/extend.sh          # output: demo/topology-governance.svg

# 3) (optional) see the same demo on the screen. The screen runs inside pqcota-ctl and opens in this machine's browser
./scripts/ui.sh              # → http://127.0.0.1:8765 · to stop: ./scripts/ui.sh stop

# 4) clean up (the core demo owns the environment)
../../pqcota/demo/scripts/down.sh
```

### Seeing it on the screen

`ui.sh` starts the screen **where a real deployment would put it**. 03 Observation calls Ansible on the machine the screen runs on, so the
screen must run on a machine that reaches the nodes. In the demo that machine is `pqcota-ctl`. The operator sees the screen from their
own machine through an SSH tunnel ([operator guide section 5](../docs/operator-guide.md#5-viewing-from-another-machine)). The demo's
`pqcota-ctl` has no SSH server, so a `docker exec` relay stands in for that tunnel.

**No port is published.** The screen has no login, so it accepts connections only on a loopback address. The relay accepts connections
only on this machine's `127.0.0.1` and joins each one to `127.0.0.1:8765` inside the container with `docker exec`. Publishing a port on
the container would let the demo nodes on the same Docker network reach the login-less screen directly, so it is not done.

On the first start it creates the workspace `/work/ui` inside the container and puts what the demo already has where a real operator
would put it.

| What it puts | The screen that uses it |
|---|---|
| The declaration `extend.sh` made | 01 Declaration |
| The asset-scope policy given to the upstream ingest (`scope-assets.csv`) | 04 · 05. It must be the same policy, or the snapshot fingerprints do not match the upstream history |
| Node access information (`hosts.csv`) and the collectors upstream built | 03 Observation runs |
| The observation results so far | 04 Reconciliation. Observing again in 03 replaces them with the new results |
| The approval signing key (outside the workspace, `/work/ui-approval.key`) | 06 Approval and generation |

02 opens with "No layers yet." The layers to edit are empty, but the asset-scope policy that 04 and 05 use (`scope-assets.csv`) is the
one given to the upstream ingest. When you create and finalize layers in 02, the new policy goes out as `asset-scope.csv`, and the
`scope-assets.csv` the demo already uses is not replaced automatically. Restarting the screen, 04 and 05 still use `scope-assets.csv`.
To use the finalized policy for reconciliation, follow "Activating a new policy" below.

The demo's collectors are built in the pqcota repository with sibling-folder replaces. So 03 shows them as "cannot be verified" and by
default does not start an observation. `ui.sh` knows this and starts the screen with `-allow-unverified-collectors`. 03 shows a verdict
line and the allowance per file, and the allowance is recorded in `/work/ui.log` ("Collector versions" in
[operator guide section 2](../docs/operator-guide.md#2-installation)).

The workspace is filled only once. After that, what you do on the screen stays, and `ui.sh stop` does not delete the folder. Bringing the
environment down with `down.sh` removes it with the container. `Ctrl+C` closes only the relay, so running `ui.sh` again continues the
same screen. If you already started another `pqcaton-ui` on 8765 on this machine, give another port such as `PQCATON_UI_PORT=8766`. The
build needs Go, as `extend.sh` does, and the relay needs python3. Without Go, give a Linux `pqcaton-ui` with `PQCATON_BIN_DIR`.

`extend.sh` creates no new container. It injects this repository's `pqcaton-report` into the running `pqcota-ctl`, reconciles the
**declaration (declaration.json)** against the `/work/results` core already collected to make the three-state inventory and the
governance topology, **judges the result to make a plan, and then really runs upstream approval → generation → apply → roll back.**

**It does not stop at reconciliation.** Observe → reconcile → judge → approve → generate → apply → roll back runs as one loop. If
anything between them were broken, the demo should show it. It did, twice. Before v0.9.0 the demo stopped at reconciliation and nobody
noticed, and in v0.16.0 an item without a chosen action kind came to block finalizing, the demo chose none, and the release went out
broken at `close`. Only by running that broken demo again did it turn out that this repository was not reading the JVM collector's result
files (`*.jsonl`).

**The person who judges and the person who approves execution are different.** The `/work/plan.json` that `pqcaton-decide close` issues is
`IN_REVIEW` with an empty approval field. The demo shows a governance approver (`governance-1`) creating and registering their own key
and raising it to `FINALIZED` with `pqcota-approve`. The judge's mark ("demo judge") stays in the judgment ledger, and upstream verifies
the execution approval signature with the registered key. The plan id carries the session id, so the session's judgments can be traced
back in `/work/judgments.jsonl`.

On a host without Go, give prebuilt `pqcaton-report` and `pqcaton-decide` with `PQCATON_BIN_DIR=<dir>`.

There is **a dedicated fixture that measures the trace chain**. Giving `PQCATON_E2E_TRACE=1` removes all assets from the declaration, so
every observation the policy keeps managed becomes UNDECLARED. Only then does the chain go around once **with a plan this repository
made**: approval → generation → record → the real snapshot id. **It is for verification, not the demo story.** Instead of inventing
assets it only empties the declaration, a state that really occurs in an organization that has not written its declaration yet. The
default run keeps the realistic declaration and shows that no action is made for assets the policy excluded from management.

### Activating a new policy

To use the policy finalized in 02 for reconciliation, re-ingest pqcota with that policy and activate it in 05. The demo re-ingests with one
command. The screen may stay up (re-ingesting does not touch the workspace).

```bash
./demo/scripts/ui.sh apply-scope
```

`apply-scope` checks the workspace files (the policy finalized in 02, the declaration, the results), `pqcota-ingest` and the pqcota DB,
then re-ingests pqcota with the `/work/ui/results` the screen uses and the policy finalized in 02. **It does not activate.**

Activation is done in 05 on the screen. Pressing "Review before activating" makes the screen reconcile, for every node, that the latest
ingest in the pqcota history matches the results under the new policy, and when they match the confirmation page shows "At the time of
checking, the pqcota history matched." Pressing activate reconciles again from the start, then activates. If any is missing, differs or
cannot be looked up, nothing is written. The same fingerprint in a past run is not enough to pass. `ui.sh` starts the demo screen with the
demo DB's `PQCOTA_DSN`. The screen does not start without the pqcota history.

After activation, 05 opens a new round rebuilt under the new policy, and earlier conclusions show only as "previous policy conclusions."
Plans finalized before the activation are not signed or generated in 06.

### End-to-end check

`make e2e-demo` starts this demo from scratch, runs the policy activation to the end along both the screen and the command paths, and
measures the failure paths too (IC-U152). Take the five upstream repositories side by side at v0.11.0 and give the upstream `pqcota`
checkout with `PQCOTA_DIR`.

```bash
PQCOTA_DIR=<upstream pqcota checkout> make e2e-demo
```

The upstream demo uses fixed names for containers and networks (`pqcota-ctl` and others). So this check starts a separate dind
container named `pqcaton-e2e-…` for each run and brings the upstream demo up on the Docker daemon inside it. Even if the pqcota side is
using its demo on the same host, the names do not collide and that demo is not touched. When it ends, even on failure, the dind container
and its volume are removed. The daemon inside dind passes umask 0000 to processes started with `docker exec`, so runs inside set that
umask to 022, the same as the host. That does not turn off any file permission check. The inner daemon pulls the base images from
scratch, so the first build takes long. The host needs the `docker:dind` image, and missing programs or `PQCOTA_DIR` fail rather than
skip.

Where the default Ubuntu mirror is slow, also give `E2E_UBUNTU_MIRROR=<mirror name>` (for example `kr.archive.ubuntu.com`). Then only the
builds inside dind resolve `archive.ubuntu.com` and `security.ubuntu.com` to that mirror's address. The upstream Dockerfiles are not
changed, and apt's signature verification stays as it is.

Also giving `E2E_KEEP_WS=<folder>` copies the workspace to that folder before cleaning up (even on failure). The copied folder stays after
the container is removed, so it can be used as it is for a test that an older version stops at the new processing-record format.

## Requirements
- **A pqcota repository checkout**: the declaration is made from its `topology.yaml`. Put it in a sibling directory (`../pqcota`) or
  give it with `PQCOTA_DIR=/path`.
- **Take pqcota at v0.11.0.** The upstream demo takes the five repositories side by side and bundles them in one image, so take all five
  at the same tag next to this repository. With another version upstream builds its history with a different ruleset, and the evidence
  of the plans this repository makes may not be found in the upstream history. The version to use with each pqcaton version is in the
  [version table](../docs/compatibility.md).

  ```bash
  for r in pqcota pqcota-common pqcota-inventory pqcota-discovery pqcota-provisioning; do git clone --branch v0.11.0 https://github.com/randyinthedev-hash/$r; done
  ```
- The pqcota discovery demo running (step 1 above).
- The build machine needs only **Go** and **python3**. From pqcota v0.5.0 the module paths equal the repository addresses, so
  `go build` fetches the contracts itself. No sibling checkout or `replace` is needed.
- **Graphviz (`dot`) is not needed on this machine.** The topology SVG is drawn inside the `pqcota-ctl` container. Without it only the
  DOT source is retrieved and everything else works ([Prerequisites](../README.md)).

## Trying changes

| What to change | Where to change it |
|---|---|
| **Environment**: nodes, network, versions, providers | pqcota's `demo/topology/topology.yaml`. After changing it, start again from `up.sh` |
| **Declaration rules**: what to declare and what to leave out | `ASSETS` and `DECLARED_PROTOS` in [`scripts/declare.py`](scripts/declare.py) |

For example, adding `ssh` to `DECLARED_PROTOS` makes UNDECLARED disappear and everything CONFIRMED. That is what **an organization with
a perfect declaration** looks like.
