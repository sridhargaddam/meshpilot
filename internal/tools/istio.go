package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IstioStatus represents the status of Istio installation
type IstioStatus struct {
	Installed  bool              `json:"installed"`
	Version    string            `json:"version,omitempty"`
	Components []ComponentStatus `json:"components"`
	Namespace  string            `json:"namespace"`
	Issues     []string          `json:"issues,omitempty"`
}

// ComponentStatus represents the status of an Istio component
type ComponentStatus struct {
	Name      string `json:"name"`
	Ready     bool   `json:"ready"`
	Replicas  int32  `json:"replicas"`
	Available int32  `json:"available"`
}

// InstallIstio installs Istio on the cluster using Helm
func (m *Manager) InstallIstio(args json.RawMessage) (*CallToolResult, error) {
	var params struct {
		Namespace        string                 `json:"namespace,omitempty"`         // default: istio-system
		Version          string                 `json:"version,omitempty"`           // Istio version
		Values           map[string]interface{} `json:"values,omitempty"`            // custom helm values
		InstallGateway   bool                   `json:"install_gateway,omitempty"`   // install ingress gateway
		GatewayNamespace string                 `json:"gateway_namespace,omitempty"` // gateway namespace
		InstallCNI       bool                   `json:"install_cni,omitempty"`       // install Istio CNI node agent
		CNIValues        map[string]interface{} `json:"cni_values,omitempty"`        // custom CNI helm values
		Timeout          string                 `json:"timeout,omitempty"`           // timeout for installation
		Wait             bool                   `json:"wait,omitempty"`              // wait for deployment to be ready
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
		params.Namespace = "istio-system"
	}
	if params.GatewayNamespace == "" {
		params.GatewayNamespace = "istio-ingress"
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

	// Add Istio Helm repository
	if err := m.addIstioHelmRepo(); err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to add Istio Helm repository: %v", err),
				},
			},
		}, nil
	}

	// Install Istio CNI node agent first if requested
	if params.InstallCNI {
		if err := m.installIstioCNI(params.Namespace, params.Version, params.CNIValues, params.Wait, params.Timeout); err != nil {
			return &CallToolResult{
				IsError: true,
				Content: []interface{}{
					TextContent{
						Type: "text",
						Text: fmt.Sprintf("Failed to install Istio CNI: %v", err),
					},
				},
			}, nil
		}
	}

	// Install Istio base chart
	if err := m.installIstioBase(params.Namespace, params.Version, params.Wait, params.Timeout); err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to install Istio base chart: %v", err),
				},
			},
		}, nil
	}

	// Install Istio discovery (istiod) chart with CNI configuration if needed
	istiodValues := params.Values
	if params.InstallCNI {
		// Enable CNI in istiod to disable init container injection
		if istiodValues == nil {
			istiodValues = make(map[string]interface{})
		}
		// Set pilot.cni.enabled=true as per documentation
		if pilot, exists := istiodValues["pilot"]; exists {
			if pilotMap, ok := pilot.(map[string]interface{}); ok {
				if cni, cniExists := pilotMap["cni"]; cniExists {
					if cniMap, cniOk := cni.(map[string]interface{}); cniOk {
						cniMap["enabled"] = true
					}
				} else {
					pilotMap["cni"] = map[string]interface{}{"enabled": true}
				}
			}
		} else {
			istiodValues["pilot"] = map[string]interface{}{
				"cni": map[string]interface{}{"enabled": true},
			}
		}
	}

	if err := m.installIstiod(params.Namespace, params.Version, istiodValues, params.Wait, params.Timeout); err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to install Istio discovery chart: %v", err),
				},
			},
		}, nil
	}

	message := fmt.Sprintf("Istio successfully installed using Helm in namespace '%s'", params.Namespace)
	if params.Version != "" {
		message += fmt.Sprintf(" (version: %s)", params.Version)
	}
	if params.InstallCNI {
		message += " with CNI node agent"
	}

	// Optionally install ingress gateway
	if params.InstallGateway {
		if err := m.installIstioGateway(params.GatewayNamespace, params.Version, params.Wait, params.Timeout); err != nil {
			logrus.Warnf("Failed to install Istio gateway: %v", err)
			message += ". Warning: Gateway installation failed."
		} else {
			message += fmt.Sprintf(". Ingress gateway installed in namespace '%s'.", params.GatewayNamespace)
		}
	}

	// Verify installation
	status, err := m.getIstioStatus(params.Namespace)
	if err != nil {
		logrus.Warnf("Failed to verify Istio installation: %v", err)
	}

	if status != nil && status.Installed {
		message += " Istio control plane is ready."
	} else {
		message += " Use check_istio_status to monitor the deployment status."
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

// UninstallIstio uninstalls Istio from the cluster using Helm
func (m *Manager) UninstallIstio(args json.RawMessage) (*CallToolResult, error) {
	var params struct {
		Namespace        string `json:"namespace,omitempty"`         // default: istio-system
		GatewayNamespace string `json:"gateway_namespace,omitempty"` // gateway namespace
		UninstallCNI     bool   `json:"uninstall_cni,omitempty"`     // uninstall Istio CNI node agent
		DeleteCRDs       bool   `json:"delete_crds,omitempty"`       // delete Istio CRDs
		Wait             bool   `json:"wait,omitempty"`              // wait for uninstall to complete
		Timeout          string `json:"timeout,omitempty"`           // timeout for wait
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
		params.Namespace = "istio-system"
	}
	if params.GatewayNamespace == "" {
		params.GatewayNamespace = "istio-ingress"
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

	var messages []string

	// Uninstall gateway if it exists
	if err := m.uninstallIstioGateway(params.GatewayNamespace, params.Wait, params.Timeout); err != nil {
		logrus.Warnf("Failed to uninstall Istio gateway: %v", err)
		messages = append(messages, "Warning: Gateway uninstall failed")
	} else {
		messages = append(messages, fmt.Sprintf("Gateway uninstalled from namespace '%s'", params.GatewayNamespace))
	}

	// Uninstall Istio discovery (istiod)
	if err := m.uninstallIstiod(params.Namespace, params.Wait, params.Timeout); err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to uninstall Istio discovery: %v", err),
				},
			},
		}, nil
	}
	messages = append(messages, "Istio discovery (istiod) uninstalled")

	// Uninstall Istio base
	if err := m.uninstallIstioBase(params.Namespace, params.Wait, params.Timeout); err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to uninstall Istio base: %v", err),
				},
			},
		}, nil
	}
	messages = append(messages, "Istio base uninstalled")

	// Uninstall CNI if requested (after base to maintain proper order)
	if params.UninstallCNI {
		if err := m.uninstallIstioCNI(params.Namespace, params.Wait, params.Timeout); err != nil {
			logrus.Warnf("Failed to uninstall Istio CNI: %v", err)
			messages = append(messages, "Warning: CNI uninstall failed")
		} else {
			messages = append(messages, "Istio CNI uninstalled")
		}
	}

	// Optionally delete CRDs
	if params.DeleteCRDs {
		if err := m.deleteIstioCRDs(); err != nil {
			logrus.Warnf("Failed to delete Istio CRDs: %v", err)
			messages = append(messages, "Warning: Failed to delete Istio CRDs")
		} else {
			messages = append(messages, "Istio CRDs deleted")
		}
	}

	return &CallToolResult{
		Content: []interface{}{
			TextContent{
				Type: "text",
				Text: fmt.Sprintf("Istio successfully uninstalled using Helm. %s", strings.Join(messages, ". ")),
			},
		},
	}, nil
}

