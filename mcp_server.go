package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/go-rod/rod/lib/proto"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Global managers for the MCP server
var (
	configManager  *ContextConfigManager
	requestManager *RequestHistoryManager
)

func StartMCPServer() {
	log.Println("Starting Sitecap MCP server...")

	// Initialize managers
	configManager = NewContextConfigManager()
	requestManager = NewRequestHistoryManager()

	// Create default empty browsing context
	defaultConfig := &BrowserContextConfig{
		Name:              "default",
		DefaultViewport:   ViewportConfig{Width: 1920, Height: 1080},
		DefaultTimeout:    30,
		DomainWhitelist:   []string{},
		Cookies:           []*proto.NetworkCookieParam{},
		Headers:           map[string]string{},
		UserAgent:         "",
		JavaScriptEnabled: true,
		RequestHistory:    []string{},
	}
	configManager.CreateOrUpdateContext("default", defaultConfig)
	log.Println("Created default empty browsing context")

	// Create the MCP server
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "sitecap",
		Title:   "Sitecap Website Screenshot Tool",
		Version: "1.0.0",
	}, nil)

	// Register all tools using the generic AddTool function
	registerTools(server)

	// Run the server with stdio transport
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
		os.Exit(1)
	}
}

// registerTools registers all MCP tools
func registerTools(server *mcp.Server) {
	// Configuration management tools
	mcp.AddTool(server, &mcp.Tool{
		Name:        "configure_context",
		Description: "Configure browser settings for a named context",
	}, handleConfigureContext)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_contexts",
		Description: "List all browser contexts",
	}, handleListContexts)

	// Page operation tools
	mcp.AddTool(server, &mcp.Tool{
		Name:        "screenshot_url",
		Description: "Take a screenshot of a webpage from URL",
	}, handleMCPScreenshot)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "screenshot_html",
		Description: "Take a screenshot by rendering arbitrary HTML content",
	}, handleMCPScreenshotHTML)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_html",
		Description: "Get rendered HTML content from a webpage",
	}, handleMCPGetHTML)

	// Request history tools
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_last_request",
		Description: "Get details about the last request made in a browser context",
	}, handleGetLastRequest)
}
