package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Global managers for the MCP server
var (
	configManager  *ContextConfigManager
	requestManager *RequestHistoryManager
)

func StartMCPServer() {
	log.Println("Starting Sitecap MCP server...")

	configManager = NewContextConfigManager()
	requestManager = NewRequestHistoryManager()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "sitecap",
		Title:   "Sitecap Website Screenshot Tool",
		Version: "1.0.0",
	}, nil)

	registerTools(server)

	// Run the server with stdio transport
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
		os.Exit(1)
	}
}

// registerTools registers all MCP tools
func registerTools(server *mcp.Server) {
	// Browser context management tools
	mcp.AddTool(server, &mcp.Tool{
		Name:        "configure_browser_context",
		Description: "Configure browser settings (viewport, timeout, cookies, headers) for a named browsing context. Use this to set up the browser environment before capturing screenshots or extracting content.",
	}, handleConfigureContext)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_browser_contexts",
		Description: "List all configured browser contexts with their settings. Use this to see available contexts and their configurations.",
	}, handleListContexts)

	// Screenshot capture tools
	mcp.AddTool(server, &mcp.Tool{
		Name:        "capture_screenshot_from_url",
		Description: "Capture a screenshot of a webpage by navigating to the specified URL. Returns a base64-encoded PNG image. Supports viewport control, image resizing, and cookie management.",
	}, handleMCPScreenshot)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "capture_screenshot_from_html",
		Description: "Capture a screenshot by rendering arbitrary HTML content in the browser. Useful for generating images from HTML templates or custom content. Returns a base64-encoded PNG image.",
	}, handleMCPScreenshotHTML)

	// Content extraction tools
	mcp.AddTool(server, &mcp.Tool{
		Name:        "extract_html_content",
		Description: "Extract the fully rendered HTML content from a webpage after JavaScript execution. Use this to get the final DOM state including dynamically generated content.",
	}, handleMCPGetHTML)

	// Request history tools
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_last_browser_request",
		Description: "Retrieve details about the most recent browser request made in a specific context. Includes request/response data, cookies, network details, and console logs if requested.",
	}, handleGetLastRequest)
}
