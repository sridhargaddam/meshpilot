package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"meshpilot/internal/k8s"
	"meshpilot/internal/mcp"
	"meshpilot/internal/tools"

	"github.com/sirupsen/logrus"
	"golang.org/x/term"
)

// toTitle converts a string to title case (replacement for deprecated strings.Title)
func toTitle(s string) string {
	if s == "" {
		return s
	}

	words := strings.Fields(s)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		}
	}
	return strings.Join(words, " ")
}

func main() {
	// Detect if running as MCP server (stdin is not a terminal AND no command line args)
	isMCPMode := !term.IsTerminal(int(os.Stdin.Fd())) && len(os.Args) == 1

	if isMCPMode {
		// Running as MCP server - disable logging to stdout/stderr
		logrus.SetLevel(logrus.ErrorLevel) // Only show errors
	} else {
		// Running interactively or with command line args
		logrus.SetLevel(logrus.InfoLevel)
		logrus.SetFormatter(&logrus.JSONFormatter{})
	}

	// Initialize Kubernetes client
	k8sClient, err := k8s.NewClient()
	if err != nil {
		if isMCPMode {
			// In MCP mode, fail silently and let the MCP client handle errors
			k8sClient = nil
		} else {
			log.Fatalf("Failed to create Kubernetes client: %v", err)
		}
	}

	// Initialize tool manager
	toolManager := tools.NewManager(k8sClient)

	// Create MCP server using official SDK
	server := mcp.NewServer("meshpilot", "0.1.0", toolManager)

	// Server creation handles tool registration automatically

	// Handle MCP mode vs interactive mode
	if isMCPMode {
		// Running as MCP server - handle stdio communication
		ctx := context.Background()
		if err := server.Serve(ctx); err != nil {
			logrus.Errorf("MCP server failed: %v", err)
			os.Exit(1)
		}
		return
	}

	// Handle command line arguments
	if len(os.Args) > 1 {
		if os.Args[1] == "--help" || os.Args[1] == "-h" {
			showHelp()
			return
		}
		if os.Args[1] == "--list-tools" {
			listTools(server)
			return
		}
		if os.Args[1] == "--tool-help" {
			if len(os.Args) < 3 {
				fmt.Println("Usage: meshpilot --tool-help <tool_name>")
				fmt.Println("Example: meshpilot --tool-help check_istio_status")
				os.Exit(1)
			}
			showDetailedToolHelp(os.Args[2])
			return
		}
		if os.Args[1] == "--tool" {
			handleDirectExecution(toolManager)
			return
		}
		fmt.Printf("Unknown argument: %s\n", os.Args[1])
		showHelp()
		return
	}

	// Start MCP server with graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logrus.Infof("Received signal %s, shutting down gracefully...", sig)
		cancel()
	}()

	// Show startup message with instructions
	fmt.Println("\n🚀 MeshPilot MCP Server is running!")
	fmt.Println("📋 Use Ctrl+C to stop the server")
	fmt.Println("💡 For direct tool execution, use: ./meshpilot --tool <tool_name> --args '<json_args>'")
	fmt.Println("📖 For help, use: ./meshpilot --help")
	fmt.Println("📝 For tool list, use: ./meshpilot --list-tools")
	fmt.Println()

	// Check for demo mode (environment variable)
	demoMode := os.Getenv("MESHPILOT_DEMO") == "true"

	var serverCtx context.Context
	var serverCancel context.CancelFunc

	if demoMode {
		// Demo mode with 30 second timeout
		serverCtx, serverCancel = context.WithTimeout(ctx, 30*time.Second)
		fmt.Println("🧪 Running in demo mode (30s timeout). Set MESHPILOT_DEMO=false for production.")
	} else {
		// Production mode - no timeout
		serverCtx, serverCancel = context.WithCancel(ctx)
		fmt.Println("🚀 Running in production mode. Use Ctrl+C to stop.")
	}
	defer serverCancel()

	done := make(chan error, 1)
	go func() {
		done <- server.Serve(serverCtx)
	}()

	select {
	case <-ctx.Done():
		fmt.Println("\n✅ MeshPilot server stopped gracefully")
	case err := <-done:
		if err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	case <-serverCtx.Done():
		if demoMode {
			fmt.Println("\n⏰ Demo timeout reached (30s). Restart without MESHPILOT_DEMO=true for continuous operation.")
		}
	}
}

