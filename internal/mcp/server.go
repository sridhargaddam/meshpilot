package mcp

import (
	"context"
	"os"

	"meshpilot/internal/tools"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"
)

// Server wraps the official MCP SDK server
type Server struct {
	mcpServer   *mcp.Server
	toolWrapper *ToolWrapper
}

// NewServer creates a new MCP server using the official SDK
func NewServer(name, version string, toolManager *tools.Manager) *Server {
	// Create server implementation
	impl := &mcp.Implementation{
		Name:    name,
		Version: version,
	}

	// Create server with options
	opts := &mcp.ServerOptions{
		Instructions: "MeshPilot MCP Server - Kubernetes and Istio service mesh management tools",
	}

	mcpServer := mcp.NewServer(impl, opts)

	// Create tool wrapper
	toolWrapper := NewToolWrapper(toolManager)

	// Register all tools
	toolWrapper.RegisterAllTools(mcpServer)

	return &Server{
		mcpServer:   mcpServer,
		toolWrapper: toolWrapper,
	}
}

// Serve starts the MCP server using stdio transport
func (s *Server) Serve(ctx context.Context) error {
	// Disable logrus output to avoid interfering with MCP protocol
	logrus.SetOutput(os.Stderr)
	logrus.SetLevel(logrus.ErrorLevel)

	// Create stdio transport with logging to stderr
	transport := mcp.NewLoggingTransport(mcp.NewStdioTransport(), os.Stderr)

	// Run the server
	return s.mcpServer.Run(ctx, transport)
}
