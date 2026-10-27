#!/usr/bin/env bash
set -euo pipefail

cluster_name="ec2-operator"
local_image="${LOCAL_IMAGE:-ec2-operator:local}"
image_repository="${local_image%:*}"
image_tag="${local_image##*:}"
helm="${HELM:-helm}"

for command in docker kind kubectl "$helm"; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "Required command '$command' was not found. Install Docker, kind, kubectl, and Helm first." >&2
    exit 1
  fi
done

if ! kind get clusters | grep -qx "$cluster_name"; then
  kind create cluster --name "$cluster_name" --wait 120s
fi
kubectl config use-context "kind-$cluster_name"

"$helm" lint ./dist/chart
docker build -t "$local_image" .
kind load docker-image "$local_image" --name "$cluster_name"

kubectl apply -k config/localstack
kubectl wait --for=condition=Available deployment/localstack -n localstack --timeout=180s

kubectl create namespace ec2-operator-system --dry-run=client -o yaml | kubectl apply -f -
kubectl create secret generic aws-credentials \
  --namespace ec2-operator-system \
  --from-literal=AWS_ACCESS_KEY_ID=test \
  --from-literal=AWS_SECRET_ACCESS_KEY=test \
  --dry-run=client -o yaml | kubectl apply -f -

"$helm" upgrade --install ec2-operator ./dist/chart \
  --namespace ec2-operator-system \
  --set manager.image.repository="$image_repository" \
  --set manager.image.tag="$image_tag" \
  --set-string manager.envOverrides.AWS_ENDPOINT=http://localstack.localstack.svc.cluster.local:4566

kubectl rollout status deployment/ec2-operator-controller-manager -n ec2-operator-system --timeout=180s
kubectl apply -f config/samples/compute_v1_ec2instance.yaml

echo "Waiting for the sample custom resource to receive an EC2 instance ID..."
for _ in $(seq 1 60); do
  instance_id="$(kubectl get ec2instance ec2instance-sample -o jsonpath='{.status.instanceId}' 2>/dev/null || true)"
  ready="$(kubectl get ec2instance ec2instance-sample -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)"
  if [[ -n "$instance_id" && "$ready" == "True" ]]; then
    kubectl get ec2instance ec2instance-sample
    echo "Local EC2 API demo succeeded with instance ID: $instance_id and Ready=True."
    echo "This validates the operator against LocalStack; it does not validate real AWS behavior."
    exit 0
  fi
  sleep 2
done

kubectl describe ec2instance ec2instance-sample || true
kubectl logs deployment/ec2-operator-controller-manager -n ec2-operator-system --tail=100 || true
echo "The operator did not report an instance ID before the timeout." >&2
exit 1
