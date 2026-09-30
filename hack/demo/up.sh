#!/usr/bin/env bash
# Creates the demo kind clusters and writes a kubeconfig for each into
# .demo/kubeconfigs. The user's ~/.kube/config is left untouched.
set -euo pipefail
source "$(dirname "$0")/lib.sh"

require docker kind

mkdir -p "$KUBECONFIG_DIR"

step "Pulling demo image $DEMO_IMAGE"
docker pull --quiet "$DEMO_IMAGE" >/dev/null || warn "pull failed; clusters will pull it themselves"

for cluster in "${CLUSTERS[@]}"; do
	kubeconfig="$KUBECONFIG_DIR/$cluster.yaml"
	step "Cluster $cluster"
	if kind get clusters 2>/dev/null | grep -qx "$cluster"; then
		info "already exists"
		# Restart it in case an earlier demo left it stopped.
		docker start "$cluster-control-plane" >/dev/null
	else
		kind create cluster --name "$cluster" --kubeconfig "$kubeconfig" --wait 120s
	fi
	kind get kubeconfig --name "$cluster" >"$kubeconfig"
	kind load docker-image "$DEMO_IMAGE" --name "$cluster" >/dev/null 2>&1 || true
	ok "ready ($kubeconfig)"
done

step "Done. Run 'make demo' to see Orkestra in action."
