# SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
# SPDX-License-Identifier: BUSL-1.1
#
# pqcaton: the stdout callback for a provisioning run started from screen 04.
#
# It records identifiers and outcomes only: which host, which task, started, and how it ended. It never reads variables,
# file contents, diffs or module messages out of a result. One JSON line per event, each written to disk before the
# run goes on. Every line carries the run id and a sequence number, so a reader can tell a complete record of this run
# from a cut one or from another run's.
#
# It is the stdout callback on purpose. Ansible skips an extra callback it cannot load and carries on; it stops before
# the first task when the stdout callback cannot be set up. So: no record, no run.
#
# The supervisor creates the record file and writes line 0. This plugin only appends to that file (it never creates
# one), writes "ready", and then waits for the supervisor's go-ahead. Without it nothing remote starts.
#
# The go-ahead is not "something exists at that path". It is a regular file whose content names this run and carries
# the token the supervisor handed this process. A directory, an empty file or another run's file is not a go-ahead.
from __future__ import annotations

import json
import os
import stat
import time

from ansible import context
from ansible.plugins.callback import CallbackBase

DOCUMENTATION = """
    name: pqcaton_events
    type: stdout
    short_description: per-task outcomes for a pqcaton provisioning run (identifiers and outcomes only)
    description:
      - Appends one JSON line per event to the file named by PQCATON_EVENTS, which must already exist.
      - Waits for the file named by PQCATON_GO before the run may start.
      - Never records variables, file contents, diffs or module messages.
"""

GO_WAIT_SECONDS = 30


class CallbackModule(CallbackBase):
    CALLBACK_VERSION = 2.0
    CALLBACK_TYPE = "stdout"
    CALLBACK_NAME = "pqcaton_events"

    def __init__(self):
        super().__init__()
        self._run = os.environ["PQCATON_RUN_ID"]
        go = os.environ["PQCATON_GO"]
        token = os.environ["PQCATON_GO_TOKEN"]
        # No O_CREAT: only the file the supervisor made for this run.
        self._fd = os.open(os.environ["PQCATON_EVENTS"], os.O_WRONLY | os.O_APPEND)
        self._seq = 0
        self._play = ""
        self._live = False
        self._emit("ready")
        deadline = time.monotonic() + GO_WAIT_SECONDS
        while not self._allowed(go, token):
            if time.monotonic() > deadline:
                raise RuntimeError("the supervisor gave no go-ahead")
            time.sleep(0.05)
        self._live = True

    def _allowed(self, path, token):
        try:
            if not stat.S_ISREG(os.lstat(path).st_mode):
                return False
            with open(path, encoding="utf-8") as f:
                said = json.load(f)
            return isinstance(said, dict) and said.get("run") == self._run and token != "" and said.get("token") == token
        except (OSError, ValueError):
            return False

    def _emit(self, ev, **fields):
        self._seq += 1
        line = json.dumps({"seq": self._seq, "run": self._run, "ev": ev, **fields}, ensure_ascii=False, separators=(",", ":")) + "\n"
        try:
            os.write(self._fd, line.encode("utf-8"))
            os.fsync(self._fd)
        except OSError:
            if not self._live:
                raise
            # The record cannot be written any more. Ansible would only warn and go on, so end the run here. The
            # supervisor reads this exit code as "the record was cut" (provrun.ExitRecord).
            os._exit(251)

    def v2_playbook_on_start(self, playbook):
        self._emit("begin", check=bool(context.CLIARGS.get("check")))

    def v2_playbook_on_play_start(self, play):
        self._play = str(play._uuid)
        hosts = play.hosts if isinstance(play.hosts, (list, tuple)) else [play.hosts]
        self._emit("play", play=self._play, hosts=[str(h) for h in hosts])

    def v2_playbook_on_task_start(self, task, is_conditional):
        self._emit("task", play=self._play, task=str(task._uuid), name=task.get_name())

    def v2_playbook_on_handler_task_start(self, task):
        self.v2_playbook_on_task_start(task, False)

    def v2_runner_on_start(self, host, task):
        self._emit("start", host=host.get_name(), task=str(task._uuid))

    def _result(self, result, status, **extra):
        self._emit("result", host=result._host.get_name(), task=str(result._task._uuid), status=status, **extra)

    def v2_runner_on_ok(self, result):
        self._result(result, "changed" if result._result.get("changed") else "ok")

    def v2_runner_on_failed(self, result, ignore_errors=False):
        self._result(result, "failed")

    def v2_runner_on_unreachable(self, result):
        self._result(result, "unreachable")

    def v2_runner_on_skipped(self, result):
        r = result._result
        if "false_condition" in r or str(r.get("skip_reason", "")).startswith("Conditional"):
            why = "conditional"
        elif context.CLIARGS.get("check"):
            why = "check_mode"
        else:
            why = "other"
        self._result(result, "skipped", skip=why)

    def v2_playbook_on_stats(self, stats):
        self._emit("end")
