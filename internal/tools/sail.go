package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SailStatus represents the status of Sail operator installation
type SailStatus struct {
	Installed bool     `json:"installed"`
	Version   string   `json:"version,omitempty"`
	Namespace string   `json:"namespace"`
	Ready     bool     `json:"ready"`
	Replicas  int32    `json:"replicas"`
	Available int32    `json:"available"`
	Issues    []string `json:"issues,omitempty"`
}

// InstallSailOperator installs the Sail operator using Helm
func (m *Manager) InstallSailOperator(args json.RawMessage) (*CallToolResult, error) {
	var params struct {
		Namespace   string                 `json:"namespace,omitempty"`    // default: sail-operator
		Version     string                 `json:"version,omitempty"`      // default: latest
		ReleaseName string                 `json:"release_name,omitempty"` // default: sail-operator
		Values      map[string]interface{} `json:"values,omitempty"`       // custom helm values
		Wait        bool                   `json:"wait,omitempty"`         // wait for deployment to be ready
		Timeout     string                 `json:"timeout,omitempty"`      // timeout for wait (default: 5m)
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

	// Set defaults
	if params.Namespace == "" {
		params.Namespace = "sail-operator"
	}
	if params.ReleaseName == "" {
		params.ReleaseName = "sail-operator"
	}
	if params.Timeout == "" {
		params.Timeout = "5m"
	}
	params.Wait = true // Always wait for deployment to be ready

	// Check if Helm is available
	if err := m.checkHelmAvailable(); err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Helm is not available: %v. Please install Helm to use this feature.", err),
				},
			},
		}, nil
	}

	// Add Helm repository
	if err := m.addSailOperatorHelmRepo(); err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to add Sail operator Helm repository: %v", err),
				},
			},
		}, nil
	}

	// Install using Helm
	if err := m.installSailOperatorWithHelm(params.Namespace, params.ReleaseName, params.Version, params.Values, params.Wait, params.Timeout); err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to install Sail operator with Helm: %v", err),
				},
			},
		}, nil
	}

	// Verify installation
	status, err := m.getSailOperatorStatus(params.Namespace)
	if err != nil {
		logrus.Warnf("Failed to verify Sail operator installation: %v", err)
	}

	message := fmt.Sprintf("Sail operator successfully installed using Helm in namespace '%s' with release name '%s'", params.Namespace, params.ReleaseName)
	if params.Version != "" {
		message += fmt.Sprintf(" (version: %s)", params.Version)
	}

	if status != nil && status.Ready {
		message += ". Operator is ready and running."
	} else {
		message += ". Use check_sail_status to monitor the deployment status."
	}

	return &CallToolResult{
		Content: []interface{}{
			TextContent{
				Type: "text",
				Text: message,
			},
		},
	}, nil
}

// UninstallSailOperator uninstalls the Sail operator using Helm
func (m *Manager) UninstallSailOperator(args json.RawMessage) (*CallToolResult, error) {
	var params struct {
		Namespace   string `json:"namespace,omitempty"`    // default: sail-operator
		ReleaseName string `json:"release_name,omitempty"` // default: sail-operator
		Wait        bool   `json:"wait,omitempty"`         // wait for uninstall to complete
		Timeout     string `json:"timeout,omitempty"`      // timeout for wait (default: 5m)
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

	// Set defaults
	if params.Namespace == "" {
		params.Namespace = "sail-operator"
	}
	if params.ReleaseName == "" {
		params.ReleaseName = "sail-operator"
	}
	if params.Timeout == "" {
		params.Timeout = "5m"
	}
	params.Wait = true // Always wait for uninstall to complete

	// Check if Helm is available
	if err := m.checkHelmAvailable(); err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Helm is not available: %v. Please install Helm to use this feature.", err),
				},
			},
		}, nil
	}

	// Uninstall using Helm
	if err := m.uninstallSailOperatorWithHelm(params.Namespace, params.ReleaseName, params.Wait, params.Timeout); err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to uninstall Sail operator with Helm: %v", err),
				},
			},
		}, nil
	}

	return &CallToolResult{
		Content: []interface{}{
			TextContent{
				Type: "text",
				Text: fmt.Sprintf("Sail operator successfully uninstalled from namespace '%s' (release: %s)", params.Namespace, params.ReleaseName),
			},
		},
	}, nil
}

// CheckSailStatus checks the status of Sail operator installation
func (m *Manager) CheckSailStatus(args json.RawMessage) (*CallToolResult, error) {
	var params struct {
		Namespace string `json:"namespace,omitempty"` // default: sail-operator
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

	if params.Namespace == "" {
		params.Namespace = "sail-operator"
	}

	// Get status using the helper function
	status, err := m.getSailOperatorStatus(params.Namespace)
	if err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to get Sail operator status: %v", err),
				},
			},
		}, nil
	}

	result, _ := json.MarshalIndent(status, "", "  ")
	return &CallToolResult{
		Content: []interface{}{
			TextContent{
				Type: "text",
				Text: string(result),
			},
		},
	}, nil
}

