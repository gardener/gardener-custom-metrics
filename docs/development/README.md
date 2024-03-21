> 🚧 Note: This is a WIP document.

### Debugging gardener-custom-metrics

1. Make sure that you have a running local Gardener setup. The steps to complete this can be found in the [Deploying Gardener Locally guide](https://github.com/gardener/gardener/blob/master/docs/deployment/getting_started_locally.md).

1. In a new terminal, navigate to the gardener-custom-metrics project root.

1. Make sure that your `KUBECONFIG` environment variable is targeting the local Gardener cluster.

1. Run `make debug`.

   This is a blocking call.  It builds and deploys a debug-instrumented pod to the cluster. It forwards the pod's log output to the console window. It forwards `localhost:56268` to the debugger port for the pod.

1. Attach debugger to `localhost:56268`.

   At this point, if you place a breakpoint somewhere, it should be hit.

### Building and publishing gardener-custom-metrics container image:

1. In a new terminal, navigate to the gardener-custom-metrics project root.

1. Run `make docker-build` to build container image.

1. Run `make docker-login` to authenticate against Artifact Registry before pushing the image.

1. Run `make docker-push` to push the container image.
