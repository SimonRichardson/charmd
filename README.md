# charmd

`charmd` is a small Pebble-inspired changes-and-tasks service. State is stored
as an atomic JSON checkpoint in `state.json` (or in `$CHARMD/state.json`).

Set `$CHARMD` to the runtime directory. Charmd reads its configuration from
`$CHARMD/config.yaml`, stores its checkpoint in `$CHARMD/state.json`, and
listens on `$CHARMD/charmd.sock`. When unset, `/var/lib/charmd/default` is
used.
When `services.charm-api` is empty or absent, charmd uses Charmhub. The
checked-in `example/config.yaml` selects this default:

```yaml
services:
  charm-api:
```

Choose a backend during daemon setup. Charmhub accepts an optional alternate
server URL and base override. The base uses the conventional
`NAME@CHANNEL/ARCHITECTURE` form, such as `ubuntu@22.04/amd64`; when unset,
Charmd detects it from the host. Juju requires the base URL of its
S3-compatible blob endpoint and downloads archives at `sha256/<digest>`:

```yaml
services:
  charm-api:
    type: juju
    url: https://juju.example.test/charm-store
```

Configured URLs must be HTTP or HTTPS and may not include credentials.

The checked-in example selects Charmhub and configures its compatible base:

```yaml
services:
  charm-api:
    type: charmhub
    base: ubuntu@22.04/amd64
```

Charm installation is asynchronous. `POST /v1/charms` accepts:

```json
{
  "action": "install",
  "name": "postgresql-k8s",
  "sha256": "optional 64-character hexadecimal checksum",
  "revision": 42,
  "channel": "14/stable"
}
```

`name` is required. The Charmhub backend uses `name`, and can additionally use
`revision` and `channel`; when `sha256` is supplied, the resolved archive must
match it. The Juju backend requires `sha256` and reads the archive from its
`sha256/<digest>` blob path; `revision` and `channel` are not used by that
backend. All selectors are persisted with the install change.

Fetched archives are stored atomically beneath `$CHARMD/charms` as
`<sha256>.charm`. The fetch task persists both the resolved download metadata
and local archive path for the subsequent install task.

The read-only commands deliberately follow Pebble's command names and table
layout:

```text
charmd changes
charmd tasks <change-id>
charmd install [--revision REVISION] [--channel CHANNEL] <charm-name>
charmd notices [--type TYPE] [--key KEY] [--timeout DURATION]
charmd notice <id>
charmd notice <type> <key>
charmd okay
```

With no command, `charmd` runs the daemon and serves the changes and notices API on
`charmd.sock` in the state directory. On Linux, notice access uses Unix peer
credentials; callers without authenticated credentials are denied notice access.
Interrupted handlers may run again after a restart; handlers must therefore be
idempotent or reconcile external side effects.

## License

Charmd contains modified source code from Canonical Pebble and is licensed
under the GNU General Public License, version 3.