// handleDirectExecution allows direct tool execution from command line
func handleDirectExecution(toolManager *tools.Manager) {
	if len(os.Args) < 3 {
		fmt.Println("Usage: meshpilot --tool <tool_name> [--args '<json_args>']")
		fmt.Println("Use --help for more information or --list-tools to see available tools")
		os.Exit(1)
	}

	toolName := os.Args[2]

	// Check if tool exists and show help if requested
	if !isValidTool(toolName) {
		fmt.Printf("❌ Unknown tool: %s\n\n", toolName)
		showToolHelp(toolName)
		fmt.Println("💡 Use --list-tools to see all available tools")
		os.Exit(1)
	}

	var args json.RawMessage

	// Handle different argument patterns
	if len(os.Args) >= 5 && os.Args[3] == "--args" {
		args = json.RawMessage(os.Args[4])
	} else if len(os.Args) == 3 {
		// No args provided, use empty JSON and show tool help
		args = json.RawMessage("{}")
		fmt.Printf("ℹ️  Executing tool '%s' with default parameters\n", toolName)
		showToolParameters(toolName)
		fmt.Println()
	} else if len(os.Args) == 4 {
		// Check if the 4th argument is JSON (user forgot --args)
		potentialJSON := os.Args[3]
		if strings.HasPrefix(potentialJSON, "{") && strings.HasSuffix(potentialJSON, "}") {
			args = json.RawMessage(potentialJSON)
			fmt.Printf("ℹ️  Detected JSON arguments (consider using --args flag for clarity)\n")
		} else {
			fmt.Printf("❌ Invalid usage. Did you mean: --args '%s'?\n\n", potentialJSON)
			showToolParameters(toolName)
			os.Exit(1)
		}
	} else {
		fmt.Println("❌ Invalid arguments format")
		fmt.Printf("Usage: meshpilot --tool %s --args '<json_args>'\n\n", toolName)
		showToolParameters(toolName)
		os.Exit(1)
	}

	result, err := toolManager.ExecuteTool(toolName, args)
	if err != nil {
		fmt.Printf("❌ Error executing tool %s: %v\n", toolName, err)
		os.Exit(1)
	}

	// Print the result in a user-friendly format
	printFormattedResult(toolName, result)
}

// showHelp displays usage information
func showHelp() {
	fmt.Println(`
🔧 MeshPilot - Kubernetes Istio Management MCP Server

USAGE:
    meshpilot [OPTIONS]

OPTIONS:
    --help, -h          Show this help message
    --list-tools        List all available tools
    --tool-help <name>  Show detailed help for a specific tool
    --tool <name>       Execute a specific tool
        --args <json>   JSON arguments for the tool (optional)

EXAMPLES:
    # Start MCP server (production mode - runs until Ctrl+C)
    ./meshpilot

    # Start MCP server in demo mode (30s timeout)
    MESHPILOT_DEMO=true ./meshpilot

    # Show available tools
    ./meshpilot --list-tools

    # Get help for a specific tool
    ./meshpilot --tool-help check_istio_status

    # Execute a tool directly
    ./meshpilot --tool list_contexts --args '{}'
    ./meshpilot --tool get_cluster_info --args '{}'
    ./meshpilot --tool install_istio --args '{"profile":"demo","namespace":"istio-system"}'

TOOL CATEGORIES:
    📋 Cluster Management: list_contexts, switch_context, get_cluster_info
    🕸️  Istio Management: install_istio, uninstall_istio, check_istio_status
    ⛵ Sail Operator: install_sail_operator, uninstall_sail_operator, check_sail_status
    📦 Sample Apps: deploy_sleep_app, deploy_httpbin_app, undeploy_*_app
    🔗 Connectivity: test_connectivity, test_sleep_to_httpbin
    📄 Logging: get_pod_logs, get_istio_proxy_logs, exec_pod_command
    🌐 Network Debug: get_iptables_rules, get_network_policies, trace_network_path

For detailed documentation, see README.md`)
}

// listTools displays all available tools in a user-friendly format
func listTools(server *mcp.Server) {
	fmt.Println("\n🛠️  Available MeshPilot Tools:")
	fmt.Println(strings.Repeat("=", 50))

	categories := map[string][]string{
		"📋 Cluster Management": {
			"list_contexts - List available Kubernetes contexts",
			"switch_context - Switch to a different Kubernetes context",
			"get_cluster_info - Get information about the current cluster",
		},
		"🕸️  Istio Management": {
			"install_istio - Install Istio on the cluster using Helm (with optional CNI support)",
			"uninstall_istio - Uninstall Istio from the cluster using Helm",
			"check_istio_status - Check Istio installation status",
		},
		"⛵ Sail Operator": {
			"install_sail_operator - Install Sail operator using Helm",
			"uninstall_sail_operator - Uninstall Sail operator using Helm",
			"check_sail_status - Check Sail operator status",
		},
		"📦 Sample Applications": {
			"deploy_sleep_app - Deploy sleep sample application",
			"deploy_httpbin_app - Deploy httpbin sample application",
			"undeploy_sleep_app - Remove sleep sample application",
			"undeploy_httpbin_app - Remove httpbin sample application",
		},
		"🔗 Connectivity Testing": {
			"test_connectivity - Test connectivity between pods",
			"test_sleep_to_httpbin - Test connectivity from sleep to httpbin",
		},
		"📄 Logging & Debugging": {
			"get_pod_logs - Get logs from a specific pod",
			"get_istio_proxy_logs - Get Istio proxy logs from a pod",
			"exec_pod_command - Execute a command in a pod",
		},
		"🌐 Network Debugging": {
			"get_iptables_rules - Get iptables rules from a pod",
			"get_network_policies - Get network policies in a namespace",
			"trace_network_path - Trace network path between pods",
		},
	}

	for category, tools := range categories {
		fmt.Printf("\n%s:\n", category)
		for _, tool := range tools {
			fmt.Printf("  • %s\n", tool)
		}
	}

	fmt.Printf("\n💡 Usage: ./meshpilot --tool <tool_name> --args '<json_args>'\n")
	fmt.Printf("📖 Help:  ./meshpilot --help\n\n")
}