// CheckIstioStatus checks the status of Istio installation
func (m *Manager) CheckIstioStatus(args json.RawMessage) (*CallToolResult, error) {
	var params struct {
		Namespace string `json:"namespace,omitempty"` // default: istio-system
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
		params.Namespace = "istio-system"
	}

	// Get status using the helper function
	status, err := m.getIstioStatus(params.Namespace)
	if err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to get Istio status: %v", err),
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

// addIstioHelmRepo adds the Istio Helm repository
func (m *Manager) addIstioHelmRepo() error {
	// Add the repository
	cmd := exec.Command("helm", "repo", "add", "istio", "https://istio-release.storage.googleapis.com/charts")
	if output, err := cmd.CombinedOutput(); err != nil {
		// Check if repo already exists
		if !strings.Contains(string(output), "already exists") {
			return fmt.Errorf("failed to add istio helm repo: %w, output: %s", err, string(output))
		}
	}

	// Update repository
	cmd = exec.Command("helm", "repo", "update", "istio")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to update istio helm repo: %w, output: %s", err, string(output))
	}

	return nil
}

// installIstioBase installs the Istio base chart (CRDs and cluster roles)
func (m *Manager) installIstioBase(namespace, version string, wait bool, timeout string) error {
	args := []string{
		"install", "istio-base", "istio/base",
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

	cmd := exec.Command("helm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("helm install istio-base failed: %w, output: %s", err, string(output))
	}

	logrus.Infof("Istio base chart install output: %s", string(output))
	return nil
}

// installIstiod installs the Istio discovery chart (istiod)
func (m *Manager) installIstiod(namespace, version string, values map[string]interface{}, wait bool, timeout string) error {
	args := []string{
		"install", "istiod", "istio/istiod",
		"--namespace", namespace,
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
		return fmt.Errorf("helm install istiod failed: %w, output: %s", err, string(output))
	}

	logrus.Infof("Istiod chart install output: %s", string(output))
	return nil
}

// installIstioGateway installs the Istio ingress gateway
func (m *Manager) installIstioGateway(namespace, version string, wait bool, timeout string) error {
	args := []string{
		"install", "istio-ingress", "istio/gateway",
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

	cmd := exec.Command("helm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("helm install istio-ingress failed: %w, output: %s", err, string(output))
	}

	logrus.Infof("Istio gateway install output: %s", string(output))
	return nil
}

// uninstallIstioGateway uninstalls the Istio gateway
func (m *Manager) uninstallIstioGateway(namespace string, wait bool, timeout string) error {
	args := []string{
		"uninstall", "istio-ingress",
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
		// Don't fail if release doesn't exist
		if strings.Contains(string(output), "not found") {
			return nil
		}
		return fmt.Errorf("helm uninstall istio-ingress failed: %w, output: %s", err, string(output))
	}

	logrus.Infof("Istio gateway uninstall output: %s", string(output))
	return nil
}

// uninstallIstiod uninstalls the Istio discovery chart
func (m *Manager) uninstallIstiod(namespace string, wait bool, timeout string) error {
	args := []string{
		"uninstall", "istiod",
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
		return fmt.Errorf("helm uninstall istiod failed: %w, output: %s", err, string(output))
	}

	logrus.Infof("Istiod uninstall output: %s", string(output))
	return nil
}

// uninstallIstioBase uninstalls the Istio base chart
func (m *Manager) uninstallIstioBase(namespace string, wait bool, timeout string) error {
	args := []string{
		"uninstall", "istio-base",
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
		return fmt.Errorf("helm uninstall istio-base failed: %w, output: %s", err, string(output))
	}

	logrus.Infof("Istio base uninstall output: %s", string(output))
	return nil
}

// deleteIstioCRDs deletes Istio Custom Resource Definitions
func (m *Manager) deleteIstioCRDs() error {
	cmd := exec.Command("kubectl", "get", "crd", "-oname")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to get CRDs: %w", err)
	}

	// Filter Istio CRDs
	lines := strings.Split(string(output), "\n")
	var istioCRDs []string
	for _, line := range lines {
		if strings.Contains(line, "istio.io") {
			istioCRDs = append(istioCRDs, strings.TrimSpace(line))
		}
	}

	// Delete Istio CRDs
	if len(istioCRDs) > 0 {
		args := append([]string{"delete"}, istioCRDs...)
		cmd = exec.Command("kubectl", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to delete Istio CRDs: %w, output: %s", err, string(output))
		}
		logrus.Infof("Deleted Istio CRDs: %s", string(output))
	}

	return nil
}

// installIstioCNI installs the Istio CNI node agent
func (m *Manager) installIstioCNI(namespace, version string, values map[string]interface{}, wait bool, timeout string) error {
	args := []string{
		"install", "istio-cni", "istio/cni",
		"--namespace", namespace,
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
				return fmt.Errorf("failed to marshal CNI value for key %s: %w", key, err)
			}
			args = append(args, "--set-json", fmt.Sprintf("%s=%s", key, string(valueJSON)))
		}
	}

	cmd := exec.Command("helm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("helm install istio-cni failed: %w, output: %s", err, string(output))
	}

	logrus.Infof("Istio CNI install output: %s", string(output))
	return nil
}

