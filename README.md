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

## Test locally

The test paths cover different parts of the project:

```bash
# Unit and controller tests (uses Kubernetes envtest binaries; no cluster or AWS account)
make test

# Static analysis
make lint

# Helm chart validation
helm lint ./dist/chart
```

The released chart is generated from the Kubebuilder `helm/v2-alpha` plugin and lives in `dist/chart`. To regenerate it, install Kubebuilder CLI v4.16.0 and run `make helm-chart`. The chart's generated manifests are refreshed while the project's CI workflow and Helm deployment targets remain intact.

### Run the complete CI suite in the Dev Container

Open the project in its Dev Container in VS Code, then run:

```bash
make ci
```

This runs dependency-tidiness, lint, unit tests, Kubebuilder E2E tests, Helm chart installation on kind, and the LocalStack EC2 integration demo. It creates and removes its own kind clusters and refuses to reuse clusters with its reserved names. The `Dev Container CI` GitHub Actions workflow builds this same Dev Container and runs `make ci` inside it; the release validation uses the same path. GitHub-hosted Actions still supplies an Ubuntu VM to build and launch the container, but the project checks and their toolchain run inside the Dev Container rather than directly on the runner.

For a manager deployment and metrics smoke test in kind, create a kind cluster named `kind` and run:

```bash
make test-e2e
```

This test does not create EC2 instances. For end-to-end testing of the EC2 API calls without AWS credentials, use the LocalStack demo in [Try it locally without AWS](#try-it-locally-without-aws). The Dev Container CI workflow runs the chart installation and LocalStack demo on pushes and pull requests. LocalStack is an emulator, so a successful demo does not prove that a request will work with AWS IAM, a particular AMI, subnet, or security group.

### Deploy to your current Kubernetes context

After building or selecting a controller image that your cluster can pull, deploy the current Kustomize manifests with:

```bash
kubectl config current-context # Confirm the target cluster first.
make cd IMG=ghcr.io/mohdcode/ec2-operator:v0.2.0
```

Replace `v0.2.0` with an image tag that exists and is accessible to your cluster. `make cd` is an alias for `make deploy`; it updates the image in `config/manager/kustomization.yaml` and applies the operator to the cluster selected by your current kubeconfig. It does not create a cluster or publish an image. To remove the deployment, run `make undeploy`.

## Test against a real AWS account

This test launches a real EC2 instance and can incur EC2, EBS, and public IPv4 charges. Use a disposable non-production account, a region where you are permitted to launch resources, and clean up by deleting the `Ec2Instance` resource as described below. Do not use the sample AMI ID unchanged: AMI IDs are region-specific and may no longer be available.

The operator can run in EKS or another Kubernetes cluster that can reach the AWS EC2 API. Give the operator pod an AWS identity through EKS Pod Identity or IRSA where available; the chart's service account supports annotations. For example, save this as a local `aws-values.yaml` and replace the role ARN:

```yaml
serviceAccount:
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/ec2-operator-test
```

Configure the role trust policy for the operator's Kubernetes service account. Grant the role `ec2:RunInstances`, `ec2:DescribeInstances`, and `ec2:TerminateInstances`, scoped to the test resources where possible. Because the sample adds EC2 tags at launch, it also needs `ec2:CreateTags` restricted to tagging during `RunInstances` (for example, with the `ec2:CreateAction` condition). Scope `ec2:RunInstances` to the AMIs and network resources you intend to use; EC2 resource-level authorization is more involved than a single instance ARN.

If workload identity is unavailable, the chart reads `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` from the optional `aws-credentials` Kubernetes Secret by default. To use a differently named Secret or different key names, override `manager.env` in a values file. Avoid putting credentials in chart values or committing the secret.

Install a published chart version (replace the version and tag with a release that exists in GHCR):

```bash
helm upgrade --install ec2-operator \
  oci://ghcr.io/mohdcode/charts/ec2-operator \
  --version 0.2.0 \
  --namespace ec2-operator-system \
  --create-namespace \
  --set manager.image.repository=ghcr.io/mohdcode/ec2-operator \
  --set manager.image.tag=v0.2.0 \
  --values aws-values.yaml
```

Copy the sample to a working file, then set an AMI valid in your target region. For a safer network setup, choose a subnet and security groups you control and leave `associatePublicIP` false unless a public address is specifically needed:

```bash
cp config/samples/compute_v1_ec2instance.yaml /tmp/ec2instance-aws-test.yaml
# Edit amiId and region; set associatePublicIP: false, and optionally set subnet and securityGroups.
kubectl apply -f /tmp/ec2instance-aws-test.yaml
kubectl get ec2instance ec2instance-sample --watch
kubectl describe ec2instance ec2instance-sample
```

Check that the resource reports an instance ID and `Ready=True`. The operator creates the EC2 instance but does not configure or install an operating system on it. The controller also does not update an existing EC2 instance when its spec is edited.

Delete the custom resource before uninstalling the chart. Its finalizer asks AWS to terminate the instance and waits for termination before allowing Kubernetes to remove the resource:

```bash
instance_id="$(kubectl get ec2instance ec2instance-sample -o jsonpath='{.status.instanceId}')"
kubectl delete -f /tmp/ec2instance-aws-test.yaml
kubectl wait --for=delete ec2instance/ec2instance-sample --timeout=6m
region=us-east-1 # Use the same region as the custom resource.
aws ec2 wait instance-terminated \
  --instance-ids "$instance_id" \
  --region "$region"
helm uninstall ec2-operator --namespace ec2-operator-system
```

If the custom resource is removed from Kubernetes without the operator running, its finalizer cannot finish the AWS cleanup. Verify termination in AWS before considering the test cleaned up.

Pushing a `vX.Y.Z` Git tag first runs the full Dev Container CI suite: dependency-tidiness verification, Go lint and tests, Kubebuilder E2E tests, Helm chart installation on kind, and the LocalStack EC2 integration demo. Only when those checks pass does the workflow publish a version-tagged container image and Helm chart to GitHub Container Registry. The image includes build provenance and an SBOM. Protect release tags with a GitHub ruleset to prevent them from being moved or reused. Publishing uses the repository's built-in GitHub token and does not deploy to AWS.

When a staging Kubernetes cluster is available, the `Deploy to staging` workflow can be run manually with a published release tag. Configure a GitHub Environment named `staging` with a `KUBECONFIG` secret first. The workflow installs or upgrades the operator and waits for its Deployment to become ready; it does not create an `Ec2Instance` or launch cloud resources. Make the GHCR image public or create an image-pull Secret in the staging namespace and set the environment variable `GHCR_PULL_SECRET_NAME` to that Secret's name. Protect the staging environment with required reviewers and configure the operator's AWS workload identity separately before testing real EC2 provisioning.

The chart uses the AWS SDK's default credential chain. To target an AWS-compatible local endpoint instead, set `manager.envOverrides.AWS_ENDPOINT` and provide an `aws-credentials` Secret if the service requires credentials; `make local-demo` demonstrates this setup.

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
- `dist/chart/`: Kubebuilder v2-alpha generated installable Helm chart
- `config/localstack/`: LocalStack deployment for the local demo
- `.github/workflows/devcontainer-ci.yml`: Dev Container CI workflow
- `terraform/`: supporting infrastructure configuration

## Contributing

Changes are welcome. Start with `make help` to see the available build, test, and deployment targets.
