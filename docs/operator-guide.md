English · [한국어](operator-guide.ko.md)

# pqcaton operator guide

<!-- SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd. -->
<!-- SPDX-License-Identifier: BUSL-1.1 -->

> **`main` is in development for v0.29.0.** It is taking in upstream pqcota v0.13.0 (ruleset `pqcota-enrich/v4`). For
> production use the [v0.28.0 tag](https://github.com/sntsoftgit/pqcaton/tree/v0.28.0) with pqcota v0.11.0.

This guide is for someone who installs the pqcaton screen (`pqcaton-ui`) on one machine and runs it alone. It covers v0.29.0.
The Korean edition is [operator-guide.ko.md](operator-guide.ko.md). The two editions have the same contents, commands and error
codes, and a test checks that.

**Scope.** The procedure inside pqcaton (declaration, scope, observation, reconciliation, judgment, finalize, approval,
playbook generation) runs on the screen without a terminal. Observations run with the Ansible and collectors the operator
installs. Applying the generated playbooks to the machines is not done by the screen but by the operator's own Ansible run.
Installing and recovering need a terminal or a file manager.

**Trust model.** The screen runs on one machine as **one OS user** and opens only on **loopback** (it has no
authentication). That user can reach the nodes to observe over SSH and run the collectors there **as root (become)**,
and, when signing at 06, **reads the approval private key.** The user's account and the workspace are therefore the
boundary of these powers. The 06 screen signs and checks with one key (single key, single approver).

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
9. [Upgrading](#9-upgrading)
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

**The binary.** Go 1.26.11 is needed. The command below makes `~/.local/bin/pqcaton-ui`. That folder must be on `PATH`.

```bash
GOBIN="$HOME/.local/bin" go install github.com/sntsoftgit/pqcaton/inventory/cmd/pqcaton-ui@v0.29.0
```

**Linux: the service and the launcher.** The service starts the screen when you log in (without opening a browser), and
the launcher starts the service and then opens the screen in a browser. Create `pqcota.env` from "pqcota connection"
below before enabling the service. The launcher appears as "pqcaton" in the desktop's
program list. The launcher opens it with `pqcaton-ui -open`, which waits (up to 15 seconds) until the server at the
recorded address answers that it is the screen of the same workspace, so it never opens a stale address left by an
abnormal exit or a screen that is not up yet. If nothing answers, it opens nothing.

```bash
mkdir -p ~/.config/systemd/user ~/.local/share/applications
curl -fsSLo ~/.config/systemd/user/pqcaton-ui.service https://raw.githubusercontent.com/sntsoftgit/pqcaton/v0.29.0/packaging/linux/pqcaton-ui.service
curl -fsSLo ~/.local/share/applications/pqcaton-ui.desktop https://raw.githubusercontent.com/sntsoftgit/pqcaton/v0.29.0/packaging/linux/pqcaton-ui.desktop
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
curl -fsSLo ~/Desktop/pqcaton-ui.command https://raw.githubusercontent.com/sntsoftgit/pqcaton/v0.29.0/packaging/macos/pqcaton-ui.command
chmod +x ~/Desktop/pqcaton-ui.command
```

**pqcota connection.** pqcaton runs only where pqcota is installed, connected and working. On start the screen connects
to the pqcota history and must pass one read of it (schema and read permission) before it comes up. If there is no
connection, or connecting and the read do not finish within 10 seconds, it stops before opening the port or writing the
address file. It only looks things up, so an account with read permission is enough.

- **The connection is `PQCOTA_DSN`.** When the environment variable is set it is used; when it is empty, the screen itself
  reads `pqcota.env` in the same user configuration folder as the approval key (Linux `~/.config/pqcaton/pqcota.env`,
  macOS `~/Library/Application Support/pqcaton/pqcota.env`). The service and the launcher use this file as it is.
- **File rules.** It must be a regular file, not a link, owned by this user, with no permission for the group or others
  (0600), with a single hard link, and outside the workspace. If any rule is broken the screen does not start and says why.
- **The content is read only as data.** Apart from blank lines and `#` comments, only one line `PQCOTA_DSN=<connection>`
  is accepted. Other variables, `export`, quotes, surrounding spaces and a second line are refused. It is not evaluated by
  a shell, so characters such as `$(…)` in the value stay part of the value.
- **The value is left nowhere.** Not in the workspace, processing records, logs, error messages or the screen, and not
  passed to the observation processes the screen starts. The screen has no place to type it either.
- **The pqcota organization is the organization of the judgments.** The screen opens only the pqcota history of the
  organization of the judgments (`-org`, or the declaration's organization when not given). `-upstream-org` accepts only the
  same value; another organization stops the start. If the review session belongs to another organization, 05 refuses with
  `upstream-org` without looking anything up. So ingest pqcota under that organization too
  (`PQCOTA_ORG=<organization> pqcota-ingest …`).

Fill the file in an editor. Do not put the value on the command line, so the password stays out of the shell history.

```bash
mkdir -p ~/.config/pqcaton && chmod 700 ~/.config/pqcaton
install -m 600 /dev/null ~/.config/pqcaton/pqcota.env
```

**Getting ready to run observations.** Running an observation from the 03 Observation screen needs two more things on this
machine. The screen checks both before it starts and, when something is missing, says what to do.

- **Ansible.** `ansible-playbook` is looked up on the `PATH`. Nodes are reached over SSH (Windows nodes over SSH or WinRM),
  and on Linux nodes the collectors run as root (`become`), so that account needs sudo rights. Connect to a new node once
  with `ssh` beforehand to accept its host key. Observing Windows nodes also needs the `ansible.windows` collection.
- **Collectors.** Put the collectors of the upstream release in the workspace's `collectors` folder (or `-collectors
  <folder>`), shaped like this. Keep them on pqcota v0.13.0 (the pqcota version for each pqcaton version is in the
  [version table](compatibility.md)), and the `collector.jar` put on the nodes on the same version.
  This screen normalizes the collection results it reads with the v0.13.0 ruleset (`pqcota-enrich/v4`). From v0.12.0
  `pqcota-jvmscan` reports the JVMs that run the same application as one deployment, and from v0.13.0 `pqcota-nodescan` and
  `pqcota-netcap` give a user service the app key `user@<uid>.service/<unit>` and give each edge to the application that
  opened the connection. A
  `pqcota-jvmscan` earlier than v0.10.7 writes its results over several lines on a node with two or more JVMs, and the
  whole ingest request that holds that node is refused. An earlier `pqcota-jvmscan` or
  `collector.jar` does not carry the per-provider list of PQC services, so the JCA readiness becomes `unknown`, and the 03
  and 04 screens name such nodes as "the JCA results cannot be confirmed as v0.10.6 collection results" (see the version
  table under 「맞는 조합」 in the README).

```text
collectors/
├── linux-amd64/     pqcota-nodescan · pqcota-netcap · pqcota-jvmscan
├── linux-arm64/     when there are arm64 nodes
├── windows-amd64/   pqcota-cngscan.exe · pqcota-jvmscan.exe (when there are Windows nodes)
└── collector.jar    without it, the provider chains of running JVMs are not observed
```

From v0.11.0 upstream publishes the collectors as release assets. Check the archives you download against the
`SHA256SUMS` published with them before unpacking (on macOS use `shasum -a 256` instead of `sha256sum`). For arm64 nodes
take `pqcota-linux-arm64.tar.gz`, and for Windows nodes `pqcota-windows-amd64.zip`, the same way, into the `linux-arm64`
and `windows-amd64` folders.

```bash
mkdir -p ~/pqcaton/collectors && cd ~/pqcaton/collectors
for f in pqcota-linux-amd64.tar.gz collector.jar SHA256SUMS; do curl -fsSLO "https://github.com/randyinthedev-hash/pqcota/releases/download/v0.13.0/$f"; done
sha256sum -c --ignore-missing SHA256SUMS
tar -xzf pqcota-linux-amd64.tar.gz && mv pqcota-linux-amd64 linux-amd64
```

**Collector versions.** Before an observation starts, 03 checks the version of each collector. It does not run them; it
reads only the Go build information. "Verified" is only a build whose collector itself is at the pinned version (tag, commit, no modification) and whose
dependencies carry no replace either.

**Every upstream collector today, the release assets included, is "cannot be verified".** The release assets, like a
build of your own next to the sibling folders, take their dependencies as replaces of those folders. A pqcota-common of
another version next to it does not show in the build information, so the screen does not count such a build as verified.
A release asset shows as "declared version: v0.13.0 · actual source: local replace (devel) · cannot be verified", and a
build of your own from a tag checkout as "collector source v0.13.0 verified · replaced dependency sources cannot be
verified". Once the archives match `SHA256SUMS`, or you have checked that the two repositories next to it are at the same
tag, start with the option below. Upstream has been asked to carry version information
([pqcota-discovery#1](https://github.com/randyinthedev-hash/pqcota-discovery/issues/1)).

- **Does not match.** When another version shows, or the collectors mix versions, the observation does not start, and no
  option lets it through. Replace them with collectors rebuilt at that version.
- **Cannot be verified.** Collectors whose version cannot be trusted do not start by default either: built with a
  replace from another repository as the upstream demo does, built with `go build -buildvcs=false`, built from a modified tree, or
  files with no build information. To use them knowingly, start the screen with `-allow-unverified-collectors`. The
  allowance and the list of files show on the 03 screen and on standard error, and are not kept in the workspace.
- **`collector.jar`.** Upstream does not put a version in it, so the screen only warns that its version was not checked
  and the observation still starts. When it carries a version that differs, the observation does not start.

**Getting the approval key ready.** Signing new approvals on the 06 screen needs an approval key. By default the screen
looks for `pqcaton/approval.key` in the user configuration folder (`~/.config/pqcaton/` on Linux,
`~/Library/Application Support/pqcaton/` on macOS). Save the output of `pqcota-keygen` there, readable only by its owner,
and it is used without any flag. The screen neither creates this file nor changes its permissions, and reads it again at
every signature. The last line puts the public key alone in `approval.pub`.

```bash
GOBIN="$HOME/.local/bin" go install github.com/randyinthedev-hash/pqcota-common/cmd/pqcota-keygen@v0.13.0
mkdir -p ~/.config/pqcaton
(umask 077 && pqcota-keygen > ~/.config/pqcaton/approval.key)
grep '^PQCOTA_VERIFY_KEY=' ~/.config/pqcaton/approval.key > ~/.config/pqcaton/approval.pub
```

On macOS only the location differs.

```bash
mkdir -p "$HOME/Library/Application Support/pqcaton"
(umask 077 && pqcota-keygen > "$HOME/Library/Application Support/pqcaton/approval.key")
grep '^PQCOTA_VERIFY_KEY=' "$HOME/Library/Application Support/pqcaton/approval.key" > "$HOME/Library/Application Support/pqcaton/approval.pub"
```

- **The verification key (`approval.pub`).** With it, plans already signed are checked and their playbooks generated even
  without the private key. A file that holds a private key line, or that other users can change, is not taken. When both
  files are there they must be the same key; if not, signing and checking are both blocked.
- **Unusable keys.** When there is no file at the default location, the screen starts normally and only does not sign.
  When there is one with wrong permissions, owner, links or format, the screen starts, but 06 and standard error say why
  and new signatures are blocked. 06 shows only the location relative to the user configuration folder and the key
  fingerprint; the private key shows nowhere.
- **Keys elsewhere.** Give them with `-approval-key <file>` and `-approval-verify-key <file>`. When a file given this way
  cannot be used, or both are given and are different keys, the screen does not start. To stop looking at the default
  location, start with `-no-default-approval-key`. It turns off both default files; files given explicitly are still used.
- **The order of choice.** The explicit private key, the explicit verification key, the default private key, then the
  default verification key. **Once either is given explicitly, neither default file is read.** With only a private key,
  its public key is derived and it signs, checks and generates; with only a verification key, it checks and generates.
  A broken private key next to a good verification key blocks signing only, and checking and generation go on. A good
  private key next to a verification key chosen with it that cannot be used, or is another key, is a configuration error
  that blocks both signing and generation.

When the screen runs as a service and the key is not at the default location, add the flag to the service. A key at the
default location needs nothing.

```bash
mkdir -p ~/.config/systemd/user/pqcaton-ui.service.d
printf '[Service]\nExecStart=\nExecStart=%%h/.local/bin/pqcaton-ui -no-browser -approval-key %%h/secrets/pqcaton-approval.key\n' > ~/.config/systemd/user/pqcaton-ui.service.d/approval.conf
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
| `asset-scope.csv` | The asset scope policy finalized in 02. The baseline for the next review; reconciliation does not use it until it is activated | Screen (02 finalize) |
| `scope-assets.csv` | The active policy. 04 and 05 reconcile with it, and it must be the one given to pqcota ingest (`pqcota-ingest -scope-assets`) | Operator, screen (policy activation) |
| `scope-assets.activation-<digest>-<session id>.csv` | The previous active policy, archived when a new policy was activated | Screen (policy activation) |
| `session.scope-activation-<session id>.json` · `session.scope-baseline-<session id>.json` | The previous review session, archived when a policy round opened. The `…-report-…json` beside it is the change report | Screen (policy round) |
| `plan.json` | The finalized plan (the contract format pqcota reads) | Screen (05 finalize) |
| `plan.signed.json` | The approved plan (FINALIZED). `plan.json` stays as it is. Never overwritten once present | Screen (06 approval) |
| `plan.provision.yml` · `plan.rollback.yml` | The apply and rollback playbooks | Screen (06 generation) |
| `plan.signed.<time>.json` and the like | Earlier signatures and playbooks renamed by "Archive the earlier signature" | Screen (06 archive) |
| `judgments.jsonl` | The judgment ledger. Only appended to | Screen (finalize) |
| `.pqcaton/` | Save records, processing records, locks, and the address of the running screen (`screen.addr`) | Screen |
| `.pqcaton.lock` | The lock file saying which state folder this folder is bound to. It goes together with `.pqcaton/` | Screen |

**Organization.** Without `-org`, judgments are bound to the organization written in the declaration. With `-org`, only a
declaration for that organization can be created.

## 4. Starting and stopping

To start it yourself, run it in a terminal. The connection is read from `pqcota.env` in section 2, "pqcota connection",
so the shell does not need it. It opens the port and then a browser (`http://127.0.0.1:8765`). To not open a browser, give `-no-browser`.

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

   **The policy finalized in 02 is not used for reconciliation right away.** Reconciliation (04, 05) uses the active
   policy `scope-assets.csv`. If it differs from the policy the upstream ingest used, pqcota cannot find the evidence of
   the plans made in 05. To use the finalized policy, first redo the upstream ingest with it, giving the workspace's
   `results/` as the results folder.

   ```bash
   PQCOTA_ORG=<organization of the judgments> pqcota-ingest -scope-assets <workspace>/asset-scope.csv <workspace>/results
   ```

   After finalizing 02, "Next" under the finalize notice leads to "View current reconciliation in 04" and "View the
   activation steps in 05", and until it is activated the top of 04 also shows "The policy finalized in 02 is not active
   yet", so that reconciliation is read as made under the active policy. "The policy finalized in 02 can be activated" at
   the top of 05 shows three steps: re-ingest pqcota, reconcile the pqcota re-ingest, activate the policy. Re-ingest with
   the command above, press "Review before activating" and activate through the confirmation page.

   **Activation always goes through the pqcota history reconciliation.** There is no operator statement to use instead.
   pqcaton goes only in this order, within one activation request:

   1. It reads a snapshot of the policy to activate and of the workspace inputs (review session, declaration, results
      folder, active policy).
   2. It reconciles, for every node, that the latest ingest in the pqcota history matches the results built from the
      inputs of step 1 under the new policy. Every node in the results folder is reconciled, even when the new policy
      leaves it no managed assets, because a policy dropping all of a node's assets is also a result of the policy change.
   3. If any is missing, differs or cannot be looked up, nothing is activated. The whole reconciliation has one 10-second
      limit; going over it is a failed reconciliation.
   4. When it succeeds, it takes the workspace's run lock right away.
   5. It checks again that the input snapshot of step 1 is unchanged.
   6. Only then does it write the active policy, the new round, the change report, the round marker and the activation
      processing record in one transaction.

   The lookup runs outside the workspace lock. That does not make pqcota optional; it keeps the lock from being held while
   a network lookup is slow. The confirmation page also reconciles ahead and shows "At the time of checking, the pqcota
   history matched" with the number of nodes, but activating starts again from step 1. **Changes to pqcota after the
   reconciliation and before the activation finishes are not detected.** The check in step 5 looks only at the workspace
   inputs. So the screen and the processing record only say the pqcota history matched at the time of checking; they do
   not say it still matches or that the ingest is proven. Nothing stronger can be guaranteed until pqcota offers a stable
   current pointer or an ingest record.

   The activation processing record (`scope-activation` in `.pqcaton/`) keeps the basis `upstream-verified`, and the time
   of checking, the pqcota organization and the per-node fingerprints reconciled go into `upstream_check` of the change
   report. Neither holds a secret. The command activates with the same reconciliation (the screen does not need to be
   stopped; it uses the same workspace, so it waits while the screen is saving something else).

   ```bash
   PQCOTA_DSN=<pqcota DB> pqcaton-decide activate-scope <workspace>/session.json -decl <workspace>/declaration.json \
     -results <workspace>/results -scope-assets <workspace>/scope-assets.csv -candidate <workspace>/asset-scope.csv \
     -scope-session <workspace>/scope-session.json -org <organization>
   ```

   The flag in `pqcaton-decide activate-scope -verify-upstream` is accepted for compatibility and changes nothing.
   `pqcaton-decide activate-scope -confirm-ingest` is no longer supported; given, it stops without writing anything.

   Activating archives the old policy and the old review session, and opens a new round rebuilt under the new policy.
   Earlier conclusions are carried only as previous values, and items the new policy no longer manages stay only in the
   change report. Plans already finalized are not signed or generated in 06, so finalize again in the new round.
   **Changing `scope-assets.csv` by hand without an activation record** locks 05 and blocks saving and finalizing. Then
   restore the previous policy file or follow the steps above.

   **This version blocks an activation that goes back to an earlier policy.** When the history already holds a snapshot
   with the same fingerprint, pqcota does not create a new snapshot on re-ingest and the node's latest snapshot does not
   change (pqcota-inventory#1). So a re-ingest under an earlier policy cannot be reconciled with the pqcota history, and 05
   refuses it with `upstream-revert`. Rounds that v0.27.x or earlier opened on an operator statement (`operator-statement`)
   are still read as past records, and 05 shows "It was not reconciled with the pqcota history" under "The policy change
   that opened this round".
3. **03 Observation.** Write the nodes to observe and how to reach them (IP, port, user, SSH key path, OS, SSH or WinRM),
   and save. They are saved to `hosts.csv` in the workspace, in the format `pqcota-hosts` reads. Passwords are not written
   on the screen: if the file has one, it shows only as "set" and is kept as is when you save. "Start the observation"
   copies the collectors to each node in the table, runs them, takes the results into `results/`, and removes them from
   the node. While it runs, the log is shown and "Stop" stops it. **The current results change, all at once, only when
   every node's results arrived and the run succeeded.** If the run failed, was stopped, or a node's results are missing,
   the current results stay and what arrived is kept for diagnosis in `observations/<run>/partial/`. The results received
   are read with the upstream decoder, and if any file is broken, belongs to another node or comes from another collector,
   they do not become current. After a run that did not finish, a cleanup removes the staging folders from the nodes.
   Stopping the screen stops a running observation too, and the screen ends only after the cleanup. The observation runs
   under a supervisor process the screen starts (`pqcaton-ui` itself), so if the screen is killed, the supervisor stops
   Ansible, cleans the nodes and removes the inventory. If the supervisor ended too (a power loss, say), the next start
   finds the run's processes, stops them and does the same cleanup. If they cannot be stopped, nothing is removed or moved:
   the run is left as "recovery needed", the 03 screen shows the PIDs, and no new observation starts. pqcaton stops a
   process only when its run marker (`PQCATON_RUN`) or the lineage of a live process shows it belongs to this
   observation. A process whose ownership cannot be shown may be unrelated, so it is not stopped and the observation is
   left as "recovery needed" instead. macOS does not let the environment of some Apple programs (such as `/bin/sh` and
   `/usr/bin/ssh`) be read, so for those a live process of this observation, as the parent or in the same process group,
   stands in for the run marker. When "recovery needed" shows, look at "Processes left behind" on the 03
   screen. The screen compares the PID and start time found when the run was marked with the process there now, and only
   when it is the same process shows its PID, parent PID, process group, user, executable and start time. The arguments
   are folded, and even unfolded the secrets in them (passwords, tokens, user information in a DSN, keys) are masked.
   Environment variables are not shown. "Check again" only reads again and changes nothing, and the screen has no button
   that stops a process. There are five states.

   - **A process this observation left behind**: the one to stop after checking.
   - **Already ended**: restart the screen and it carries on with the cleanup that is left.
   - **Replaced by another process**: another process uses the PID. Do not stop it.
   - **No permission, or lookup failed**: its belonging cannot be checked, so do not stop it. After a failed lookup, check
     again in a moment.
   - **Cannot tell whether it is the same process**: a record from an earlier version that kept no start time. Only then
     look yourself in a terminal.

   ```bash
   ps -p <PID> -o pid=,ppid=,pgid=,user=,lstart=,command=
   ```

   Only when a process is confirmed to have been started by this observation, stop it normally and start the screen
   again. If it is unrelated, or its ownership cannot be confirmed, do not stop it: wait until it ends on its own, then
   start the screen again. The restarted screen carries on with the remaining cleanup. Observations started
   by the screen do not share SSH connections (`ControlMaster=no`). If the nodes could not be reached, the 03 screen names
   the staging folder that may be left, with the run ID: `/tmp/pqcaton-observe-<run ID>` (on Windows `C:\Windows\Temp\pqcaton-observe-<run ID>`). The network observation waits for handshakes on the
   node's default-route interface for the window you set (60 seconds by default).
4. **04 Reconciliation.** When the observation finishes, or pqcota collection writes results to `results/`, reload the
   screen. With no results, the screen points at that folder.
5. **05 Judgment.** Write a conclusion for each policy; for the items going into the plan, pick the action kind, target and
   level; and sign. "Finalize" shows a confirmation page with what will be written, and confirming writes the session,
   plan and ledger together. Finalizing the same session with the same content again writes nothing again.
6. **06 Approval & generation.** Check the actions of the plan to approve, enter the approver's name and confirm the
   review, and press "Sign the approval". A confirmation page shows the plan, the plan file's hash, the approver and the
   key fingerprint, and signing happens only on a second press there. If the plan or the key changed after the
   confirmation, nothing is signed. The signed plan is kept separately in `plan.signed.json` (FINALIZED) and `plan.json`
   stays as it is. **An existing signed file is never overwritten.** If it does not match the current plan, "Archive the
   earlier signature" renames it with the current time beside it, and then the new plan can be signed. Then pick the
   default delegation level and press "Generate the playbooks": `plan.provision.yml` and `plan.rollback.yml` are written. When the generator flags gaps, their number and content are shown. Applying them to the machines is not
   done by the screen; the operator runs the commands in the 06 screen's help. When 05 is finalized again and the plan
   changes, the 06 card reads "the signature does not match the plan", and generating needs the earlier signature archived
   and a new signature on the new plan.
7. **The next round.** A finalized session cannot be changed and is shown read-only. When new observations arrive or a
   conclusion must change, press "Start a new review" on the 05 screen ("Start a new scope review" on the 02 screen). A new
   round starts under a new session id: conclusions and plan fields carry over, and a new signature is needed. The earlier
   round's plan and ledger entries stay as they are.
   **06 signs and generates only plans finalized in this workspace.** Finalizing in 05 records, in the state folder's
   processing record together with the plan, the digest of the policy the plan was made under, and 06 compares it with the active
   policy. A plan brought in from another workspace, or finalized before this version, can only be viewed and archived.
   Approve and generate a plan brought in from elsewhere with pqcota's `pqcota-approve` and `pqcota-provision`. To handle a
   plan finalized before this version in 06, open a new review round in 05 under the current policy and finalize again.


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

## 9. Upgrading

### v0.28.0 to v0.29.0

v0.29.0 takes in upstream pqcota v0.13.0. Upstream v0.12.0 moved the ruleset to `pqcota-enrich/v4` (JCA deployment identity), so the
sessions and plans built with v0.28.0 move on through a ruleset change round. The state format and the session and plan formats are
the same as v0.28.0. There is no need to pass through upstream v0.12.0; upgrade straight to v0.13.0.

1. Stop the screen, check that `-recover-status` prints `no unfinished transactions`, and back up the workspace as
   [section 7](#7-backup-and-restore) shows. Back up the upstream database too.
2. **Upgrade the upstream tools that read the same database first.** The v0.11.0 `pqcota-inventory` cannot open the history of a
   node that holds a snapshot written by v0.12.0 or later, and the v0.11.0 `pqcota-provision --dsn` can write the playbook on such a
   database and then fail with exit status 1. Keep the five repositories together at v0.13.0. If you do not upgrade upstream yet,
   keep pqcaton at v0.28.0 too.
3. Replace the three collectors on the nodes (`pqcota-nodescan`, `pqcota-netcap`, `pqcota-jvmscan`) and `collector.jar` with the
   v0.13.0 release assets as [section 2](#2-installation) shows. The release assets are also "cannot be verified", so
   `-allow-unverified-collectors` is still needed.
4. Replace the binary with the `go install` of [section 2](#2-installation), start the screen and observe again on 03. **Re-ingest the
   pqcota history with v4 too.** The ruleset is part of the snapshot fingerprint, so with a history that v0.11.0 ingested under v3
   the activation and baseline reconciliation on 05 do not pass. Re-ingest with
   `PQCOTA_ORG=<judgment org> pqcota-ingest -scope-assets <workspace>/scope-assets.csv <workspace>/results`.
5. When 05 shows "This session was opened under a different upstream ruleset", press "Start a new round under the current rules"
   (`pqcaton-decide new-round` on the command line). **Earlier conclusions move over only as reference values.** The evidence of Java
   assets changes to new per-deployment findings, so they show as "changed" in the report, and "removed" and "added" can include the
   effect of the identity rule change. Judge each item again or press "Adopt again". No rule links old conclusions to the new
   evidence automatically.
6. **If an asset scope rule has a user service `user@<uid>.service` in its `app_key`, fix it.** The v0.13.0 collectors give a user
   service the app key `user@<uid>.service/<unit>`, so the old value no longer matches. Find such rules as below, fix them on 02,
   finalize, and activate on 05.

   ```bash
   grep -n 'user@[0-9]*\.service' <workspace>/asset-scope.csv <workspace>/scope-assets.csv
   ```

7. **Do not compare an edge's observation count (`observed_count`) across the upgrade.** From v0.13.0 it counts the distinct
   connections seen in the observation window, not handshake messages. A lower count does not mean less traffic. Edges are also
   split per application that opened the connection, which upstream's diff shows as removed and added without a ruleset warning.
   pqcaton counts an edge by source, destination, port and protocol, so its reconciliation states stay the same.
8. The 06 screen does not sign or generate a plan of the v3 ruleset. A v3 plan already signed and its playbooks are only viewed and
   archived. Sign the plan finalized in the new round.

### v0.27.1 to v0.28.0

From v0.28.0 the screen does not start without the pqcota history, and activating a policy or adopting a baseline always
goes through the pqcota history reconciliation. The state format, the session and plan formats and the ruleset
(`pqcota-enrich/v3`) are the same as v0.27.1, so the workspace opens as it is. Upstream stays at v0.11.0.

1. **Put the connection in place first.** Create `pqcota.env` (0600, one line `PQCOTA_DSN=<connection>`) as "pqcota
   connection" in [section 2](#2-installation) shows, or have `PQCOTA_DSN` in the shell that starts the screen. A service
   or a launcher needs the file. Without it the screen does not start.
2. **Ingest pqcota under the organization of the judgments.** The pqcota history's organization must be the organization
   of the judgments. If you ingested under another organization (such as the default one), ingest again with
   `PQCOTA_ORG=<organization of the judgments> pqcota-ingest -scope-assets <workspace>/scope-assets.csv <workspace>/results`.
   Commands that gave `-upstream-org` a value other than the organization of the judgments now stop.
3. Stop the screen and replace the binary with `go install` from [section 2](#2-installation).
4. **Fix scripts that used `pqcaton-decide activate-scope -confirm-ingest`.** This version stops without writing anything when given that flag. Activate
   in 05 of the screen, or run `pqcaton-decide activate-scope` without the flag while `PQCOTA_DSN` is set.
5. **An activation that goes back to an earlier policy cannot be done in this version** (`upstream-revert`). There is no
   operator statement to use instead as in v0.27.x, so wait until the upstream issue is resolved or finalize a new policy.

### v0.27.0 to v0.27.1

v0.27.1 takes in upstream pqcota v0.11.0. The state format, the session and plan formats and the ruleset
(`pqcota-enrich/v3`) are the same as v0.27.0, so the workspace opens as it is.

1. **Upgrade the upstream tools that read records first.** The v0.10.7 `pqcota-records` cannot read records that v0.11.0
   writes. Replace `pqcota-records` on the same database with v0.11.0 before upgrading `pqcota-ingest`, `pqcota-approve` and
   `pqcota-provision`. Keep the five repositories at the same tag.
2. Stop the screen and take v0.27.1 with `go install …@v0.27.1`.
3. Take the collectors from the v0.11.0 release assets as [section 2](#2-installation) shows, checking them against
   `SHA256SUMS`. The release assets are "cannot be verified" too, so `-allow-unverified-collectors` is still needed.
4. **06 does not sign or generate a plan that carries source evidence or an approval guide** (`plan-fields`). Plans made
   by 05 carry no such fields, so the procedure you use stays the same. Check such a plan with `pqcota-approve --review`,
   then approve and generate it with the upstream tools.

### v0.26.x to v0.27.0

v0.27.0 keeps the state format, the session and plan formats and the ruleset (`pqcota-enrich/v3`) of v0.26.3, so the
workspace opens as it is. The pqcota version to use is still v0.10.7.

1. Stop the screen and replace the binary with `go install` from [section 2](#2-installation). If you run v0.26.2, its
   security fix is in this version as well.
2. **03 checks the collector versions before an observation starts.** Upstream builds today take their dependencies as
   replaces of the folders next to them, so they all show as "cannot be verified", and the observation does not start by
   default. Check that the two repositories next to them are at the same tag, then start the screen with
   `-allow-unverified-collectors` ("Collector versions" in [section 2](#2-installation)). If the screen runs as a
   service, add it to the service's `ExecStart` too. Collectors of another version cannot be used even with this
   option; rebuild them at v0.10.7.
3. **With the approval key at the default location no flag is needed.** A setup that keeps using `-approval-key` works as
   before. To move to the default location, follow "Getting the approval key ready" in [section 2](#2-installation) and
   put the verification key (`approval.pub`) there as well. When a key given explicitly cannot be used, the screen now
   does not start.
4. When "needs reclaiming" shows, "Processes left behind" on 03 shows their state. Records left before this version have
   no start time and show as "cannot tell whether it is the same process".

### v0.26.x to v0.26.3

v0.26.3 is a security patch that takes in upstream pqcota v0.10.7. v0.26.2 carries the same fix but cannot be installed
with `go install`, so it was released again as v0.26.3. If you already run v0.26.2, replace only the binary. The state format, the session and plan formats and the
ruleset (`pqcota-enrich/v3`) are the same as v0.26.1, so the workspace opens as it is.

1. Stop the screen and replace the binary with `go install` from [section 2](#2-installation).
2. **Upgrade upstream to v0.10.7 too.** `pqcota-approve` and `pqcota-provision` in particular must be v0.10.7: an
   approval signature in v0.10.6 could also pass on a different plan when a string in the plan contains a NUL character.
   Replace `pqcota-jvmscan` on the nodes with v0.10.7 as well.
3. A plan with a NUL character in a string is not finalized by 05 (`session-text`), and such a plan finalized earlier is
   not signed or generated by 06 (`plan-text`). Open a new review round, correct the value, and finalize again. A plan
   without a NUL character has the same approval canonical form as in v0.10.6, so existing approvals stay valid.

### v0.25 to v0.26

v0.26 records the asset scope policy's digest in the review session and the finalize processing record, and uses the policy
finalized in 02 for reconciliation only through the formal steps.

1. Stop the screen, check that `-recover-status` prints `no unfinished transactions`, and back the workspace up as in
   [section 7](#7-backup-and-restore). **There is a point of no return.** Finalizing 05, activating a policy or opening a
   baseline round in this version writes format 2 processing records into the state folder, and just before that the state document
   (`.pqcaton/state.json`) is raised to format 2. A state folder this version creates starts at format 2. From then on
   v0.25.x or earlier does not open the workspace: the screen does not start, and commands that write the state folder
   stop with "state.json is damaged or from another version". It is not damage; the older version cannot read the newer
   state folder. The older version does not stop until it reads a format 2 processing record, and until then it even offers to sign
   a plan made before an activation, so this version stops it at start through the state document. To go back to the
   older version, restore the backup.
2. Replace the binary with `go install` from [section 2](#2-installation) and start the screen. Upstream stays at v0.10.6.
3. **A review session made before this version records no policy, so 05 locks.** Ingest pqcota with the current
   `scope-assets.csv`, then open the baseline round at the top of 05. Like the activation, it reconciles the pqcota history
   against the current policy first and opens only when it matches. The old session is archived, a new round rebuilt under
   the current policy opens, and earlier conclusions are carried only as previous values. On the command line it is
   `pqcaton-decide baseline` (`PQCOTA_DSN` is needed; `pqcaton-decide baseline -confirm-ingest` is no longer supported).
4. Plans finalized before this version can only be viewed and archived in 06. Sign the plan finalized again in the
   baseline round.
5. Operators who finalize from the command line give `pqcaton-decide close` the same `-scope-assets` the session was
   opened with. If it differs from the session's policy, nothing is finalized.

### v0.24 to v0.25

v0.25 normalizes with the ruleset of upstream pqcota v0.10.6 (`pqcota-enrich/v3`). The sessions, plans and judgment ledger
built with v0.24.x stay as they are, and a round is opened to judge again under the current rules.

1. Stop the screen, check that `-recover-status` prints `no unfinished transactions`, and back up the workspace as
   [section 7](#7-backup-and-restore) says.
2. **Upgrade upstream with it.** Move the five upstream repositories to v0.10.6 (the pqcota version for each pqcaton
   version is in the [version table](compatibility.md), in Korean), and replace the collectors and
   `collector.jar` on the nodes with v0.10.6 too. Back up the upstream database before upgrading (as upstream v0.10.6 says).
   Mixed with the history of upstream v0.10.5 or earlier, the plan's evidence cannot be found. If upstream is not
   upgraded yet, keep pqcaton on v0.24.1 too.
3. Replace the binary with the `go install` in [section 2](#2-installation) and start the screen.
4. When the review screen (05) says "This session was opened under a different upstream ruleset", press "Start a new round
   under the current rules". The old session is archived byte for byte as `session.ruleset-<alias>-<session id>.json`, a
   new session under the current rules takes its place, and a change report (`session.ruleset-change-<session id>.json`)
   is written beside it. The command is `pqcaton-decide new-round`.
5. **Earlier conclusions are carried only as reference values.** Judge each item again, or press "Adopt again". What the
   report shows as removed, added or changed may come from the rule change alone.
6. When there is more than one installation under an asset key (the same-named OpenSSL seen at different paths), choose
   "I confirm this action applies to all of the installations below" before finalizing. Different actions per
   installation cannot be expressed in this version.
7. The 06 screen does not sign or generate a plan under the old ruleset. A plan already signed under it and its playbooks
   are left as they are, and can only be viewed and archived. Sign the plan finalized in the new round.
8. When the 03 or 04 screen names nodes whose JCA results cannot be confirmed as v0.10.6 collection results, check the
   version of the collectors and `collector.jar` on those nodes.

### v0.22 to v0.23

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
| `plan-ruleset` | The plan was judged under a different upstream ruleset, or the ruleset of its evidence cannot be confirmed, so nothing was signed or generated. Existing files are unchanged | Finalize it again in a new round. If upstream still runs v0.10.5 or earlier, generate with pqcaton v0.24.x or upstream `pqcota-provision` v0.10.5 |
| `plan-scope` | The plan does not lead back to a finalize recorded in this workspace (it was brought in from elsewhere, the plan file changed after it was finalized, or it was finalized before this version and records no policy), or the asset-scope policy it was made under differs from the active one, so nothing was signed or generated. Existing files are unchanged | Open a new review round in 05 under the current policy and finalize again. Approve and generate a plan brought in from elsewhere with `pqcota-approve` and `pqcota-provision` |
| `plan-fields` | The plan (or the signed plan) carries source evidence or an approval guide (upstream v0.11.0), so nothing was signed or generated. The plan is not damaged: pqcaton does not yet review that content on 06 or put it into playbooks. Existing files are unchanged | Check the content with `pqcota-approve --review`, then approve and generate with `pqcota-approve` and `pqcota-provision`. A signed plan that has it can be archived |
| `plan-text` | The plan (or the signed plan) contains a NUL character in a string, so nothing was signed or generated. An approval of such a plan could also pass on a different plan, so pqcota refuses to sign or generate it. Existing files are unchanged | Open a new review round, correct the value and finalize again. If the signed plan has it, archive that first |
| `session-text` | The plan this finalize would write contains a NUL character in a string, so nothing was finalized. The session, plan and processing record are unchanged | Correct the value and finalize again |
| `scope-policy` | The asset-scope policy the review session was built under differs from the current one, or the session records no policy (made before this version), so nothing was finalized. The files are unchanged | Finalize under the policy the session was built with, or open a new session under the current policy. On the command line, give `pqcaton-decide close` the same `-scope-assets` the session was opened with |
| `scope-baseline` | The review session records no asset-scope policy (made before this version), so nothing was saved or finalized | Ingest pqcota with the current `scope-assets.csv`, then open the baseline round at the top of the review screen. It goes through the pqcota history reconciliation |
| `scope-unactivated` | `scope-assets.csv` changed without an activation record, so nothing was saved or finalized | Restore the previous policy file, or re-ingest upstream with the new policy and go through the policy activation |
| `upstream-unreachable` | The pqcota history could not be reached, or the lookup did not finish within 10 seconds, so no policy was activated or adopted as the baseline (503). The files are unchanged | Check the pqcota connection and try again. There is no operator statement to use instead |
| `upstream-org` | The connected pqcota history, the organization of the judgments and the review session's organization are not the same, so nothing was looked up. Nothing was written | Use the pqcota history of the organization of the judgments. Another organization's history cannot activate a policy or adopt a baseline |
| `upstream-missing` | In the pqcota history, the latest ingest of some nodes is missing or differs from the results under this policy. Nothing was written | Re-ingest pqcota with this policy using the command shown in 05, then try again |
| `upstream-empty` | The results folder holds no observations to reconcile with the pqcota history. Nothing was written | Observe in 03, then try again |
| `upstream-revert` | Results under this policy are in the pqcota history but are not the latest ingest of their nodes; reverting to an earlier policy looks like this. Nothing was written | This version blocks reverting to an earlier policy (until pqcota-inventory#1 is resolved) |
| `scope-same` | The policy finalized in 02 is already the active policy (only notes or spacing may differ), so there is nothing to activate. Nothing was written | Nothing to do |
| `scope-not-finalized` | There is no policy finalized in 02, `asset-scope.csv` changed after it was finalized, or it cannot be read, so nothing was activated | Finalize it in 02. A hand-edited file is not activated |
| `ruleset-changed` | The session was opened under a different upstream ruleset, or records none, so nothing was saved or finalized, to keep the two from mixing | Press "Start a new round under the current rules" at the top of the review screen (the command is `pqcaton-decide new-round`). Earlier conclusions are carried only as reference values, so judge each item again |
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
| `signed-exists` | A signed plan file already exists and was not overwritten | If it does not match the current plan, press "Archive the earlier signature" on the 06 screen, then sign |
| `not-found` | There is no screen at this address | Start again from the first screen |
| `method` | This kind of request is not accepted | Use the screen's buttons |
| `bad-request` | The request could not be handled | Reload the screen and do it again |
| `input-missing` | A file this screen needs is not there | Do the earlier step first (declaration, results) |
| `input-broken` | An input file could not be read | Find the reference in the server log and fix that file |
| `forbidden` | The request protection refused it | Open the screen again from the start in a browser on the machine running it. From another machine, follow [section 5](#5-viewing-from-another-machine) |
| `internal` | It could not be handled | Find the reference in the server log and ask for support |
