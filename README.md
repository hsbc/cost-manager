# cost-manager

cost-manager is a collection of Kubernetes
[controllers](https://kubernetes.io/docs/concepts/architecture/controller/) that automate cost
reductions for the cluster they are running on.

## Controllers

Here we provide details of the various controllers and how they work.

### spot-migrator

Spot VMs are unused compute capacity that many cloud providers support access to at significantly
reduced costs (e.g. on GCP spot VMs provide a [60-91%
discount](https://cloud.google.com/compute/docs/instances/spot#pricing)). Since spot VM availability
can fluctuate it is common to configure workloads to be able to run on spot VMs but to allow
fallback to on-demand VMs if spot VMs are unavailable. However, even if spot VMs are available, if
workloads are already running on on-demand VMs there is no reason for them to migrate.

To improve spot VM utilisation, [spot-migrator](./pkg/controller/spot_migrator.go) periodically
attempts to migrate workloads from on-demand VMs to spot VMs by draining on-demand Nodes to force
cluster scale up and relying on the fact that the cluster autoscaler [attempts to expand the least
expensive possible node
group](https://github.com/kubernetes/autoscaler/blob/600cda52cf764a1f08b06fc8cc29b1ef95f13c76/cluster-autoscaler/proposals/pricing.md).
If an on-demand VM is added to the cluster then spot-migrator assumes that there are currently no
more spot VMs available and waits for the next migration attempt (currently every hour) however if
no on-demand VMs are added then spot-migrator continues to drain on-demand VMs until there are no
more on-demand VMs left in the cluster (and all workloads are running on spot VMs). Node draining
respects [Pod Disruption Budgets](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/)
to ensure that workloads are migrated whilst maintaining desired levels of availability.

Currently only [GKE
Standard](https://cloud.google.com/kubernetes-engine/docs/concepts/types-of-clusters) clusters are
supported. To allow spot-migrator to migrate workloads to spot VMs with fallback to on-demand VMs
your cluster must be running at least one on-demand node pool and at least one spot node pool.

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
