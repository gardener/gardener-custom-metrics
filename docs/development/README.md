### To debug gardener-custom-metrics (GCMx):
- Prerequisite: [Gardener local dev setup]
- Open a terminal
- Set current directory to project root
- Point $KUBECONFIG to the K8s cluster from the [Gardener local dev setup]
- Run `make debug`:
  - This is a blocking call
  - It builds and deploys a debug-instrumented pod to the cluster
  - It forwards the pod's log output to the console window
  - It forwards localhost:56268 to the debugger port for the pod
- Attach debugger to localhost:56268. At this point, if you place a breakpoint somewhere, it should be hit.

### To build and publish GCMx:
<mark>These instructions are a work in progress and may contain errors</mark>
- Open a terminal
- Set current directory to project root
- Run `make docker-build`
- Run `make docker-login`
- Run `make docker-push`

[Gardener local dev setup]: https://gardener.cloud/docs/gardener/deployment/getting_started_locally