// isValidTool checks if a tool name is valid
func isValidTool(toolName string) bool {
	validTools := []string{
		"list_contexts", "switch_context", "get_cluster_info",
		"install_istio", "uninstall_istio", "check_istio_status",
		"install_sail_operator", "uninstall_sail_operator", "check_sail_status",
		"deploy_sleep_app", "deploy_httpbin_app", "undeploy_sleep_app", "undeploy_httpbin_app",
		"test_connectivity", "test_sleep_to_httpbin",
		"get_pod_logs", "get_istio_proxy_logs", "exec_pod_command",
		"get_iptables_rules", "get_network_policies", "trace_network_path",
	}

	for _, valid := range validTools {
		if toolName == valid {
			return true
		}
	}
	return false
}

// showToolHelp shows help for a specific tool or suggests similar tools
func showToolHelp(toolName string) {
	fmt.Printf("🔍 Did you mean one of these similar tools?\n")

	// Simple fuzzy matching
	suggestions := []string{}
	validTools := []string{
		"list_contexts", "switch_context", "get_cluster_info",
		"install_istio", "uninstall_istio", "check_istio_status",
		"install_sail_operator", "uninstall_sail_operator", "check_sail_status",
		"deploy_sleep_app", "deploy_httpbin_app", "undeploy_sleep_app", "undeploy_httpbin_app",
		"test_connectivity", "test_sleep_to_httpbin",
		"get_pod_logs", "get_istio_proxy_logs", "exec_pod_command",
		"get_iptables_rules", "get_network_policies", "trace_network_path",
	}

	for _, valid := range validTools {
		if strings.Contains(valid, toolName) || strings.Contains(toolName, strings.Split(valid, "_")[0]) {
			suggestions = append(suggestions, valid)
		}
	}

	if len(suggestions) > 0 {
		for _, suggestion := range suggestions {
			fmt.Printf("  • %s\n", suggestion)
		}
	} else {
		fmt.Printf("  No similar tools found.\n")
	}
}

