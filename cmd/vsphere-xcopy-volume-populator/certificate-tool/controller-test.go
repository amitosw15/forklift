package main

import (
	"bytes"
	"context"
	"fmt"
	rbacv1 "k8s.io/api/rbac/v1"
	"os"
	"text/template"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1" // Import the core/v1 package for Namespace
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	// Load kubeconfig
	kubeconfigPath := "/home/amit/.kube/config"
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		panic(err)
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	// Load and parse YAML with variables
	rawYaml, err := os.ReadFile("vsphere-populator.yaml")
	if err != nil {
		panic(err)
	}

	// Replace env-style placeholders
	tmpl, err := template.New("populator").Delims("${", "}").Parse(string(rawYaml))
	if err != nil {
		panic(err)
	}

	var buf bytes.Buffer
	vars := map[string]string{
		"TEST_NAMESPACE":       "vsphere-populator-test",
		"TEST_IMAGE_LABEL":     "devel",
		"TEST_LABELS":          "vsphere-populator",
		"TEST_POPULATOR_IMAGE": "quay.io/rgolangh/vsphere-xcopy-volume-populator",
	}

	if err := p.Execute(&buf, vars); err != nil {
		panic(err)
	}

	// Decode YAML to Deployment object
	decoder := yaml.NewYAMLOrJSONDecoder(&buf, 1024)
	var deploy appsv1.Deployment
	if err := decoder.Decode(&deploy); err != nil {
		panic(err)
	}

	// Create namespace (if not exists)
	_, err = clientset.CoreV1().Namespaces().Get(context.TODO(), vars["TEST_NAMESPACE"], metav1.GetOptions{})
	if err != nil {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: vars["TEST_NAMESPACE"],
			},
		}
		_, err = clientset.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
		if err != nil {
			panic(err)
		}
		fmt.Println("Created namespace:", vars["TEST_NAMESPACE"])
	}
	defer func() {
		fmt.Printf("Deleting namespace")
		if err := clientset.CoreV1().Namespaces().Delete(context.TODO(), vars["TEST_NAMESPACE"], metav1.DeleteOptions{}); err != nil {
			fmt.Fprintf(os.Stderr, "failed to delete namespace: %v\n", err)
			os.Exit(1) // or handle the error as appropriate
		}
		os.Exit(0)
	}()
	//create sa
	_, err = clientset.CoreV1().ServiceAccounts(vars["TEST_NAMESPACE"]).Get(context.TODO(), "forklift-populator-controller", metav1.GetOptions{})
	if err != nil {
		sa := &corev1.ServiceAccount{ // Use corev1.Namespace instead of metav1.Namespace
			ObjectMeta: metav1.ObjectMeta{
				Name:      "forklift-populator-controller",
				Namespace: vars["TEST_NAMESPACE"],
			},
		}

		_, err = clientset.CoreV1().ServiceAccounts(vars["TEST_NAMESPACE"]).Create(context.TODO(), sa, metav1.CreateOptions{})
		if err != nil {
			panic(err)
		}
		fmt.Println("Created service account: forklift-populator-controller")
	}
	// Define the ClusterRole object
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "forklift-populator-controller-role",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods", "persistentvolumeclaims", "persistentvolumes", "storageclasses", "secrets"},
				Verbs:     []string{"get", "list", "watch", "patch", "create", "update", "delete"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs:     []string{"create", "patch", "update"},
			},
			{
				APIGroups: []string{"storage.k8s.io"},
				Resources: []string{"storageclasses"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"forklift.konveyor.io"},
				Resources: []string{"ovirtvolumepopulators", "vspherexcopyvolumepopulators", "openstackvolumepopulators"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
		},
	}

	// Check if the ClusterRole already exists
	_, err = clientset.RbacV1().ClusterRoles().Get(context.TODO(), clusterRole.Name, metav1.GetOptions{})
	if err != nil {
		// If not found, create it
		createdCR, err := clientset.RbacV1().ClusterRoles().Create(context.TODO(), clusterRole, metav1.CreateOptions{})
		if err != nil {
			panic(fmt.Sprintf("Failed to create ClusterRole: %v", err))
		}
		fmt.Printf("ClusterRole %q created.\n", createdCR.Name)
	} else {
		fmt.Printf("ClusterRole %q already exists.\n", clusterRole.Name)
	}

	// Define the ClusterRoleBinding object
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "forklift-populator-controller-binding",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "forklift-populator-controller",
				Namespace: vars["TEST_NAMESPACE"], // Change this if your SA is in a different namespace
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "forklift-populator-controller-role",
		},
	}

	// Check if the ClusterRoleBinding already exists
	_, err = clientset.RbacV1().ClusterRoleBindings().Get(context.TODO(), clusterRoleBinding.Name, metav1.GetOptions{})
	if err != nil {
		createdCRB, err := clientset.RbacV1().ClusterRoleBindings().Create(context.TODO(), clusterRoleBinding, metav1.CreateOptions{})
		if err != nil {
			panic(fmt.Sprintf("Failed to create ClusterRoleBinding: %v", err))
		}
		fmt.Printf("ClusterRoleBinding %q created.\n", createdCRB.Name)
	} else {
		fmt.Printf("ClusterRoleBinding %q already exists.\n", clusterRoleBinding.Name)
	}
	// Deploy
	result, err := clientset.AppsV1().Deployments(vars["TEST_NAMESPACE"]).Create(context.TODO(), &deploy, metav1.CreateOptions{})
	if err != nil {
		panic(err)
	}

	fmt.Println("Deployment created:", result.Name)
}
