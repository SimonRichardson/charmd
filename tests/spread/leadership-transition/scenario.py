"""A gated real hook retains its snapshot while election work takes priority."""
from harness import Daemon

with Daemon() as daemon:
    daemon.install()
    daemon.settled(2)
    gate = daemon.root / "block-config-changed"
    gate.touch()
    daemon.config("first")
    daemon.wait(lambda: (daemon.root / "waiting").exists(), "blocked config hook")

    # Queue another config while the old hook is still running, then gain leadership.
    daemon.config("second")
    daemon.wait(lambda: len(daemon.changes()) >= 4, "queued config change")
    daemon.leadership(True)
    daemon.wait(lambda: any("Accept leadership" in c["summary"] for c in daemon.changes()),
                "queued election")
    assert daemon.history()[-1] == ["begin", "config-changed", "false", "first"]
    assert not (daemon.root / "overlap").exists()

    gate.unlink()
    (daemon.root / "release").touch()
    daemon.settled(5)
    expected = [("install", False, "world"), ("config-changed", False, "world"),
                ("config-changed", False, "first"), ("leader-elected", True, "first"),
                ("config-changed", True, "second")]
    daemon.expect(expected)

    # Loss emits no election hook; the next hook must observe false.
    daemon.leadership(False)
    daemon.config("lost")
    daemon.settled(6)
    expected.append(("config-changed", False, "lost"))
    daemon.expect(expected)

    # Regaining leadership schedules a fresh election before the next config hook.
    daemon.leadership(True)
    daemon.wait(lambda: len(daemon.changes()) >= 7, "second election")
    daemon.settled(7)
    daemon.config("regained")
    daemon.settled(8)
    expected.extend([("leader-elected", True, "lost"),
                     ("config-changed", True, "regained")])
    daemon.expect(expected)
