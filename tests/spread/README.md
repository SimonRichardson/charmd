# Lifecycle integration tests

Run `spread -v` from the repository root with Go, Spread, and a configured LXD
installation available on the host. Use `spread -list` to list scenarios, or
`spread -v tests/spread/initial-leader` to run one scenario. For scheduling
regressions, use `spread -repeat 5`.

Spread builds the current charmd source into a static Linux binary on the host
and runs it in an isolated Ubuntu 24.04 container of the same architecture.
Guests need Python 3, which is included in the Ubuntu image. No charm download
or Go installation is needed in the guest.

Each task starts a fresh daemon with a generated executable charm archive and
real filesystem charm, config, and leadership APIs. The charm invokes hookd's
`config-get`, `is-leader`, `juju-log`, and `status-set` commands. Both dispatch
and standalone hook entry points are exercised. The scenarios check complete
hook histories and terminal change statuses, including:

- Install before config, with no start hook.
- Install before leader-elected before initial config when already leader.
- Leadership gain during a blocked hook, immutable leadership and config within
  that hook, election priority over queued config, loss, and reelection.
- Semantic config deduplication, invalid updates, and watcher recovery.
- Failed install preventing all subsequent lifecycle hooks.
- Global hook serialization across two different charm installations.
- Graceful cancellation checkpointing queued work for retry after restart.
- Abrupt install/config interruption entering an error state and blocking later hooks.
- Queued, executed, and committed checkpoints resuming at the appropriate phase.

A shared nonblocking file lock detects overlapping hooks. Gate files let the
leadership scenario queue work while a hook is demonstrably running. Polls have
deadlines; quiet windows are used only when checking that no work occurs.

The harness stops and reaps the daemon even if a scenario fails. Spread prints
the daemon log and hook history on failure before removing task scratch data.
Use Spread's `-debug` option to inspect failures interactively.

The interruption scenarios kill the daemon and active hook process group to
simulate process-tree loss. Commit-boundary scenarios restore a checkpoint derived
from a real completed invocation while the daemon is stopped. This makes the tiny
execution/commit window deterministic without adding production failpoints. The
Go recovery tests separately restore every checkpoint actually written by the
backend during hook execution.
