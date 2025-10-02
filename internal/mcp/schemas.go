package mcp

import (
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Helper functions for creating JSON values
func jsonString(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

func jsonBool(b bool) json.RawMessage {
	bytes, _ := json.Marshal(b)
	return bytes
}

func jsonInt(i int) json.RawMessage {
	b, _ := json.Marshal(i)
	return b
}

// Simple schema creation helper
func createObjectSchema(properties map[string]*jsonschema.Schema, required []string) *jsonschema.Schema {
	schema := &jsonschema.Schema{
		Type:       "object",
		Properties: properties,
	}
	if len(required) > 0 {
		schema.Required = required
	}
	return schema
}

// GetToolDefinitions returns tool definitions with proper schemas
func GetToolDefinitions() map[string]*mcp.Tool {
	return map[string]*mcp.Tool{
		"list_contexts": {
			Name:        "list_contexts",
			Description: "List available Kubernetes contexts",
			InputSchema: createObjectSchema(map[string]*jsonschema.Schema{}, nil),
		},
		"switch_context": {
			Name:        "switch_context",
			Description: "Switch to a different Kubernetes context",
			InputSchema: createObjectSchema(map[string]*jsonschema.Schema{
				"context": {
					Type:        "string",
					Description: "The name of the Kubernetes context to switch to",
				},
			}, []string{"context"}),
		},
		"get_cluster_info": {
			Name:        "get_cluster_info",
			Description: "Get information about the current cluster",
			InputSchema: createObjectSchema(map[string]*jsonschema.Schema{}, nil),
		},
		"install_istio": {
			Name:        "install_istio",
			Description: "Install Istio service mesh on the cluster using Helm",
			InputSchema: createObjectSchema(map[string]*jsonschema.Schema{
				"version": {
					Type:        "string",
					Description: "Istio version to install (default: latest)",
					Default:     jsonString("latest"),
				},
				"values": {
					Type:        "string",
					Description: "Custom Helm values in YAML format",
				},
				"install_gateway": {
					Type:        "boolean",
					Description: "Whether to install Istio gateway (default: false)",
					Default:     jsonBool(false),
				},
				"gateway_namespace": {
					Type:        "string",
					Description: "Namespace for gateway installation (default: istio-ingress)",
					Default:     jsonString("istio-ingress"),
				},
				"install_cni": {
					Type:        "boolean",
					Description: "Whether to install Istio CNI (default: false)",
					Default:     jsonBool(false),
				},
				"cni_values": {
					Type:        "string",
					Description: "Custom CNI Helm values in YAML format",
				},
				"timeout": {
					Type:        "string",
					Description: "Helm timeout for installation (default: 10m)",
					Default:     jsonString("10m"),
				},
				"wait": {
					Type:        "boolean",
					Description: "Wait for installation to complete (default: true)",
					Default:     jsonBool(true),
				},
			}, nil),
		},
		"uninstall_istio": {
			Name:        "uninstall_istio",
			Description: "Uninstall Istio service mesh from the cluster using Helm",
			InputSchema: createObjectSchema(map[string]*jsonschema.Schema{
				"delete_crds": {
					Type:        "boolean",
					Description: "Whether to delete Istio CRDs (default: false)",
					Default:     jsonBool(false),
				},
				"uninstall_cni": {
					Type:        "boolean",
					Description: "Whether to uninstall CNI components (default: false)",
					Default:     jsonBool(false),
				},
				"timeout": {
					Type:        "string",
					Description: "Helm timeout for uninstallation (default: 10m)",
					Default:     jsonString("10m"),
				},
			}, nil),
		},
		"check_istio_status": {
			Name:        "check_istio_status",
			Description: "Check the status of Istio installation",
			InputSchema: createObjectSchema(map[string]*jsonschema.Schema{
				"namespace": {
					Type:        "string",
					Description: "Namespace to check Istio status (default: istio-system)",
					Default:     jsonString("istio-system"),
				},
			}, nil),
		},
		"install_sail_operator": {
			Name:        "install_sail_operator",
			Description: "Install Sail operator for Istio management using Helm",
			InputSchema: createObjectSchema(map[string]*jsonschema.Schema{
				"version": {
					Type:        "string",
					Description: "Sail operator version to install (default: latest)",
					Default:     jsonString("latest"),
				},
				"release_name": {
					Type:        "string",
					Description: "Helm release name (default: sail-operator)",
					Default:     jsonString("sail-operator"),
				},
				"values": {
					Type:        "string",
					Description: "Custom Helm values in YAML format",
				},
				"timeout": {
					Type:        "string",
					Description: "Helm timeout for installation (default: 10m)",
					Default:     jsonString("10m"),
				},
				"wait": {
					Type:        "boolean",
					Description: "Wait for installation to complete (default: true)",
					Default:     jsonBool(true),
				},
			}, nil),
		},
		"uninstall_sail_operator": {
			Name:        "uninstall_sail_operator",
			Description: "Uninstall Sail operator from the cluster using Helm",
			InputSchema: createObjectSchema(map[string]*jsonschema.Schema{
				"release_name": {
					Type:        "string",
					Description: "Helm release name (default: sail-operator)",
					Default:     jsonString("sail-operator"),
				},
				"timeout": {
					Type:        "string",
					Description: "Helm timeout for uninstallation (default: 10m)",
					Default:     jsonString("10m"),
				},
			}, nil),
		},
		"check_sail_status": {
			Name:        "check_sail_status",
			Description: "Check the status of Sail operator installation",
			InputSchema: createObjectSchema(map[string]*jsonschema.Schema{
				"namespace": {
					Type:        "string",
					Description: "Namespace to check Sail operator status (default: sail-operator)",
					Default:     jsonString("sail-operator"),
				},
			}, nil),
		},
		"deploy_sleep_app": {
			Name:        "deploy_sleep_app",
			Description: "Deploy sleep sample application for testing",
			InputSchema: createObjectSchema(map[string]*jsonschema.Schema{
				"namespace": {
					Type:        "string",
					Description: "Namespace to deploy sleep app (default: default)",
					Default:     jsonString("default"),
				},
			}, nil),
		},
		"deploy_httpbin_app": {
			Name:        "deploy_httpbin_app",
			Description: "Deploy httpbin sample application for testing",
			InputSchema: createObjectSchema(map[string]*jsonschema.Schema{
				"namespace": {
					Type:        "string",
					Description: "Namespace to deploy httpbin app (default: default)",
					Default:     jsonString("default"),
				},
			}, nil),
		},
		"undeploy_sleep_app": {
			Name:        "undeploy_sleep_app",
			Description: "Remove sleep sample application",
			InputSchema: createObjectSchema(map[string]*jsonschema.Schema{
				"namespace": {
					Type:        "string",
					Description: "Namespace to remove sleep app from (default: default)",
					Default:     jsonString("default"),
				},
			}, nil),
		},
		"undeploy_httpbin_app": {
			Name:        "undeploy_httpbin_app",
			Description: "Remove httpbin sample application",
			InputSchema: createObjectSchema(map[string]*jsonschema.Schema{
				"namespace": {
					Type:        "string",
					Description: "Namespace to remove httpbin app from (default: default)",
					Default:     jsonString("default"),
				},
			}, nil),
		},
		"test_connectivity": {
			Name:        "test_connectivity",
			Description: "Test network connectivity between pods",
			InputSchema: createObjectSchema(map[string]*jsonschema.Schema{
				"source_pod": {
					Type:        "string",
					Description: "Name of the source pod",
				},
				"source_namespace": {
					Type:        "string",
					Description: "Namespace of the source pod (default: default)",
					Default:     jsonString("default"),
				},
				"target_service": {
					Type:        "string",
					Description: "Target service name or IP",
				},
				"target_port": {
					Type:        "integer",
					Description: "Target port number",
					Minimum:     float64Ptr(1),
					Maximum:     float64Ptr(65535),
				},
				"protocol": {
					Type:        "string",
					Description: "Protocol to test (http, https, tcp) (default: http)",
					Default:     jsonString("http"),
					Enum:        []interface{}{"http", "https", "tcp"},
				},
			}, []string{"source_pod", "target_service", "target_port"}),
		},
		"test_sleep_to_httpbin": {
			Name:        "test_sleep_to_httpbin",
			Description: "Test connectivity from sleep pod to httpbin service",
			InputSchema: createObjectSchema(map[string]*jsonschema.Schema{
				"sleep_namespace": {
					Type:        "string",
					Description: "Namespace of the sleep pod (default: default)",
					Default:     jsonString("default"),
				},
				"httpbin_namespace": {
					Type:        "string",
					Description: "Namespace of the httpbin service (default: default)",
					Default:     jsonString("default"),
				},
				"path": {
					Type:        "string",
					Description: "HTTP path to test (default: /get)",
					Default:     jsonString("/get"),
				},
			}, nil),
		},
		"get_pod_logs": {
			Name:        "get_pod_logs",
			Description: "Get logs from a specific pod container",
			InputSchema: createObjectSchema(map[string]*jsonschema.Schema{
				"pod_name": {
					Type:        "string",
					Description: "Name of the pod to get logs from",
				},
				"namespace": {
					Type:        "string",
					Description: "Namespace of the pod (default: default)",
					Default:     jsonString("default"),
				},
				"container": {
					Type:        "string",
					Description: "Container name (optional)",
				},
				"tail_lines": {
					Type:        "integer",
					Description: "Number of lines to tail (default: 100)",
					Default:     jsonInt(100),
					Minimum:     float64Ptr(1),
				},
				"follow": {
					Type:        "boolean",
					Description: "Follow log output (default: false)",
					Default:     jsonBool(false),
				},
			}, []string{"pod_name"}),
		},
		"get_istio_proxy_logs": {
			Name:        "get_istio_proxy_logs",
			Description: "Get Istio sidecar proxy logs from a pod",
			InputSchema: createObjectSchema(map[string]*jsonschema.Schema{
				"pod_name": {
					Type:        "string",
					Description: "Name of the pod to get Istio proxy logs from",
				},
				"namespace": {
					Type:        "string",
					Description: "Namespace of the pod (default: default)",
					Default:     jsonString("default"),
				},
				"tail_lines": {
					Type:        "integer",
					Description: "Number of lines to tail (default: 100)",
					Default:     jsonInt(100),
					Minimum:     float64Ptr(1),
				},
			}, []string{"pod_name"}),
		},
		"exec_pod_command": {
			Name:        "exec_pod_command",
			Description: "Execute a command inside a pod container",
			InputSchema: createObjectSchema(map[string]*jsonschema.Schema{
				"pod_name": {
					Type:        "string",
					Description: "Name of the pod to execute command in",
				},
				"namespace": {
					Type:        "string",
					Description: "Namespace of the pod (default: default)",
					Default:     jsonString("default"),
				},
				"container": {
					Type:        "string",
					Description: "Container name (optional)",
				},
				"command": {
					Type: "array",
					Items: &jsonschema.Schema{
						Type: "string",
					},
					Description: "Command to execute as array of strings",
				},
			}, []string{"pod_name", "command"}),
		},
		"get_iptables_rules": {
			Name:        "get_iptables_rules",
			Description: "Get iptables rules from inside a pod",
			InputSchema: createObjectSchema(map[string]*jsonschema.Schema{
				"pod_name": {
					Type:        "string",
					Description: "Name of the pod to get iptables rules from",
				},
				"namespace": {
					Type:        "string",
					Description: "Namespace of the pod (default: default)",
					Default:     jsonString("default"),
				},
				"table": {
					Type:        "string",
					Description: "Iptables table to query (default: filter)",
					Default:     jsonString("filter"),
					Enum:        []interface{}{"filter", "nat", "mangle", "raw"},
				},
			}, []string{"pod_name"}),
		},
		"get_network_policies": {
			Name:        "get_network_policies",
			Description: "List Kubernetes network policies",
			InputSchema: createObjectSchema(map[string]*jsonschema.Schema{
				"namespace": {
					Type:        "string",
					Description: "Namespace to list network policies (default: all namespaces)",
				},
			}, nil),
		},
		"trace_network_path": {
			Name:        "trace_network_path",
			Description: "Trace network path between pods",
			InputSchema: createObjectSchema(map[string]*jsonschema.Schema{
				"source_pod": {
					Type:        "string",
					Description: "Name of the source pod",
				},
				"source_namespace": {
					Type:        "string",
					Description: "Namespace of the source pod (default: default)",
					Default:     jsonString("default"),
				},
				"target_ip": {
					Type:        "string",
					Description: "Target IP address to trace to",
				},
				"target_port": {
					Type:        "integer",
					Description: "Target port number (optional)",
					Minimum:     float64Ptr(1),
					Maximum:     float64Ptr(65535),
				},
			}, []string{"source_pod", "target_ip"}),
		},
	}
}

// Helper function for float64 pointers
func float64Ptr(f float64) *float64 {
	return &f
}
