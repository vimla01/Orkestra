#!/usr/bin/env bash
# Deletes the demo kind clusters and all local demo state.
set -euo pipefail
source "$(dirname "$0")/lib.sh"

require kind

for cluster in "${CLUSTERS[@]}"; do
	if kind get clusters 2>/dev/null | grep -qx "$cluster"; then
		step "Deleting $cluster"
		kind delete cluster --name "$cluster"
	fi
done

rm -rf "$DEMO_DIR"
step "Demo environment removed."
