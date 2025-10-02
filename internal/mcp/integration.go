package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"meshpilot/internal/tools"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolWrapper wraps our existing tool manager to work with the MCP SDK
type ToolWrapper struct {
	manager *tools.Manager
}

// NewToolWrapper creates a new tool wrapper
func NewToolWrapper(manager *tools.Manager) *ToolWrapper {
	return &ToolWrapper{
		manager: manager,
	}
}

// WrapTool creates an MCP tool handler that wraps our existing tool functions
func (tw *ToolWrapper) WrapTool(toolName string) mcp.ToolHandler {
	return func(ctx context.Context, ss *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]any]) (*mcp.CallToolResultFor[any], error) {
		// Convert arguments to JSON
		argsJSON, err := json.Marshal(params.Arguments)
		if err != nil {
			return &mcp.CallToolResultFor[any]{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "Failed to marshal arguments: " + err.Error()},
				},
				IsError: true,
			}, nil
		}

		// Call our existing tool
		result, err := tw.manager.ExecuteTool(toolName, argsJSON)
		if err != nil {
			return &mcp.CallToolResultFor[any]{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "Tool execution failed: " + err.Error()},
				},
				IsError: true,
			}, nil
		}

		// Convert our result to MCP format
		mcpResult := &mcp.CallToolResultFor[any]{
			IsError: result.IsError,
		}

		// Convert content
		for _, content := range result.Content {
			if textContent, ok := content.(tools.TextContent); ok {
				mcpResult.Content = append(mcpResult.Content, &mcp.TextContent{
					Text: textContent.Text,
				})
			} else {
				// Fallback: convert to string
				contentStr := ""
				if str, ok := content.(string); ok {
					contentStr = str
				} else {
					// Try to marshal to JSON string
					if bytes, err := json.Marshal(content); err == nil {
						contentStr = string(bytes)
					} else {
						contentStr = fmt.Sprintf("%v", content)
					}
				}
				mcpResult.Content = append(mcpResult.Content, &mcp.TextContent{
					Text: contentStr,
				})
			}
		}

		return mcpResult, nil
	}
}

// RegisterAllTools registers all available tools with the MCP server using proper schemas
func (tw *ToolWrapper) RegisterAllTools(server *mcp.Server) {
	toolDefs := GetToolDefinitions()

	// Register all tools with their proper schemas
	for toolName, toolDef := range toolDefs {
		server.AddTool(toolDef, tw.WrapTool(toolName))
	}
}
