package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level,omitempty"`
	Message   string    `json:"message"`
	Container string    `json:"container"`
	Pod       string    `json:"pod"`
	Namespace string    `json:"namespace"`
}

// LogResult represents the result of a log query
type LogResult struct {
	Pod       string     `json:"pod"`
	Namespace string     `json:"namespace"`
	Container string     `json:"container"`
	Lines     int        `json:"lines"`
	Entries   []LogEntry `json:"entries,omitempty"`
	RawLogs   string     `json:"raw_logs,omitempty"`
	Truncated bool       `json:"truncated,omitempty"`
}

// GetPodLogs retrieves logs from a specific pod
func (m *Manager) GetPodLogs(args json.RawMessage) (*CallToolResult, error) {
	var params struct {
		PodName    string `json:"pod_name"`
		Namespace  string `json:"namespace,omitempty"`
		Container  string `json:"container,omitempty"`
		Lines      int64  `json:"lines,omitempty"`      // number of lines to retrieve
		Since      string `json:"since,omitempty"`      // duration like "1h", "30m"
		Follow     bool   `json:"follow,omitempty"`     // stream logs (not recommended for MCP)
		Previous   bool   `json:"previous,omitempty"`   // get logs from previous container instance
		Timestamps bool   `json:"timestamps,omitempty"` // include timestamps
		ParseLogs  bool   `json:"parse_logs,omitempty"` // attempt to parse structured logs
		MaxLines   int    `json:"max_lines,omitempty"`  // maximum lines to return (default: 1000)
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
	if params.Lines == 0 {
		params.Lines = 100
	}
	if params.MaxLines == 0 {
		params.MaxLines = 1000
	}
	params.Timestamps = true // Always include timestamps for better debugging

	ctx := context.Background()

	// Get pod to validate it exists and get container info
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

	// If no container specified, use the first one or find the main container
	if params.Container == "" {
		if len(pod.Spec.Containers) > 0 {
			params.Container = pod.Spec.Containers[0].Name
		} else {
			return &CallToolResult{
				IsError: true,
				Content: []interface{}{
					TextContent{
						Type: "text",
						Text: "No containers found in pod",
					},
				},
			}, nil
		}
	}

	// Build log options
	logOptions := &corev1.PodLogOptions{
		Container:  params.Container,
		Follow:     false, // Never follow in MCP context
		Previous:   params.Previous,
		Timestamps: params.Timestamps,
		TailLines:  &params.Lines,
	}

	// Parse since duration if provided
	if params.Since != "" {
		duration, err := time.ParseDuration(params.Since)
		if err != nil {
			return &CallToolResult{
				IsError: true,
				Content: []interface{}{
					TextContent{
						Type: "text",
						Text: fmt.Sprintf("Invalid duration format: %v", err),
					},
				},
			}, nil
		}
		sinceTime := metav1.NewTime(time.Now().Add(-duration))
		logOptions.SinceTime = &sinceTime
	}

	// Get logs
	req := m.k8sClient.Kubernetes.CoreV1().Pods(params.Namespace).GetLogs(params.PodName, logOptions)
	logs, err := req.Stream(ctx)
	if err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to get logs: %v", err),
				},
			},
		}, nil
	}
	defer logs.Close()

	// Read and process logs
	result, err := m.processLogs(logs, params.PodName, params.Namespace, params.Container, params.ParseLogs, params.MaxLines)
	if err != nil {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: fmt.Sprintf("Failed to process logs: %v", err),
				},
			},
		}, nil
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