// checkHelmAvailable checks if Helm is available in the system
func (m *Manager) checkHelmAvailable() error {
	cmd := exec.Command("helm", "version", "--short")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm command not found or not working: %w", err)
	}
	return nil
}

// addSailOperatorHelmRepo adds the Sail operator Helm repository
func (m *Manager) addSailOperatorHelmRepo() error {
	// Add the repository
	cmd := exec.Command("helm", "repo", "add", "sail-operator", "https://istio-ecosystem.github.io/sail-operator")
	if output, err := cmd.CombinedOutput(); err != nil {
		// Check if repo already exists
		if !strings.Contains(string(output), "already exists") {
			return fmt.Errorf("failed to add sail-operator helm repo: %w, output: %s", err, string(output))
		}
	}

	// Update repository
	cmd = exec.Command("helm", "repo", "update", "sail-operator")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to update sail-operator helm repo: %w, output: %s", err, string(output))
	}

	return nil
}

// installSailOperatorWithHelm installs Sail operator using Helm
func (m *Manager) installSailOperatorWithHelm(namespace, releaseName, version string, values map[string]interface{}, wait bool, timeout string) error {
	args := []string{
		"install", releaseName, "sail-operator/sail-operator",
		"--namespace", namespace,
		"--create-namespace",
	}

	// Add version if specified
	if version != "" {
		args = append(args, "--version", version)
	}

	// Add wait flag
	if wait {
		args = append(args, "--wait")
		if timeout != "" {
			args = append(args, "--timeout", timeout)
		}
	}

	// Add custom values if provided
	if len(values) > 0 {
		// Convert values to JSON and use --set-json
		for key, value := range values {
			valueJSON, err := json.Marshal(value)
			if err != nil {
				return fmt.Errorf("failed to marshal value for key %s: %w", key, err)
			}
			args = append(args, "--set-json", fmt.Sprintf("%s=%s", key, string(valueJSON)))
		}
	}

	cmd := exec.Command("helm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("helm install failed: %w, output: %s", err, string(output))
	}

	logrus.Infof("Helm install output: %s", string(output))
	return nil
}

// uninstallSailOperatorWithHelm uninstalls Sail operator using Helm
func (m *Manager) uninstallSailOperatorWithHelm(namespace, releaseName string, wait bool, timeout string) error {
	args := []string{
		"uninstall", releaseName,
		"--namespace", namespace,
	}

	// Add wait flag
	if wait {
		args = append(args, "--wait")
		if timeout != "" {
			args = append(args, "--timeout", timeout)
		}
	}

	cmd := exec.Command("helm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("helm uninstall failed: %w, output: %s", err, string(output))
	}

	logrus.Infof("Helm uninstall output: %s", string(output))
	return nil
}

// getSailOperatorStatus gets the current status of Sail operator
func (m *Manager) getSailOperatorStatus(namespace string) (*SailStatus, error) {
	ctx := context.Background()

	// Try to find the deployment (it might have a different name based on Helm chart)
	var deployments *appsv1.DeploymentList
	deployments, err := m.k8sClient.Kubernetes.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/component=sail-operator",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	if len(deployments.Items) == 0 {
		// Try alternative label selectors for backward compatibility
		deployments, err = m.k8sClient.Kubernetes.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=sail-operator",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list deployments with second selector: %w", err)
		}
	}

	if len(deployments.Items) == 0 {
		// Try the old manual deployment label selector for backward compatibility
		deployments, err = m.k8sClient.Kubernetes.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=sail-operator",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list deployments with fallback selector: %w", err)
		}
	}

	if len(deployments.Items) == 0 {
		return &SailStatus{
			Installed: false,
			Namespace: namespace,
			Issues:    []string{"Sail operator deployment not found"},
		}, nil
	}

	deployment := deployments.Items[0]
	var issues []string
	ready := deployment.Status.ReadyReplicas == deployment.Status.Replicas && deployment.Status.Replicas > 0

	if !ready {
		issues = append(issues, "Sail operator is not ready")
	}

	// Extract version from image tag if possible
	version := "unknown"
	if len(deployment.Spec.Template.Spec.Containers) > 0 {
		image := deployment.Spec.Template.Spec.Containers[0].Image
		if strings.Contains(image, ":") {
			parts := strings.Split(image, ":")
			if len(parts) > 1 {
				version = parts[len(parts)-1]
			}
		}
	}

	// Try to get Helm release information for better version info
	if helmVersion, err := m.getIstioHelmReleaseVersion(namespace, "sail-operator"); err == nil {
		version = helmVersion
	}

	return &SailStatus{
		Installed: true,
		Version:   version,
		Namespace: namespace,
		Ready:     ready,
		Replicas:  deployment.Status.Replicas,
		Available: deployment.Status.AvailableReplicas,
		Issues:    issues,
	}, nil
}
