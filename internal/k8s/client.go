package k8s

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"github.com/sirupsen/logrus"
	istioclient "istio.io/client-go/pkg/clientset/versioned"
)

// Client wraps Kubernetes and Istio clients
type Client struct {
	Kubernetes kubernetes.Interface
	Istio      istioclient.Interface
	Config     *rest.Config
	Context    context.Context
}

// NewClient creates a new Kubernetes client
func NewClient() (*Client, error) {
	config, err := getKubeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	// Create Kubernetes client
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Create Istio client
	istioClient, err := istioclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Istio client: %w", err)
	}

	return &Client{
		Kubernetes: kubeClient,
		Istio:      istioClient,
		Config:     config,
		Context:    context.Background(),
	}, nil
}

// getKubeConfig returns the Kubernetes configuration
func getKubeConfig() (*rest.Config, error) {
	// Try in-cluster config first
	if config, err := rest.InClusterConfig(); err == nil {
		logrus.Info("Using in-cluster Kubernetes configuration")
		return config, nil
	}

	// Fall back to kubeconfig file
	var kubeconfig string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	// Override with KUBECONFIG environment variable if set
	if kubeconfigEnv := os.Getenv("KUBECONFIG"); kubeconfigEnv != "" {
		kubeconfig = kubeconfigEnv
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build config from kubeconfig: %w", err)
	}

	logrus.Infof("Using kubeconfig: %s", kubeconfig)
	return config, nil
}

// GetCurrentContext returns the current Kubernetes context
func (c *Client) GetCurrentContext() (string, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.RawConfig()
	if err != nil {
		return "", err
	}

	return config.CurrentContext, nil
}

// SwitchContext switches to a different Kubernetes context
func (c *Client) SwitchContext(contextName string) error {
	// This would typically involve updating the kubeconfig and recreating clients
	// For now, we'll just log the action
	logrus.Infof("Switching to context: %s", contextName)
	return nil
}