// showToolParameters shows the parameters for a specific tool
func showToolParameters(toolName string) {
	fmt.Printf("📋 Parameters for '%s':\n", toolName)

	toolParams := map[string]string{
		"list_contexts": "No parameters required - lists all available Kubernetes contexts\n  Example: --args '{}'",

		"switch_context": "Required: context (string)\n  Example: --args '{\"context\":\"my-cluster\"}'",

		"get_cluster_info": "No parameters required - gets current cluster information\n  Example: --args '{}'",

		"install_istio": "Optional: namespace (string, default: \"istio-system\"), version (string), values (object), install_gateway (bool), gateway_namespace (string, default: \"istio-ingress\"), install_cni (bool), cni_values (object), timeout (string, default: \"5m\")\n  Example: --args '{\"namespace\":\"istio-system\",\"version\":\"1.26.3\",\"install_gateway\":true,\"install_cni\":true}'",

		"uninstall_istio": "Optional: namespace (string, default: \"istio-system\"), gateway_namespace (string, default: \"istio-ingress\"), uninstall_cni (bool), delete_crds (bool, default: false), timeout (string, default: \"5m\")\n  Example: --args '{\"namespace\":\"istio-system\",\"uninstall_cni\":true,\"delete_crds\":true}'",

		"check_istio_status": "Optional: namespace (string, default: \"istio-system\")\n  Example: --args '{\"namespace\":\"istio-system\"}'",

		"install_sail_operator": "Optional: namespace (string, default: \"sail-operator\"), version (string), release_name (string, default: \"sail-operator\"), values (object), timeout (string, default: \"5m\")\n  Example: --args '{\"namespace\":\"sail-operator\",\"version\":\"1.24.0\"}'",

		"uninstall_sail_operator": "Optional: namespace (string, default: \"sail-operator\"), release_name (string, default: \"sail-operator\"), timeout (string, default: \"5m\")\n  Example: --args '{\"namespace\":\"sail-operator\"}'",

		"check_sail_status": "Optional: namespace (string, default: \"sail-operator\")\n  Example: --args '{\"namespace\":\"sail-operator\"}'",

		"deploy_sleep_app": "Optional: namespace (string, default: \"default\"), replicas (int, default: 1)\n  Example: --args '{\"namespace\":\"default\",\"replicas\":1}'",

		"deploy_httpbin_app": "Optional: namespace (string, default: \"default\"), replicas (int, default: 1)\n  Example: --args '{\"namespace\":\"default\",\"replicas\":1}'",

		"undeploy_sleep_app": "Optional: namespace (string, default: \"default\")\n  Example: --args '{\"namespace\":\"default\"}'",

		"undeploy_httpbin_app": "Optional: namespace (string, default: \"default\")\n  Example: --args '{\"namespace\":\"default\"}'",

		"test_connectivity": "Required: source_pod (string), target_service (string), target_port (int)\n  Optional: source_namespace (string), protocol (string), timeout (int)\n  Example: --args '{\"source_pod\":\"sleep-xxx\",\"target_service\":\"httpbin.default.svc.cluster.local\",\"target_port\":8000}'",

		"test_sleep_to_httpbin": "Optional: source_namespace (string, default: \"default\"), target_namespace (string, default: \"default\")\n  Example: --args '{\"source_namespace\":\"default\",\"target_namespace\":\"default\"}'",

		"get_pod_logs": "Required: pod_name (string)\n  Optional: namespace (string), container (string), lines (int), since (string)\n  Example: --args '{\"pod_name\":\"my-pod\",\"namespace\":\"default\",\"lines\":100}'",

		"get_istio_proxy_logs": "Required: pod_name (string)\n  Optional: namespace (string), lines (int), since (string)\n  Example: --args '{\"pod_name\":\"my-pod\",\"namespace\":\"default\"}'",

		"exec_pod_command": "Required: pod_name (string), command (array of strings)\n  Optional: namespace (string), container (string)\n  Example: --args '{\"pod_name\":\"my-pod\",\"command\":[\"ls\",\"-la\"]}'",

		"get_iptables_rules": "Required: pod_name (string)\n  Optional: namespace (string), container (string), tables (array), verbose (bool)\n  Example: --args '{\"pod_name\":\"my-pod\",\"namespace\":\"default\"}'",

		"get_network_policies": "Optional: namespace (string, default: \"default\"), pod_name (string)\n  Example: --args '{\"namespace\":\"default\"}'",

		"trace_network_path": "Required: source_pod (string), target_host OR target_pod (string)\n  Optional: source_namespace, target_namespace (string), max_hops (int)\n  Example: --args '{\"source_pod\":\"sleep-xxx\",\"target_host\":\"httpbin.default.svc.cluster.local\"}'",
	}

	if params, exists := toolParams[toolName]; exists {
		fmt.Printf("  %s\n", params)
	} else {
		fmt.Printf("  No parameter information available for this tool.\n")
	}
}

// showDetailedToolHelp shows comprehensive help for a specific tool
func showDetailedToolHelp(toolName string) {
	if !isValidTool(toolName) {
		fmt.Printf("❌ Unknown tool: %s\n\n", toolName)
		showToolHelp(toolName)
		fmt.Println("💡 Use --list-tools to see all available tools")
		return
	}

	fmt.Printf("🛠️  Detailed Help for '%s'\n", toolName)
	fmt.Println(strings.Repeat("=", 50))

	// Tool descriptions
	descriptions := map[string]string{
		"list_contexts":           "Lists all available Kubernetes contexts from your kubeconfig",
		"switch_context":          "Switches to a different Kubernetes context in your kubeconfig",
		"get_cluster_info":        "Retrieves detailed information about the current Kubernetes cluster",
		"install_istio":           "Installs Istio service mesh on the cluster with specified profile",
		"uninstall_istio":         "Removes Istio service mesh from the cluster",
		"check_istio_status":      "Checks the installation status and health of Istio components",
		"install_sail_operator":   "Installs the Sail operator for managing Istio",
		"uninstall_sail_operator": "Removes the Sail operator from the cluster",
		"check_sail_status":       "Checks the status and health of the Sail operator",
		"deploy_sleep_app":        "Deploys the sleep sample application for testing",
		"deploy_httpbin_app":      "Deploys the httpbin sample application for testing",
		"undeploy_sleep_app":      "Removes the sleep sample application",
		"undeploy_httpbin_app":    "Removes the httpbin sample application",
		"test_connectivity":       "Tests network connectivity between pods",
		"test_sleep_to_httpbin":   "Tests connectivity from sleep pod to httpbin service",
		"get_pod_logs":            "Retrieves logs from a specific pod and container",
		"get_istio_proxy_logs":    "Gets Istio sidecar proxy logs from a pod",
		"exec_pod_command":        "Executes a command inside a pod container",
		"get_iptables_rules":      "Inspects iptables rules inside a pod (useful for debugging)",
		"get_network_policies":    "Lists network policies affecting pods in a namespace",
		"trace_network_path":      "Traces the network path between two pods",
	}

	if desc, exists := descriptions[toolName]; exists {
		fmt.Printf("\n📝 Description:\n  %s\n", desc)
	}

	fmt.Printf("\n")
	showToolParameters(toolName)

	// Usage examples
	examples := map[string][]string{
		"check_istio_status": {
			"# Check Istio status in default namespace (istio-system)",
			"./meshpilot --tool check_istio_status",
			"",
			"# Check Istio status in a specific namespace",
			"./meshpilot --tool check_istio_status --args '{\"namespace\":\"my-namespace\"}'",
		},
		"install_istio": {
			"# Install Istio with demo profile",
			"./meshpilot --tool install_istio",
			"",
			"# Install Istio with specific profile and namespace",
			"./meshpilot --tool install_istio --args '{\"profile\":\"minimal\",\"namespace\":\"istio-system\"}'",
		},
		"get_pod_logs": {
			"# Get logs from a pod (will show error if pod_name not provided)",
			"./meshpilot --tool get_pod_logs --args '{\"pod_name\":\"my-pod\"}'",
			"",
			"# Get last 50 lines from specific container",
			"./meshpilot --tool get_pod_logs --args '{\"pod_name\":\"my-pod\",\"container\":\"app\",\"lines\":50}'",
		},
		"deploy_sleep_app": {
			"# Deploy sleep app in default namespace",
			"./meshpilot --tool deploy_sleep_app",
			"",
			"# Deploy with custom settings",
			"./meshpilot --tool deploy_sleep_app --args '{\"namespace\":\"test\",\"replicas\":2}'",
		},
	}

	if exampleList, exists := examples[toolName]; exists {
		fmt.Printf("\n💡 Usage Examples:\n")
		for _, example := range exampleList {
			if strings.HasPrefix(example, "#") {
				fmt.Printf("  %s\n", example)
			} else if example == "" {
				fmt.Println()
			} else {
				fmt.Printf("  %s\n", example)
			}
		}
	}

	fmt.Printf("\n📚 More Help:\n")
	fmt.Printf("  --help           General help\n")
	fmt.Printf("  --list-tools     All available tools\n")
	fmt.Printf("  --tool-help <name>  Help for other tools\n\n")
}

