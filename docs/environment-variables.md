English · [한국어](environment-variables.ko.md)

# Environment variables

[Overview](../README.md) · [Operator guide](operator-guide.md) · [Build](build.md)

The environment variables pqcaton reads. Secrets are read only from the environment (or from the file named below) and are not written to
the workspace, processing records, logs, error messages or the screen.

## Running

| Variable | Read by | Meaning |
|---|---|---|
| `PQCOTA_DSN` | `pqcaton-ui`, `pqcaton-decide baseline` · `activate-scope` | The connection of the pqcota history. **The screen does not start without it.** When it is empty, `pqcaton-ui` reads `pqcota.env` next to the default approval key (owner-only, one line `PQCOTA_DSN=<value>`; [operator guide section 2](operator-guide.md#2-installation)). Not passed to the observation processes the screen starts |
| `PQCOTA_ORG` | `pqcota-ingest` (upstream) | The organization pqcota ingests under. It must be the organization of the judgments, or the reconciliation in 03 does not find the ingest |
| `PQCOTA_VERIFY_KEY` | `pqcota-ingest` (upstream) | The public keys that check collector results: `<collector-id>:<public key>` entries, comma-separated, the same value as `verify.pub`, which 01 trusts. Check with `-verify-keys` first, then ingest with the same value ([operator guide section 6](operator-guide.md#6-a-days-procedure)). pqcaton does not read this variable; it reads the `verify.pub` file |
| `PQCOTA_AUTO_DDL` | upstream writing commands | Set it to `0` for the loading account. An account that does not own the tables fails without it, trying to run schema statements. The schema is created with `pqcota-inventory -init-schema` ([operator guide section 2](operator-guide.md#preparing-pqcota)) |
| `PQCATON_SCOPE_ASSETS` | `pqcaton-report` | The asset-scope policy file, the same one given to `pqcota-ingest -scope-assets` |
| `PQCATON_REQUIRE_RLS` | judgment ledger on Postgres | With `1`, the ledger does not open when row-level security is not in force (a superuser or `BYPASSRLS` role) ([design §2.2](developers.md#22-row-level-security-one-layer-the-db-enforces)) |
| `PQCATON_RUN` | set by `pqcaton-ui` | The marker the screen puts on the observation processes it starts, so it ends only processes proven to be its own. Not set by hand |
| `XDG_RUNTIME_DIR` | `pqcaton-ui` (01 observation) | When it is a folder only this user can open, the temporary signing keys of each observation (`pqcaton-keys-<run ID>`) go under it; otherwise under the system temporary folder (`TMPDIR`) |

**A 01 observation does not pass `ANSIBLE_*` environment variables on.** Ansible gets the configuration file made for the run (`ANSIBLE_CONFIG`)
and the allowed variables only (`PATH` · `HOME` · `USER` · `LOGNAME` · `LANG` · `LANGUAGE` · `LC_*` · `TZ` · `TMPDIR` · `SSH_AUTH_SOCK`, and
`OBJC_DISABLE_INITIALIZE_FORK_SAFETY` on macOS) ([operator guide section 2](operator-guide.md#the-ansible-configuration-02-uses)). The
signing keys are not passed as environment variables.

The collector signing keys of 01 are files too, not environment variables (`collector-keys/<ID>.key` · `verify.pub`, or `-collector-keys` ·
`-collector-verify-keys`). Their content is the output of `pqcota-keygen -collector <ID>`. The collectors read `PQCOTA_SIGN_KEY`, but 01
passes that value on the standard input of the collector on the node and keeps it out of the environment on the screen's side.

The approval keys of 04 are files, not environment variables (`approval.key` · `approval.pub`, or `-approval-key` ·
`-approval-verify-key`). Their content is the output of `pqcota-keygen` (`PQCOTA_SIGN_KEY` · `PQCOTA_VERIFY_KEY` lines). The upstream
commands `pqcota-approve` and `pqcota-provision` read `PQCOTA_APPROVAL_KEY` and `PQCOTA_APPROVAL_KEYS` themselves.

## Demo and checks

| Variable | Read by | Meaning |
|---|---|---|
| `PQCOTA_DIR` | `make e2e-demo`, demo scripts | The upstream `pqcota` checkout (the five repositories side by side at the version `go.mod` pins) |
| `PQCATON_BIN_DIR` | demo scripts | Prebuilt Linux binaries to use instead of building with Go |
| `PQCATON_UI_PORT` | `demo/scripts/ui.sh` | The port on this machine for the relay (default 8765) |
| `PQCATON_E2E_TRACE` | `demo/scripts/extend.sh` | With `1`, runs the dedicated fixture that measures the trace chain |
| `E2E_KEEP_WS` | `make e2e-demo` | A folder to copy the workspace to before cleaning up |
| `E2E_UBUNTU_MIRROR` | `make e2e-demo` | An Ubuntu mirror for the builds inside dind |
| `PQCOTA_TEST_DSN` | tests | A Postgres for the tests that need one; without it those tests are skipped |
