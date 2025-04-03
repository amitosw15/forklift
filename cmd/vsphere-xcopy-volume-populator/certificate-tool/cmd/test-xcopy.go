// cmd/create_test.go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	testEnvKubeconfig string
	testEnvNamespace  string
	pvcYamlPath       string
	crYamlPath        string
)

var createTestCmd = &cobra.Command{
	Use:   "test-xcopy",
	Short: "Creates the test environment: namespace, PVC, and CR instance",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Creating test environment...")
		config, err := clientcmd.BuildConfigFromFlags("", testEnvKubeconfig)
		if err != nil {
			panic(err)
		}
		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			panic(err)
		}

		// Ensure the test namespace exists.
		if err := EnsureNamespace(clientset, testEnvNamespace); err != nil {
			panic(err)
		}

		// Process and apply PVC YAML.
		_, err = ProcessTemplate(pvcYamlPath, nil, "${", "}")
		if err != nil {
			panic(err)
		}
		// You can decode and create the PVC resource similar to the Deployment above.
		fmt.Println("Applying PVC YAML from", pvcYamlPath)
		// (Implement your PVC creation logic here.)

		// Process and apply the CR instance YAML.
		_, err = ProcessTemplate(crYamlPath, nil, "${", "}")
		if err != nil {
			panic(err)
		}
		fmt.Println("Applying CR instance YAML from", crYamlPath)
		// (Implement your CR instance creation logic here.)

		fmt.Println("Test environment created successfully.")
	},
}

func init() {
	rootCmd.AddCommand(createTestCmd)
	createTestCmd.Flags().StringVar(&testEnvKubeconfig, "kubeconfig", "/home/amit/.kube/config", "Path to the kubeconfig file")
	createTestCmd.Flags().StringVar(&testEnvNamespace, "test-namespace", "test-environment", "Namespace for the test environment")
	createTestCmd.Flags().StringVar(&pvcYamlPath, "pvc-yaml", "pvc.yaml", "Path to the PVC YAML file")
	createTestCmd.Flags().StringVar(&crYamlPath, "cr-yaml", "cr.yaml", "Path to the CR instance YAML file")
}