// uninstallIstioCNI uninstalls the Istio CNI node agent
func (m *Manager) uninstallIstioCNI(namespace string, wait bool, timeout string) error {
	args := []string{
		"uninstall", "istio-cni",
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
		// Don't fail if release doesn't exist
		if strings.Contains(string(output), "not found") {
			return nil
		}
		return fmt.Errorf("helm uninstall istio-cni failed: %w, output: %s", err, string(output))
	}

	logrus.Infof("Istio CNI uninstall output: %s", string(output))
	return nil
}

// getIstioStatus gets the current status of Istio installation
func (m *Manager) getIstioStatus(namespace string) (*IstioStatus, error) {
	ctx := context.Background()

	// Check if namespace exists
	_, err := m.k8sClient.Kubernetes.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return &IstioStatus{
			Installed: false,
			Namespace: namespace,
			Issues:    []string{"Istio namespace not found"},
		}, nil
	}

	// Check Istio components (try both Helm and manual deployment labels)
	components := []string{"istiod"}
	var componentStatuses []ComponentStatus
	var issues []string
	installed := false

	// Check for CNI DaemonSet in addition to regular components
	cniDS, err := m.k8sClient.Kubernetes.AppsV1().DaemonSets(namespace).Get(ctx, "istio-cni-node", metav1.GetOptions{})
	if err == nil {
		// CNI is installed
		ready := cniDS.Status.NumberReady == cniDS.Status.DesiredNumberScheduled && cniDS.Status.DesiredNumberScheduled > 0
		componentStatuses = append(componentStatuses, ComponentStatus{
			Name:      "istio-cni-node",
			Ready:     ready,
			Replicas:  cniDS.Status.DesiredNumberScheduled,
			Available: cniDS.Status.NumberReady,
		})
		if !ready {
			issues = append(issues, "istio-cni-node is not ready")
		}
		installed = true
	}

	for _, componentName := range components {
		// Try to find deployment with Helm labels first
		deployments, err := m.k8sClient.Kubernetes.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app=%s", componentName),
		})
		if err != nil {
			issues = append(issues, fmt.Sprintf("Failed to list %s deployments: %v", componentName, err))
			continue
		}

		if len(deployments.Items) == 0 {
			componentStatuses = append(componentStatuses, ComponentStatus{
				Name:  componentName,
				Ready: false,
			})
			continue
		}

		// Use the first deployment found
		deployment := deployments.Items[0]
		installed = true
		ready := deployment.Status.ReadyReplicas == deployment.Status.Replicas && deployment.Status.Replicas > 0
		componentStatuses = append(componentStatuses, ComponentStatus{
			Name:      componentName,
			Ready:     ready,
			Replicas:  deployment.Status.Replicas,
			Available: deployment.Status.AvailableReplicas,
		})

		if !ready {
			issues = append(issues, fmt.Sprintf("%s is not ready", componentName))
		}
	}

	// Try to get Helm release version
	version := "unknown"
	if helmVersion, err := m.getIstioHelmReleaseVersion(namespace, "istiod"); err == nil {
		version = helmVersion
	}

	return &IstioStatus{
		Installed:  installed,
		Version:    version,
		Components: componentStatuses,
		Namespace:  namespace,
		Issues:     issues,
	}, nil
}

// getIstioHelmReleaseVersion gets the version of a Helm release
func (m *Manager) getIstioHelmReleaseVersion(namespace, releaseName string) (string, error) {
	cmd := exec.Command("helm", "list", "--namespace", namespace, "--filter", releaseName, "--output", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get helm release info: %w", err)
	}

	var releases []struct {
		Name       string `json:"name"`
		Namespace  string `json:"namespace"`
		Revision   string `json:"revision"`
		Updated    string `json:"updated"`
		Status     string `json:"status"`
		Chart      string `json:"chart"`
		AppVersion string `json:"app_version"`
	}

	if err := json.Unmarshal(output, &releases); err != nil {
		return "", fmt.Errorf("failed to parse helm release info: %w", err)
	}

	if len(releases) == 0 {
		return "", fmt.Errorf("release %s not found", releaseName)
	}

	// Return chart version or app version
	if releases[0].Chart != "" {
		return releases[0].Chart, nil
	}
	if releases[0].AppVersion != "" {
		return releases[0].AppVersion, nil
	}

	return "unknown", nil
}
