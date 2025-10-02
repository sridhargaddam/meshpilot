package tools

import (
	"encoding/json"
	"fmt"
	"meshpilot/internal/k8s"
)

// Manager handles all tool operations
type Manager struct {
	k8sClient *k8s.Client
}

// NewManager creates a new tool manager
func NewManager(k8sClient *k8s.Client) *Manager {
	return &Manager{
		k8sClient: k8sClient,
	}
}

// CallToolResult represents the result of a tool call
type CallToolResult struct {
	IsError bool          `json:"is_error,omitempty"`
	Content []interface{} `json:"content"`
}

// TextContent represents text content in a result
type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ExecuteTool executes a tool by name with given arguments
func (m *Manager) ExecuteTool(toolName string, args json.RawMessage) (*CallToolResult, error) {
	// Check if k8s client is available
	if m.k8sClient == nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: "Kubernetes client not available. Please ensure kubeconfig is properly configured.",
				},
			},
		}, nil
	}
	switch toolName {
	// Cluster management tools
	case "list_contexts":
		return m.ListContexts(args)
	case "switch_context":
		return m.SwitchContext(args)
	case "get_cluster_info":
		return m.GetClusterInfo(args)

	// Istio management tools
	case "install_istio":
		return m.InstallIstio(args)
	case "uninstall_istio":
		return m.UninstallIstio(args)
	case "check_istio_status":
		return m.CheckIstioStatus(args)

	// Sail operator tools
	case "install_sail_operator":
		return m.InstallSailOperator(args)
	case "uninstall_sail_operator":
		return m.UninstallSailOperator(args)
	case "check_sail_status":
		return m.CheckSailStatus(args)

	// Sample application tools
	case "deploy_sleep_app":
		return m.DeploySleepApp(args)
	case "deploy_httpbin_app":
		return m.DeployHttpbinApp(args)
	case "undeploy_sleep_app":
		return m.UndeploySleepApp(args)
	case "undeploy_httpbin_app":
		return m.UndeployHttpbinApp(args)

	// Connectivity testing tools
	case "test_connectivity":
		return m.TestConnectivity(args)
	case "test_sleep_to_httpbin":
		return m.TestSleepToHttpbin(args)

	// Logging and debugging tools
	case "get_pod_logs":
		return m.GetPodLogs(args)
	case "get_istio_proxy_logs":
		return m.GetIstioProxyLogs(args)
	case "exec_pod_command":
		return m.ExecPodCommand(args)

	// Network debugging tools
	case "get_iptables_rules":
		return m.GetIptablesRules(args)
	case "get_network_policies":
		return m.GetNetworkPolicies(args)
	case "trace_network_path":
		return m.TraceNetworkPath(args)

	default:
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Unknown tool: %s", toolName),
				},
			},
		}, nil
	}
}
