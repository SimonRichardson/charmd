"""An unsuccessful install must never release dependent lifecycle hooks."""
import time
from harness import Daemon

with Daemon(leader=True, fail_install=True) as daemon:
    change = daemon.install()
    daemon.wait(lambda: any(c["id"] == change and c["status"] == "Error"
                           for c in daemon.changes()), "failed install")
    # Notifications after the failure must also remain gated on successful install.
    daemon.config("after-failure")
    deadline = time.monotonic() + 3
    while time.monotonic() < deadline:
        assert daemon.history() == [["begin", "install", "true", "world"]]
        assert len(daemon.changes()) == 1
        time.sleep(0.1)
