#!/usr/bin/env bash
# Build on the host and exercise a local charm install in an LXC container.
set -euo pipefail

usage() {
    echo "Usage: $0 [--keep] <container> <charm-archive>" >&2
    echo "Missing containers are created from ubuntu:22.04; stopped containers are started." >&2
    echo "TIMEOUT sets the readiness/install/config timeout in seconds (default: 120)." >&2
    echo "The container is deleted on exit (even if reused); --keep preserves it." >&2
}

keep=false
positional=()
while (( $# )); do
    case $1 in
        --keep) keep=true ;;
        --help|-h) usage; exit 0 ;;
        --) shift; positional+=("$@"); break ;;
        -*) usage; exit 2 ;;
        *) positional+=("$1") ;;
    esac
    shift
done
if [[ ${#positional[@]} != 2 ]]; then
    usage
    exit 2
fi
container=${positional[0]}
charm=${positional[1]}
timeout=${TIMEOUT:-120}
if [[ ! $timeout =~ ^[1-9][0-9]*$ ]]; then
    echo "TIMEOUT must be a positive integer." >&2
    exit 2
fi
if [[ ! -f $charm || ! -r $charm ]]; then
    echo "Charm archive is not a readable file: $charm" >&2
    exit 2
fi
for command in go lxc sha256sum mktemp; do
    command -v "$command" >/dev/null || { echo "Missing command: $command" >&2; exit 1; }
done
repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
build_dir=$(mktemp -d)
container_ready=false
cleanup_host() {
    result=$?
    trap - EXIT
    if [[ $container_ready == true && $keep == false ]]; then
        echo "Removing container $container..."
        if ! lxc delete --force "$container"; then
            echo "Failed to remove container $container." >&2
            if (( result == 0 )); then result=1; fi
        fi
    fi
    rm -rf -- "$build_dir"
    exit "$result"
}
trap cleanup_host EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# Query successfully before deciding the container is absent: connection or
# permission failures must not be mistaken for a missing container.
instances=$(lxc list "$container" --format csv -c ns)
status=
while IFS=, read -r name instance_status; do
    if [[ $name == "${container##*:}" ]]; then
        status=$instance_status
        break
    fi
done <<< "$instances"
case $status in
    '')
        echo "Creating container $container from ubuntu:24.04..."
        lxc launch ubuntu:24.04 "$container"
        ;;
    STOPPED)
        container_ready=true
        echo "Starting container $container..."
        lxc start "$container"
        ;;
    RUNNING) ;;
    *) echo "Container $container has unsupported status: $status" >&2; exit 1 ;;
esac
container_ready=true

# A static Linux binary avoids depending on the container's libc version.
machine=$(lxc exec "$container" -- uname -m)
case $machine in
    x86_64) arch=amd64 ;;
    aarch64|arm64) arch=arm64 ;;
    *) echo "Unsupported container architecture: $machine" >&2; exit 1 ;;
esac
echo "Building charmd for linux/$arch..."
(cd "$repo" && CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -o "$build_dir/charmd" ./cmd/charmd)
checksum=$(sha256sum -- "$charm")
checksum=${checksum%% *}
runtime=$(lxc exec "$container" -- mktemp -d /var/tmp/charmd-test.XXXXXXXX)
echo "Container runtime directory: $runtime"
lxc exec "$container" -- mkdir -p "$runtime/source"
lxc file push "$build_dir/charmd" "$container$runtime/charmd"
lxc file push "$charm" "$container$runtime/source/$checksum"

# Keep daemon and client in the same exec session so cleanup always stops the
# daemon. Leave the runtime directory intact for inspecting state and logs.
lxc exec "$container" -- env CHARMD="$runtime" bash -s -- "$checksum" "$timeout" "$keep" <<'CONTAINER'
set -euo pipefail
checksum=$1
timeout=$2
keep=$3
cd "$CHARMD"
chmod +x ./charmd
printf 'services:\n  charm-api:\n    path: %s/source\n  config-api:\n    path: %s/charm-config.yaml\n  leadership-api:\n    path: %s/charm-leadership.yaml\n' "$CHARMD" "$CHARMD" "$CHARMD" > config.yaml
printf 'charms:\n  "%s":\n    status: "world"\n' "$checksum" > charm-config.yaml
printf 'charms:\n  "%s": false\n' "$checksum" > charm-leadership.yaml
printf 'name: local-test\n' > "source/$checksum.manifest"
./charmd > daemon.log 2>&1 &
daemon_pid=$!
cleanup() {
    result=$?
    trap - EXIT
    kill "$daemon_pid" 2>/dev/null || true
    wait "$daemon_pid" 2>/dev/null || true
    if (( result != 0 )); then
        echo "Charm install test failed. Daemon log:" >&2
        cat daemon.log >&2
    fi
    if [[ $keep == true ]]; then
        echo "Runtime files retained at $CHARMD"
    fi
    exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
deadline=$((SECONDS + timeout))
until ./charmd changes >/dev/null 2>&1; do
    if ! kill -0 "$daemon_pid" 2>/dev/null || (( SECONDS >= deadline )); then
        echo "Daemon failed to become ready." >&2
        exit 1
    fi
    sleep 1
done
output=$(./charmd install "$checksum")
echo "$output"
if [[ $output =~ ^Change\ ([0-9]+)\ created\.$ ]]; then
    change=${BASH_REMATCH[1]}
else
    echo "Could not determine install change ID." >&2
    exit 1
fi
deadline=$((SECONDS + timeout))
while true; do
    changes=$(./charmd changes)
    status=$(awk -v id="$change" '$1 == id {print $2}' <<< "$changes")
    case $status in
        Done) break ;;
        Error|Hold|Undone|Abort)
            ./charmd tasks "$change"
            echo "Install change $change finished with status $status." >&2
            exit 1
            ;;
    esac
    if (( SECONDS >= deadline )); then
        ./charmd tasks "$change"
        echo "Timed out waiting for install change $change (status: $status)." >&2
        exit 1
    fi
    sleep 1
done
# Config notifications are handled in a separate change after install succeeds.
# Wait for that change explicitly before cleanup stops the daemon.
deadline=$((SECONDS + timeout))
config_change=
while true; do
    changes=$(./charmd changes)
    config_change=$(awk -v checksum="$checksum" '$1 ~ /^[0-9]+$/ && index($0, "Apply config for charm") && index($0, checksum) {print $1; exit}' <<< "$changes")
    status=not-created
    if [[ -n $config_change ]]; then
        status=$(awk -v id="$config_change" '$1 == id {print $2}' <<< "$changes")
        case $status in
            Done) break ;;
            Error|Hold|Undone|Abort)
                ./charmd tasks "$config_change"
                echo "Config change $config_change finished with status $status." >&2
                exit 1
                ;;
        esac
    fi
    if ! kill -0 "$daemon_pid" 2>/dev/null || (( SECONDS >= deadline )); then
        ./charmd changes
        if [[ -n $config_change ]]; then ./charmd tasks "$config_change"; fi
        echo "Timed out waiting for config change (id: ${config_change:-not-created}, status: ${status:-unknown})." >&2
        exit 1
    fi
    sleep 1
done
./charmd changes
./charmd tasks "$change"
./charmd tasks "$config_change"
test -d "$CHARMD/charms/$checksum"
echo "Charm installed and configured at $CHARMD/charms/$checksum"
CONTAINER
