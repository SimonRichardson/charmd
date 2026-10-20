# charmd

`charmd` is a small Pebble-inspired changes-and-tasks service. State is stored
as an atomic JSON checkpoint in `state.json` (or in `$CHARMD/state.json`).

Set `$CHARMD` to the runtime directory. Charmd reads its configuration from
`$CHARMD/config.yaml`, stores its checkpoint in `$CHARMD/state.json`, and
listens on `$CHARMD/charmd.sock`. When unset, `/var/lib/charmd/default` is
used.
When `services.charm-api` is empty or absent, charmd uses its internal charm
client. The checked-in `example/config.yaml` selects this default:

```yaml
services:
  charm-api:
```

An external charm API can be configured with a URL instead. It must be an HTTP
or HTTPS URL without embedded credentials; startup fails if a configured URL is
invalid.

The read-only commands deliberately follow Pebble's command names and table
layout:

```text
charmd changes
charmd tasks <change-id>
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
