# cost-manager

cost-manager is a collection of Kubernetes controllers that helps to reduce the costs of the
Kubernetes cluster it is running on. Currently it is specific to GKE:

- [spot-migrator](./pkg/controller/spot_migrator.go): Periodically attempts to migrate workloads
  from on-demand VMs to [spot VMs](https://cloud.google.com/compute/docs/instances/spot). It does
  this by draining on-demand Nodes to force cluster scale up and relying on the fact that the
  cluster autoscaler [attempts to expand the least expensive possible node
  pool](https://cloud.google.com/kubernetes-engine/docs/concepts/cluster-autoscaler#operating_criteria)

## Quickstart

cost-manager can be run locally:

```sh
# Generate Application Default Credentials with the roles/compute.instanceAdmin role
# Generate kubeconfig for the target Kubernetes cluster
make run
```

Alternatively, you can run cost-manager within a Kubernetes cluster:

```sh
# Build the Docker image
make image
REPOSITORY=""
docker tag cost-manager "$REPOSITORY"
docker push "$REPOSITORY"
# GCP service account bound to the roles/compute.instanceAdmin role
GCP_SERVICE_ACCOUNT_EMAIL_ADDRESS="cost-manager@example.iam.gserviceaccount.com"
kubectl create namespace cost-manager --dry-run=client -o yaml | kubectl apply -f
helm template ./charts/cost-manager \
    -n cost-manager \
    --set image.repository="$REPOSITORY" \
    --set iam.gcpServiceAccount="$GCP_SERVICE_ACCOUNT_EMAIL_ADDRESS" \
    --set vpa.enabled=true | kubectl apply -f -
```

## Contributing

Contributions are greatly appreciated. The project follows the typical GitHub pull request model.
See [CONTRIBUTING.md](CONTRIBUTING.md) for more details.