// printFormattedResult prints tool results in a user-friendly format
func printFormattedResult(toolName string, result *tools.CallToolResult) {
	if result.IsError {
		fmt.Printf("❌ Error: %v\n", result.Content)
		return
	}

	// Extract the actual text content
	var textContent string
	if len(result.Content) > 0 {
		if tc, ok := result.Content[0].(tools.TextContent); ok {
			textContent = tc.Text
		} else {
			// Fallback to string conversion
			textContent = fmt.Sprintf("%v", result.Content[0])
		}
	}

	// Try to parse as JSON for structured formatting
	var jsonData interface{}
	if err := json.Unmarshal([]byte(textContent), &jsonData); err == nil {
		// Successfully parsed as JSON, format based on tool type
		formatStructuredResult(toolName, jsonData)
	} else {
		// Not JSON or parsing failed, print as-is with some formatting
		fmt.Printf("📋 %s Result:\n", toTitle(strings.ReplaceAll(toolName, "_", " ")))
		fmt.Printf("%s\n", textContent)
	}
}

// formatStructuredResult formats JSON results based on tool type
func formatStructuredResult(toolName string, data interface{}) {
	switch toolName {
	case "test_sleep_to_httpbin", "test_connectivity":
		formatConnectivityTest(data)
	case "list_contexts":
		formatContextList(data)
	case "check_istio_status":
		formatIstioStatus(data)
	case "get_cluster_info":
		formatClusterInfo(data)
	case "get_pod_logs":
		formatPodLogs(data)
	case "get_istio_proxy_logs":
		formatIstioProxyLogs(data)
	case "exec_pod_command":
		formatExecPodCommand(data)
	case "check_sail_status":
		formatSailStatus(data)
	default:
		// Generic formatting for other tools
		formatGenericResult(toolName, data)
	}
}

