package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IptablesRules represents iptables rules from a pod
type IptablesRules struct {
	Pod       string            `json:"pod"`
	Namespace string            `json:"namespace"`
	Container string            `json:"container"`
	Tables    map[string]string `json:"tables"` // table name -> rules
	Timestamp time.Time         `json:"timestamp"`
}

// NetworkPolicyInfo represents network policy information
type NetworkPolicyInfo struct {
	Name      string                         `json:"name"`
	Namespace string                         `json:"namespace"`
	Spec      networkingv1.NetworkPolicySpec `json:"spec"`
	Status    string                         `json:"status"`
}

// NetworkTrace represents network path tracing information
type NetworkTrace struct {
	Source      PodInfo   `json:"source"`
	Destination PodInfo   `json:"destination"`
	Path        []string  `json:"path"`
	Success     bool      `json:"success"`
	Issues      []string  `json:"issues,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

// GetIptablesRules retrieves iptables rules from a pod
func (m *Manager) GetIptablesRules(args json.RawMessage) (*CallToolResult, error) {
	var params struct {
		PodName   string   `json:"pod_name"`
		Namespace string   `json:"namespace,omitempty"`
		Container string   `json:"container,omitempty"`
		Tables    []string `json:"tables,omitempty"` // specific tables to query
		Verbose   bool     `json:"verbose,omitempty"`
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
		params.Namespace = "default"
	}
	if len(params.Tables) == 0 {
		params.Tables = []string{"filter", "nat", "mangle"}
	}

	ctx := context.Background()

	// Get pod to validate it exists
	pod, err := m.k8sClient.Kubernetes.CoreV1().Pods(params.Namespace).Get(ctx, params.PodName, metav1.GetOptions{})
	if err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to get pod: %v", err),
				},
			},
		}, nil
	}

	// Determine container - prefer istio-proxy if available, otherwise first container
	if params.Container == "" {
		params.Container = "istio-proxy"
		found := false
		for _, container := range pod.Spec.Containers {
			if container.Name == "istio-proxy" {
				found = true
				break
			}
		}
		if !found && len(pod.Spec.Containers) > 0 {
			params.Container = pod.Spec.Containers[0].Name
		}
	}

	result := &IptablesRules{
		Pod:       params.PodName,
		Namespace: params.Namespace,
		Container: params.Container,
		Tables:    make(map[string]string),
		Timestamp: time.Now(),
	}

	// Query each iptables table using kubectl debug with istio/base image
	for _, table := range params.Tables {
		var iptablesArgs []string
		if params.Verbose {
			iptablesArgs = []string{"-t", table, "-L", "-v", "-n", "--line-numbers"}
		} else {
			iptablesArgs = []string{"-t", table, "-L", "-n"}
		}

		output, err := m.getIptablesWithDebug(ctx, params.Namespace, params.PodName, table, iptablesArgs)
		if err != nil {
			logrus.Warnf("Failed to get iptables rules for table %s: %v", table, err)
			result.Tables[table] = fmt.Sprintf("Error: %v", err)
		} else {
			result.Tables[table] = output
		}
	}

	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	return &CallToolResult{
		Content: []interface{}{
			TextContent{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

// getIptablesWithDebug uses kubectl debug to attach an ephemeral container to get iptables rules
func (m *Manager) getIptablesWithDebug(ctx context.Context, namespace, podName, table string, iptablesArgs []string) (string, error) {
	// Create a unique debug container name
	debugContainerName := fmt.Sprintf("debug-iptables-%d", time.Now().Unix())

	// Step 1: Create ephemeral container using kubectl debug (without interactive flags for programmatic use)
	kubectlArgs := []string{
		"debug",
		podName,
		"-n", namespace,
		"--image=istio/base",
		"--profile=sysadmin",
		"--quiet",
		"--attach=false",
		"--stdin=false",
		"-c", debugContainerName,
		"--",
		"iptables-nft",
	}
	kubectlArgs = append(kubectlArgs, iptablesArgs...)

	logrus.Debugf("Running kubectl debug command: kubectl %s", strings.Join(kubectlArgs, " "))

	// Execute kubectl debug command to create ephemeral container with timeout
	debugCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(debugCtx, "kubectl", kubectlArgs...)

	// Set up the command to not expect any input
	cmd.Stdin = nil

	debugOutput, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create ephemeral container: %w, output: %s", err, string(debugOutput))
	}

	logrus.Debugf("Debug command output: %s", string(debugOutput))

	// Step 2: Wait for the ephemeral container to be ready and complete
	// We need to wait for the container to be ready before getting logs
	maxWaitTime := 30 * time.Second
	waitInterval := 2 * time.Second
	startTime := time.Now()

	var output []byte
	var logsErr error

	for time.Since(startTime) < maxWaitTime {
		// Try to get logs from the ephemeral container
		logsCmd := exec.CommandContext(ctx, "kubectl", "logs", podName, "-n", namespace, "-c", debugContainerName)
		output, logsErr = logsCmd.CombinedOutput()

		// If we got logs successfully, break out of the loop
		if logsErr == nil && len(output) > 0 {
			break
		}

		// If the error is about the container still initializing, wait and retry
		if logsErr != nil && strings.Contains(string(output), "PodInitializing") {
			logrus.Debugf("Container %s still initializing, waiting %v...", debugContainerName, waitInterval)
			time.Sleep(waitInterval)
			continue
		}

		// If it's a different error, wait a bit and try once more
		if logsErr != nil {
			logrus.Debugf("Error getting logs from %s: %v, retrying...", debugContainerName, logsErr)
			time.Sleep(waitInterval)
			continue
		}

		// If we got empty output, the container might still be running
		time.Sleep(waitInterval)
	}

	if logsErr != nil {
		return "", fmt.Errorf("failed to get logs from ephemeral container after %v: %w, output: %s", maxWaitTime, logsErr, string(output))
	}

	return string(output), nil
}

// GetNetworkPolicies retrieves network policies in a namespace
func (m *Manager) GetNetworkPolicies(args json.RawMessage) (*CallToolResult, error) {
	var params struct {
		Namespace     string `json:"namespace,omitempty"`
		PodName       string `json:"pod_name,omitempty"`       // filter policies affecting this pod
		LabelSelector string `json:"label_selector,omitempty"` // filter by labels
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
		params.Namespace = "default"
	}

	ctx := context.Background()

	// List network policies
	listOptions := metav1.ListOptions{}
	if params.LabelSelector != "" {
		listOptions.LabelSelector = params.LabelSelector
	}

	policies, err := m.k8sClient.Kubernetes.NetworkingV1().NetworkPolicies(params.Namespace).List(ctx, listOptions)
	if err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to list network policies: %v", err),
				},
			},
		}, nil
	}

	var policyInfos []NetworkPolicyInfo
	var podLabels map[string]string

	// If pod name is specified, get its labels for filtering
	if params.PodName != "" {
		pod, err := m.k8sClient.Kubernetes.CoreV1().Pods(params.Namespace).Get(ctx, params.PodName, metav1.GetOptions{})
		if err != nil {
			logrus.Warnf("Failed to get pod %s for label filtering: %v", params.PodName, err)
		} else {
			podLabels = pod.Labels
		}
	}

	for _, policy := range policies.Items {
		policyInfo := NetworkPolicyInfo{
			Name:      policy.Name,
			Namespace: policy.Namespace,
			Spec:      policy.Spec,
			Status:    "active",
		}

		// If pod filtering is requested, check if policy applies to the pod
		if params.PodName != "" && podLabels != nil {
			if !m.policyAppliesToPod(policy.Spec.PodSelector, podLabels) {
				continue
			}
		}

		policyInfos = append(policyInfos, policyInfo)
	}

	result := map[string]interface{}{
		"namespace": params.Namespace,
		"count":     len(policyInfos),
		"policies":  policyInfos,
	}

	if params.PodName != "" {
		result["filtered_for_pod"] = params.PodName
	}

	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	return &CallToolResult{
		Content: []interface{}{
			TextContent{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

// TraceNetworkPath traces the network path between two pods
func (m *Manager) TraceNetworkPath(args json.RawMessage) (*CallToolResult, error) {
	var params struct {
		SourcePod       string `json:"source_pod"`
		SourceNamespace string `json:"source_namespace,omitempty"`
		TargetPod       string `json:"target_pod,omitempty"`
		TargetNamespace string `json:"target_namespace,omitempty"`
		TargetHost      string `json:"target_host,omitempty"`
		TargetPort      int    `json:"target_port,omitempty"`
		MaxHops         int    `json:"max_hops,omitempty"`
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
	if params.SourceNamespace == "" {
		params.SourceNamespace = "default"
	}
	if params.TargetNamespace == "" {
		params.TargetNamespace = "default"
	}
	if params.MaxHops == 0 {
		params.MaxHops = 30
	}

	ctx := context.Background()

	// Get source pod info
	sourcePod, err := m.k8sClient.Kubernetes.CoreV1().Pods(params.SourceNamespace).Get(ctx, params.SourcePod, metav1.GetOptions{})
	if err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to get source pod: %v", err),
				},
			},
		}, nil
	}

	trace := &NetworkTrace{
		Source: PodInfo{
			Name:      sourcePod.Name,
			Namespace: sourcePod.Namespace,
			IP:        sourcePod.Status.PodIP,
			Node:      sourcePod.Spec.NodeName,
		},
		Timestamp: time.Now(),
	}

	// Determine target
	var targetHost string
	var targetInfo PodInfo

	if params.TargetPod != "" {
		// Target is another pod
		targetPod, err := m.k8sClient.Kubernetes.CoreV1().Pods(params.TargetNamespace).Get(ctx, params.TargetPod, metav1.GetOptions{})
		if err != nil {
			return &CallToolResult{
				IsError: true,
				Content: []interface{}{
					TextContent{
						Type: "text",
						Text: fmt.Sprintf("Failed to get target pod: %v", err),
					},
				},
			}, nil
		}
		targetHost = targetPod.Status.PodIP
		targetInfo = PodInfo{
			Name:      targetPod.Name,
			Namespace: targetPod.Namespace,
			IP:        targetPod.Status.PodIP,
			Node:      targetPod.Spec.NodeName,
		}
	} else if params.TargetHost != "" {
		// Target is a host/service
		targetHost = params.TargetHost
		targetInfo = PodInfo{
			Name: params.TargetHost,
			IP:   params.TargetHost,
		}
	} else {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: "Either target_pod or target_host must be specified",
				},
			},
		}, nil
	}

	trace.Destination = targetInfo

	// Perform traceroute
	command := []string{"traceroute", "-n", "-m", fmt.Sprintf("%d", params.MaxHops), targetHost}
	if params.TargetPort > 0 {
		command = append(command, "-p", fmt.Sprintf("%d", params.TargetPort))
	}

	output, err := m.execCommandInPod(ctx, params.SourceNamespace, params.SourcePod, "sleep", command)
	if err != nil {
		// Try with different traceroute implementations
		command = []string{"tracepath", targetHost}
		output, err = m.execCommandInPod(ctx, params.SourceNamespace, params.SourcePod, "sleep", command)
	}

	if err != nil {
		trace.Success = false
		trace.Issues = append(trace.Issues, fmt.Sprintf("Traceroute failed: %v", err))
	} else {
		trace.Success = true
		trace.Path = m.parseTracerouteOutput(output)
	}

	// Additional network diagnostics
	if err := m.addNetworkDiagnostics(ctx, trace, params.SourceNamespace, params.SourcePod); err != nil {
		trace.Issues = append(trace.Issues, fmt.Sprintf("Failed to get network diagnostics: %v", err))
	}

	resultJSON, _ := json.MarshalIndent(trace, "", "  ")
	return &CallToolResult{
		Content: []interface{}{
			TextContent{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

// Helper functions

// policyAppliesToPod checks if a network policy applies to a pod based on label selectors
func (m *Manager) policyAppliesToPod(selector metav1.LabelSelector, podLabels map[string]string) bool {
	if len(selector.MatchLabels) == 0 && len(selector.MatchExpressions) == 0 {
		// Empty selector matches all pods
		return true
	}

	// Check MatchLabels
	for key, value := range selector.MatchLabels {
		if podLabels[key] != value {
			return false
		}
	}

	// Check MatchExpressions (simplified implementation)
	for _, expr := range selector.MatchExpressions {
		podValue, exists := podLabels[expr.Key]

		switch expr.Operator {
		case metav1.LabelSelectorOpIn:
			if !exists {
				return false
			}
			found := false
			for _, value := range expr.Values {
				if podValue == value {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		case metav1.LabelSelectorOpNotIn:
			if exists {
				for _, value := range expr.Values {
					if podValue == value {
						return false
					}
				}
			}
		case metav1.LabelSelectorOpExists:
			if !exists {
				return false
			}
		case metav1.LabelSelectorOpDoesNotExist:
			if exists {
				return false
			}
		}
	}

	return true
}

// parseTracerouteOutput parses traceroute output to extract hop information
func (m *Manager) parseTracerouteOutput(output string) []string {
	var path []string
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "traceroute") || strings.HasPrefix(line, "tracepath") {
			continue
		}

		// Extract meaningful information from each hop
		fields := strings.Fields(line)
		if len(fields) > 1 {
			hop := fields[0] + " " + fields[1]
			if len(fields) > 2 {
				hop += " (" + fields[2] + ")"
			}
			path = append(path, hop)
		}
	}

	return path
}

// addNetworkDiagnostics adds additional network diagnostic information
func (m *Manager) addNetworkDiagnostics(ctx context.Context, trace *NetworkTrace, namespace, podName string) error {
	// Get routing table
	routeOutput, err := m.execCommandInPod(ctx, namespace, podName, "sleep", []string{"ip", "route"})
	if err == nil {
		trace.Path = append(trace.Path, "=== Routing Table ===")
		for _, line := range strings.Split(routeOutput, "\n") {
			if strings.TrimSpace(line) != "" {
				trace.Path = append(trace.Path, "route: "+line)
			}
		}
	}

	// Get network interfaces
	ifOutput, err := m.execCommandInPod(ctx, namespace, podName, "sleep", []string{"ip", "addr"})
	if err == nil {
		trace.Path = append(trace.Path, "=== Network Interfaces ===")
		for _, line := range strings.Split(ifOutput, "\n") {
			if strings.TrimSpace(line) != "" && (strings.Contains(line, "inet") || strings.Contains(line, "link")) {
				trace.Path = append(trace.Path, "interface: "+line)
			}
		}
	}

	return nil
}
