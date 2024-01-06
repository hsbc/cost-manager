# cost-manager

cost-manager is a collection of Kubernetes
[controllers](https://kubernetes.io/docs/concepts/architecture/controller/) that automate cost
reductions for the cluster they are running on.

## Controllers

Here we provide details of the various cost-manager controllers.

### spot-migrator

Spot VMs are unused compute capacity that many cloud providers support access to at significantly
reduced costs (e.g. on GCP spot VMs provide a [60-91%
discount](https://cloud.google.com/compute/docs/instances/spot#pricing)). Since spot VM availability
can fluctuate it is common to configure workloads to be able to run on spot VMs but to allow
fallback to on-demand VMs if spot VMs are unavailable. However, even if spot VMs are available, if
workloads are already running on on-demand VMs there is no reason for them to migrate.

To improve spot VM utilisation, [spot-migrator](./pkg/controller/spot_migrator.go) periodically
attempts to migrate workloads from on-demand VMs to spot VMs by draining on-demand Nodes to force
cluster scale up, relying on the fact that the cluster autoscaler [attempts to expand the least
expensive possible node
group](https://github.com/kubernetes/autoscaler/blob/600cda52cf764a1f08b06fc8cc29b1ef95f13c76/cluster-autoscaler/proposals/pricing.md),
taking into account the reduced cost of spot VMs. If an on-demand VM is added to the cluster then
spot-migrator assumes that there are currently no more spot VMs available and waits for the next
migration attempt (currently every hour) however if no on-demand VMs were added then spot-migrator
continues to drain on-demand VMs until there are no more left in the cluster (and all workloads are
running on spot VMs). Node draining respects
[PodDisruptionBudgets](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/) to ensure
that workloads are migrated whilst maintaining desired levels of availability.

Currently only [GKE
Standard](https://cloud.google.com/kubernetes-engine/docs/concepts/types-of-clusters) clusters are
supported. To allow spot-migrator to migrate workloads to spot VMs with fallback to on-demand VMs
your cluster must be running at least one on-demand node pool and at least one spot node pool.

```yaml
apiVersion: cost-manager.io/v1alpha1
kind: CostManagerConfiguration
controllers:
- spot-migrator
cloudProvider:
  name: gcp
```

### pod-safe-to-evict-annotator

Certain [types of
Pods](https://github.com/kubernetes/autoscaler/blob/bb72e46cb0697090683969c932a38afec9089978/cluster-autoscaler/FAQ.md#what-types-of-pods-can-prevent-ca-from-removing-a-node)
can prevent the cluster autoscaler from removing a Node (e.g. Pods in the kube-system Namespace that
do not have a PodDisruptionBudget) leading to more Nodes in the cluster than necessary. This can be
particularly problematic for workloads that cluster operators are not in control of and can have a
high number of replicas, such as kube-dns or the [Konnectivity
agent](https://kubernetes.io/docs/tasks/extend-kubernetes/setup-konnectivity/), which are typically
installed by cloud providers.

To allow the cluster autoscaler to evict all Pods that have not been explicitly marked as unsafe for
eviction, [pod-safe-to-evict-annotator](./pkg/controller/pod_safe_to_evict_annotator.go) adds the
`cluster-autoscaler.kubernetes.io/safe-to-evict: "true"` annotation to all Pods that have not
already been annotated; note that PodDisruptionBudgets can still be used to maintain desired levels
of availability.

```yaml
apiVersion: cost-manager.io/v1alpha1
kind: CostManagerConfiguration
controllers:
- pod-safe-to-evict-annotator
podSafeToEvictAnnotator:
  namespaceSelector:
    matchExpressions:
    - key: kubernetes.io/metadata.name
      operator: In
      values:
      - kube-system
```

## Installation

You can install cost-manager into a GKE cluster with [Workload
Identity](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity) enabled as
follows:

```sh
NAMESPACE="cost-manager"
kubectl get namespace "$NAMESPACE" || kubectl create namespace "$NAMESPACE"
LATEST_RELEASE_TAG="$(curl -s https://api.github.com/repos/hsbc/cost-manager/releases/latest | jq -r .tag_name)"
# GCP service account bound to the roles/compute.instanceAdmin role
GCP_SERVICE_ACCOUNT_EMAIL_ADDRESS="cost-manager@example.iam.gserviceaccount.com"
cat <<EOF > values.yaml
image:
  tag: $LATEST_RELEASE_TAG
config:
  apiVersion: cost-manager.io/v1alpha1
  kind: CostManagerConfiguration
  controllers:
  - spot-migrator
  - pod-safe-to-evict-annotator
  cloudProvider:
    name: gcp
  podSafeToEvictAnnotator:
    namespaceSelector:
      matchExpressions:
      - key: kubernetes.io/metadata.name
        operator: In
        values:
        - kube-system
serviceAccount:
  annotations:
    iam.gke.io/gcp-service-account: $GCP_SERVICE_ACCOUNT_EMAIL_ADDRESS
EOF
helm template ./charts/cost-manager -n "$NAMESPACE" -f values.yaml | kubectl apply -f -
```

## Testing

Build Docker image and run E2E tests using [kind](https://github.com/kubernetes-sigs/kind):

```sh
make image e2e
```

## Contributing

Contributions are greatly appreciated. The project follows the typical GitHub pull request model.
See [CONTRIBUTING.md](CONTRIBUTING.md) for more details.
