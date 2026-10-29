# pqcaton operator guide

<!-- SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd. -->
<!-- SPDX-License-Identifier: BUSL-1.1 -->

This guide is for someone who installs the pqcaton screen (`pqcaton-ui`) on one machine and runs it alone. It covers v0.23.1.
The Korean edition is [operator-guide.md](operator-guide.md). The two editions have the same contents, commands and error
codes, and a test checks that.

**Scope.** The procedure inside pqcaton (declaration, scope, observation, reconciliation, judgment, finalize, approval,
playbook generation) runs on the screen without a terminal. Observations run with the Ansible and collectors the operator
installs. Applying the generated playbooks to the machines is not done by the screen but by the operator's own Ansible run.
Installing and recovering need a terminal or a file manager.

**The commands are run for real.** Every command in this guide marked `<!-- run -->` is run in order, in an empty home
folder, by a test (`guide_run_test.go`) that checks the result.

## Contents

1. [Supported platforms](#1-supported-platforms)
2. [Installation](#2-installation)
3. [Workspace](#3-workspace)
4. [Starting and stopping](#4-starting-and-stopping)
5. [Viewing from another machine](#5-viewing-from-another-machine)
6. [A day's procedure](#6-a-days-procedure)
7. [Backup and restore](#7-backup-and-restore)
8. [Recovery](#8-recovery)
9. [Upgrading (v0.22 to v0.23)](#9-upgrading-v022-to-v023)
10. [Error table](#10-error-table)

## 1. Supported platforms

| Platform | Status |
|---|---|
| Linux (x86-64) | **Supported.** Saving, finalizing and recovery are verified. Uses a systemd user service and a desktop launcher |
| macOS Intel (amd64) | **Supported.** Saving, finalizing, recovery and the launcher (`pqcaton-ui.command`) are verified on a local file system on macOS 26.6.2 (v0.23.1) |
| macOS Apple Silicon (arm64) | Runs, but not verified yet. It logs a warning on start |
| Windows | View only. The screen opens but does not save |

The screen opens only on this machine (a loopback address), because it has no login. From another machine, use an SSH
tunnel ([section 5](#5-viewing-from-another-machine)).

## 2. Installation

**The binary.** Go 1.26.10 is needed. The command below makes `~/.local/bin/pqcaton-ui`. That folder must be on `PATH`.

```bash
GOBIN="$HOME/.local/bin" go install github.com/sntsoftgit/pqcaton/inventory/cmd/pqcaton-ui@v0.23.1
```

**Linux: the service and the launcher.** The service starts the screen when you log in (without opening a browser), and
the launcher starts the service and then opens the screen in a browser. The launcher appears as "pqcaton" in the desktop's
program list. The launcher opens it with `pqcaton-ui -open`, which waits (up to 15 seconds) until the server at the
recorded address answers that it is the screen of the same workspace, so it never opens a stale address left by an
abnormal exit or a screen that is not up yet. If nothing answers, it opens nothing.

```bash
mkdir -p ~/.config/systemd/user ~/.local/share/applications
curl -fsSLo ~/.config/systemd/user/pqcaton-ui.service https://raw.githubusercontent.com/sntsoftgit/pqcaton/v0.23.1/packaging/linux/pqcaton-ui.service
curl -fsSLo ~/.local/share/applications/pqcaton-ui.desktop https://raw.githubusercontent.com/sntsoftgit/pqcaton/v0.23.1/packaging/linux/pqcaton-ui.desktop
systemctl --user daemon-reload
systemctl --user enable --now pqcaton-ui.service
```

The service's log is shown by the command below. Use it to find the cause behind a reference the screen shows
(`error ref=…`).

```bash
journalctl --user -u pqcaton-ui.service
```

**macOS: the launcher.** Double-click the downloaded file: it starts the screen in a Terminal window and opens a
browser. If the screen is already running, it only opens that screen. To stop it, press Ctrl+C in that Terminal window.

```bash
curl -fsSLo ~/Desktop/pqcaton-ui.command https://raw.githubusercontent.com/sntsoftgit/pqcaton/v0.23.1/packaging/macos/pqcaton-ui.command
chmod +x ~/Desktop/pqcaton-ui.command
```

**Getting ready to run observations.** Running an observation from the 03 Observation screen needs two more things on this
machine. The screen checks both before it starts and, when something is missing, says what to do.

- **Ansible.** `ansible-playbook` is looked up on the `PATH`. Nodes are reached over SSH (Windows nodes over SSH or WinRM),
  and on Linux nodes the collectors run as root (`become`), so that account needs sudo rights. Connect to a new node once
  with `ssh` beforehand to accept its host key. Observing Windows nodes also needs the `ansible.windows` collection.
- **Collectors.** Put the build output of pqcota-discovery in the workspace's `collectors` folder (or `-collectors
  <folder>`), shaped like this.

```text
collectors/
├── linux-amd64/     pqcota-nodescan · pqcota-netcap · pqcota-jvmscan
├── linux-arm64/     when there are arm64 nodes
├── windows-amd64/   pqcota-cngscan.exe · pqcota-jvmscan.exe (when there are Windows nodes)
└── collector.jar    without it, the provider chains of running JVMs are not observed
```

**Getting the approval key ready.** Signing approvals on the 06 screen needs an approval key. Save the output of
`pqcota-keygen` to a file **outside** the workspace, readable only by its owner, and start the screen with
`-approval-key <file>`. The screen reads the file again at every signature, and does not use it when other users can read
it or when it sits inside the workspace.

```bash
GOBIN="$HOME/.local/bin" go install github.com/randyinthedev-hash/pqcota-common/cmd/pqcota-keygen@v0.10.4
mkdir -p ~/.config/pqcaton
(umask 077 && pqcota-keygen > ~/.config/pqcaton/approval.key)
```

When the screen runs as a service, add the flag to the service.

```bash
mkdir -p ~/.config/systemd/user/pqcaton-ui.service.d
printf '[Service]\nExecStart=\nExecStart=%%h/.local/bin/pqcaton-ui -no-browser -approval-key %%h/.config/pqcaton/approval.key\n' > ~/.config/systemd/user/pqcaton-ui.service.d/approval.conf
systemctl --user daemon-reload
systemctl --user restart pqcaton-ui.service
```

## 3. Workspace

Started without arguments, the screen uses the `pqcaton` folder in your home, and creates it readable by you only (0700)
if it is not there. To use another folder, give `-workspace <folder>`. **The file names are fixed and cannot be changed.**
The path flags (such as `-decl`) cannot be combined with a workspace. The behavior flags (`-addr`, `-no-browser`, `-org`)
can.

| Place | What | Written by |
|---|---|---|
| `declaration.json` | The declaration (managed nodes, assets, edges) | Screen (01) |
| `hosts.csv` | Access to the nodes to observe. A password written here **stays in this file and goes into backups.** Only the owner reads it (0600) | Screen (03) |
| `observations/` | One place per observation run: its record, its log (passwords masked), the results it replaced (`previous/`), and results that did not become current (`partial/`) | Screen (03) |
| `collectors/` | The collectors observations use. The screen only reads them | Operator |
| `results/` | Observed results | Screen (03 Observation), pqcota collection |
| `session.json` | The review session | Screen (05) |
| `layers/*.csv` | Asset scope layers. They stack in name order, and where rules clash on the same asset the later layer applies | Screen (02) |
| `scope-session.json` | The scope session | Screen (02) |
| `asset-scope.csv` | The finalized asset scope policy. When present, it is the baseline for the next review | Screen (02 finalize) |
| `scope-assets.csv` | The same policy given to pqcota ingest (`pqcota-ingest`). When present, reconciliation uses it | Operator |
| `plan.json` | The finalized plan (the contract format pqcota reads) | Screen (05 finalize) |
| `plan.signed.json` | The approved plan (FINALIZED). `plan.json` stays as it is | Screen (06 approval) |
| `provision.yml` · `provision-rollback.yml` | The apply and rollback playbooks | Screen (06 generation) |
| `judgments.jsonl` | The judgment ledger. Only appended to | Screen (finalize) |
| `.pqcaton/` | Save records, receipts, locks, and the address of the running screen (`screen.addr`) | Screen |
| `.pqcaton.lock` | The lock file saying which state folder this folder is bound to. It goes together with `.pqcaton/` | Screen |

**Organization.** Without `-org`, judgments are bound to the organization written in the declaration. With `-org`, only a
declaration for that organization can be created.

## 4. Starting and stopping

To start it yourself, run it in a terminal. It opens the port and then a browser (`http://127.0.0.1:8765`). To not open a
browser, give `-no-browser`.

<!-- run: background -->
```bash
pqcaton-ui -no-browser
```

The address of the running screen is recorded in the workspace.

<!-- run -->
```bash
cat ~/pqcaton/.pqcaton/screen.addr
```

**If it is already running, it does not start a second one.** Run again with the same workspace and it opens the running
screen, only when the recorded address is a loopback address and the server there answers that it is the screen of the
same workspace.

<!-- run -->
```bash
pqcaton-ui -no-browser
```

To open the running screen without ever starting one, use `-open` (what the launcher does). With `-no-browser` it opens
nothing and only prints the confirmed address.

<!-- run -->
```bash
pqcaton-ui -open -no-browser
```

**Stopping.** Press Ctrl+C in the terminal that started it, or for the service run `systemctl --user stop pqcaton-ui.service`.
To stop it from another terminal, do as below. On stopping it removes the recorded address, and the second command below
prints nothing.

<!-- run -->
```bash
pkill -INT -u "$USER" -x pqcaton-ui
```

<!-- run: until -->
```bash
! pgrep -u "$USER" -x pqcaton-ui
```

If an input file is broken, the screen opens **for diagnostics only**. Every page shows the name of the file it could not
read and a reference, and saving is stopped. Fix the file and start the screen again.

## 5. Viewing from another machine

The screen opens only on a loopback address. Given any other address, it does not start.

<!-- run: fails -->
```bash
pqcaton-ui -addr 0.0.0.0:8765 -no-browser
```

To view it from another machine, forward the port with SSH from that machine and open `http://127.0.0.1:8765` in that
machine's browser. Replace `operator@screen-host` with the account and name of the machine running the screen.

```bash
ssh -N -L 8765:127.0.0.1:8765 operator@screen-host
```

## 6. A day's procedure

The six step cards at the top of the screen are the procedure, in order. Each card shows where that step stands, and the
two steps that finalize (02 and 05) also show the finalize state (not finalized, finalized, superseded, recovery
required).

1. **01 Declaration.** The first time, enter the organization name to create an empty declaration. Write the managed nodes
   and their IPs, the crypto assets used on each node, and the communication edges, and save. The screen points at the
   fields that do not add up (an IP's format, a shared IP, port 0 and the like). Saving is not blocked, but left as is,
   reconciliation is wrong without any error.
2. **02 Asset scope.** The first time, enter a layer name to create an empty layer. Write the rules, a conclusion for each
   changed rule, sign, and finalize. Finalizing creates `asset-scope.csv`.
3. **03 Observation.** Write the nodes to observe and how to reach them (IP, port, user, SSH key path, OS, SSH or WinRM),
   and save. They are saved to `hosts.csv` in the workspace, in the format `pqcota-hosts` reads. Passwords are not written
   on the screen: if the file has one, it shows only as "set" and is kept as is when you save. "Start the observation"
   copies the collectors to each node in the table, runs them, takes the results into `results/`, and removes them from
   the node. While it runs, the log is shown and "Stop" stops it. **The current results change, all at once, only when
   every node's results arrived and the run succeeded.** If the run failed, was stopped, or a node's results are missing,
   the current results stay and what arrived is kept for diagnosis in `observations/<run>/partial/`. After a run that did
   not finish, a cleanup removes the staging folders from the nodes. Stopping the screen stops a running observation too,
   and the screen ends only after the cleanup. The network observation waits for handshakes on the
   node's default-route interface for the window you set (60 seconds by default).
4. **04 Reconciliation.** When the observation finishes, or pqcota collection writes results to `results/`, reload the
   screen. With no results, the screen points at that folder.
5. **05 Judgment.** Write a conclusion for each policy; for the items going into the plan, pick the action kind, target and
   level; and sign. "Finalize" shows a confirmation page with what will be written, and confirming writes the session,
   plan and ledger together. Finalizing the same session with the same content again writes nothing again.
6. **06 Approval & generation.** Check the actions of the plan to approve, enter the approver's name and confirm the
   review, and sign. The signed plan is kept separately in `plan.signed.json` (FINALIZED) and `plan.json` stays as it is.
   Then pick the default delegation level and press "Generate the playbooks": `provision.yml` and `provision-rollback.yml`
   are written. When the generator flags gaps, their number and content are shown. Applying them to the machines is not
   done by the screen; the operator runs the commands in the 06 screen's help. When 05 is finalized again and the plan
   changes, the 06 card reads "the signature does not match the plan", and generating needs a new signature on the new plan.
7. **The next round.** A finalized session cannot be changed and is shown read-only. When new observations arrive or a
   conclusion must change, press "Start a new review" on the 05 screen ("Start a new scope review" on the 02 screen). A new
   round starts under a new session id: conclusions and plan fields carry over, and a new signature is needed. The earlier
   round's plan and ledger entries stay as they are.

The inventory lookup (beside the cards) is not a step: come in any time, search with conditions, and see one asset's
judgment history.

## 7. Backup and restore

<!-- setup: guidetool seed "$HOME/pqcaton" -->

**Back up only with the screen stopped.** A running screen may be in the middle of a save. Backing up while it runs is not
supported.

1. Stop the screen ([section 4](#4-starting-and-stopping)).
2. Check that it stopped. It must print `stopped`.

<!-- run -->
```bash
pgrep -u "$USER" -x pqcaton-ui || echo stopped
```

3. Copy the whole workspace, `.pqcaton/` included. The save records and the lock files go together, so do not copy only a
   part. If `hosts.csv` holds passwords, they go into the backup as well, so guard the backup like the workspace. With SSH
   keys instead of passwords, no secret ends up in the backup.

<!-- run -->
```bash
cp -a ~/pqcaton ~/pqcaton-backup-$(date +%Y%m%d)
```

**Restoring** also puts the whole folder back with the screen stopped. Move the current folder aside rather than deleting
it. The last line checks that it matches the backup.

<!-- run -->
```bash
mv ~/pqcaton ~/pqcaton-before-restore
cp -a ~/pqcaton-backup-$(date +%Y%m%d) ~/pqcaton
diff -r ~/pqcaton-backup-$(date +%Y%m%d) ~/pqcaton && echo restored
```

After restoring, start the screen and check that it opens.

<!-- run: background -->
```bash
pqcaton-ui -no-browser
```

<!-- run -->
```bash
curl -fsS -o /dev/null "http://$(cat ~/pqcaton/.pqcaton/screen.addr)/review" && echo ok
```

<!-- run -->
```bash
pkill -INT -u "$USER" -x pqcaton-ui
```

<!-- run: until -->
```bash
! pgrep -u "$USER" -x pqcaton-ui
```

## 8. Recovery

<!-- setup: guidetool stuck "$HOME/pqcaton" -->

If a save stopped part way left a record and a file in that record was changed outside pqcaton, the screen opens in the
**recovery required** state. Every page shows the remaining record and where each file stands, and saving is stopped.
**The screen does not decide which file is right, so it does not fix anything by itself.** There is no button to fix it
either. Follow the steps below.

1. **Keep the current state.** Press "Download the diagnostics file" on the screen, or run the command below in a
   terminal. The screen does not need to be stopped.

<!-- run -->
```bash
pqcaton-ui -recover-status -diagnostics ~/pqcaton-diagnostics.json
```

   Each line of the output has a verdict. `done` is already applied, `pending` is applied by the next start, `partial` is
   half written to the ledger and the next start writes the rest, and `mismatch` was changed outside pqcaton. **Only
   `mismatch` needs fixing.**

2. **Keep the `mismatch` file as it is now.** From the output, put the path on the `mismatch` line in `TARGET`, and the path
   after `prepared copy at` below it in `PREPARED` (the lines below are an example).

```bash
TARGET="$HOME/pqcaton/session.json"
PREPARED="$HOME/pqcaton/.session.json.pqcaton-<txid>.tmp"
```

<!-- run -->
```bash
cp -a "$TARGET" ~/pqcaton-kept-$(date +%Y%m%d%H%M%S)-$(basename "$TARGET")
```

3. **Put it in the state the record expects.** If the prepared copy is `present, matches`, put it in the target's place.
   Checking again, that line is now `done`.

<!-- run -->
```bash
cp "$PREPARED" "$TARGET"
```

<!-- run -->
```bash
pqcaton-ui -recover-status
```

4. **Stop the screen and start it again.** On start it carries the remaining record through, and the recovery required
   state clears.

<!-- run: background -->
```bash
pqcaton-ui -no-browser
```

<!-- run -->
```bash
curl -fsS -o /dev/null "http://$(cat ~/pqcaton/.pqcaton/screen.addr)/review" && echo ok
```

5. Compare with the file you kept, and if the change made outside is still needed, make it again on the screen.

<!-- run -->
```bash
pkill -INT -u "$USER" -x pqcaton-ui
```

<!-- run: until -->
```bash
! pgrep -u "$USER" -x pqcaton-ui
```

**When it cannot be put in the expected state, stop and ask for support.** That is the case when the prepared copy is
missing or `present, differs`, or when the start of the ledger changed (the ledger is only appended to, so it is never
cut). Send the diagnostics file and the files you kept. The diagnostics file holds paths and hashes only, no file
contents.

## 9. Upgrading (v0.22 to v0.23)

1. Stop the screen and replace the binary with the `go install` in [section 2](#2-installation).
2. **The old screens are gone.** Bookmarks to `/classic/…` and `-next` addresses move to the current screens (301).
3. **Explicit mode is unchanged.** Starting with a session file and path flags
   (`pqcaton-ui session.json -decl … -results …`) works as in v0.22. Always give `-org` so judgments are not bound to the
   default organization (`local`). Put the session file before the flags: after them, it is a usage error and the screen
   does not start.
4. **To move to a workspace,** first check that `-recover-status` prints `no unfinished transactions`. Stop the screen and
   copy the files you used into `~/pqcaton` under the names in [section 3](#3-workspace). Do not copy the old `.pqcaton/`.
   The plan file and the ledger carry on, but the step cards show "not finalized" until you finalize again in the new
   workspace.

## 10. Error table

An error page shows what did not work, what to do next, the role and name of the files involved, and the **error code**.
Errors whose cause cannot be put in the screen's words also show a **reference**: the same reference is in the server log
(the terminal or `journalctl`) next to the original error. Full paths and original error texts never appear on the
screen.

| Code | What did not work | What to do |
|---|---|---|
| `stale` | The files changed after the screen was opened, so nothing was saved | Reload the screen and do it again |
| `finalized` | The session is already finalized, so nothing was saved | Do it in a new session |
| `busy` | Another save to the same files is in progress, or another command holds the state folder | Try again shortly. If it persists, check whether another `pqcaton-ui` or command is running |
| `recovery` | Saving is stopped in the recovery required state | Follow [section 8](#8-recovery) |
| `view-only` | This platform does not save | Run it on Linux |
| `expired` | The confirmation page expired (10 minutes) | Start the finalize again |
| `confirm` | A finalize that did not come through the confirmation page | Press "Finalize" and confirm on the confirmation page |
| `unsafe-file` | The file is a link or writable by others, so it was not written | Fix the file's type and permissions (a regular file, writable by the owner only) |
| `state-mismatch` | The folder is bound to another state folder | Check whether the folder was moved or copied. Put the whole folder back as in [section 7](#7-backup-and-restore) |
| `damaged` | A save record or lock file is damaged | Stop the screen, get the diagnostics file and ask for support |
| `diagnostic` | An input file could not be read, so the screen opened for diagnostics only | Fix the file the screen shows and start it again |
| `exists` | The file to create is already there, so it was not overwritten | Reload the screen |
| `run-not-ready` | Something needed before an observation (access, Ansible, collectors) is missing | Provide the items marked ✗ in the checklist on the 03 screen |
| `run-busy` | An observation is already running | Start again after it finishes or is stopped |
| `sign-not-ready` | The approval key cannot be used, the approver's name is empty or holds a colon, or the review was not confirmed | See the approval key check on the 06 screen and fill in the form |
| `plan-not-approvable` | The plan is not in review (IN_REVIEW), has no actions, or is already signed with this key | Finalize again at 05, or use the signed plan as it is |
| `generate-not-ready` | The current plan has no approval that checks out with this key | Sign the approval first |
| `not-found` | There is no screen at this address | Start again from the first screen |
| `method` | This kind of request is not accepted | Use the screen's buttons |
| `bad-request` | The request could not be handled | Reload the screen and do it again |
| `input-missing` | A file this screen needs is not there | Do the earlier step first (declaration, results) |
| `input-broken` | An input file could not be read | Find the reference in the server log and fix that file |
| `forbidden` | The request protection refused it | Open the screen again from the start in a browser on the machine running it. From another machine, follow [section 5](#5-viewing-from-another-machine) |
| `internal` | It could not be handled | Find the reference in the server log and ask for support |
