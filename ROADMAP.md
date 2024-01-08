# Roadmap

There is currently not a well-defined roadmap for cost-manager, however here we describe some
Kubernetes cost optimisations that could be automated as cost-manager controllers in the future:

- Generating VerticalPodAutoscalers: The [Vertical Pod Autoscaler
  (VPA)](https://github.com/kubernetes/autoscaler/tree/master/vertical-pod-autoscaler) automatically
  adjusts Kubernetes Pod resource requests and limits based on actual usage can help reduce
  over-provisioning. By automatically generating VPA resources for all cluster workloads (being
  careful to consider the [limitations](https://github.com/kubernetes/autoscaler/issues/6247) when
  using VPA together with HPA) operators can make sure that workloads are only requesting the
  resources that they need
- Garbage collecting disks and load balancing infrastructure: PersistentVolumeClaims and Services
  can be used to automatically provision of cloud infrastructure, however if the cluster is deleted
  without first deleting these resources then the cloud resources can become orphaned. By using the
  description on these resources, a controller can be used to automatically detect orphaned cloud
  resources provisioned through Kubernetes and clean them up to save costs
- Scheduled cluster scale down: There are many cases where cluster workloads do not need to be
  running all the time (e.g. CI infrastructure or development clusters). On a schedule,
  [ResourceQuotas](https://kubernetes.io/docs/concepts/policy/resource-quotas/) could be used to
  limit Pods in all Namespaces (except for the cost-manager Namespace) and then all Pods deleted to
  allow the cluster to scale down. To scale back up, the ResourceQuotas can simply be deleted.
