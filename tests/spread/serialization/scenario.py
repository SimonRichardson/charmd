"""Hold one charm's install while another charm finishes fetching and unpacking."""
import hashlib
import zipfile
from harness import Daemon

fixture = Daemon()
source = fixture.root / "source"
second_archive = fixture.root / "second.zip"
second_archive.write_bytes((source / fixture.checksum).read_bytes())
with zipfile.ZipFile(second_archive, "a") as archive:
    archive.writestr("identity", "second charm")
checksum = hashlib.sha256(second_archive.read_bytes()).hexdigest()
second_archive.rename(source / checksum)
(source / (checksum + ".manifest")).write_text("name: spread-test\n")
fixture.write("charm-config.yaml", "charms:\n" + "".join(
    f'  "{key}": {{status: world}}\n' for key in (fixture.checksum, checksum)))
fixture.write("charm-leadership.yaml", "charms:\n" + "".join(
    f'  "{key}": false\n' for key in (fixture.checksum, checksum)))
(fixture.root / "block-install").touch()

with fixture as daemon:
    daemon.install()
    daemon.wait(lambda: (daemon.root / "waiting").exists(), "first charm install gate")
    daemon.install(checksum)
    # Unpacking can proceed concurrently; executing its install hook must wait.
    daemon.wait(lambda: (daemon.root / "charms" / checksum / "dispatch").exists(),
                "second charm unpacked")
    assert daemon.history() == [["begin", "install", "false", "world"]]
    assert not (daemon.root / "overlap").exists()

    (daemon.root / "block-install").unlink()
    (daemon.root / "release").touch()
    daemon.settled(4)
    records = daemon.history()
    assert len(records) == 12, records

    # Charm scheduling order is unspecified; every begin must have its own end
    # before another hook begins.
    seen = {}
    for begin, end in zip(records[::2], records[1::2]):
        assert begin[0] == "begin" and end == ["end", *begin[1:]], records

        seen[begin[1]] = seen.get(begin[1], 0) + 1
    assert seen == {"install": 2, "config-changed": 2, "start": 2}, records
