package gardener

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var _ = Describe("RESTOptions", func() {
	const (
		customKubeconfig = "my-kubeconfig"
		envKubeconfig    = "env-kubeconfig"
		kapiUrl          = "my-apiserver"
		inCluster        = "in-cluster-apiserver"
	)

	type testOutput struct {
		ForwardedMasterUrl  string
		ForwardedKubeconfig string
	}

	var newRestOptions = func() (*RESTOptions, *testOutput) {
		var output testOutput
		options := NewRESTOptions()
		options.testIsolation = testIsolation{
			K8sBuildConfigFromFlags: func(masterUrl, kubeconfigPath string) (*rest.Config, error) {
				output.ForwardedMasterUrl = masterUrl
				output.ForwardedKubeconfig = kubeconfigPath
				return nil, nil
			},
			K8sInClusterConfig: func() (*rest.Config, error) {
				output.ForwardedMasterUrl = inCluster
				output.ForwardedKubeconfig = inCluster
				return nil, nil
			},
			OsGetenv: func(_ string) string {
				return envKubeconfig
			},
		}

		return options, &output
	}

	Describe("options completion", func() {
		It("should use the specified master URL and kubeconfig", func() {
			// Arrange
			options, output := newRestOptions()
			options.Kubeconfig = customKubeconfig
			options.MasterURL = kapiUrl

			// Act
			err := options.Complete()

			// Assert
			Expect(err).To(Succeed())
			Expect(options.Completed().Config).To(BeNil())
			Expect(output.ForwardedKubeconfig).To(Equal(customKubeconfig))
			Expect(output.ForwardedMasterUrl).To(Equal(kapiUrl))
		})

		It("should use the specified master URL and fall back to kubeconfig from environment variable", func() {
			// Arrange
			options, output := newRestOptions()
			options.MasterURL = kapiUrl

			// Act
			err := options.Complete()

			// Assert
			Expect(err).To(Succeed())
			Expect(options.Completed().Config).To(BeNil())
			Expect(output.ForwardedKubeconfig).To(Equal(envKubeconfig))
			Expect(output.ForwardedMasterUrl).To(Equal(kapiUrl))
		})

		It("should fall back to in-cluster config", func() {
			// Arrange
			options, output := newRestOptions()
			options.MasterURL = kapiUrl
			options.testIsolation.OsGetenv = func(_ string) string {
				return ""
			}

			// Act
			err := options.Complete()

			// Assert
			Expect(err).To(Succeed())
			Expect(options.Completed().Config).To(BeNil())
			Expect(output.ForwardedMasterUrl).To(Equal(inCluster))
			Expect(output.ForwardedKubeconfig).To(Equal(inCluster))
		})

		It("should fall back to default kubeconfig location", func() {
			// Arrange
			options, output := newRestOptions()
			options.MasterURL = kapiUrl
			options.testIsolation.OsGetenv = func(_ string) string {
				return ""
			}
			options.testIsolation.K8sInClusterConfig = func() (*rest.Config, error) {
				return nil, fmt.Errorf("my error")
			}

			// Act
			err := options.Complete()

			// Assert
			Expect(err).To(Succeed())
			Expect(options.Completed().Config).To(BeNil())
			Expect(output.ForwardedKubeconfig).To(Equal(clientcmd.RecommendedHomeFile))
		})
	})
})
