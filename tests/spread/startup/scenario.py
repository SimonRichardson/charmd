"""Startup obeys config ordering and the durable hook execution protocol."""
import json
import sys
import time
from harness import Daemon

variant = sys.argv[1]
with Daemon(leader=True) as daemon:
    if variant in ("config-failure", "start-failure"):
        failed = "config-changed" if variant == "config-failure" else "start"
        (daemon.root / ("fail-" + failed)).touch()
    elif variant in ("canceled", "interrupted"):
        (daemon.root / "block-start").touch()
    daemon.install()
    if variant in ("config-failure", "start-failure"):
        daemon.wait(lambda: any(c["status"] == "Error" for c in daemon.changes()),
                    "startup failure")
        history = daemon.history()
        assert not any(row[:2] == ["end", "start"] for row in history), history

        if variant == "config-failure":
            assert not any(row[1] == "start" for row in history), history

        daemon.config("after-error")
        deadline = time.monotonic() + 3
        while time.monotonic() < deadline:
            assert daemon.history() == history, daemon.history()
            time.sleep(0.1)
    else:
        if variant in ("canceled", "interrupted"):
            daemon.wait(lambda: (daemon.root / "waiting").exists(), "active start hook")
            history = daemon.history()
            change = next(c["id"] for c in daemon.changes() if c["status"] == "Doing")
            if variant == "interrupted":
                daemon.crash()
            else:
                daemon.stop()
            (daemon.root / "block-start").unlink()
            saved = daemon.checkpoint()
            start = next(task for task in saved["tasks"].values()
                         if task["kind"] == "run-hook" and task["data"]["hook-name"] == "start")
            expected = "pending" if variant == "interrupted" else "queued"
            assert start["data"]["hook-state"]["step"] == expected, start

            lifecycle = saved["data"]["charm-lifecycle-" + start["data"]["hook-install-task"]]
            assert lifecycle["started"] is False, lifecycle

            daemon.start()
            if variant == "interrupted":
                daemon.wait(lambda: any(c["id"] == change and c["status"] == "Error"
                                       for c in daemon.changes()), "interrupted startup error")
                daemon.config("after-error")
                deadline = time.monotonic() + 3
                while time.monotonic() < deadline:
                    assert daemon.history() == history, daemon.history()
                    time.sleep(0.1)
        if variant != "interrupted":
            daemon.settled(3)
            if variant != "canceled":
                daemon.expect([("install", True, "world"), ("leader-elected", True, "world"),
                               ("config-changed", True, "world"), ("start", True, "world")])
            daemon.stop()
            saved = daemon.checkpoint()
            start = next(task for task in saved["tasks"].values()
                         if task["kind"] == "run-hook" and task["data"]["hook-name"] == "start")
            key = "charm-lifecycle-" + start["data"]["hook-install-task"]
            assert saved["data"][key]["started"] is True, saved["data"][key]

            if variant == "executed":
                # Model the checkpoint after start exits but before lifecycle commit.
                # Backend unit tests also restore this exact boundary from real writes.
                start["status"] = 3
                start.pop("ready-time", None)
                start["data"]["hook-state"]["step"] = "done"
                saved["data"][key]["started"] = False
                saved["changes"][start["change"]].pop("ready-time", None)
                daemon.write("state.json", json.dumps(saved))
            before = daemon.history()
            daemon.start()
            daemon.config("after-restart")
            daemon.wait(lambda: ["end", "config-changed", "true", "after-restart"]
                        in daemon.history(), "post-start configuration")
            daemon.settled(4)
            records = daemon.history()
            assert records[:len(before)] == before, records
            assert sum(row[:2] == ["end", "start"] for row in records) == 1, records
            assert sum(row[:2] == ["begin", "start"] for row in records) == (2 if variant == "canceled" else 1), records
            assert daemon.checkpoint()["data"][key]["started"] is True
