package tools

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

// ClusterInfo represents cluster information
type ClusterInfo struct {
	Name       string            `json:"name"`
	Server     string            `json:"server"`
	Version    string            `json:"version"`
	Nodes      int               `json:"nodes"`
	Namespaces []string          `json:"namespaces"`
	Context    string            `json:"context"`
	Labels     map[string]string `json:"labels,omitempty"`
}

// ContextInfo represents a Kubernetes context
type ContextInfo struct {
	Name      string `json:"name"`
	Cluster   string `json:"cluster"`
	User      string `json:"user"`
	Namespace string `json:"namespace"`
	Current   bool   `json:"current"`
}

// ListContexts lists available Kubernetes contexts
func (m *Manager) ListContexts(args json.RawMessage) (*CallToolResult, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.RawConfig()
	if err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to load kubeconfig: %v", err),
				},
			},
		}, nil
	}

	var contexts []ContextInfo
	for name, context := range config.Contexts {
		contexts = append(contexts, ContextInfo{
			Name:      name,
			Cluster:   context.Cluster,
			User:      context.AuthInfo,
			Namespace: context.Namespace,
			Current:   name == config.CurrentContext,
		})
	}

	result, _ := json.MarshalIndent(contexts, "", "  ")
	return &CallToolResult{
		Content: []interface{}{
			TextContent{
				Type: "text",
				Text: string(result),
			},
		},
	}, nil
}

// SwitchContext switches to a different Kubernetes context
func (m *Manager) SwitchContext(args json.RawMessage) (*CallToolResult, error) {
	var params struct {
		Context string `json:"context"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Invalid parameters: %v", err),
				},
			},
		}, nil
	}

	if params.Context == "" {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: "Context name is required",
				},
			},
		}, nil
	}

	// Switch context using clientcmd
	pathOptions := clientcmd.NewDefaultPathOptions()
	config, err := pathOptions.GetStartingConfig()
	if err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to get kubeconfig: %v", err),
				},
			},
		}, nil
	}

	if _, exists := config.Contexts[params.Context]; !exists {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Context '%s' does not exist", params.Context),
				},
			},
		}, nil
	}

	config.CurrentContext = params.Context
	if err := clientcmd.ModifyConfig(pathOptions, *config, true); err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to switch context: %v", err),
				},
			},
		}, nil
	}

	return &CallToolResult{
		Content: []interface{}{
			TextContent{
				Type: "text",
				Text: fmt.Sprintf("Successfully switched to context: %s", params.Context),
			},
		},
	}, nil
}

// GetClusterInfo gets information about the current cluster
func (m *Manager) GetClusterInfo(args json.RawMessage) (*CallToolResult, error) {
	ctx := context.Background()

	// Get server version
	version, err := m.k8sClient.Kubernetes.Discovery().ServerVersion()
	if err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to get server version: %v", err),
				},
			},
		}, nil
	}

	// Get nodes
	nodes, err := m.k8sClient.Kubernetes.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to get nodes: %v", err),
				},
			},
		}, nil
	}

	// Get namespaces
	namespaces, err := m.k8sClient.Kubernetes.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to get namespaces: %v", err),
				},
			},
		}, nil
	}

	// Get current context
	currentContext, err := m.k8sClient.GetCurrentContext()
	if err != nil {
		currentContext = "unknown"
	}

	var nsNames []string
	for _, ns := range namespaces.Items {
		nsNames = append(nsNames, ns.Name)
	}

	clusterInfo := ClusterInfo{
		Name:       currentContext,
		Server:     m.k8sClient.Config.Host,
		Version:    version.GitVersion,
		Nodes:      len(nodes.Items),
		Namespaces: nsNames,
		Context:    currentContext,
	}

	result, _ := json.MarshalIndent(clusterInfo, "", "  ")
	return &CallToolResult{
		Content: []interface{}{
			TextContent{
				Type: "text",
				Text: string(result),
			},
		},
	}, nil
}
