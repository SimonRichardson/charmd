"""Bounded end-to-end helpers shared by the independent Spread scenarios."""
import hashlib
import http.client
import json
import os
from pathlib import Path
import socket
import subprocess
import time
import zipfile


class UnixConnection(http.client.HTTPConnection):
    """Speak the daemon's real HTTP API over its Unix socket."""

    def connect(self):
        """Connect with a timeout so a broken daemon cannot hang a task."""
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(5)
        self.sock.connect(self.host)


class Daemon:
    """Own an isolated daemon, local APIs, charm archive, and hook event history."""

    def __init__(self, leader=False, dispatch=True, fail_install=False):
        """Create the complete fixture before any watchers are started."""
        self.root = Path("/var/tmp/charmd-spread")
        self.root.mkdir()
        self.process = None
        self.log = None
        self.binary = os.environ["CHARMD_BINARY"]
        source = self.root / "source"
        source.mkdir()
        archive = self.root / "fixture.zip"
        script = Path(__file__).with_name("dispatch").read_bytes()
        with zipfile.ZipFile(archive, "w") as charm:
            charm.writestr("metadata.yaml", "name: spread-test\n")
            charm.writestr("config.yaml", "options:\n  status:\n    type: string\n    default: world\n")
            scripts = ["dispatch"] if dispatch else ["hooks/" + hook for hook in
                       ("install", "leader-elected", "config-changed", "start")]
            for name in scripts:
                info = zipfile.ZipInfo(name)
                info.external_attr = 0o100755 << 16
                charm.writestr(info, script)
        self.checksum = hashlib.sha256(archive.read_bytes()).hexdigest()
        archive.rename(source / self.checksum)
        (source / (self.checksum + ".manifest")).write_text("name: spread-test\n")
        (self.root / "config.yaml").write_text(
            "services:\n"
            f"  charm-api:\n    path: {source}\n"
            f"  config-api:\n    path: {self.root}/charm-config.yaml\n"
            f"  leadership-api:\n    path: {self.root}/charm-leadership.yaml\n")
        self.config("world")
        self.leadership(leader)
        if fail_install:
            (self.root / "fail-install").touch()

    def __enter__(self):
        """Start the daemon and wait for API readiness rather than sleeping blindly."""
        self.log = (self.root / "daemon.log").open("w")
        self.process = subprocess.Popen([self.binary], env={**os.environ,
            "CHARMD": str(self.root), "CHARMD_TEST_ROOT": str(self.root)},
            stdout=self.log, stderr=subprocess.STDOUT)
        try:
            self.wait(lambda: self.changes() is not None, "daemon readiness")
        except BaseException:
            self.__exit__(None, None, None)
            raise
        return self

    def __exit__(self, *unused):
        """Stop and reap the daemon on success, assertion failure, or timeout."""
        if self.process is not None:
            self.process.terminate()
            try:
                self.process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                self.process.kill()
                self.process.wait()
        if self.log is not None:
            self.log.close()

    def changes(self):
        """Fetch structured changes without parsing human-readable CLI tables."""
        connection = UnixConnection(str(self.root / "charmd.sock"))
        try:
            connection.request("GET", "/v1/changes?select=all", headers={"Host": "localhost"})
            response = connection.getresponse()
            body = response.read()
            assert response.status == 200, (response.status, body)
            return json.loads(body)["result"] or []
        finally:
            connection.close()

    def wait(self, predicate, description):
        """Poll an observable condition with a deadline and useful failure output."""
        deadline = time.monotonic() + 60
        while time.monotonic() < deadline:
            if self.process is not None and self.process.poll() is not None:
                raise AssertionError("daemon exited: " + (self.root / "daemon.log").read_text())
            try:
                if predicate():
                    return
            except (FileNotFoundError, ConnectionRefusedError):
                pass
            time.sleep(0.1)
        raise AssertionError(f"timed out: {description}\nhistory: {self.history()}\n"
                             + (self.root / "daemon.log").read_text())

    def install(self, checksum=None):
        """Submit an install through the shipped CLI and return its change ID."""
        output = subprocess.check_output([self.binary, "install", checksum or self.checksum],
            env={**os.environ, "CHARMD": str(self.root)}, text=True)
        return output.split()[1]

    def settled(self, count):
        """Wait for the expected changes and require every one to finish Done."""
        def done():
            """Reject terminal failures immediately and wait for all other work."""
            changes = self.changes()
            assert not any(c["status"] in ("Error", "Hold", "Undone", "Abort") for c in changes), changes
            return len(changes) >= count and all(c["status"] == "Done" for c in changes)

        self.wait(done, f"{count} completed changes")
        assert not (self.root / "overlap").exists(), "concurrent hooks"

    def history(self):
        """Return hook records written while holding the shared exclusion lock."""
        path = self.root / "history"
        return [json.loads(line) for line in path.read_text().splitlines()] if path.exists() else []

    def expect(self, hooks):
        """Compare complete begin/end pairs, catching extra hooks and bad ordering."""
        expected = []
        for hook, leader, status in hooks:
            for phase in ("begin", "end"):
                expected.append([phase, hook, str(leader).lower(), status])
        assert self.history() == expected, self.history()
        assert not (self.root / "overlap").exists(), "concurrent hooks"

    def write(self, filename, content):
        """Atomically replace API files, exercising directory inotify watches."""
        temporary = self.root / (filename + ".new")
        temporary.write_text(content)
        temporary.replace(self.root / filename)

    def config(self, value):
        """Publish one configuration value to the local config API."""
        self.write("charm-config.yaml", f'charms:\n  "{self.checksum}":\n    status: {json.dumps(value)}\n')

    def leadership(self, leader):
        """Publish the local API's current leadership snapshot."""
        self.write("charm-leadership.yaml", f'charms:\n  "{self.checksum}": {str(leader).lower()}\n')
