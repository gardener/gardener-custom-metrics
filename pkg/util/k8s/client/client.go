// Package client provides K8s client utilities.
package client

import (
	"os"

	kclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/gardener/gardener-custom-metrics/pkg/util/errutil"
)

// GetClientSet returns a [kubernetes.Clientset] from the given kubeconfig path.
// If kubeconfigPath is nil, the function assumes that the process is running in a K8s pod, and attempts to create
// client set based on the convention for in-cluster configuration.
func GetClientSet(kubeconfigPath string) (*kclient.Clientset, error) {
	if kubeconfigPath == "" {
		errorPrefix := "creating client set based on in-cluster configuration"
		// In-cluster mode
		config, err := rest.InClusterConfig()
		if err != nil {
			return nil, errutil.Wrap(errorPrefix, err)
		}

		clientSet, err := kclient.NewForConfig(config)
		return clientSet, errutil.Wrap(errorPrefix, err)
	}

	// Load Kubernetes config
	kubeconfigRaw, err := os.ReadFile(kubeconfigPath) //nolint:gosec
	if err != nil {
		return nil, errutil.Wrap("opening kubeconfig file '%s'", err, kubeconfigPath)
	}
	config, err := clientcmd.Load(kubeconfigRaw)
	if err != nil {
		return nil, errutil.Wrap("loading kubeconfig file '%s'", err, kubeconfigPath)
	}

	// Create client config
	clientConfig, err := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, errutil.Wrap("creating client config from file '%s'", err, kubeconfigPath)
	}

	// Create client set
	clientSet, err := kclient.NewForConfig(clientConfig)
	return clientSet, errutil.Wrap("creating client set from file '%s'", err, kubeconfigPath)
}