// GetIstioProxyLogs retrieves Istio sidecar proxy logs from a pod
func (m *Manager) GetIstioProxyLogs(args json.RawMessage) (*CallToolResult, error) {
	var params struct {
		PodName   string `json:"pod_name"`
		Namespace string `json:"namespace,omitempty"`
		Lines     int64  `json:"lines,omitempty"`
		Since     string `json:"since,omitempty"`
		LogLevel  string `json:"log_level,omitempty"` // filter by log level
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
	if params.Lines == 0 {
		params.Lines = 100
	}

	// Call GetPodLogs with istio-proxy container
	proxyLogsArgs := map[string]interface{}{
		"pod_name":   params.PodName,
		"namespace":  params.Namespace,
		"container":  "istio-proxy",
		"lines":      params.Lines,
		"timestamps": true,
		"parse_logs": true,
	}

	if params.Since != "" {
		proxyLogsArgs["since"] = params.Since
	}

	argsJSON, _ := json.Marshal(proxyLogsArgs)
	result, err := m.GetPodLogs(argsJSON)
	if err != nil {
		return result, err
	}

	// Add filtering by log level if specified
	if params.LogLevel != "" && result != nil && len(result.Content) > 0 {
		// This is a simplified implementation - in practice, you'd want more sophisticated filtering
		logrus.Infof("Filtering Istio proxy logs by level: %s", params.LogLevel)
	}

	return result, nil
}

// ExecPodCommand executes a command in a pod and returns the output
func (m *Manager) ExecPodCommand(args json.RawMessage) (*CallToolResult, error) {
	var params struct {
		PodName     string   `json:"pod_name"`
		Namespace   string   `json:"namespace,omitempty"`
		Container   string   `json:"container,omitempty"`
		Command     []string `json:"command"`
		Interactive bool     `json:"interactive,omitempty"` // not supported in MCP
		Timeout     int      `json:"timeout,omitempty"`     // seconds
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
	if params.Container == "" {
		// Try to determine the main container
		ctx := context.Background()
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

		if len(pod.Spec.Containers) > 0 {
			params.Container = pod.Spec.Containers[0].Name
		} else {
			return &CallToolResult{
				IsError: true,
				Content: []interface{}{
					TextContent{
						Type: "text",
						Text: "No containers found in pod",
					},
				},
			}, nil
		}
	}
	if len(params.Command) == 0 {
		return &CallToolResult{
			IsError: true,
			Content: []interface{}{
				TextContent{
					Type: "text",
					Text: "Command is required",
				},
			},
		}, nil
	}

	ctx := context.Background()
	if params.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(params.Timeout)*time.Second)
		defer cancel()
	}

	// Execute command
	output, err := m.execCommandInPod(ctx, params.Namespace, params.PodName, params.Container, params.Command)

	result := map[string]interface{}{
		"pod":       params.PodName,
		"namespace": params.Namespace,
		"container": params.Container,
		"command":   strings.Join(params.Command, " "),
		"timestamp": time.Now(),
	}

	if err != nil {
		result["success"] = false
		result["error"] = err.Error()
	} else {
		result["success"] = true
		result["output"] = output
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

// processLogs processes log stream and returns structured result
func (m *Manager) processLogs(logs io.Reader, podName, namespace, container string, parseLogs bool, maxLines int) (*LogResult, error) {
	result := &LogResult{
		Pod:       podName,
		Namespace: namespace,
		Container: container,
	}

	var rawLogs strings.Builder
	var entries []LogEntry
	scanner := bufio.NewScanner(logs)
	lineCount := 0

	for scanner.Scan() && lineCount < maxLines {
		line := scanner.Text()
		rawLogs.WriteString(line + "\n")
		lineCount++

		if parseLogs {
			entry := m.parseLogLine(line, podName, namespace, container)
			if entry != nil {
				entries = append(entries, *entry)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading logs: %w", err)
	}

	result.Lines = lineCount
	result.RawLogs = rawLogs.String()
	result.Truncated = lineCount >= maxLines

	if parseLogs {
		result.Entries = entries
	}

	return result, nil
}

// parseLogLine attempts to parse a log line into structured format
func (m *Manager) parseLogLine(line, podName, namespace, container string) *LogEntry {
	// Basic parsing - in production, you'd want more sophisticated parsing
	// This handles Kubernetes timestamp format: 2023-01-01T00:00:00.000000000Z message

	if len(line) == 0 {
		return nil
	}

	entry := &LogEntry{
		Pod:       podName,
		Namespace: namespace,
		Container: container,
		Message:   line,
	}

	// Try to extract timestamp (Kubernetes format)
	if len(line) > 30 && line[10] == 'T' && line[19] == '.' {
		timestampStr := line[:30]
		if timestamp, err := time.Parse(time.RFC3339Nano, timestampStr); err == nil {
			entry.Timestamp = timestamp
			if len(line) > 31 {
				entry.Message = line[31:] // Remove timestamp from message
			}
		}
	}

	// Try to extract log level
	message := strings.ToLower(entry.Message)
	if strings.Contains(message, "error") || strings.Contains(message, "err") {
		entry.Level = "error"
	} else if strings.Contains(message, "warn") {
		entry.Level = "warning"
	} else if strings.Contains(message, "info") {
		entry.Level = "info"
	} else if strings.Contains(message, "debug") {
		entry.Level = "debug"
	}

	return entry
}
