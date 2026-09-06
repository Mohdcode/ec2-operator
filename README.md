# EC2 Operator

A Kubernetes-native operator, written in Go, that manages the full lifecycle of AWS EC2 instances declaratively — through Kubernetes Custom Resource Definitions (CRDs) — instead of manual provisioning via the AWS Console or CLI.

## Description

This project lets you describe the EC2 instance you want as a Kubernetes resource, and the operator takes care of actually creating, updating, and deleting it in AWS to match. It follows the same reconciliation pattern Kubernetes itself uses internally: you declare *desired state*, and a controller continuously works to make *actual state* match it.

Built with [kubebuilder](https://book.kubebuilder.io/) and [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime), using the AWS SDK for Go to interact with EC2.

### What it does

- Watches `Ec2Instance` custom resources and reconciles them against real AWS EC2 state
- Creates, updates, and terminates EC2 instances based on the CR spec
- Uses a finalizer to ensure the AWS instance is properly terminated before the CR is removed from Kubernetes, avoiding orphaned cloud resources
- Writes live instance data (public IP, instance ID) back into the CR's `status`, so `kubectl get` shows real-time state
- Packaged as a Helm chart for installation, including CRDs, RBAC, and the controller deployment

### Custom Resource spec

An `Ec2Instance` resource supports the following fields:

| Field | Type | Required | Description |
|---|---|---|---|
| `amiId` | string | Yes | AMI to launch the instance from |
| `instanceType` | string | Yes | EC2 instance type (e.g. `t2.micro`) |
| `region` | string | Yes | AWS region to provision in |
| `availabilityZone` | string | No | Specific AZ within the region |
| `subnet` | string | No | Subnet to launch into |
| `securityGroups` | []string | No | Security groups to attach |
| `associatePublicIP` | boolean | No | Whether to assign a public IP |
| `keyPair` | string | No | SSH key pair name |
| `storage` | object | No | Storage configuration for the instance |
| `tags` | map[string]string | No | Tags to apply to the instance |
| `userData` | string | No | Instance bootstrap script |

## Getting Started

### Prerequisites

- Go v1.23+
- Docker
- kubectl
- Access to a Kubernetes cluster (tested locally with [kind](https://kind.sigs.k8s.io/))
- Helm v3+
- AWS credentials with EC2 permissions

### Deploying with Helm (recommended)

This is the tested, working installation path:

```bash
# 1. Create a namespace and install the chart
helm install ec2-operator ./helm/charts/ec2-operator \
  --namespace ec2-operator-system --create-namespace
```

> **Note:** the chart currently creates its own `ec2-operator` namespace as part of its templates, so resources land there regardless of the `--namespace` flag passed to Helm. This is a known detail I plan to clean up by templating the namespace name properly (`{{ .Release.Namespace }}`).

Verify the controller is running:

```bash
kubectl get pods -n ec2-operator
```

You should see the controller pod in `Running` state.

### Deploying manually (without Helm)

```bash
# Install the CRDs
make install

# Build and push your image
make docker-build docker-push IMG=<your-registry>/ec2operator:tag

# Deploy the controller
make deploy IMG=<your-registry>/ec2operator:tag
```

### Creating an instance

```bash
kubectl apply -k config/samples/
```

Check the resulting resource, including its live AWS status:

```bash
kubectl get ec2instances
kubectl describe ec2instance <name>
```

### Cleaning up

```bash
kubectl delete -k config/samples/
helm uninstall ec2-operator -n ec2-operator-system
make uninstall
```

## Why I built this

I built this after a CNCF LFX Mentorship on the Kyverno project, where I worked deeply with Kubernetes' controller and reconciliation model. I wanted to build one myself from scratch — CRD design, reconciliation logic, and real integration with an external system (AWS) — rather than just working within an existing controller.

## Known limitations / next steps

- AWS credentials are read from environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`) inside the controller, injected via `secretKeyRef` from the `aws-config` Kubernetes Secret rather than being embedded in the CR spec or container image — this is the correct pattern; an earlier version of this project passed credentials directly in the CR spec, which is not secure, and was corrected.
- The chart's namespace is currently hardcoded rather than following `{{ .Release.Namespace }}`.
- No support yet for detecting and reacting to changes made directly in AWS outside of Kubernetes (e.g. via EventBridge) — drift is currently only corrected on the next scheduled reconcile, not in real time.

## Contributing

Issues and PRs are welcome. Run `make help` for available `make` targets.

More on the underlying framework: [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)