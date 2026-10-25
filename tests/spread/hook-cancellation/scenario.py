"""Graceful shutdown recognizes cancellation and safely queues another attempt."""
from harness import Daemon

with Daemon() as daemon:
    gate = daemon.root / "block-install"
    gate.touch()
    daemon.install()
    daemon.wait(lambda: (daemon.root / "waiting").exists(), "running install")
    daemon.stop()
    hooks = [task for task in daemon.checkpoint()["tasks"].values()
             if task["kind"] == "run-hook"]
    assert len(hooks) == 1, hooks
    assert hooks[0]["data"]["hook-state"]["step"] == "queued", hooks

    gate.unlink()
    daemon.start()
    daemon.settled(2)
    assert daemon.history()[0] == ["begin", "install", "false", "world"]

    # The canceled attempt has no end; its replacement must complete once. The
    # local config watcher announces initial values again on each connection, so
    # restart may also deliver another independent config notification.
    actual = daemon.history()
    assert actual[1:3] == [["begin", "install", "false", "world"],
                           ["end", "install", "false", "world"]], actual
    configs = actual[3:]
    assert len(configs) >= 2 and len(configs) % 2 == 0, actual

    for begin, end in zip(configs[::2], configs[1::2]):
        assert begin == ["begin", "config-changed", "false", "world"], actual
        assert end == ["end", "config-changed", "false", "world"], actual
