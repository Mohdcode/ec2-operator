# EC2 Operator

A Kubernetes operator written in Go that provisions and terminates AWS EC2 instances from `Ec2Instance` custom resources. The controller reports the instance ID and observed state to the Kubernetes resource and uses a finalizer to request EC2 termination when the resource is deleted.

The project includes a no-AWS local demo using kind and LocalStack, plus a GitHub Actions workflow that runs the same demo on pull requests and pushes.

## Try it locally without AWS

### Prerequisites

- Docker
- kind
- kubectl
- Helm 3

Run:

```bash
make local-demo
```

This builds the controller image, starts a kind cluster, deploys LocalStack with the EC2 API enabled, installs the Helm chart, and applies a sample `Ec2Instance`. It creates a Kubernetes Secret with LocalStack's test-only credentials; no AWS account or cloud credentials are used.

Watch the resource status:

```bash
kubectl get ec2instances --watch
kubectl describe ec2instance ec2instance-sample
```

Remove the demo resources and cluster:

```bash
make local-demo-clean
```

The local run checks the operator against LocalStack's EC2 API implementation. Emulator behavior can differ from AWS, so this does not establish that a deployment or every EC2 feature works in a real AWS account.

## Run checks

```bash
make test
make lint
helm lint ./helm/charts/ec2-operator
```

The `Local EC2 integration` GitHub Actions workflow builds the image and runs `make local-demo` on pull requests and pushes. It needs no AWS credentials or hosted Kubernetes cluster. The existing `test-e2e` target covers the controller manager and metrics endpoint in kind.

Pushing a `vX.Y.Z` Git tag runs the release workflow, which publishes the container image to GitHub Container Registry and the Helm chart as an OCI artifact. This publishing step uses the repository's built-in GitHub token and does not deploy to AWS.

When a staging Kubernetes cluster is available, the `Deploy to staging` workflow can be run manually with a published release tag. Configure a GitHub Environment named `staging` with a `KUBECONFIG` secret first. The workflow installs or upgrades the operator and waits for its Deployment to become ready; it does not create an `Ec2Instance` or launch cloud resources. Make the GHCR image public or create an image-pull Secret in the staging namespace and set the environment variable `GHCR_PULL_SECRET_NAME` to that Secret's name. The operator needs AWS credentials or a workload identity in the cluster only when it is asked to manage an EC2 resource.

## Install against AWS

The chart uses the AWS SDK's default credential chain, so it can use environment credentials or a workload identity/pod role. To provide credentials through a pre-created Kubernetes Secret, create one in the release namespace:

```bash
kubectl create namespace ec2-operator-system
kubectl create secret generic aws-credentials \
  --namespace ec2-operator-system \
  --from-literal=AWS_ACCESS_KEY_ID="$AWS_ACCESS_KEY_ID" \
  --from-literal=AWS_SECRET_ACCESS_KEY="$AWS_SECRET_ACCESS_KEY"

helm upgrade --install ec2-operator ./helm/charts/ec2-operator \
  --namespace ec2-operator-system \
  --set aws.credentialsSecretName=aws-credentials
```

Do not put real credentials in chart values or commit them to the repository. To target an AWS-compatible local endpoint, set `aws.endpoint` and reference a Secret containing that service's test credentials.

## Custom resource

The API currently defines these desired-state fields:

| Field | Description |
| --- | --- |
| `amiId` | AMI ID used to launch the instance |
| `instanceType` | EC2 instance type |
| `region` | AWS region |
| `availabilityZone`, `subnet`, `securityGroups`, `associatePublicIP` | Network and placement settings |
| `keyPair`, `userData`, `tags`, `storage` | Access, bootstrap, tags, and volume settings |

Example: [`config/samples/compute_v1_ec2instance.yaml`](config/samples/compute_v1_ec2instance.yaml).

At launch, the controller passes the declared placement, networking, key pair, user data, tags, and EBS volume settings to EC2. It then records the instance ID and periodically refreshes observed state and IP information. Editing a resource's spec after creation does not yet update the existing EC2 instance, so this is not full desired-state drift correction.

## Project layout

- `api/v1/`: `Ec2Instance` API types and generated CRD source
- `internal/controller/`: reconciliation and EC2 client logic
- `helm/charts/ec2-operator/`: installable Helm chart
- `config/localstack/`: LocalStack deployment for the local demo
- `.github/workflows/local-integration.yml`: no-cloud integration workflow
- `terraform/`: supporting infrastructure configuration

## Contributing

Changes are welcome. Start with `make help` to see the available build, test, and deployment targets.
