English · [한국어](operator-guide.ko.md)

# pqcaton operator guide

<!-- SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd. -->
<!-- SPDX-License-Identifier: BUSL-1.1 -->

This guide is for someone who installs the pqcaton screen (`pqcaton-ui`) on one machine and runs it alone. It covers v0.33.0.
The Korean edition is [operator-guide.ko.md](operator-guide.ko.md). The two editions have the same contents, commands and error
codes, and a test checks that.

**Scope.** The procedure inside pqcaton (declaration, scope, observation, reconciliation, judgment, finalize, approval,
playbook generation) runs on the screen without a terminal. Observations run with the Ansible and collectors the operator
installs. After a separate confirmation the screen applies the L2 playbook to the target nodes. Activating and rolling back are the operator's job.
Installing and recovering need a terminal or a file manager.

**Trust model.** The screen runs on one machine as **one OS user** and opens only on **loopback** (it has no
authentication). That user can reach the nodes to observe over SSH and run the collectors there **as root (become)**,
and, when signing at 06, **reads the approval private key.** The user's account and the workspace are therefore the
boundary of these powers. The 06 screen signs and checks with one key (single key, single approver).

**Trust around the collector signing keys.** 03 reads the signing key of each collector ID ("Preparing the collector signing keys" in
section 2) and passes it to the nodes at every observation. The key goes only on the standard input of the collector on the node and is
left in no file, command line or Ansible configuration, but **while the observation runs it is in the environment of the collector process
on the node, where that node's root can read it.** On the controller, the account that runs the screen and root can read the key files
and the temporary keys. The node check looks only at sudo's I/O logging. **What audit tools outside sudo (auditd, tlog, pam_tty_audit and
the like), other sudo policy plugins, or programs replaced by the node's root record is the responsibility of the environment.** If the key
of an ID leaks, results of every node can be forged under that ID, so change the key then.

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

When this section is done, the screen starts on this machine. Do 1 to 3 once; do 4 and 5 when you use those steps.

