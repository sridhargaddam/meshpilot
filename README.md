# MeshPilot MCP Server

MeshPilot is a Model Context Protocol (MCP) server designed for managing Kubernetes clusters with Istio service mesh capabilities. Built using the **official MCP Go SDK**, it provides tools for deploying, managing, and debugging Istio installations, sample applications, and network connectivity in both KIND and OpenShift clusters.

## Features

### 🚀 Cluster Management
- List and switch between Kubernetes contexts
- Get detailed cluster information
- Support for both KIND and OpenShift clusters

### 🕸️ Istio Service Mesh
- Install and uninstall Istio with different profiles
- Check Istio installation status and health
- Manage Istio components and configurations

### ⛵ Sail Operator
- Install and manage the Sail operator
- Monitor Sail operator status and health
- Automated RBAC and service account management

### 📦 Sample Applications
- Deploy sleep and httpbin sample applications
- Automatic Istio sidecar injection
- Easy cleanup and removal

### 🔗 Connectivity Testing
- Test connectivity between pods
- Specialized sleep-to-httpbin connectivity tests
- HTTP/HTTPS/TCP protocol support
- Detailed response analysis

### 📋 Logging & Debugging
- Retrieve pod logs with filtering and parsing
- Get Istio proxy (Envoy) logs
- Execute commands in pods
- Structured log analysis

### 🌐 Network Debugging
- Inspect iptables rules in pods
- Analyze network policies
- Network path tracing between pods
- Routing table and interface inspection

## Installation

### Prerequisites

- Go 1.23 or later (required for MCP SDK)
- Access to a Kubernetes cluster (KIND, OpenShift, etc.)
- `kubectl` configured with cluster access
- Docker (for KIND clusters)
- Helm 3.x (for Istio and Sail operator installations)

### Architecture

MeshPilot uses the **official MCP Go SDK** (`github.com/modelcontextprotocol/go-sdk`) for robust protocol compliance and future-proof compatibility. The architecture includes:

- **Official SDK Integration**: Full MCP 2025-06-18 specification compliance
- **Tool Wrapper System**: Seamless integration between existing tools and MCP protocol
- **Dual Mode Support**: Both CLI and MCP server modes in a single binary
- **Automatic Schema Generation**: JSON schemas automatically generated for all tools

### Build from Source

```bash
git clone <repository-url>
cd meshpilot
go mod tidy
go build -o meshpilot main.go
```

### Configuration

MeshPilot uses your existing kubeconfig for cluster access. Ensure your `kubectl` is properly configured:

```bash
kubectl config current-context
kubectl cluster-info
```

You can set the `KUBECONFIG` environment variable to use a specific kubeconfig file:

```bash
export KUBECONFIG=/path/to/your/kubeconfig
```

## Usage

MeshPilot can be used in three different modes:

### 1. As an MCP Server (Recommended)

Use with Claude CLI or other MCP clients:

```bash
# Add to Claude CLI
claude mcp add meshpilot /path/to/meshpilot/build/meshpilot

# Verify connection
claude mcp list

# Use in conversations with Claude
# Just ask Claude to: "Check my Istio status" or "List my Kubernetes contexts"
```

### 2. Direct Tool Execution

```bash
# Execute tools directly
./meshpilot --tool check_istio_status --args '{}'
./meshpilot --tool list_contexts --args '{}'

# Get help for specific tools
./meshpilot --tool-help check_istio_status
```

### 3. Interactive Server Mode

```bash
# Start as standalone server (for development/testing)
./meshpilot
```

The server automatically detects the mode:
- **MCP Mode**: When stdin is not a terminal (used by MCP clients)
- **Interactive Mode**: When run from a terminal with arguments
- **Server Mode**: When run from a terminal without arguments

### Available Tools

#### Cluster Management Tools

- `list_contexts` - List available Kubernetes contexts
- `switch_context` - Switch to a different Kubernetes context
- `get_cluster_info` - Get information about the current cluster

#### Istio Management Tools

- `install_istio` - Install Istio on the cluster
- `uninstall_istio` - Uninstall Istio from the cluster
- `check_istio_status` - Check Istio installation status

#### Sail Operator Tools

- `install_sail_operator` - Install Sail operator
- `uninstall_sail_operator` - Uninstall Sail operator
- `check_sail_status` - Check Sail operator status

#### Sample Application Tools

- `deploy_sleep_app` - Deploy sleep sample application
- `deploy_httpbin_app` - Deploy httpbin sample application
- `undeploy_sleep_app` - Remove sleep sample application
- `undeploy_httpbin_app` - Remove httpbin sample application

#### Connectivity Testing Tools