// formatConnectivityTest formats connectivity test results
func formatConnectivityTest(data interface{}) {
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		fmt.Printf("📋 Connectivity Test Result:\n%v\n", data)
		return
	}

	fmt.Printf("🔗 Connectivity Test Results\n")
	fmt.Printf("═══════════════════════════════\n\n")

	if summary, exists := dataMap["summary"]; exists {
		fmt.Printf("📊 %s\n\n", summary)
	}

	if results, exists := dataMap["results"]; exists {
		if resultsArray, ok := results.([]interface{}); ok {
			for i, result := range resultsArray {
				if resultMap, ok := result.(map[string]interface{}); ok {
					fmt.Printf("🧪 Test %d:\n", i+1)

					// Source info
					if source, exists := resultMap["source"]; exists {
						if sourceMap, ok := source.(map[string]interface{}); ok {
							fmt.Printf("  📤 Source: %s (%s)\n", sourceMap["name"], sourceMap["namespace"])
							if ip, exists := sourceMap["ip"]; exists {
								fmt.Printf("       IP: %s\n", ip)
							}
						}
					}

					// Destination info
					if dest, exists := resultMap["destination"]; exists {
						if destMap, ok := dest.(map[string]interface{}); ok {
							fmt.Printf("  📥 Target: %s (%s)\n", destMap["name"], destMap["namespace"])
							if ip, exists := destMap["ip"]; exists {
								fmt.Printf("       IP: %s\n", ip)
							}
						}
					}

					// Status
					if success, exists := resultMap["success"]; exists {
						if success == true {
							fmt.Printf("  ✅ Status: SUCCESS")
						} else {
							fmt.Printf("  ❌ Status: FAILED")
						}
					}

					if statusCode, exists := resultMap["status_code"]; exists {
						if code, ok := statusCode.(float64); ok {
							fmt.Printf(" (HTTP %d)", int(code))
						}
					}
					fmt.Printf("\n")

					// Duration
					if duration, exists := resultMap["duration"]; exists {
						fmt.Printf("  ⏱️  Duration: %s\n", duration)
					}

					// Command used
					if command, exists := resultMap["command"]; exists {
						fmt.Printf("  🔧 Command: %s\n", command)
					}

					fmt.Printf("\n")
				}
			}
		}
	}
}

// formatContextList formats Kubernetes context list
func formatContextList(data interface{}) {
	fmt.Printf("🎯 Kubernetes Contexts\n")
	fmt.Printf("═══════════════════════\n\n")

	// Data is directly an array of contexts
	if contextsArray, ok := data.([]interface{}); ok {
		for _, context := range contextsArray {
			if contextMap, ok := context.(map[string]interface{}); ok {
				name := contextMap["name"]
				current := contextMap["current"]

				if current == true {
					fmt.Printf("➤ %s (current)\n", name)
				} else {
					fmt.Printf("  %s\n", name)
				}

				if cluster, exists := contextMap["cluster"]; exists {
					fmt.Printf("    Cluster: %s\n", cluster)
				}
				if user, exists := contextMap["user"]; exists {
					fmt.Printf("    User: %s\n", user)
				}
				if namespace, exists := contextMap["namespace"]; exists && namespace != "" {
					fmt.Printf("    Namespace: %s\n", namespace)
				}
				fmt.Printf("\n")
			}
		}
	} else {
		// Fallback if data structure is unexpected
		fmt.Printf("📋 Contexts:\n%v\n", data)
	}
}

// formatIstioStatus formats Istio status information
func formatIstioStatus(data interface{}) {
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		fmt.Printf("📋 Istio Status:\n%v\n", data)
		return
	}

	fmt.Printf("🕸️  Istio Service Mesh Status\n")
	fmt.Printf("═══════════════════════════════\n\n")

	if installed, exists := dataMap["installed"]; exists {
		if installed == true {
			fmt.Printf("✅ Status: Installed\n")
		} else {
			fmt.Printf("❌ Status: Not Installed\n")
		}
	}

	if namespace, exists := dataMap["namespace"]; exists {
		fmt.Printf("📦 Namespace: %s\n\n", namespace)
	}

	if components, exists := dataMap["components"]; exists {
		if componentsArray, ok := components.([]interface{}); ok {
			fmt.Printf("🔧 Components:\n")
			for _, component := range componentsArray {
				if compMap, ok := component.(map[string]interface{}); ok {
					name := compMap["name"]
					ready := compMap["ready"]
					replicas := compMap["replicas"]
					available := compMap["available"]

					status := "❌"
					if ready == true {
						status = "✅"
					}

					fmt.Printf("  %s %s - %v/%v replicas ready\n", status, name, available, replicas)
				}
			}
		}
	}
	fmt.Printf("\n")
}

// formatClusterInfo formats cluster information
func formatClusterInfo(data interface{}) {
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		fmt.Printf("📋 Cluster Info:\n%v\n", data)
		return
	}

	fmt.Printf("☸️  Cluster Information\n")
	fmt.Printf("═══════════════════════\n\n")

	if name, exists := dataMap["name"]; exists {
		fmt.Printf("📛 Name: %s\n", name)
	}
	if version, exists := dataMap["version"]; exists {
		fmt.Printf("🏷️  Version: %s\n", version)
	}
	if platform, exists := dataMap["platform"]; exists {
		fmt.Printf("🖥️  Platform: %s\n", platform)
	}

	if nodeCount, exists := dataMap["node_count"]; exists {
		fmt.Printf("🖥️  Nodes: %v\n", nodeCount)
	}

	if namespaces, exists := dataMap["namespaces"]; exists {
		if nsArray, ok := namespaces.([]interface{}); ok {
			fmt.Printf("📂 Namespaces: %d\n", len(nsArray))
			for _, ns := range nsArray {
				fmt.Printf("   • %s\n", ns)
			}
		}
	}
	fmt.Printf("\n")
}

