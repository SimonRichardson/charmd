"""Both legacy entry points must install before receiving configuration."""
import sys
from harness import Daemon

with Daemon(dispatch=sys.argv[1] == "dispatch") as daemon:
    daemon.install()
    daemon.settled(2)
    daemon.expect([("install", False, "world"), ("config-changed", False, "world"), ("start", False, "world")])
    assert (daemon.root / "charms" / daemon.checksum).is_dir()
