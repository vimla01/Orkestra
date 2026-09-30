#!/usr/bin/env bash
# End-to-end Orkestra demo on local kind clusters:
#   register 3 clusters -> propagate nginx to 2 of them -> kill one cluster
#   -> watch Orkestra fail the deployment over to the spare cluster.
#
# Run `make demo-up` first. The control plane runs only for the length of
# this script; the clusters are left running for further experiments.
set -euo pipefail
source "$(dirname "$0")/lib.sh"

require docker kind go

for cluster in "${CLUSTERS[@]}"; do
	[[ -f "$KUBECONFIG_DIR/$cluster.yaml" ]] || die "cluster $cluster not set up; run 'make demo-up' first"
done

cd "$ROOT"
ORK="$ROOT/bin/orkestra"
SERVER_LOG="$DEMO_DIR/server.log"
FAILED_CLUSTER="orkestra-a"
export ORKESTRA_SERVER="http://localhost:8080"

server_pid=""
cleanup() {
	if [[ -n "$server_pid" ]]; then
		kill "$server_pid" 2>/dev/null || true
		wait "$server_pid" 2>/dev/null || true
	fi
	# Bring the stopped cluster back so the next run starts from a clean slate.
	docker start "$FAILED_CLUSTER-control-plane" >/dev/null 2>&1 || true
}
trap cleanup EXIT

# Make sure every cluster is up, even if a previous run was interrupted.
for cluster in "${CLUSTERS[@]}"; do
	docker start "$cluster-control-plane" >/dev/null
done

step "Building Orkestra"
go build -o "$ORK" ./cmd/orkestra
ok "bin/orkestra"

step "Starting the control plane (log: .demo/server.log)"
if curl -sf "$ORKESTRA_SERVER/api/v1/health" >/dev/null 2>&1; then
	die "something is already listening on :8080; stop it and retry"
fi
rm -f "$DEMO_DIR/state.json"
"$ORK" serve --config hack/demo/config.yaml >"$SERVER_LOG" 2>&1 &
server_pid=$!
wait_for "control plane to start" 20 curl -sf "$ORKESTRA_SERVER/api/v1/health"
ok "listening on $ORKESTRA_SERVER"

step "Registering member clusters"
for cluster in "${CLUSTERS[@]}"; do
	# Wait for each API server to accept connections after a restart.
	wait_for "$cluster API server" 60 \
		"$ORK" cluster register --name "$cluster" --kubeconfig "$KUBECONFIG_DIR/$cluster.yaml"
	ok "$cluster"
done

all_healthy() {
	[[ $("$ORK" cluster list | grep -c ' Healthy ') -eq ${#CLUSTERS[@]} ]]
}
step "Waiting for the first health checks"
wait_for "all clusters Healthy" 60 all_healthy
"$ORK" cluster list

step "Propagating examples/nginx-deployment.yaml to orkestra-a and orkestra-b"
"$ORK" deploy --file examples/nginx-deployment.yaml --clusters orkestra-a,orkestra-b

deployment_ready() {
	"$ORK" deployment status web | head -1 | grep -q ': Ready$'
}
step "Waiting for the rollout on both clusters"
wait_for "web to roll out" 180 deployment_ready
"$ORK" deployment status web

step "Simulating an outage: stopping $FAILED_CLUSTER"
docker stop "$FAILED_CLUSTER-control-plane" >/dev/null
warn "$FAILED_CLUSTER is down; health checks will mark it Unhealthy"

cluster_unhealthy() {
	"$ORK" cluster list | grep "^$FAILED_CLUSTER " | grep -q Unhealthy
}
wait_for "$FAILED_CLUSTER to be marked Unhealthy" 60 cluster_unhealthy
"$ORK" cluster list

failed_over() {
	"$ORK" deployment list | grep '^default ' | grep -q orkestra-c
}
step "Waiting out the 15s grace period for failover"
wait_for "web to fail over to orkestra-c" 90 failed_over
ok "Orkestra moved web off $FAILED_CLUSTER"

step "Waiting for the rollout on the new cluster"
wait_for "web to roll out after failover" 180 deployment_ready
"$ORK" deployment status web

step "Demo complete"
info "Server log:      .demo/server.log"
info "Run it again:    make demo"
info "Explore:         make demo-server   (then use bin/orkestra in another terminal)"
info "Clean up:        make demo-down"
