"""Loss of the process tree leaves Pending, which must enter hook-error recovery."""
import sys
import time
from harness import Daemon

with Daemon() as daemon:
    hook = "install"
    if sys.argv[1] == "config":
        daemon.install()
        daemon.settled(2)
        hook = "config-changed"
    (daemon.root / ("block-" + hook)).touch()
    if hook == "install":
        change = daemon.install()
    else:
        daemon.config("interrupted")
        daemon.wait(lambda: len(daemon.changes()) == 3, "config change created")
        change = next(c["id"] for c in daemon.changes() if c["status"] != "Done")
    daemon.wait(lambda: (daemon.root / "waiting").exists(), "running hook")
    history = daemon.history()
    daemon.crash()
    saved = [task for task in daemon.checkpoint()["tasks"].values()
             if task["kind"] == "run-hook" and task["data"]["hook-state"]["step"] == "pending"]
    assert len(saved) == 1, saved

    (daemon.root / ("block-" + hook)).unlink()
    daemon.start()
    daemon.wait(lambda: any(c["id"] == change and c["status"] == "Error"
                           for c in daemon.changes()), "interrupted hook error")
    daemon.config("after-interruption")
    # A fresh config notification must not bypass the interrupted operation.
    deadline = time.monotonic() + 3
    while time.monotonic() < deadline:
        assert daemon.history() == history, daemon.history()
        assert not (daemon.root / "overlap").exists()
        time.sleep(0.1)
