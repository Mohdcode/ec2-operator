#!/usr/bin/env bash
set -euo pipefail

kind_version="v0.27.0"
kubebuilder_version="v4.16.0"
kubectl_version="v1.32.0"

case "$(uname -m)" in
  x86_64) arch="amd64" ;;
  aarch64 | arm64) arch="arm64" ;;
  *)
    echo "Unsupported Dev Container architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

temporary_directory="$(mktemp -d)"
cleanup() {
  rm -f "$temporary_directory/kind" \
    "$temporary_directory/kubebuilder" \
    "$temporary_directory/kubectl"
  rmdir "$temporary_directory"
}
trap cleanup EXIT

curl -fsSLo "$temporary_directory/kind" \
  "https://kind.sigs.k8s.io/dl/${kind_version}/kind-linux-${arch}"
curl -fsSLo "$temporary_directory/kubebuilder" \
  "https://github.com/kubernetes-sigs/kubebuilder/releases/download/${kubebuilder_version}/kubebuilder_linux_${arch}"
curl -fsSLo "$temporary_directory/kubectl" \
  "https://dl.k8s.io/release/${kubectl_version}/bin/linux/${arch}/kubectl"

install -m 0755 "$temporary_directory/kind" /usr/local/bin/kind
install -m 0755 "$temporary_directory/kubebuilder" /usr/local/bin/kubebuilder
install -m 0755 "$temporary_directory/kubectl" /usr/local/bin/kubectl

kind version
kubebuilder version
docker --version
go version
kubectl version --client
