# End-to-end tests

## With [kind](https://kind.sigs.k8s.io/)

[`kind`](https://kind.sigs.k8s.io/) and `kubectl` are automatically downloaded
except if `SKIP_INSTALL=true` is set.
A `kind` cluster is automatically created before the tests, and deleted after
the tests.
The `prometheus-adapter` container image is build locally and imported
into the cluster.

```bash
KIND_E2E=true make test-e2e
```

## With an existing Kubernetes cluster

If you already have a Kubernetes cluster, you can use:

```bash
KUBECONFIG="/path/to/kube/config" REGISTRY="my.registry/prefix" make test-e2e
```

- The cluster should not have a namespace `prometheus-adapter-e2e`.
  The namespace will be created and deleted as part of the E2E tests.
- `KUBECONFIG` is the path of the [`kubeconfig` file].
  **Optional**, defaults to `${HOME}/.kube/config`
- `REGISTRY` is the image registry where the container image should be pushed.
  **Required**.

[`kubeconfig` file]: https://kubernetes.io/docs/concepts/configuration/organize-cluster-access-kubeconfig/

## Additional environment variables

These environment variables may also be used (with any non-empty value):

- `SKIP_INSTALL`: skip the installation of `kind` and `kubectl` binaries;
- `SKIP_CLEAN_AFTER`: skip the deletion of resources (`Kind` cluster or
  Kubernetes namespace) and of the temporary directory `.e2e`;
- `CLEAN_BEFORE`: clean before running the tests, e.g. if `SKIP_CLEAN_AFTER`
  was used on the previous run.
