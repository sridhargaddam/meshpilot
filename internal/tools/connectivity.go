package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// ConnectivityTestResult represents the result of a connectivity test
type ConnectivityTestResult struct {
	Source      PodInfo   `json:"source"`
	Destination PodInfo   `json:"destination"`
	Success     bool      `json:"success"`
	StatusCode  int       `json:"status_code,omitempty"`
	Response    string    `json:"response,omitempty"`
	Error       string    `json:"error,omitempty"`
	Duration    string    `json:"duration,omitempty"`
	Command     string    `json:"command"`
	Timestamp   time.Time `json:"timestamp"`
}

// PodInfo represents information about a pod
type PodInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	IP        string `json:"ip,omitempty"`
	Node      string `json:"node,omitempty"`
}

// TestConnectivity tests connectivity between two pods
func (m *Manager) TestConnectivity(args json.RawMessage) (*CallToolResult, error) {
	var params struct {
		SourcePod       string `json:"source_pod"`
		SourceNamespace string `json:"source_namespace,omitempty"`
		TargetService   string `json:"target_service"`
		TargetPort      int    `json:"target_port"`        // Required in schema
		Protocol        string `json:"protocol,omitempty"` // http, https, tcp
		Path            string `json:"path,omitempty"`     // for HTTP requests
		Timeout         int    `json:"timeout,omitempty"`  // seconds
		Method          string `json:"method,omitempty"`   // GET, POST, etc.
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

	// Validate required parameters
	if params.SourcePod == "" {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: "source_pod is required",
				},
			},
		}, nil
	}

	if params.TargetService == "" {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: "target_service is required",
				},
			},
		}, nil
	}

	if params.TargetPort == 0 {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: "target_port is required",
				},
			},
		}, nil
	}

	// Set defaults
	if params.SourceNamespace == "" {
		params.SourceNamespace = "default"
	}
	if params.Protocol == "" {
		params.Protocol = "http"
	}
	if params.Path == "" {
		params.Path = "/"
	}
	if params.Timeout == 0 {
		params.Timeout = 10
	}
	if params.Method == "" {
		params.Method = "GET"
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

	// Build command based on protocol
	var command []string
	switch params.Protocol {
	case "http", "https":
		url := fmt.Sprintf("%s://%s:%d%s", params.Protocol, params.TargetService, params.TargetPort, params.Path)
		command = []string{"curl", "-s", "-w", "\\nHTTP_CODE:%{http_code}\\nTIME_TOTAL:%{time_total}\\n",
			"-X", params.Method, "--connect-timeout", fmt.Sprintf("%d", params.Timeout), url}
	case "tcp":
		command = []string{"nc", "-z", "-v", "-w", fmt.Sprintf("%d", params.Timeout), params.TargetService, fmt.Sprintf("%d", params.TargetPort)}
	default:
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Unsupported protocol: %s", params.Protocol),
				},
			},
		}, nil
	}

	// Execute command in pod
	startTime := time.Now()
	output, err := m.execCommandInPod(ctx, params.SourceNamespace, params.SourcePod, "sleep", command)
	duration := time.Since(startTime)

	// Parse results
	result := ConnectivityTestResult{
		Source: PodInfo{
			Name:      sourcePod.Name,
			Namespace: sourcePod.Namespace,
			IP:        sourcePod.Status.PodIP,
			Node:      sourcePod.Spec.NodeName,
		},
		Destination: PodInfo{
			Name: params.TargetService,
			IP:   params.TargetService,
		},
		Command:   strings.Join(command, " "),
		Duration:  duration.String(),
		Timestamp: startTime,
	}

	if err != nil {
		result.Success = false
		result.Error = err.Error()
	} else {
		result.Success = true
		result.Response = output

		// Parse HTTP response if applicable
		if params.Protocol == "http" || params.Protocol == "https" {
			if strings.Contains(output, "HTTP_CODE:") {
				parts := strings.Split(output, "HTTP_CODE:")
				if len(parts) > 1 {
					codePart := strings.Split(parts[1], "\n")[0]
					if code, parseErr := fmt.Sscanf(codePart, "%d", &result.StatusCode); parseErr == nil && code == 1 {
						result.Success = result.StatusCode >= 200 && result.StatusCode < 400
					}
				}
			}
		}
	}

	// Format output similar to TestSleepToHttpbin for consistent display
	var status string
	if result.Success {
		status = "SUCCESS"
	} else {
		status = "FAILED"
	}
	summary := fmt.Sprintf("Connectivity test from %s to %s: %s",
		result.Source.Name,
		result.Destination.Name,
		status)

	resultData := map[string]interface{}{
		"summary": summary,
		"results": []ConnectivityTestResult{result},
	}

	resultJSON, _ := json.MarshalIndent(resultData, "", "  ")
	return &CallToolResult{
		Content: []interface{}{
			TextContent{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

// TestSleepToHttpbin tests connectivity from sleep pod to httpbin service
func (m *Manager) TestSleepToHttpbin(args json.RawMessage) (*CallToolResult, error) {
	var params struct {
		SourceNamespace string   `json:"source_namespace,omitempty"`
		TargetNamespace string   `json:"target_namespace,omitempty"`
		TestEndpoints   []string `json:"test_endpoints,omitempty"` // endpoints to test
		Timeout         int      `json:"timeout,omitempty"`
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
	if params.Timeout == 0 {
		params.Timeout = 10
	}
	if len(params.TestEndpoints) == 0 {
		params.TestEndpoints = []string{"/get", "/headers", "/status/200", "/delay/1"}
	}

	ctx := context.Background()

	// Find sleep pod
	sleepPods, err := m.k8sClient.Kubernetes.CoreV1().Pods(params.SourceNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=sleep",
	})
	if err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to list sleep pods: %v", err),
				},
			},
		}, nil
	}

	if len(sleepPods.Items) == 0 {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: "No sleep pods found",
				},
			},
		}, nil
	}

	sleepPod := sleepPods.Items[0]

	// Get httpbin service
	httpbinService, err := m.k8sClient.Kubernetes.CoreV1().Services(params.TargetNamespace).Get(ctx, "httpbin", metav1.GetOptions{})
	if err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to get httpbin service: %v", err),
				},
			},
		}, nil
	}

	var results []ConnectivityTestResult
	serviceHost := fmt.Sprintf("httpbin.%s.svc.cluster.local", params.TargetNamespace)
	servicePort := 8000

	// Test each endpoint
	for _, endpoint := range params.TestEndpoints {
		url := fmt.Sprintf("http://%s:%d%s", serviceHost, servicePort, endpoint)
		command := []string{"curl", "-s", "-w", "\\nHTTP_CODE:%{http_code}\\nTIME_TOTAL:%{time_total}\\n",
			"--connect-timeout", fmt.Sprintf("%d", params.Timeout), url}

		startTime := time.Now()
		output, execErr := m.execCommandInPod(ctx, sleepPod.Namespace, sleepPod.Name, "sleep", command)
		duration := time.Since(startTime)

		result := ConnectivityTestResult{
			Source: PodInfo{
				Name:      sleepPod.Name,
				Namespace: sleepPod.Namespace,
				IP:        sleepPod.Status.PodIP,
				Node:      sleepPod.Spec.NodeName,
			},
			Destination: PodInfo{
				Name:      "httpbin",
				Namespace: params.TargetNamespace,
				IP:        httpbinService.Spec.ClusterIP,
			},
			Command:   strings.Join(command, " "),
			Duration:  duration.String(),
			Timestamp: startTime,
		}

		if execErr != nil {
			result.Success = false
			result.Error = execErr.Error()
		} else {
			result.Success = true
			result.Response = output

			// Parse HTTP status code
			if strings.Contains(output, "HTTP_CODE:") {
				parts := strings.Split(output, "HTTP_CODE:")
				if len(parts) > 1 {
					codePart := strings.Split(parts[1], "\n")[0]
					if code, parseErr := fmt.Sscanf(codePart, "%d", &result.StatusCode); parseErr == nil && code == 1 {
						result.Success = result.StatusCode >= 200 && result.StatusCode < 400
					}
				}
			}
		}

		results = append(results, result)
	}

	// Summary
	successful := 0
	for _, result := range results {
		if result.Success {
			successful++
		}
	}

	summary := fmt.Sprintf("Sleep to Httpbin connectivity test completed: %d/%d tests successful", successful, len(results))

	output := map[string]interface{}{
		"summary": summary,
		"results": results,
	}

	resultJSON, _ := json.MarshalIndent(output, "", "  ")
	return &CallToolResult{
		Content: []interface{}{
			TextContent{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

// execCommandInPod executes a command inside a pod container
func (m *Manager) execCommandInPod(ctx context.Context, namespace, podName, containerName string, command []string) (string, error) {
	req := m.k8sClient.Kubernetes.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec")

	req.VersionedParams(&corev1.PodExecOptions{
		Container: containerName,
		Command:   command,
		Stdout:    true,
		Stderr:    true,
	}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(m.k8sClient.Config, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("failed to create executor: %w", err)
	}

	var stdout, stderr strings.Builder
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})

	if err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("command failed: %s, stderr: %s", err.Error(), stderr.String())
		}
		return "", fmt.Errorf("command failed: %w", err)
	}

	if stderr.Len() > 0 {
		logrus.Warnf("Command stderr: %s", stderr.String())
	}

	return stdout.String(), nil
}