// formatPodLogs formats pod log output
func formatPodLogs(data interface{}) {
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		fmt.Printf("📋 Pod Logs:\n%v\n", data)
		return
	}

	fmt.Printf("📜 Pod Logs\n")
	fmt.Printf("═══════════\n\n")

	if pod, exists := dataMap["pod"]; exists {
		fmt.Printf("🏷️  Pod: %s\n", pod)
	}
	if namespace, exists := dataMap["namespace"]; exists {
		fmt.Printf("📦 Namespace: %s\n", namespace)
	}
	if container, exists := dataMap["container"]; exists {
		fmt.Printf("📦 Container: %s\n", container)
	}
	if lines, exists := dataMap["lines"]; exists {
		fmt.Printf("📊 Lines: %v\n", lines)
	}
	if truncated, exists := dataMap["truncated"]; exists && truncated == true {
		fmt.Printf("⚠️  Logs truncated (showing latest entries)\n")
	}

	// Look for raw_logs field (this is what the LogResult struct uses)
	if rawLogs, exists := dataMap["raw_logs"]; exists {
		fmt.Printf("\n📄 Logs:\n")
		fmt.Printf("─────────\n")
		logsStr := fmt.Sprintf("%v", rawLogs)
		if logsStr != "" && logsStr != "<nil>" {
			fmt.Printf("%s", logsStr)
		} else {
			fmt.Printf("(No logs found or logs are empty)\n")
		}
	} else {
		fmt.Printf("\n📄 No logs available\n")
	}
}

// formatIstioProxyLogs formats Istio proxy (Envoy) logs with enhanced readability
func formatIstioProxyLogs(data interface{}) {
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		fmt.Printf("📋 Istio Proxy Logs:\n%v\n", data)
		return
	}

	fmt.Printf("🕸️  Istio Proxy Logs (Envoy Sidecar)\n")
	fmt.Printf("════════════════════════════════════\n\n")

	if pod, exists := dataMap["pod"]; exists {
		fmt.Printf("🏷️  Pod: %s\n", pod)
	}
	if namespace, exists := dataMap["namespace"]; exists {
		fmt.Printf("📦 Namespace: %s\n", namespace)
	}
	if container, exists := dataMap["container"]; exists {
		fmt.Printf("🔧 Container: %s\n", container)
	}
	if lines, exists := dataMap["lines"]; exists {
		fmt.Printf("📊 Lines: %v\n", lines)
	}
	if truncated, exists := dataMap["truncated"]; exists && truncated == true {
		fmt.Printf("⚠️  Logs truncated (showing latest entries)\n")
	}

	// Look for raw_logs field (this is what the LogResult struct uses)
	if rawLogs, exists := dataMap["raw_logs"]; exists {
		fmt.Printf("\n📄 Envoy Proxy Logs:\n")
		fmt.Printf("────────────────────\n")
		logsStr := fmt.Sprintf("%v", rawLogs)
		if logsStr != "" && logsStr != "<nil>" {
			// Process Istio/Envoy logs for better readability
			processedLogs := processEnvoyLogs(logsStr)
			fmt.Printf("%s", processedLogs)
		} else {
			fmt.Printf("(No proxy logs found - pod may not have Istio sidecar)\n")
		}
	} else {
		fmt.Printf("\n📄 No proxy logs available\n")
		fmt.Printf("💡 Tip: Ensure the pod has Istio sidecar injection enabled\n")
	}
}

// processEnvoyLogs processes raw Envoy logs to highlight important information
func processEnvoyLogs(logs string) string {
	lines := strings.Split(logs, "\n")
	var processed []string

	for _, line := range lines {
		if line == "" {
			continue
		}

		// Add color coding and icons based on log level/content
		switch {
		case strings.Contains(line, "[error]") || strings.Contains(line, "ERROR"):
			processed = append(processed, fmt.Sprintf("❌ %s", line))
		case strings.Contains(line, "[warning]") || strings.Contains(line, "WARN"):
			processed = append(processed, fmt.Sprintf("⚠️  %s", line))
		case strings.Contains(line, "[info]") || strings.Contains(line, "INFO"):
			processed = append(processed, fmt.Sprintf("ℹ️  %s", line))
		case strings.Contains(line, "[debug]") || strings.Contains(line, "DEBUG"):
			processed = append(processed, fmt.Sprintf("🔍 %s", line))
		case strings.Contains(line, "inbound") || strings.Contains(line, "outbound"):
			processed = append(processed, fmt.Sprintf("🌐 %s", line))
		case strings.Contains(line, "cluster") || strings.Contains(line, "endpoint"):
			processed = append(processed, fmt.Sprintf("🎯 %s", line))
		case strings.Contains(line, "listener") || strings.Contains(line, "filter"):
			processed = append(processed, fmt.Sprintf("🔧 %s", line))
		default:
			processed = append(processed, fmt.Sprintf("   %s", line))
		}
	}

	return strings.Join(processed, "\n") + "\n"
}

