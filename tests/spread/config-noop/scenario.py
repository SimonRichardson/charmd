"""The local config API compares semantics and retains its last valid baseline."""
import time
from harness import Daemon

with Daemon() as daemon:
    daemon.install()
    daemon.settled(2)
    expected = [("install", False, "world"), ("config-changed", False, "world")]
    daemon.write("charm-config.yaml", f"# same value, different YAML\ncharms: {{'{daemon.checksum}': {{status: world}}}}\n")
    # A bounded quiet window detects unexpected changes as well as hook execution.
    deadline = time.monotonic() + 3
    while time.monotonic() < deadline:
        assert len(daemon.changes()) == 2
        daemon.expect(expected)
        time.sleep(0.1)

    daemon.write("charm-config.yaml", "charms: [invalid\n")
    time.sleep(1)
    daemon.config("world")
    time.sleep(1)
    assert len(daemon.changes()) == 2
    daemon.expect(expected)

    # A later valid update proves that the watcher is still alive.
    daemon.config("changed")
    daemon.settled(3)
    expected.append(("config-changed", False, "changed"))
    daemon.expect(expected)