- `test_connectivity` - Test connectivity between pods
- `test_sleep_to_httpbin` - Test connectivity from sleep to httpbin

#### Logging and Debugging Tools

- `get_pod_logs` - Get logs from a specific pod
- `get_istio_proxy_logs` - Get Istio proxy logs from a pod
- `exec_pod_command` - Execute a command in a pod

#### Network Debugging Tools

- `get_iptables_rules` - Get iptables rules from a pod
- `get_network_policies` - Get network policies in a namespace
- `trace_network_path` - Trace network path between pods

## Example Workflows

### Setting Up a Complete Istio Environment

1. **Install Istio**:
   ```json
   {
     "tool": "install_istio",
     "arguments": {
       "profile": "minimal",
       "namespace": "istio-system"
     }
   }
   ```

2. **Check Installation Status**:
   ```json
   {
     "tool": "check_istio_status",
     "arguments": {
       "namespace": "istio-system"
     }
   }
   ```

3. **Deploy Sample Applications**:
   ```json
   {
     "tool": "deploy_sleep_app",
     "arguments": {
       "namespace": "default",
       "istio_injection": true
     }
   }
   ```
   
   ```json
   {
     "tool": "deploy_httpbin_app",
     "arguments": {
       "namespace": "default",
       "istio_injection": true
     }
   }
   ```

4. **Test Connectivity**:
   ```json
   {
     "tool": "test_sleep_to_httpbin",
     "arguments": {
       "source_namespace": "default",
       "target_namespace": "default"
     }
   }
   ```

### Debugging Network Issues

1. **Check Network Policies**:
   ```json
   {
     "tool": "get_network_policies",
     "arguments": {
       "namespace": "default",
       "pod_name": "sleep-xxx"
     }
   }
   ```

2. **Inspect iptables Rules**:
   ```json
   {
     "tool": "get_iptables_rules",
     "arguments": {
       "pod_name": "sleep-xxx",
       "namespace": "default",
       "container": "istio-proxy"
     }
   }
   ```

3. **Trace Network Path**:
   ```json
   {
     "tool": "trace_network_path",
     "arguments": {
       "source_pod": "sleep-xxx",
       "source_namespace": "default",
       "target_host": "httpbin.default.svc.cluster.local"
     }
   }
   ```

### Log Analysis

1. **Get Application Logs**:
   ```json
   {
     "tool": "get_pod_logs",
     "arguments": {
       "pod_name": "httpbin-xxx",
       "namespace": "default",
       "lines": 100,
       "parse_logs": true
     }
   }
   ```

2. **Get Istio Proxy Logs**:
   ```json
   {
     "tool": "get_istio_proxy_logs",
     "arguments": {
       "pod_name": "httpbin-xxx",
       "namespace": "default",
       "lines": 50
     }
   }
   ```

## Cluster Support

### KIND (Kubernetes in Docker)

MeshPilot works seamlessly with KIND clusters. To set up a KIND cluster:

```bash
kind create cluster --name meshpilot-test
kubectl config use-context kind-meshpilot-test
```

### OpenShift

For OpenShift clusters, ensure you have the proper permissions and are logged in:

```bash
oc login <cluster-url>
oc project <namespace>
```

## Tool Parameters

### Common Parameters

Most tools accept these common parameters:

- `namespace` - Kubernetes namespace (default: "default" or "istio-system" for Istio tools)
- `timeout` - Operation timeout in seconds
- `verbose` - Enable verbose output

### Specific Tool Parameters

Refer to the tool implementations in the `internal/tools/` directory for detailed parameter documentation.

## Architecture

```
meshpilot/
├── main.go                 # Entry point
├── internal/
│   ├── k8s/
│   │   └── client.go      # Kubernetes client management
│   ├── mcp/
│   │   └── server.go      # MCP server setup and tool registration
│   └── tools/
│       ├── manager.go     # Tool manager
│       ├── cluster.go     # Cluster management tools
│       ├── istio.go       # Istio management tools
│       ├── sail.go        # Sail operator tools
│       ├── sampleapps.go  # Sample application tools
│       ├── connectivity.go # Connectivity testing tools
│       ├── logging.go     # Logging and debugging tools
│       └── network.go     # Network debugging tools
├── go.mod
├── go.sum
└── README.md
```

## Error Handling

MeshPilot provides comprehensive error handling and logging:

- All errors are returned in MCP-compatible format
- Detailed error messages with context
- Structured logging using logrus
- Graceful handling of missing resources

## Security Considerations

- MeshPilot requires cluster-admin permissions for some operations
- Network debugging tools may require privileged access
- Always review the permissions and security implications before deployment
- Use appropriate RBAC policies in production environments
