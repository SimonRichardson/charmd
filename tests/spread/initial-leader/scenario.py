"""Initial leadership cannot overtake install or be overtaken by config."""
from harness import Daemon

with Daemon(leader=True) as daemon:
    daemon.install()
    daemon.settled(3)
    daemon.expect([("install", True, "world"), ("leader-elected", True, "world"),
                   ("config-changed", True, "world")])