1. [Installing the screen](#installing-the-screen): the `pqcaton-ui` binary
2. [Preparing pqcota](#preparing-pqcota): PostgreSQL 16 server and psql (ready first), roles, upstream commands, schema, privileges and connection
3. Starting the screen: [section 4](#4-starting-and-stopping). To start it at login, [the service and the launcher](#the-service-and-the-launcher)
4. [Getting ready to run observations](#getting-ready-to-run-observations): to observe from 03
5. [Getting the approval key ready](#getting-the-approval-key-ready): to sign on 06

<a id="installing-the-screen"></a>

**Installing the screen.** Go 1.26.11 is needed. The command below makes `~/.local/bin/pqcaton-ui`. That folder must be on `PATH`.

```bash
GOBIN="$HOME/.local/bin" go install github.com/sntsoftgit/pqcaton/inventory/cmd/pqcaton-ui@v0.33.0
```

When `pqcaton-ui -h` prints the list of flags, it is installed. It ends with exit code 2 after printing the help, which is normal.
An install script that uses `set -e` stops at that line, so check with `command -v pqcaton-ui` in scripts.
That only checks that the command is found on `PATH`; it does not run it. The screen starts only after "Preparing pqcota" next.

<a id="preparing-pqcota"></a>

**Preparing pqcota.** The screen reads the pqcota history (PostgreSQL), so prepare it before the screen. Follow steps 1 to 5 below and
the screen starts. With a pqcota history already in use, follow "Moving an existing history to three roles" in
[section 9, "v0.30.0 to v0.31.0"](#v0300-to-v0310).

1. **PostgreSQL and three roles.** **The PostgreSQL 16 server you will connect to, and `psql` on the machine where you run the
   commands below, must be ready first.** The server can be on this machine or another one. If a component is missing, follow the
   installation steps for its operating system on the PostgreSQL [downloads page](https://www.postgresql.org/download/)
   ([Linux](https://www.postgresql.org/download/linux/), [macOS](https://www.postgresql.org/download/macosx/)). Once ready, create the
   three roles with the commands below. Each role gets its own privileges; the names are examples.

   | Role | What it does |
   |---|---|
   | `pqcota_schema` | Creates the schema and owns the tables. Only `pqcota-inventory -init-schema` runs as this role |
   | `pqcota_ingest` | Loads (`pqcota-ingest` and the other writing commands). It has only read and write privileges on the tables and runs with `PQCOTA_AUTO_DDL=0` |
   | `pqcaton_ui` | The screen reads the history. Read privileges only |

   Run this in `psql` as an administrator. The passwords are typed only where `\password` asks for them, so they stay out of command
   lines and shell history.

   ```sql
   CREATE ROLE pqcota_schema LOGIN;
   \password pqcota_schema
   CREATE ROLE pqcota_ingest LOGIN;
   \password pqcota_ingest
   CREATE ROLE pqcaton_ui LOGIN;
   \password pqcaton_ui
   CREATE DATABASE pqcota OWNER pqcota_schema;
   REVOKE ALL ON DATABASE pqcota FROM PUBLIC;
   GRANT CONNECT ON DATABASE pqcota TO pqcota_ingest, pqcaton_ui;
   ```

   Keep the passwords out of the connection information, in `~/.pgpass` (0600). The file goes on the machine that runs the commands
   and the screen, not the database server, in the home directory of the user who runs them: one line per account,
   `host:port:database:user:password`, filled in with an editor. To keep it elsewhere, point `PGPASSFILE` at it. The connection
   information can then be written without a secret, as in `postgres://pqcaton_ui@<host>:5432/pqcota`.

   Create an empty file only if there is none. Recreating an existing `~/.pgpass` with `install` empties it, so an existing one is
   left alone.

   ```bash
   [ -e ~/.pgpass ] || install -m 600 /dev/null ~/.pgpass
   ```

   Write one line per account; only the roles used on this machine need a line. For example (server on the same machine, `<…>` is
   the real password):

   ```text
   127.0.0.1:5432:pqcota:pqcota_schema:<schema role password>
   127.0.0.1:5432:pqcota:pqcota_ingest:<loading role password>
   127.0.0.1:5432:pqcota:pqcaton_ui:<query role password>
   ```

   The first matching line from the top is used. The host field must match the connection information character for character
   (`localhost` and `127.0.0.1` count as different values).

2. **Upstream central commands.** The loading, query, approval, generation and key commands. The upstream release bundle
   (`pqcota-ctl`) exists **for Linux only**, so pick the path for this machine. Either way the commands go into `~/.local/bin`, which
   must be on your `PATH`.

   **Linux (amd64, arm64): the release bundle.** Check its checksum and its attestation (pinned to the upstream repository, its release
   workflow and the tag), then unpack it. It needs `gh` (2.49 or later, logged in). On an arm64 machine use `linux-arm64` instead of
   `linux-amd64`.

   ```bash
   mkdir -p ~/pqcaton/pqcota-ctl-v0.20.0 && cd ~/pqcaton/pqcota-ctl-v0.20.0
   gh release download v0.20.0 -R randyinthedev-hash/pqcota -p 'pqcota-ctl-linux-amd64.tar.gz' -p SHA256SUMS
   sha256sum -c --ignore-missing SHA256SUMS
   gh attestation verify pqcota-ctl-linux-amd64.tar.gz --repo randyinthedev-hash/pqcota \
     --signer-workflow randyinthedev-hash/pqcota/.github/workflows/release.yml --source-ref refs/tags/v0.20.0 --deny-self-hosted-runners
   tar -xzf pqcota-ctl-linux-amd64.tar.gz
   install -m 755 pqcota-ctl-linux-amd64/pqcota-* ~/.local/bin/
   ```

   **macOS (Intel, Apple Silicon): a source build pinned to the tag.** Put the five upstream repositories side by side at the v0.20.0
   tag and build. It needs Git and Go. Most upstream commands cannot be installed with `go install …@v0.20.0` because of the `replace`
   lines in their `go.mod`.

   ```bash
   mkdir -p ~/pqcaton/pqcota-src-v0.20.0 && cd ~/pqcaton/pqcota-src-v0.20.0
   for r in pqcota pqcota-common pqcota-inventory pqcota-discovery pqcota-provisioning; do git clone -q --branch v0.20.0 https://github.com/randyinthedev-hash/$r; done
   cd pqcota
   D=github.com/randyinthedev-hash
   go build -o ~/.local/bin/ $D/pqcota-common/cmd/... $D/pqcota-discovery/cmd/... $D/pqcota-inventory/cmd/... $D/pqcota-provisioning/cmd/...
   ```

   When `pqcota-inventory -h` prints the list of flags, it is done. Git on macOS may warn "refs/tags/v0.20.0 … is not a commit!" while
   cloning. If it does, check that the clone ended without failure (exit status 0) and that, from the `pqcota` folder,
   `for r in pqcota pqcota-common pqcota-inventory pqcota-discovery pqcota-provisioning; do git -C ../$r describe --tags --exact-match; done`
   prints `v0.20.0` on all five lines. If any differs or fails, stop the installation, check the state of that repository, and clone again into a new empty folder. This is the "Build" section of the upstream
   [build guide (v0.20.0)](https://github.com/randyinthedev-hash/pqcota/blob/v0.20.0/docs/build.md), and it works on Linux too.

3. **Creating the history schema.** Create the schema by running `pqcota-inventory -init-schema` once as `pqcota_schema`. It runs in one
   transaction and ends with a line such as `schema ready: 11 created, 0 already present`. Running it again leaves the existing tables and
   data as they are and, after an upgrade, adds only what the new version needs, so **run it once after every upstream upgrade.** If you
   keep generation records outside the screen with `pqcota-provision`, also run `pqcota-records -init-schema` as the same role. From
   upstream v0.16.0 the query commands create no tables, and the screen also opens the history for reading only. Without the schema the
   screen does not start ("the pqcota history schema is missing or incomplete; run `pqcota-inventory -init-schema` as the role that owns
   the schema"). Replace each `<…>` with the real value before running it.

   ```bash
   PQCOTA_DSN='postgres://pqcota_schema@<host>:5432/pqcota' pqcota-inventory -init-schema
   ```

4. **Privileges of the loading and screen accounts.** After the schema exists, run this as an administrator in the `pqcota` database.
   The first lines cover the tables that exist now; the last three (`ALTER DEFAULT PRIVILEGES FOR ROLE pqcota_schema`) cover the tables
   `pqcota_schema` creates from now on. The latter alone grants nothing on tables that already exist, so run both.

   ```sql
   GRANT USAGE ON SCHEMA public TO pqcota_ingest, pqcaton_ui;
   GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO pqcota_ingest;
   GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO pqcota_ingest;
   GRANT SELECT ON ALL TABLES IN SCHEMA public TO pqcaton_ui;
   ALTER DEFAULT PRIVILEGES FOR ROLE pqcota_schema IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO pqcota_ingest;
   ALTER DEFAULT PRIVILEGES FOR ROLE pqcota_schema IN SCHEMA public GRANT USAGE ON SEQUENCES TO pqcota_ingest;
   ALTER DEFAULT PRIVILEGES FOR ROLE pqcota_schema IN SCHEMA public GRANT SELECT ON TABLES TO pqcaton_ui;
   ```

   - **Run the loading account with `PQCOTA_AUTO_DDL=0`.** Without it the writing commands run the schema statements every time they
     open the store, and only the owner of the tables may do that. A loading account that does not own them fails with "permission
     denied for schema public".
   - **The screen account needs only `SELECT` on every history table and `USAGE` on the schema.** On start the screen checks every table
     and column of the history store. Without the privileges it does not start ("this account lacks privileges to read the pqcota
     history (SELECT on its tables, USAGE on their schema); initializing the schema does not change them"). Initializing does not fix
     it; grant the privileges.
   - The full table of roles and privileges is upstream's [setting up the database (v0.20.0)](https://github.com/randyinthedev-hash/pqcota-inventory/blob/v0.20.0/cmd/README.md#setting-up-the-database).

5. **Screen connection.** Keep the connection of the screen account as one line in `pqcota.env`. Do not put the value on the
   command line, so the password stays out of the shell history. Create the empty file (0600) with the commands below, then put one
   line, `PQCOTA_DSN=postgres://pqcaton_ui@<host>:5432/pqcota`, in it with an editor. The file rules are in "pqcota connection" below.

   ```bash
   mkdir -p ~/.config/pqcaton && chmod 700 ~/.config/pqcaton
   install -m 600 /dev/null ~/.config/pqcaton/pqcota.env
   ```

   On macOS only the location differs.

   ```bash
   mkdir -p "$HOME/Library/Application Support/pqcaton" && chmod 700 "$HOME/Library/Application Support/pqcaton"
   install -m 600 /dev/null "$HOME/Library/Application Support/pqcaton/pqcota.env"
   ```

Now run `pqcaton-ui -no-browser` as in [section 4](#4-starting-and-stopping). The line `screen: http://127.0.0.1:8765/  (workspace …)`
means the screen is up. If it does not start, look up the reason after `❌` in "pqcota connection" below and in the
[error table in section 10](#10-error-table).

**pqcota connection.** pqcaton runs only where pqcota is installed, connected and working. On start the screen connects
to the pqcota history and must pass one read of it (schema and read permission) before it comes up. If there is no
connection, or connecting and the read do not finish within 10 seconds, it stops before opening the port or writing the
address file. It only looks things up, so an account with read permission is enough.

- **The connection is `PQCOTA_DSN`.** When the environment variable is set it is used; when it is empty, the screen itself
  reads `pqcota.env` in the same user configuration folder as the approval key (Linux `~/.config/pqcaton/pqcota.env`,
  macOS `~/Library/Application Support/pqcaton/pqcota.env`). The service and the launcher use this file as it is.
- **To ingest into the history from the screen, split the connection in two.** Put the read connection in
  `PQCOTA_READONLY_DSN` and the ingest account's connection to the same database in `PQCOTA_DSN`. For each name the
  environment variable comes first; when it is empty, the line of the same name in `pqcota.env` is used.

  | Set | The screen reads with | Ingesting from the screen |
  |---|---|---|
  | only `PQCOTA_DSN` | `PQCOTA_DSN` (as before) | off |
  | only `PQCOTA_READONLY_DSN` | `PQCOTA_READONLY_DSN` | off |
  | both | `PQCOTA_READONLY_DSN` | the value of `PQCOTA_DSN` is handed to the ingest process |

  An upgrade alone never makes `PQCOTA_DSN` an ingest connection. To turn ingesting on, move the current read account's
  value to `PQCOTA_READONLY_DSN` and put the ingest account's value in `PQCOTA_DSN`. **The name does not restrict
  privileges.** That the read account is read-only must be guaranteed by PostgreSQL privileges. The screen keeps reading
  with the read-only account, but **a screen server with the ingest connection set can read that value** (it hands it to a
  separate process when ingesting).
- **File rules.** It must be a regular file, not a link, owned by this user, with no permission for the group or others
  (0600), with a single hard link, and outside the workspace. If any rule is broken the screen does not start and says why.
- **The content is read only as data.** Apart from blank lines and `#` comments, only the lines `PQCOTA_DSN=<connection>`
  and `PQCOTA_READONLY_DSN=<connection>` are accepted (each name once). Other variables, `export`, quotes, surrounding spaces and a second line are refused. It is not evaluated by
  a shell, so characters such as `$(…)` in the value stay part of the value.
- **The value is left nowhere.** Not in the workspace, processing records, logs, error messages or the screen, and not
  passed to the observation processes the screen starts. The screen has no place to type it either.
- **The pqcota organization is the organization of the judgments.** The screen opens only the pqcota history of the
  organization of the judgments (`-org`, or the declaration's organization when not given). `-upstream-org` accepts only the
  same value; another organization stops the start. If the review session belongs to another organization, 05 refuses with
  `upstream-org` without looking anything up. So ingest pqcota under that organization too
  (`PQCOTA_ORG='<organization>' pqcota-ingest …`).

Once the screen is up, each task needs more, as below. The only thing the screen runs itself is `ansible-playbook` for observation;
it signs approvals and generates playbooks inside the screen.

| Task | What it needs |
|---|---|
| 03 Observation | Ansible and node access, the official collectors, the collector signing keys and the trusted public keys ("Getting ready to run observations" and "Preparing the collector signing keys" below) |
| Loading observation results into the history | `pqcota-ingest` and a loading account. The results of 03 are checked (`pqcota-ingest -verify-keys`) and then loaded under the judgment organization (section 6). Reconciliation in 04 and 05 and policy activation in 05 look at this load |
| 06 Provisioning (approval signature and playbook generation) | An approval key (`pqcota-keygen` in "Getting the approval key ready" below). `pqcota-approve` and `pqcota-provision` only to sign or generate outside the screen |
| Applying to the nodes and rolling back | `pqcota-hosts` (to make the Ansible inventory) and `ansible-playbook` (as 06 explains) |
| Trimming old history | `pqcota-prune` (optional) |

<a id="the-service-and-the-launcher"></a>

**Linux: the service and the launcher.** The service starts the screen when you log in (without opening a browser), and
the launcher starts the service and then opens the screen in a browser. Finish "Preparing pqcota" above before enabling the service (the service reads the same `pqcota.env`). The launcher appears as "pqcaton" in the desktop's
program list. The launcher opens it with `pqcaton-ui -open`, which waits (up to 15 seconds) until the server at the
recorded address answers that it is the screen of the same workspace, so it never opens a stale address left by an
abnormal exit or a screen that is not up yet. If nothing answers, it opens nothing.

```bash
mkdir -p ~/.config/systemd/user ~/.local/share/applications
curl -fsSLo ~/.config/systemd/user/pqcaton-ui.service https://raw.githubusercontent.com/sntsoftgit/pqcaton/v0.33.0/packaging/linux/pqcaton-ui.service
curl -fsSLo ~/.local/share/applications/pqcaton-ui.desktop https://raw.githubusercontent.com/sntsoftgit/pqcaton/v0.33.0/packaging/linux/pqcaton-ui.desktop
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
curl -fsSLo ~/Desktop/pqcaton-ui.command https://raw.githubusercontent.com/sntsoftgit/pqcaton/v0.33.0/packaging/macos/pqcaton-ui.command
chmod +x ~/Desktop/pqcaton-ui.command
```

<a id="getting-ready-to-run-observations"></a>

**Getting ready to run observations.** Running an observation from the 03 Observation screen needs Ansible and the collectors on this
machine and an account on the nodes. The screen checks these and the signing keys ("Preparing the collector signing keys" below) before
it starts and, when something is missing, says what to do.

Prepare them in this order. The details follow in the items after it.

1. **Ansible.** Install ansible-core (checked with 2.16.3 and 2.21.6) so that `ansible-playbook` is on the `PATH`.
2. **Node access.** Make the Linux nodes reachable over SSH as root, or as an account with password-less sudo (`NOPASSWD`), and connect to
   each node once with `ssh` to accept its host key.
3. **Collectors.** Fetch and check them as in "Checking the collectors you download" below.
4. **Signing keys.** Make them as in "Preparing the collector signing keys" below.

Then start the screen again and see that the check lines on 03 show nothing missing.

- **Ansible.** `ansible-playbook` is looked up on the `PATH`. It was checked with ansible-core 2.16.3 and 2.21.6. 03 **does not read the
  operator's Ansible configuration** ("The Ansible configuration 03 uses" below).
- **Nodes and the account.** In this version 03 observes **Linux nodes only**. The nodes are reached over SSH and the collectors run as
  root. The account is one of two:
  - **Reached as root.** Ansible skips privilege escalation when the account and the become user are the same, so sudo is not used.
  - **Reached as an ordinary account that becomes root with sudo.** That account must use sudo **without a password** (`NOPASSWD`), and
    sudo must not record the input and output of commands ("Node check" below).

  Connect to a new node once with `ssh` beforehand to accept its host key. The screen checks host keys.
- **Windows nodes are not observed.** In this version 03 does not pass signing keys to Windows nodes, so their results would not pass the
  signature check. With a Windows node in the access file, 03 shows it and does not start. Take Windows nodes out of the access file and
  observe them by running `pqcota-cngscan` and `pqcota-jvmscan` yourself with their signing keys.
- **Collectors.** Put the collectors of the upstream release in the workspace's `collectors` folder (or `-collectors
  <folder>`), shaped like this. Keep them on pqcota v0.20.0 (the pqcota version for each pqcaton version is in the
  [version table](compatibility.md)), and the `collector.jar` put on the nodes on the same version.
  This screen normalizes the collection results it reads with the v0.20.0 ruleset (`pqcota-enrich/v4`). From v0.12.0
  `pqcota-jvmscan` reports the JVMs that run the same application as one deployment, and from v0.13.0 `pqcota-nodescan` and
  `pqcota-netcap` give a user service the app key `user@<uid>.service/<unit>` and give each edge to the application that
  opened the connection. From v0.15.0 every collector signs its results with `PQCOTA_SIGN_KEY`, and one given a key it cannot sign with
  stops without emitting a result. A
  `pqcota-jvmscan` earlier than v0.10.7 writes its results over several lines on a node with two or more JVMs, and the
  whole ingest request that holds that node is refused. An earlier `pqcota-jvmscan` or
  `collector.jar` does not carry the per-provider list of PQC services, so the JCA readiness becomes `unknown`, and the 03
  and 04 screens name such nodes as "the JCA results cannot be confirmed as v0.10.6 collection results" (see the version
  table under 「맞는 조합」 in the README).

```text
collectors/
├── linux-amd64/     pqcota-nodescan · pqcota-netcap · pqcota-jvmscan
├── linux-arm64/     when there are arm64 nodes
└── collector.jar    without it, the provider chains of running JVMs are not observed
```

**Checking the collectors you download.** The upstream collectors come as release assets, and fetching and checking them is done
with the preparation tool `pqcaton-collectors`. It fetches only the assets of the upstream version go.mod pins (v0.20.0 for this
version) and never runs what it downloads.

```bash
GOBIN="$HOME/.local/bin" go install github.com/sntsoftgit/pqcaton/inventory/cmd/pqcaton-collectors@v0.33.0
pqcaton-collectors fetch -arch linux-amd64 -out ~/pqcaton/collectors-v0.20.0
pqcaton-collectors verify -arch linux-amd64 -dir ~/pqcaton/collectors-v0.20.0
```

- **Conditions.** `gh` (GitHub CLI 2.49 or later) must be there and logged in (`gh auth login`). Checking the attestations and
  fetching the trust root need access to GitHub (`github.com` and the asset download hosts). `verify` fetches the trust root again,
  so it needs the network too. To use the collectors on a machine without network access, move a folder prepared and checked on a
  machine that has it.
- **Target (`-arch`).** Always give the architecture of the nodes that run the collectors (`linux-amd64` or `linux-arm64`). It is
  written to `provenance/TARGET` in the prepared folder and `verify` compares it again. The OS and architecture in the programs' Go
  build information must match it too. With nodes of both architectures, fetch once per target.
- **What `fetch` does.** It downloads the assets into a new temporary folder next to the target, checks their attestations (pinned to
  the upstream repository, its release workflow and that tag), compares them with `SHA256SUMS`, unpacks them safely (links, devices
  and path escapes are refused), compares each file's hash, version, commit and content hash (h1) with the release manifest, checks
  the attestations of the unpacked programs, and moves the folder into place at once only when exactly the expected files are
  there. If anything fails, the target is not created. An existing target is never overwritten.
- **Using it.** Check it again with `verify` before starting the screen, and give it to the screen with
  `-collectors ~/pqcaton/collectors-v0.20.0/collectors`.

<a id="preparing-the-collector-signing-keys"></a>

**Preparing the collector signing keys.** 03 has the collectors sign their results and uses as current only results whose signature
checks out. The upstream ingest (`pqcota-ingest`) checks them with the same keys. So an observation **starts only with a signing key for
each collector ID.**

`pqcota-keygen` is in the `pqcota-ctl` bundle of "Preparing pqcota". The third line gathers the public keys of the three into one
line of `verify.pub`. It runs under umask 077 too, so the file is never writable by the group or others, from the moment it is
created. Made plainly on a distribution whose default umask is 002, it would be group-writable, and 03 refuses that. The final `chmod`
opens it to others for reading only; 03 also takes it at 0600.

```bash
mkdir -p ~/.config/pqcaton && mkdir -m 700 ~/.config/pqcaton/collector-keys
for id in openssl-collector network-collector jvm-collector; do (umask 077 && pqcota-keygen -collector "$id" > ~/.config/pqcaton/collector-keys/"$id".key); done
(umask 077 && for id in openssl-collector network-collector jvm-collector; do sed -n 's/^PQCOTA_VERIFY_KEY=//p' ~/.config/pqcaton/collector-keys/"$id".key; done | paste -sd, - | sed 's/^/PQCOTA_VERIFY_KEY=/' > ~/.config/pqcaton/collector-keys/verify.pub)
chmod 644 ~/.config/pqcaton/collector-keys/verify.pub
```

On macOS the place is `~/Library/Application Support/pqcaton/collector-keys`; replace `~/.config/pqcaton` in the commands above with it.
Collectors you run yourself outside 03 must sign with the key of the same ID too, or the ingest refuses their results. To change a key,
first add the new public key to `verify.pub` and to the ingest's value (an ID can hold two), then replace the signing key file, check
with an observation and `pqcota-ingest -verify-keys`, and only then drop the old public key.

- **Collector IDs and keys.** The ID written in a result is fixed per command: `pqcota-nodescan` writes `openssl-collector`,
  `pqcota-netcap` `network-collector`, and `pqcota-jvmscan` `jvm-collector`. Without `collector.jar` the `jvm-collector` key is not needed.
  There is one key per ID, and **every node running a collector of that ID shares its key.** Upstream does not bind keys to nodes yet, so
  whoever holds the key of an ID can sign results of any node under that ID. What tells the nodes apart is the registration list and the
  machine fingerprint check.
- **Where.** The signing keys are `pqcaton/collector-keys/<ID>.key` in the user configuration folder, and the trusted public keys are
  `verify.pub` in the same folder. Keep the folder outside the workspace, open only to its owner (0700), and each key file readable only by
  its owner (0600). This is a different place and purpose from the approval key (06). To keep them elsewhere, give
  `-collector-keys <folder>` and `-collector-verify-keys <file>`.
- **Trusted public keys (`verify.pub`).** One line `PQCOTA_VERIFY_KEY=<ID>:<public key>,…`, **the same value you give the ingest as
  `PQCOTA_VERIFY_KEY`.** 03 checks results only with this value (it never derives it from the private keys). A file others can change, a
  file holding a private key, and the old list that binds no key to an ID are refused.
- **What 03 blocks.** A missing ID, a key of another ID than its file name, a key with no collector ID (an approval-style key made
  without the flag), permissions too wide, a signing key not in `verify.pub`, one public key under two IDs, and the approval key used as a
  signing key. The "Collector signing keys" and "Trusted collector public keys" lines on 03 show the missing IDs and why.

**What the two checks look at.** The release check (`pqcaton-collectors`) asks **is this file the very file that upstream's official
release published**: who built it from which tag (the attestation) and whether the bytes equal the published hashes. The version
check on the 03 screen asks, right before an observation, **is this file the build of the version this release requires**: the
version and content hash (h1) in the Go build information, and the JAR's version, commit and modified flag. The screen uses no
network and does not look at origin, so it does not replace the release check. Use both.

**Collector versions.** Before an observation starts, 03 checks the version of each collector. It does not run them; it
reads only the Go build information. "Verified" is only a build whose collector itself is at the pinned version, whose main
module content hash (h1) in the build information equals the value this version pins (the repository's `go.sum`), and whose
dependencies carry no replace either.

**The release assets of upstream v0.20.0 are "verified".** From v0.14.0 the upstream release builds the sibling modules from
the same tag's modules instead of the folders next to it, so the build information keeps the collector itself and its
dependencies at that version with no replace. A build of your own next to the sibling folders (as the upstream demo does) is still
"cannot be verified": a collector built itself through a replace shows "actual source: local replace (devel)", and one built
from a tag checkout shows "collector source v0.20.0 verified · replaced dependency sources cannot be verified". A build of your
own carries no h1 for its main module, so even without a replace it cannot be verified: "no module content hash (h1) (built
locally)".

- **Does not match.** When another version shows, the collectors mix versions, or **the version is the same but the content
  hash (h1) differs from the pinned one** (other content under the same version), the observation does not start, and no
  option lets it through. Replace them with the upstream release assets of that version.
- **Cannot be verified.** Collectors whose version cannot be trusted do not start by default either: built with a
  replace from another repository as the upstream demo does, built with `go build -buildvcs=false`, built from a modified tree, or
  files with no build information. To use them knowingly, start the screen with `-allow-unverified-collectors`. The
  allowance and the list of files show on the 03 screen and on standard error, and are not kept in the workspace.
- **`collector.jar`.** It is optional (without it there is no such check). When it is there, **from v0.30.0 it is checked as
  strictly as the collectors.** From v0.14.0 the official JAR carries its version (`Implementation-Version`), commit
  (`X-Pqcota-Commit`) and whether it was modified (`X-Pqcota-Dirty`) in its manifest.
  - **Verified**: the release version is the pinned one, the commit is 40 or 64 hexadecimal characters and the modified flag
    is `false`. 03's collector.jar row shows its version and the first 12 characters of its commit, with the full commit in the
    row's description (the `title` shown on hover).
  - **Does not match**: the release version differs. No option lets it through.
  - **Cannot be verified**: no version, the version of a local upstream build (`devel`, a `git describe` form), a modified
    tree (`X-Pqcota-Dirty: true`), or a missing or malformed commit or modified flag. It does not start by default and turns
    into a warning with `-allow-unverified-collectors`. The version of a JAR that cannot be verified is not counted when
    checking whether the collectors mix versions.

  **Up to v0.29.x a JAR with no version only got a warning.** Keeping a JAR you built yourself now needs the override. The
  usual fix is to replace it with the upstream release `collector.jar`. Manifest values are cut at 64 characters on the screen.

<a id="the-ansible-configuration-03-uses"></a>

**The Ansible configuration 03 uses.** 03 passes the signing keys on the standard input of the nodes. Anything else in that path could
leak them (in the lab, a callback plugin in the operator's folder received the keys, and so did a replaced sudo executable), so only the
verified conditions are used.

- Each run reads only one configuration file made by the product. **The operator's `ansible.cfg` (`./ansible.cfg`, `~/.ansible.cfg`,
  `/etc/ansible/ansible.cfg`) and `ANSIBLE_*` environment variables are not used by 03 observations.** Plugins and collections in the
  operator's folders are not loaded either.
- The connection is the built-in ssh, privilege escalation the built-in sudo (`sudo -H -S -n`, root), pipelining is on, and no remote
  temporary file is kept.
- The ssh client configuration (`ProxyJump`, `IdentityFile`, `Port` and so on in `~/.ssh/config`) and `~/.ssh/known_hosts` are read by ssh
  and still apply, and so does the ssh agent (`SSH_AUTH_SOCK`).
- How to move the values you kept in `ansible.cfg` is in [section 9, "v0.30.0 to v0.31.0"](#v0300-to-v0310).

<a id="node-check"></a>

**Node check.** When an observation starts, 03 first checks every node **without any key**. For each node it reads the account's uid and,
when it is not root, `LC_ALL=C sudo -n -l`. Keys are made and the observation starts only when every node passes. If one node does not
pass, no node is observed, the run fails, and the node cleanup ends as "not needed" (the observation never reached the nodes and the check
leaves nothing there). The run log keeps the node, the reason, the policy lines and the command to check it on the node.

| Verdict | When |
|---|---|
| Pass | Reached as root, or **no I/O logging setting was found** in the output form of `sudo -n -l` this check supports. What this check cannot see is in "Trust" below |
| Refused | I/O logging is on (`log_input`, `log_output`, `log_stdin`, `log_stdout`, `log_stderr`, `log_ttyin`, `log_ttyout`), a rule's `LOG_INPUT:` or `LOG_OUTPUT:`, or `requiretty`. The key would be kept in those records |
| Cannot be confirmed | sudo asks for a password (not `NOPASSWD`), `sudo -n -l` fails, the output is in another form, logging is both turned on and off (`!log_input`, `NOLOG_INPUT:`), there are command- or runas-specific settings, or there are rules other than one running every command as root. Which setting really applies to `/bin/sh` cannot be told from the list, so the run stops |

To clear "cannot be confirmed", keep the node's sudoers simple: give the account only the rule `<account> ALL=(ALL) NOPASSWD: ALL` and
turn I/O logging off for it. On the node, run `LC_ALL=C sudo -n -l` as that account and compare it with the policy lines shown in the 03 log.

<a id="getting-the-approval-key-ready"></a>

**Getting the approval key ready.** Signing new approvals on the 06 screen needs an approval key. The commands below make the
approval key `approval.key` and the public key `approval.pub` at the default location (`~/.config/pqcaton/` on Linux,
`~/Library/Application Support/pqcaton/` on macOS), where they are used without any flag. Both files are created under umask 077, so
they are never writable by the group or others, from the moment they exist (06 refuses a public key file the group can change). The
final `chmod` opens it to others for reading only; 06 also takes it at 0600.

```bash
GOBIN="$HOME/.local/bin" go install github.com/randyinthedev-hash/pqcota-common/cmd/pqcota-keygen@v0.20.0
mkdir -p ~/.config/pqcaton
(umask 077 && pqcota-keygen > ~/.config/pqcaton/approval.key)
(umask 077 && grep '^PQCOTA_VERIFY_KEY=' ~/.config/pqcaton/approval.key > ~/.config/pqcaton/approval.pub)
chmod 644 ~/.config/pqcaton/approval.pub
```

On macOS only the location differs.

```bash
mkdir -p "$HOME/Library/Application Support/pqcaton"
(umask 077 && pqcota-keygen > "$HOME/Library/Application Support/pqcaton/approval.key")
(umask 077 && grep '^PQCOTA_VERIFY_KEY=' "$HOME/Library/Application Support/pqcaton/approval.key" > "$HOME/Library/Application Support/pqcaton/approval.pub")
chmod 644 "$HOME/Library/Application Support/pqcaton/approval.pub"
```

When the screen starts again, its start lines show `approval key: <path> (default location)`. A line starting with `warning:` means: see "Unusable keys" below.

- **The key file.** The output of `pqcota-keygen` run **without flags**. A collector signing key made with `pqcota-keygen -collector` is
  not taken as the approval key (state "collector"). `pqcota-keygen` is in the `pqcota-ctl` bundle too. The screen neither creates this
  file nor changes its permissions, and reads it again at every signature.
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

Once it is up, the line `screen: http://127.0.0.1:8765/  (workspace <workspace folder>)` appears on standard error. If it does
not start, look up the reason after `❌` in the [error table in section 10](#10-error-table).

To check the current organization, the file paths, where the connection comes from and the state of the keys, open "Settings" at
the top of the screen.

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
   fields that do not add up (a shared IP, port 0 and the like). Saving is not blocked, but left as is,
   reconciliation is wrong without any error. A row with its name or IP emptied, an IP that is not an address, and a port that is
   not a number are not saved; the screen points at the field and keeps what you typed. To delete a node or an asset, use
   Remove.
2. **02 Asset scope.** The first time, enter a layer name to create an empty layer. Write the rules, a conclusion for each
   changed rule, sign, and finalize. Finalizing creates `asset-scope.csv`.

   **The policy finalized in 02 is not used for reconciliation right away.** Reconciliation (04, 05) uses the active
   policy `scope-assets.csv`. If it differs from the policy the upstream ingest used, pqcota cannot find the evidence of
   the plans made in 05. To use the finalized policy, first redo the upstream ingest with it, giving the workspace's
   `results/` as the results folder. There are three steps.

   1. **03 checks the signatures.** The results 03 received carry collector signatures, and 03 puts in `results/` only results whose
      signature checked out (step 3).
   2. **Check again with `pqcota-ingest -verify-keys`.** It decides as the ingest does and stores nothing. It prints, for each result, the collector
      ID, whether it is signed, the fingerprint of the key that matched and the verdict, and exits 1 if any result would be refused. Do
      not ingest then.
   3. **Ingest under the judgment organization.** It ingests with the same `PQCOTA_VERIFY_KEY`, so a result whose signature does not
      match is refused before anything is stored.

   `PQCOTA_VERIFY_KEY` is the value of `verify.pub`, which 03 trusts (public keys, not a secret). Give the loading account's connection
   in `PQCOTA_DSN`; its password is read from the `~/.pgpass` of "Preparing pqcota". Run the loading account with
   `PQCOTA_AUTO_DDL=0`. Replace each `<…>` with the real value before running it.

   ```bash
   export PQCOTA_VERIFY_KEY="$(sed -n 's/^PQCOTA_VERIFY_KEY=//p' ~/.config/pqcaton/collector-keys/verify.pub)"
   pqcota-ingest -verify-keys '<workspace>/results'
   PQCOTA_DSN='postgres://pqcota_ingest@<host>:5432/pqcota' PQCOTA_ORG='<organization of the judgments>' PQCOTA_AUTO_DDL=0 pqcota-ingest -scope-assets '<workspace>/asset-scope.csv' '<workspace>/results'
   ```

   The "Re-ingest pqcota" step on 05 shows the same two commands (with paths relative to the workspace).

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
   stopped; it uses the same workspace, so it waits while the screen is saving something else). It only reads the history,
   so give the screen account's connection; its password is read from `~/.pgpass`. Replace each `<…>` with the real value before running it.

   ```bash
   PQCOTA_DSN='postgres://pqcaton_ui@<host>:5432/pqcota' pqcaton-decide activate-scope '<workspace>/session.json' \
     -decl '<workspace>/declaration.json' -results '<workspace>/results' -scope-assets '<workspace>/scope-assets.csv' \
     -candidate '<workspace>/asset-scope.csv' -scope-session '<workspace>/scope-session.json' -org '<organization>'
   ```

   The flag in `pqcaton-decide activate-scope -verify-upstream` is accepted for compatibility and changes nothing.
   `pqcaton-decide activate-scope -confirm-ingest` is no longer supported; given, it stops without writing anything.

   Activating archives the old policy and the old review session, and opens a new round rebuilt under the new policy.
   Earlier conclusions are carried only as previous values, and items the new policy no longer manages stay only in the
   change report. Plans already finalized are not signed or generated in 06, so finalize again in the new round.
   **Changing `scope-assets.csv` by hand without an activation record** locks 05 and blocks saving and finalizing. Then
   restore the previous policy file or follow the steps above.

   **An activation that goes back to an earlier policy is done after re-ingesting the results under that policy.** Even when
   the results of the earlier policy are in the pqcota history, the reconciliation does not pass while the node's latest
   ingest is another policy's, and 05 refuses it with `upstream-revert`. Re-ingesting the results under that policy makes the
   state loaded last the node's latest, and the activation can be done (pqcota v0.13.1 or later). Rounds that v0.27.x or earlier opened on an operator statement (`operator-statement`)
   are still read as past records, and 05 shows "It was not reconciled with the pqcota history" under "The policy change
   that opened this round".
3. **03 Observation.** Write the nodes to observe and how to reach them (IP, port, user, SSH key path, OS), and save. With a
   Windows node the observation does not start ("Getting ready to run observations" in section 2). They are saved to `hosts.csv` in the workspace, in the format `pqcota-hosts` reads. Passwords are not written
   on the screen: if the file has one, it shows only as "set" and is kept as is when you save. "Start the observation" runs in
   this order. While it runs, the log is shown and "Stop" stops it.

   1. **Node check.** Every node is checked without any key ("Node check" in section 2). If one does not pass, it ends here, no node
      is observed, and the node cleanup is "not needed".
   2. **Temporary signing keys.** The signing keys (`collector-keys/`) are checked again, and temporary keys for this run only are made
      **outside the workspace**: under `XDG_RUNTIME_DIR` (when only this user can open it) or else under the system temporary folder, as
      `pqcaton-keys-<run ID>`, the folder 0700 and the key files 0600 from the start. The run log says where. The place is written first
      to `run-keys.json` in the run directory, so it can be found even after a crash. The key values are put in no environment or command
      line of Ansible or ssh; they go only on the standard input of the collectors on the nodes.
   3. **Observation.** The collectors are copied to each node in the table, run, their results taken, and then removed from the node.
      What the collectors write on standard error is moved into the run log.
   4. **Signature check.** Each result received is checked against `verify.pub` (as the ingest decides). A pass leaves "signature
      checked (key <fingerprint>)" in the run log. With no signature (not signed), a key of another ID (said only when that key verifies
      it), or no match with the registered keys (changed after signing, or a key not listed), the reason shows under "Results that did
      not arrive or do not check out".
   5. **Replacing the results.** **The current results change, all at once, only when every node's results arrived, read with the
      upstream decoder, belong to the right node and collector, and their signatures checked out.** Otherwise the current results stay
      and what arrived is kept for diagnosis in `observations/<run>/partial/`.
   6. **Reclaiming.** The temporary keys are removed. After a run that did not finish (failed, stopped), a node cleanup runs once more.
      It first ends the collectors still running from this run's staging directory (a remote collector started through sudo keeps
      running to the end of its window even when the controller ends ssh), then removes the staging folder.

   **Temporary keys that cannot be removed mean "recovery needed".** It is kept apart from the run's result (done, failed and so on); 03
   shows the place and the reason and starts no new observation. Fix the cause (the folder's permissions, say) and start the screen
   again: the recovery at start removes them again and clears "recovery needed". If they cannot be removed, remove the folder
   (`pqcaton-keys-<run ID>`) yourself and start the screen again. **If the screen and the supervisor both end abnormally (a power loss,
   say), the temporary keys can stay in place until the next start.** The folder is open only to its owner, and the next start's recovery
   finds them through the run record and removes them. During an observation the key is in the environment of the collector process on
   the node, where that node's root can read it.
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
     look yourself in a terminal. Replace each `<…>` with the real value before running it.

   ```bash
   ps -p '<PID>' -o pid=,ppid=,pgid=,user=,lstart=,command=
   ```

   Only when a process is confirmed to have been started by this observation, stop it normally and start the screen
   again. If it is unrelated, or its ownership cannot be confirmed, do not stop it: wait until it ends on its own, then
   start the screen again. The restarted screen carries on with the remaining cleanup. Observations started
   by the screen do not share SSH connections (`ControlMaster=no`). If the cleanup could not reach the nodes, the 03 screen names
   the staging folder that may be left, with the run ID: `/tmp/pqcaton-observe-<run ID>`. On that node, also check that no process
   whose command line starts with that place is left. The network observation waits for handshakes on the node's default-route
   interface for the window you set (60 seconds by default).
   **Ingesting into the history.** The screen never ingests on its own when an observation ends. When ingesting from the
   screen is set up (both connections in section 4), "Ingest into the history" under 03 ingests the signed results in the
   results folder **under the active policy**. The screen shows, per node, the expected reference and the history's latest
   reference now; "Ingest into the history: go to the confirmation" opens a confirmation with the organization, the active
   policy's fingerprint, the number of result files with a verified signature and the number of assets the policy excludes.
   Nothing is ingested until "Ingest into the history" is pressed there. When even one result is unsigned or does not
   verify, the whole set is not ingested and the file, collector ID and reason are shown. If a result file or the active
   policy changed after the confirmation, confirm again. When every node's latest reference is already the one for these
   results and this policy, it ends with "Already ingested". When an ingest ended partway, the screen reads the history
   back and shows per node "latest reference is the expected one", "the body exists but is not the latest reference",
   "not recorded" or "cannot be checked". The history is append-only: what is ingested cannot be undone. When ingesting
   from the screen is not set up, ingest from a terminal as before (the commands in item 2).
4. **04 Reconciliation.** When the observation finishes, or pqcota collection writes results to `results/`, reload the
   screen. With no results, the screen points at that folder.

   **To see which app talked to what**, open "App connections from the history" under the topology on 04. The picture on 04 is the
   workspace results reconciled with the declaration; this screen shows the observations loaded into the pqcota history as they are.
   It opens with nothing more than the read privileges of the screen's account.

   1. **Check the organization queried and the comparison.** The top names the organization queried (the judgment organization) and
      each node's history snapshot, and says whether it is the same as the observation results in the workspace: "same", "different"
      or "cannot compare". That is decided by the snapshot digest of the same node. "different" means the results were observed again
      after the load, or loaded under another policy; to see the current results, load them again as 05 describes. "cannot compare"
      means the workspace has no results for that node. "The pqcota history holds no snapshot for this organization" means the
      results of 03 have not been loaded under that organization yet.
   2. **Choose a node and read its apps and peers.** In "Summary by node" choose a node and press "Show": that node's apps, the
      libraries observed with the same app key and the stored connection records (direction, peer, connection, count) remain. A
      record is stored per observing node, so a connection seen by both nodes is two rows; this number is therefore not the
      "negotiated connections" count of 04. A library with the same app key is not evidence that the connection used it.
   3. **The picture and the DOT.** The picture covers the whole organization and is drawn after the screen appears (so is the
      reconciliation picture on 04). Without `dot`
      (Graphviz), or when drawing exceeds the time or size limit, the reason shows in its place. The table and "Download DOT" work
      regardless. The downloaded file can be drawn on another machine with `dot -Tsvg pqcota-topology.dot -o topology.svg`.

   **When a peer shows only as an address.** This screen ties a peer to a node only through the endpoints registered for the judgment
   organization. It does not use the IPs of the declaration or another organization's registrations. With no endpoint registered the
   screen says so and writes every peer as an address. To see node names, register the endpoints for the judgment organization with
   upstream `pqcota-hosts`; the `hosts.csv` that 03 saves is in its format. Run, as the loading account,
   `PQCOTA_ORG='<judgment organization>' PQCOTA_AUTO_DDL=0 pqcota-hosts --dsn 'postgres://pqcota_ingest@<host>:5432/pqcota' '<workspace>/hosts.csv'`.
   When it ends it reports the number registered and the organization, as in
   `[hosts] upserted 3 endpoints into the inventory (organization <judgment organization>; no secrets)`. Reload the screen and those
   peers show as node names. No connection secret goes into the history; only the node ID, name, IP and port are registered. The registered IP is the one address
   SSH reaches, so a node that talked on another of its addresses still shows as an address. An IP shared by several registered nodes
   is reported as such and tied to none of them.
5. **05 Judgment.** Write a conclusion for each policy; for the items going into the plan, pick the action kind, target and
   level; and sign. "Finalize" shows a confirmation page with what will be written, and confirming writes the session,
   plan and ledger together. Finalizing the same session with the same content again writes nothing again.
6. **06 Provisioning.** This screen does the approval signature and the playbook generation. Check the actions of the plan to approve, enter the approver's name and confirm the
   review, and press "Sign the approval". A confirmation page shows the plan, the plan file's hash, the approver and the
   key fingerprint, and signing happens only on a second press there. If the plan or the key changed after the
   confirmation, nothing is signed. The signed plan is kept separately in `plan.signed.json` (FINALIZED) and `plan.json`
   stays as it is. **An existing signed file is never overwritten.** If it does not match the current plan, "Archive the
   earlier signature" renames it with the current time beside it, and then the new plan can be signed. Then pick the
   default delegation level and press "Generate the playbooks": `plan.provision.yml` and `plan.rollback.yml` are written. When the generator flags gaps, their number and content are shown.
   When the playbook was generated at L2 and only places configuration files, "Preview on N nodes" runs it against the
   target nodes with `--check` to show what would change. The preview does not change the nodes, and it runs only on nodes
   that are in the access details (`hosts.csv`). When there is a task that stages a provider module, an L3 action or a
   manual step, the screen names the node, the task and the reason, and runs nothing. When a node that started a task does
   not answer for 120 seconds, the run is stopped and that node reads "Result unknown". A remote task that had already
   started may still finish later.
   **The 06 preview, apply and recheck only run with ansible-core 2.16.3.** The screen checks the version of the
   `ansible-playbook` it would actually run; with another version, or one it cannot read, it shows the detected version and
   the reason and starts nothing. Make the `ansible-playbook` found first on the PATH of the environment that started the
   screen be 2.16.3. Signing the approval, generating the playbooks and reading earlier run records work regardless of that
   version. Observations at 03 are not under this limit.
   **Do not replace the controller's Ansible installation while a 06 remote run is going.** Upgrade after the run has
   ended. The screen checks the version just before it starts a run, so if the installation changes after that check, an
   Ansible other than the one checked may run.
   When the preview ends with changes to make, "Apply to N nodes: go to the confirmation" appears. The confirmation page shows
   the organization, the target nodes, the approved plan, the generated playbook, when the preview ended and until when it is
   good, and the tasks that would change on each node; the apply starts only when "Apply to N nodes" is pressed there. A
   preview is good for 10 minutes after it ended and for one apply. If the plan, the playbook or the access details changed
   after the preview, preview again. The apply goes as far as the L2 placement: it only places configuration files and does
   not activate services or roll back. When a node fails, the nodes after it are not started, and nodes already changed stay
   as they are. The result table reads "L2 placement done", "Only some tasks finished", "Result unknown" or "Not run" per
   node, and "Recheck this run" tells whether the target files match the expected state at that moment. To undo, the operator
   runs `plan.rollback.yml`.
   A playbook the screen does not apply (L1, L3, a provider module) is run by the operator with the commands in the 06
   screen's help. When 05 is finalized again and the plan
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
   after `prepared copy at` below it in `PREPARED` (the lines below are an example). Replace each `<…>` with the real value before running it.

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

### v0.32.0 to v0.33.0

v0.33.0 adds "App connections from the history", opened from 04. The pqcota to use with it (v0.20.0), the ruleset, the state format, the
sessions and the plan format are the same as v0.32.0, so the workspace and the history are used as they are. Stop the screen and replace
the binary with the `go install` of [section 2](#2-installation). The new screen opens with the read privileges the screen's account
already has, so grant nothing more. To see peers as node names, register the endpoints for the judgment organization as "When a peer
shows only as an address" under 04 in [section 6](#6-a-days-procedure) describes. The topology picture on 04 is now drawn after the screen
appears and embedded only after the SVG that `dot` wrote is checked, and drawing has a time limit (10 seconds) and a size limit (4 MiB).
When it is not drawn, the reason and the DOT source show.

### v0.31.1 to v0.32.0

v0.32.0 takes in upstream pqcota v0.20.0 (which includes v0.17.0, v0.18.0 and v0.19.0). The ruleset (`pqcota-enrich/v4`), the contract and
the history's tables and columns are the same as v0.16.0, and the state format, the sessions and the plan format are the same as v0.31.1,
so the workspace opens as it is.

1. Stop the screen.
2. **Upgrade the upstream tools together to v0.20.0** (the `pqcota-ctl` bundle of "Preparing pqcota" in section 2). Keep the tools that
   load into, prune or record in the same history at the same version. The schema is the same as v0.16.0, so `pqcota-inventory -init-schema`
   does not have to be run again.
3. Install `pqcaton-ui` and the preparation tool `pqcaton-collectors` at v0.32.0 with the `go install` of [section 2](#2-installation), and
   fetch the v0.20.0 release assets as in "Checking the collectors you download" to replace the collector folder. 03 refuses the v0.16.0
   collectors as not matching.
4. Keep the collector signing keys and the verification value (`PQCOTA_VERIFY_KEY` in the collector-bound form) as they are. Do not move
   to the per-node key file of upstream v0.17.0 (`PQCOTA_VERIFY_KEY_FILE`): 03 of this version signs with one key per collector ID.
5. Start the screen and observe again on 03. The `pqcota-netcap` of v0.20.0 also records the connections a node accepts, so nodes that
   only accept connections can get new communication edges. 04 shows the two records of one connection as one edge, and shows the
   connections whose direction was not confirmed separately under the reconciliation table. Upstream `pqcota-inventory -diff` lists the
   accepting node's records as added; the assets did not change.

### v0.31.0 to v0.31.1

v0.31.1 is a patch that fixes a defect in saving the 01 declaration (there since v0.6.0). The state format, the session and plan
formats, the ruleset and the pqcota used with it are the same as v0.31.0, so the workspace opens as it is. Stop the screen and replace
the binary with the `go install` of [section 2](#2-installation). 01 no longer saves a node whose name or IP is empty. If the
declaration file already has a node without an IP (a hand-edited file), 01 shows that row, and saving needs its IP filled in or the
row removed with Remove.

### v0.30.0 to v0.31.0

v0.31.0 takes in upstream pqcota v0.16.0 (which includes v0.15.0). The ruleset (`pqcota-enrich/v4`), the contract and the history's tables
and columns are the same as v0.14.0, and the state format and the session and plan formats are the same as v0.30.0, so the workspace opens
as it is **without a ruleset change round**. Three things change: **a 03 observation does not start without the collector signing keys,
does not use the operator's `ansible.cfg`, and does not observe Windows nodes.** The pqcota history gets its schema from a dedicated
command, and the query commands no longer create tables.

1. Stop the screen, check that `-recover-status` prints `no unfinished transactions`, and back up the workspace as in
   [section 7](#7-backup-and-restore).
2. **Upgrade the upstream tools together to v0.16.0** (the `pqcota-ctl` bundle of "Preparing pqcota" in section 2). The tools that load
   into, prune or record in the same history must be at the same version.
3. **Prepare the pqcota history.** Follow "Moving an existing history to three roles" below. Even if you keep the two roles you have,
   run `pqcota-inventory -init-schema` once as the role that owns the tables. From v0.16.0 the query commands create no tables, so they stop
   when the tables this version adds (metadata, source evidence) are missing.
4. Install `pqcaton-ui` and the preparation tool `pqcaton-collectors` at v0.31.0 with the `go install` of [section 2](#2-installation), and
   fetch the v0.16.0 release assets as in "Checking the collectors you download" to replace the collector folder. 03 refuses the v0.14.0
   collectors as "does not match".
5. **Prepare the collector signing keys** ("Preparing the collector signing keys" in section 2). **03 in v0.31.0 does not start an
   observation without them.**
6. **Get the nodes ready.** Take the Windows nodes out of the access file. For nodes reached as an ordinary account, give that account
   `NOPASSWD` sudo and turn I/O logging off ("Node check" in section 2).
7. **Move the values of `ansible.cfg`** (the table below). 03 reads neither the operator's `ansible.cfg` nor `ANSIBLE_*` environment
   variables.
8. Start the screen, check that every item of the 03 checklist is ✓, and observe. In the run log, look for each node passing the check
   and "signature checked" for each result.
9. **Switch the ingest to the form bound to collector IDs.** Upstream's order is central tools → collectors → check → the new value.
   Steps 2 and 4 are the first two; check the results from step 8 with `pqcota-ingest -verify-keys`, then set the ingest's
   `PQCOTA_VERIFY_KEY` to the value of `verify.pub` (the re-ingest commands in [section 6](#6-a-days-procedure)). **Switch only after the
   collectors you run yourself outside 03 sign with the same keys.** Switching first refuses every result of a collector that does not
   sign. Upstream still accepts the old list (keys without IDs), with a warning.

<a id="moving-an-existing-history-to-three-roles"></a>

**Moving an existing history to three roles.** A history made by the v0.30.0 guide has the database and tables owned by the loading
account (`pqcota_ingest`). Only the owner of a table may change it, so `pqcota-inventory -init-schema` run by a new schema role fails with "must be owner of
table pqcota_snapshots … (rolled back, nothing was changed)" and changes nothing. So move the ownership of the tables to the schema role.
Run these as an administrator.

1. See who owns what (in the `pqcota` database).

   ```sql
   SELECT tablename, tableowner FROM pg_tables WHERE tablename LIKE 'pqcota\_%';
   ```

2. Create the schema role and give it the connection and schema privileges (the first three lines in the `postgres` database, the last
   in the `pqcota` database).

   ```sql
   CREATE ROLE pqcota_schema LOGIN;
   \password pqcota_schema
   GRANT CONNECT ON DATABASE pqcota TO pqcota_schema;
   GRANT USAGE, CREATE ON SCHEMA public TO pqcota_schema;
   ```

3. In the `pqcota` database, move the ownership of the pqcota tables and grant the privileges of step 4 of "Preparing pqcota" in
   section 2. The first line runs `ALTER TABLE … OWNER TO pqcota_schema` for each table through psql's `\gexec`; a table's sequences and
   indexes move with it. `REASSIGN OWNED BY pqcota_ingest` moves everything that role owns, so use it only on a database used by pqcota
   alone.

   ```sql
   SELECT format('ALTER TABLE %I OWNER TO pqcota_schema', tablename) FROM pg_tables WHERE schemaname = 'public' AND tablename LIKE 'pqcota\_%' \gexec
   GRANT USAGE ON SCHEMA public TO pqcota_ingest, pqcaton_ui;
   GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO pqcota_ingest;
   GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO pqcota_ingest;
   GRANT SELECT ON ALL TABLES IN SCHEMA public TO pqcaton_ui;
   ALTER DEFAULT PRIVILEGES FOR ROLE pqcota_schema IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO pqcota_ingest;
   ALTER DEFAULT PRIVILEGES FOR ROLE pqcota_schema IN SCHEMA public GRANT USAGE ON SEQUENCES TO pqcota_ingest;
   ALTER DEFAULT PRIVILEGES FOR ROLE pqcota_schema IN SCHEMA public GRANT SELECT ON TABLES TO pqcaton_ui;
   ```

4. Run `pqcota-inventory -init-schema` as `pqcota_schema`. It leaves the existing tables as they are, creates only the tables this version adds, and ends
   with "schema ready: N created, M already present". From then on run the loading account with `PQCOTA_AUTO_DDL=0`: it no longer owns the
   tables, so ingesting without it fails with "permission denied for schema public". Replace each `<…>` with the real value before
   running it.

   ```bash
   PQCOTA_DSN='postgres://pqcota_schema@<host>:5432/pqcota' pqcota-inventory -init-schema
   ```

**Values to move from `ansible.cfg`.** 03 reads only one configuration made by the product for each run. Move the values you kept in
`ansible.cfg` as below, or they are not used in this version. When you apply the playbooks of 06 outside 03, your own `ansible.cfg`
applies as before.

| Value in `ansible.cfg` | In 03 |
|---|---|
| `remote_user` | Write it per node as the account (`ssh_user`) of the access file (`hosts.csv`) |
| `private_key_file` | Write it as the SSH key path (`ssh_key`) of the access file, or keep it as `IdentityFile` in `~/.ssh/config`. The ssh agent is used too |
| `remote_port` | Write it as the port of the access file |
| `ssh_args`, jump hosts (`ProxyJump`, `ProxyCommand`) | Move them to `~/.ssh/config`; ssh reads it as it is. 03 turns connection sharing (`ControlMaster`, `ControlPersist`) off. The port always comes from the access file, and the user follows `~/.ssh/config` only for nodes whose account is left empty in the access file |
| `host_key_checking = False` | Not supported. 03 checks host keys, so accept each node's key into `~/.ssh/known_hosts` |
| Privilege escalation (`become_method`, `become_user`, `become_exe`, `become_flags`, `become_ask_pass`) | Cannot be changed. 03 becomes root with the built-in sudo and never asks for a password. Reach the node as root or give sudo `NOPASSWD` |
| `pipelining`, `keep_remote_files`, `verbosity` | 03 fixes them (pipelining on, no remote files kept, default output) |
| `log_path`, `callbacks_enabled`, `stdout_callback` | Not supported. The run log is kept in `observations/<run>/log.txt` and on the 03 screen |
| Plugin, collection and role paths, `library` | Not used. 03 uses built-in modules only |
| `interpreter_python` | Not supported. The node's Python is found automatically (the node needs Python 3) |
| `forks`, `timeout`, `gathering` | Not supported. Ansible's defaults are used |
| `inventory`, vault settings | Not used. 03 builds the inventory from the access file for each run, and the access secrets are in `hosts.csv` |

### v0.29.1 to v0.30.0

v0.30.0 takes in upstream pqcota v0.14.0. The ruleset stays `pqcota-enrich/v4` and the state format and the session and plan
formats are the same as v0.29.1, so the workspace opens as it is **without a ruleset change round**. The pqcota history does not
need to be loaded again either.

1. Stop the screen, check that `-recover-status` prints `no unfinished transactions`, and back up the workspace as in [section
   7](#7-backup-and-restore).
2. Upgrade the upstream tools (`pqcota-ingest`, `pqcota-cbom-ingest`, `pqcota-prune`, `pqcota-approve` and `pqcota-provision`)
   to v0.14.0 together. **The tools that load into and prune the same pqcota history must be at the same version.** From
   upstream v0.13.1 a load and a prune take a per-node lock together, so an older loader or prune tool left behind can still
   overlap with the new ones.
3. Install `pqcaton-ui` and the preparation tool `pqcaton-collectors` at v0.30.0 with the `go install` of [section
   2](#2-installation).
4. Fetch the v0.14.0 release assets with `pqcaton-collectors fetch` as in "Checking the collectors you download" (section 2), and
   replace the three collectors on the nodes and `collector.jar` with them. 03 refuses v0.13.0 collectors as "does not match".
5. **With the official assets in place, start without `-allow-unverified-collectors`.** Up to v0.29.x even the release assets were
   "cannot be verified" and needed this option, but the v0.14.0 release assets are "verified" on 03. After starting the screen,
   check on 03 that the collectors and `collector.jar` are all "verified", then observe again.
6. **The `collector.jar` check is stricter.** A JAR with no version, one you built yourself, or one from a modified tree is now
   "cannot be verified" and does not start without `-allow-unverified-collectors`. The usual fix is the upstream v0.14.0 release
   `collector.jar`.
7. **An activation that goes back to an earlier policy can be done after re-ingesting the results under that policy.** Before
   that re-ingest it is still refused with `upstream-revert`.

### v0.29.0 to v0.29.1

v0.29.1 is a patch that fixes a defect of the reap after an observation (there since v0.27.0) and one that accepted a new observation on top of one still finishing (there since v0.24.0). The state format, the session and plan
formats, the ruleset and the pqcota used with it are the same as v0.29.0, so the workspace opens as it is. Stop the screen and replace
the binary with the `go install` of [section 2](#2-installation). If 03's "Processes left behind" shows something left from before this
version, check it as the screen says and then end it.

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
   `PQCOTA_ORG='<judgment org>' pqcota-ingest -scope-assets '<workspace>/scope-assets.csv' '<workspace>/results'`.
5. When 05 shows "This session was opened under a different upstream ruleset", press "Start a new round under the current rules"
   (`pqcaton-decide new-round` on the command line). **Earlier conclusions move over only as reference values.** The evidence of Java
   assets changes to new per-deployment findings, so they show as "changed" in the report, and "removed" and "added" can include the
   effect of the identity rule change. Judge each item again or press "Adopt again". No rule links old conclusions to the new
   evidence automatically.
6. **If an asset scope rule has a user service `user@<uid>.service` in its `app_key`, fix it.** The v0.13.0 collectors give a user
   service the app key `user@<uid>.service/<unit>`, so the old value no longer matches. Find such rules as below, fix them on 02,
   finalize, and activate on 05. Replace each `<…>` with the real value before running it.

   ```bash
   grep -n 'user@[0-9]*\.service' '<workspace>/asset-scope.csv' '<workspace>/scope-assets.csv'
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
   `PQCOTA_ORG='<organization of the judgments>' pqcota-ingest -scope-assets '<workspace>/scope-assets.csv' '<workspace>/results'`.
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
| `upstream-revert` | The latest ingest in the pqcota history differs from this policy; results under this policy are in the history but are not the latest of their nodes. Nothing was written | Re-ingest the results under this policy, then activate it |
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
| `run-not-ready` | Something needed before an observation (access, Ansible, collectors, collector signing keys, trusted public keys) is missing, there is a Windows node, or an earlier observation needs recovery | Provide the items marked ✗ in the checklist on the 03 screen |
| `run-busy` | An observation is already running | Start again after it finishes or is stopped |
| `ingest-not-ready` | The conditions for the 03 ingest are not met (ingesting from the screen is not set up · a result is unsigned or does not verify · already ingested · the history could not be read · the read and ingest databases differ) | Resolve the reason shown under "Ingest into the history" on the 03 screen |
| `ingest-busy` | An ingest is already running | Try again after it ends |
| `ingest-replayed` | This ingest confirmation was already used. No new ingest was started | The link on the refusal page goes to the record of the ingest that confirmation started. To ingest again, confirm anew on the 03 screen |
| `ingest-results-changed` | The result files changed after the confirmation, so no ingest was started | Check again on the 03 screen and confirm what will be ingested |
| `ingest-policy-changed` | The active policy changed after the confirmation, so no ingest was started | Check again on the 03 screen and confirm what will be ingested |
| `ingest-keys-changed` | The verification keys (trusted collector public keys) changed after the confirmation, so no ingest was started | Check again on the 03 screen and confirm what will be ingested |
| `ingest-inputs-changed` | More than one input to the ingest changed after the confirmation, so no ingest was started | Check again on the 03 screen and confirm what will be ingested |
| `prov-not-ready` | The conditions for running the 06 preview, apply or recheck from the screen are not met (approval, an L2 playbook, access details, Ansible and its version (ansible-core 2.16.3), or a kind of task the screen does not run) | Provide what "Why the screen does not run this" on the 06 screen lists |
| `prov-busy` | A 06 run is already going | Start again after it finishes or is stopped |
| `prov-grant` | There is no valid preview to apply from (none · ended more than 10 minutes ago · an input changed after it · already used once) | Preview again on the 06 screen, then apply |
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
