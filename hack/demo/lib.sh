# Shared settings and helpers for the demo scripts. Source, don't execute.

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DEMO_DIR="$ROOT/.demo"
KUBECONFIG_DIR="$DEMO_DIR/kubeconfigs"
CLUSTERS=(orkestra-a orkestra-b orkestra-c)
DEMO_IMAGE="nginx:1.27-alpine"

bold=$'\033[1m'; green=$'\033[32m'; yellow=$'\033[33m'; red=$'\033[31m'; reset=$'\033[0m'

step() { printf '\n%s==> %s%s\n' "$bold" "$*" "$reset"; }
info() { printf '    %s\n' "$*"; }
ok()   { printf '    %s✔ %s%s\n' "$green" "$*" "$reset"; }
warn() { printf '    %s! %s%s\n' "$yellow" "$*" "$reset"; }
die()  { printf '%s✘ %s%s\n' "$red" "$*" "$reset" >&2; exit 1; }

require() {
	for tool in "$@"; do
		command -v "$tool" >/dev/null || die "$tool is required but not installed"
	done
}

# wait_for <description> <timeout-seconds> <command...>
# Re-runs the command every 2s until it succeeds or the timeout expires.
wait_for() {
	local desc=$1 timeout=$2
	shift 2
	local deadline=$((SECONDS + timeout))
	until "$@" >/dev/null 2>&1; do
		((SECONDS < deadline)) || die "timed out after ${timeout}s waiting for: $desc"
		sleep 2
	done
}
