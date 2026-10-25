"""Restore deterministic boundary checkpoints without production timing failpoints."""
import json
import sys
from harness import Daemon

step = sys.argv[1]
with Daemon(leader=True) as daemon:
    daemon.install()
    daemon.settled(3)
    daemon.stop()
    saved = daemon.checkpoint()
    # Use a real successful invocation and roll back only its durable operation
    # boundary. Unit tests additionally restore every actual backend checkpoint.
    name = "config-changed" if step == "queued" else "leader-elected"
    task = next(task for task in saved["tasks"].values()
                if task["kind"] == "run-hook" and task["data"]["hook-name"] == name)
    progress = task["data"]["hook-state"]
    assert progress["step"] == "committed", progress

    progress["step"] = step
    task["status"] = 3  # Doing: handler was interrupted before runner completion.
    task.pop("ready-time", None)
    if step == "done":
        task["data"].pop("charm-install-path")
        task["data"].pop("hook-config")
    change = saved["changes"][task["change"]]
    change.pop("ready-time", None)
    if name == "leader-elected":
        saved["data"]["leadership-accepted-" + daemon.checksum] = False
    daemon.write("state.json", json.dumps(saved))
    history = daemon.history()
    daemon.start()
    daemon.settled(3)
    daemon.config("after-recovery")
    daemon.wait(lambda: ["end", "config-changed", "true", "after-recovery"]
                in daemon.history(), "config after recovery")
    daemon.settled(4)
    records = daemon.history()
    assert records[:len(history)] == history
    assert sum(row[:2] == ["begin", "install"] for row in records) == 1, records
    assert sum(row[:2] == ["begin", "leader-elected"] for row in records) == 1, records
    assert sum(row[:2] == ["begin", "start"] for row in records) == 1, records

    if step == "queued":
        assert len(records) >= len(history) + 4, records
    restored = daemon.checkpoint()["tasks"][task["id"]]
    assert restored["data"]["hook-state"]["step"] == "committed", restored
    assert restored["data"]["charm-install-path"] == progress["invocation"]["dir"]
