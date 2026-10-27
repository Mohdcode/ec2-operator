#!/usr/bin/env bash
set -euo pipefail

helm="${HELM:-${LOCALBIN:-$(pwd)/bin}/helm}"
e2e_cluster="kind-ci-e2e"
chart_cluster="kind-ci-chart"
local_demo_started=false
e2e_cluster_created=false
chart_cluster_created=false

for cluster in "$e2e_cluster" "$chart_cluster" ec2-operator; do
  if kind get clusters | grep -Fxq "$cluster"; then
    echo "Refusing to use existing kind cluster '$cluster'." >&2
    exit 1
  fi
done

cleanup() {
  status=$?
  trap - EXIT

  if [[ "$status" -ne 0 ]]; then
    kubectl get pods -A -o wide || true
    kubectl get events -A --sort-by=.lastTimestamp || true
    kubectl logs -n ec2operator-system \
      deployment/ec2operator-controller-manager --all-containers --tail=100 || true
    kubectl logs -n ec2-operator-system \
      deployment/ec2-operator-controller-manager --all-containers --tail=100 || true
    kubectl logs -n localstack deployment/localstack --tail=100 || true
  fi

  if [[ "$local_demo_started" == true ]]; then
    make local-demo-clean || status=1
  fi
  if [[ "$chart_cluster_created" == true ]]; then
    kind delete cluster --name "$chart_cluster" || status=1
  fi
  if [[ "$e2e_cluster_created" == true ]]; then
    kind delete cluster --name "$e2e_cluster" || status=1
  fi

  exit "$status"
}
trap cleanup EXIT

echo "::group::Go dependency, lint, and unit-test checks"
go mod tidy -diff
make lint
make test
"$helm" lint ./dist/chart
echo "::endgroup::"

echo "::group::Kubebuilder E2E tests"
kind create cluster --name "$e2e_cluster" --wait 120s
e2e_cluster_created=true
KIND_CLUSTER="$e2e_cluster" make test-e2e
kind delete cluster --name "$e2e_cluster"
e2e_cluster_created=false
echo "::endgroup::"

echo "::group::Release chart installation test"
kind create cluster --name "$chart_cluster" --wait 120s
chart_cluster_created=true
docker build -t ec2-operator:ci .
kind load docker-image ec2-operator:ci --name "$chart_cluster"
"$helm" upgrade --install ec2-operator ./dist/chart \
  --namespace ec2-operator-system \
  --create-namespace \
  --set manager.image.repository=ec2-operator \
  --set manager.image.tag=ci \
  --set manager.image.pullPolicy=IfNotPresent \
  --wait \
  --timeout 5m
kubectl rollout status deployment/ec2-operator-controller-manager \
  --namespace ec2-operator-system \
  --timeout=5m
kubectl get crd ec2instances.compute.cloud.com
kind delete cluster --name "$chart_cluster"
chart_cluster_created=false
echo "::endgroup::"

echo "::group::LocalStack EC2 integration test"
local_demo_started=true
make local-demo
make local-demo-clean
local_demo_started=false
echo "::endgroup::"
