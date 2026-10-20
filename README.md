# charmd

`charmd` is a small Pebble-inspired changes-and-tasks service. State is stored
as an atomic JSON checkpoint in `state.json` (or in `$CHARMD_STATE_DIR`).

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