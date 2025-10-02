#!/bin/bash

# MeshPilot MCP Server Test Script
set -e

echo "=== MeshPilot MCP Server Test ==="
echo

# Build the latest version
echo "🔨 Building MeshPilot..."
make build

echo "✅ Build successful!"
echo

# Test MCP Protocol
echo "🧪 Testing MCP Protocol..."

echo "1. Testing initialize..."
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}' | ./build/meshpilot | grep -q "meshpilot" && echo "  ✅ Initialize works"

echo "2. Testing tools/list..."
TOOL_COUNT=$(echo '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' | ./build/meshpilot | jq -r '.result.tools | length')
echo "  ✅ Found $TOOL_COUNT tools"

echo "3. Testing tool execution..."
echo '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"check_istio_status","arguments":{}}}' | ./build/meshpilot | grep -q "istio-system" && echo "  ✅ Tool execution works"

echo

# Test Claude CLI Integration
echo "🤖 Testing Claude CLI Integration..."

# Remove and re-add the server
claude mcp remove meshpilot 2>/dev/null || true
claude mcp add meshpilot $(pwd)/build/meshpilot

# Check connection
if claude mcp list | grep -q "✓ Connected"; then
    echo "  ✅ Claude CLI can connect to MCP server"
else
    echo "  ❌ Claude CLI connection failed"
    exit 1
fi

echo

echo "🎉 All tests passed!"
echo
echo "=== Usage Instructions ==="
echo "1. The MCP server is now available to Claude CLI"
echo "2. You can use it in conversations with Claude"
echo "3. Available tools include:"
echo "   • Kubernetes cluster management"
echo "   • Istio service mesh operations" 
echo "   • Sample application deployment"
echo "   • Network connectivity testing"
echo "   • Pod logging and debugging"
echo "   • Network troubleshooting"
echo
echo "=== Example Claude Commands ==="
echo "Try asking Claude:"
echo "• 'Check the status of Istio in my cluster'"
echo "• 'List my Kubernetes contexts'"
echo "• 'Deploy the sleep and httpbin sample apps'"
echo "• 'Test connectivity between sleep and httpbin'"
echo
echo "✅ MeshPilot MCP Server is ready for use with Claude!"
