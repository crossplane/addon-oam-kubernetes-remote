# addon-oam-kubernetes-remote

The OAM Kubernetes Remote Addon allows for scheduling the resources created by the `ApplicationConfiguration` controller to a remote Kubernetes cluster.

## Examples

The functionality of this addon can be demonstrated with the following steps:

1. Install Crossplane and `provider-gcp`

Directions for this step can be found in the [Crossplane docs](https://crossplane.io/docs/master/install-crossplane.html).

2. Add GCP provider credentials

Directions for this step can be found in the [Crossplane docs](https://crossplane.io/docs/master/cloud-providers/gcp/gcp-provider.html).

3. Create `GKEClusterClass`

```
kubectl apply -f examples/gkeclusterclass.yaml
```

4. Create `KubernetesCluster` claim

```
kubectl apply -f examples/k8scluster.yaml
```

5. Once the `KubernetesCluster` becomes bound, create the `ApplicationConfiguration` and its corresponding resources

```
kubectl apply -f examples/wordpress/app.yaml
```

6. View created `KubernetesApplication` and `KubernetesApplicationResources`

```
kubectl get kubernetesapplications
kubectl get kubernetesapplicationresources
```

7. View resources in remote cluster

```
kubectl get secret k8scluster --template={{.data.kubeconfig}} | base64 --decode > remote.kubeconfig
kubectl --kubeconfig=remote.kubeconfig get deployments
kubectl --kubeconfig=remote.kubeconfig get services
```