// formatExecPodCommand formats pod command execution results
func formatExecPodCommand(data interface{}) {
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		fmt.Printf("📋 Command Execution:\n%v\n", data)
		return
	}

	fmt.Printf("🖥️  Pod Command Execution\n")
	fmt.Printf("════════════════════════\n\n")

	// Display metadata
	if pod, exists := dataMap["pod"]; exists {
		fmt.Printf("🏷️  Pod: %s\n", pod)
	}
	if namespace, exists := dataMap["namespace"]; exists {
		fmt.Printf("📦 Namespace: %s\n", namespace)
	}
	if container, exists := dataMap["container"]; exists {
		fmt.Printf("🔧 Container: %s\n", container)
	}
	if command, exists := dataMap["command"]; exists {
		fmt.Printf("⚡ Command: %s\n", command)
	}
	if timestamp, exists := dataMap["timestamp"]; exists {
		fmt.Printf("🕐 Executed: %s\n", timestamp)
	}

	// Display success status
	if success, exists := dataMap["success"]; exists {
		if successBool, ok := success.(bool); ok && successBool {
			fmt.Printf("✅ Status: Success\n")
		} else {
			fmt.Printf("❌ Status: Failed\n")
		}
	}

	// Display command output
	if output, exists := dataMap["output"]; exists {
		fmt.Printf("\n📄 Command Output:\n")
		fmt.Printf("──────────────────\n")
		outputStr := fmt.Sprintf("%v", output)
		if outputStr != "" && outputStr != "<nil>" {
			// Process and format the output for better readability
			processedOutput := processCommandOutput(outputStr)
			fmt.Printf("%s", processedOutput)
		} else {
			fmt.Printf("(No output)\n")
		}
	}

	// Display error if command failed
	if errorMsg, exists := dataMap["error"]; exists && errorMsg != nil {
		fmt.Printf("\n❌ Error Details:\n")
		fmt.Printf("─────────────────\n")
		fmt.Printf("%s\n", errorMsg)
	}
}

// processCommandOutput processes command output to improve readability
func processCommandOutput(output string) string {
	// Decode any escaped characters (like \u003c for <)
	decoded := strings.ReplaceAll(output, "\\u003c", "<")
	decoded = strings.ReplaceAll(decoded, "\\u003e", ">")
	decoded = strings.ReplaceAll(decoded, "\\n", "\n")
	decoded = strings.ReplaceAll(decoded, "\\t", "\t")

	// For network interface commands (ip a, ifconfig), add some visual enhancement
	lines := strings.Split(decoded, "\n")
	var processed []string

	for _, line := range lines {
		if line == "" {
			processed = append(processed, "")
			continue
		}

		// Add visual indicators for different types of network information
		switch {
		case strings.Contains(line, ": lo:") || strings.Contains(line, ": eth") || strings.Contains(line, ": wlan"):
			// Network interfaces
			processed = append(processed, fmt.Sprintf("🌐 %s", line))
		case strings.Contains(line, "inet ") && strings.Contains(line, "scope"):
			// IP addresses
			processed = append(processed, fmt.Sprintf("🔗 %s", line))
		case strings.Contains(line, "link/"):
			// Link layer info
			processed = append(processed, fmt.Sprintf("🔧 %s", line))
		case strings.Contains(line, "valid_lft") || strings.Contains(line, "preferred_lft"):
			// Lifetime info
			processed = append(processed, fmt.Sprintf("⏱️  %s", line))
		default:
			processed = append(processed, fmt.Sprintf("   %s", line))
		}
	}

	return strings.Join(processed, "\n") + "\n"
}

// formatSailStatus formats Sail operator status
func formatSailStatus(data interface{}) {
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		fmt.Printf("📋 Sail Status:\n%v\n", data)
		return
	}

	fmt.Printf("⛵ Sail Operator Status\n")
	fmt.Printf("══════════════════════\n\n")

	if installed, exists := dataMap["installed"]; exists {
		if installed == true {
			fmt.Printf("✅ Status: Installed\n")
		} else {
			fmt.Printf("❌ Status: Not Installed\n")
		}
	}

	if version, exists := dataMap["version"]; exists {
		fmt.Printf("🏷️  Version: %s\n", version)
	}

	if namespace, exists := dataMap["namespace"]; exists {
		fmt.Printf("📦 Namespace: %s\n", namespace)
	}
	fmt.Printf("\n")
}

// formatGenericResult provides generic formatting for other tools
func formatGenericResult(toolName string, data interface{}) {
	title := toTitle(strings.ReplaceAll(toolName, "_", " "))
	fmt.Printf("📋 %s Result\n", title)
	fmt.Printf("═%s═\n\n", strings.Repeat("═", len(title)+7))

	// Pretty print JSON
	output, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Printf("%v\n", data)
	} else {
		fmt.Printf("%s\n", output)
	}
}
