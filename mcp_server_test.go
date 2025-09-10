package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// saveTestScreenshot saves a screenshot to the test results directory if the environment variable is set
func saveTestScreenshot(t *testing.T, screenshotData []byte) {
	// Check if we should save screenshots to disk for manual inspection
	if saveScreenshots := os.Getenv("SITECAP_SAVE_TEST_SCREENSHOTS"); saveScreenshots != "" {
		// Create test results directory
		testResultsDir := "test_results"
		if err := os.MkdirAll(testResultsDir, 0755); err != nil {
			t.Logf("Warning: Failed to create test results directory: %v", err)
			return
		}

		// Save screenshot to disk using test name
		screenshotPath := filepath.Join(testResultsDir, t.Name()+".png")
		if err := os.WriteFile(screenshotPath, screenshotData, 0644); err != nil {
			t.Logf("Warning: Failed to save screenshot to %s: %v", screenshotPath, err)
		} else {
			t.Logf("Screenshot saved to %s for manual inspection", screenshotPath)
		}
	}
}

// validateImageContent validates that the image content contains a valid PNG image and reports its dimensions
func validateImageContent(t *testing.T, imageContent *mcp.ImageContent) {
	if len(imageContent.Data) == 0 {
		t.Error("Expected image data, got empty data")
		return
	}
	if imageContent.MIMEType == "" {
		t.Error("Expected MIME type to be set")
		return
	}

	// Use image data directly (already raw bytes, not base64)
	imageBytes := imageContent.Data

	// Create reader from image bytes
	reader := bytes.NewReader(imageBytes)

	// Decode the image to verify it's valid and get dimensions
	img, format, err := image.Decode(reader)
	if err != nil {
		t.Errorf("Failed to decode image: %v", err)
		return
	}

	// Get image dimensions
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Verify dimensions are reasonable (greater than 0)
	if width <= 0 || height <= 0 {
		t.Errorf("Invalid image dimensions: %dx%d", width, height)
		return
	}

	// Log image details for debugging
	t.Logf("Valid %s image decoded: %dx%d pixels, MIME type: %s", format, width, height, imageContent.MIMEType)
}

// setupTestServer creates a test MCP server instance
func setupTestServer() *mcp.Server {
	// Initialize global managers (same as in RunMCPServer)
	configManager = NewContextConfigManager()
	requestManager = NewRequestHistoryManager()

	// Create the MCP server
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "sitecap-test",
		Version: "1.0.0-test",
	}, nil)

	registerTools(server)

	return server
}

// TestMCPServerInitialization tests that the server initializes correctly
func TestMCPServerInitialization(t *testing.T) {
	server := setupTestServer()

	if server == nil {
		t.Fatal("Server should not be nil")
	}

	// Verify that global managers are initialized
	if configManager == nil {
		t.Error("configManager should be initialized")
	}
	if requestManager == nil {
		t.Error("requestManager should be initialized")
	}
}

// TestMCPServerToolsList tests the tools/list endpoint using proper MCP client-server communication
func TestMCPServerToolsList(t *testing.T) {
	// Create in-memory transports for testing
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	// Setup test server
	server := setupTestServer()

	// Run server in background
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		err := server.Run(ctx, serverTransport)
		if err != nil && err != context.Canceled {
			t.Errorf("Server run error: %v", err)
		}
	}()

	// Create client and connect to server
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("Failed to connect client to server: %v", err)
	}
	defer session.Close()

	// Call tools/list
	toolsResult, err := session.ListTools(ctx, &mcp.ListToolsParams{})

	toolsJSON, err := json.MarshalIndent(toolsResult, "", "  ")
	if err != nil {
		t.Fatalf("Failed to serialize tools result: %v", err)
	}
	t.Logf("Tools List Result: %s", toolsJSON)

	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	// Verify all expected tools are present
	expectedTools := map[string]string{
		"configure_browser_context":    "Configure browser settings (viewport, timeout, cookies, headers) for a named browsing context. Use this to set up the browser environment before capturing screenshots or extracting content.",
		"list_browser_contexts":        "List all configured browser contexts with their settings. Use this to see available contexts and their configurations.",
		"capture_screenshot_from_url":  "Capture a screenshot of a webpage by navigating to the specified URL. Returns a base64-encoded PNG image. Supports viewport control, image resizing, and cookie management.",
		"capture_screenshot_from_html": "Capture a screenshot by rendering arbitrary HTML content in the browser. Useful for generating images from HTML templates or custom content. Returns a base64-encoded PNG image.",
		"extract_html_content":         "Extract the fully rendered HTML content from a webpage after JavaScript execution. Use this to get the final DOM state including dynamically generated content.",
		"get_last_browser_request":     "Retrieve details about the most recent browser request made in a specific context. Includes request/response data, cookies, network details, and console logs if requested.",
	}

	if len(toolsResult.Tools) != len(expectedTools) {
		t.Errorf("Expected %d tools, got %d", len(expectedTools), len(toolsResult.Tools))
	}

	// Check each tool
	foundTools := make(map[string]bool)
	for _, tool := range toolsResult.Tools {
		foundTools[tool.Name] = true

		expectedDesc, exists := expectedTools[tool.Name]
		if !exists {
			t.Errorf("Unexpected tool: %s", tool.Name)
			continue
		}

		if tool.Description != expectedDesc {
			t.Errorf("Tool %s: expected description %q, got %q", tool.Name, expectedDesc, tool.Description)
		}

		if tool.Name == "" {
			t.Errorf("Tool name should not be empty")
		}

		// Verify tool has input schema (should be auto-generated by AddTool)
		if tool.InputSchema == nil {
			t.Errorf("Tool %s should have input schema", tool.Name)
		}
	}

	// Verify all expected tools were found
	for expectedTool := range expectedTools {
		if !foundTools[expectedTool] {
			t.Errorf("Expected tool not found: %s", expectedTool)
		}
	}

	// Stop server
	cancel()
	wg.Wait()
}

// TestHTTPServer creates a test HTTP server that sets cookies
func createTestHTTPServer(t *testing.T) (string, func()) {
	// Create listener on random port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	baseURL := fmt.Sprintf("http://localhost:%d", port)

	// Create HTTP server with cookie-setting endpoints
	mux := http.NewServeMux()

	// Page that renders the cookies
	mux.HandleFunc("/cookies", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<!DOCTYPE html><html><head><title>Cookies Page</title></head><body><h1>Cookies</h1><ul>")
		for _, cookie := range r.Cookies() {
			fmt.Fprintf(w, "<li>%s: %s</li>", cookie.Name, cookie.Value)
		}
		fmt.Fprint(w, "</ul></body></html>")
	})

	// Main page that sets cookies
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Set multiple cookies using both http.SetCookie and raw headers to ensure they appear
		http.SetCookie(w, &http.Cookie{
			Name:     "session_id",
			Value:    "abc123xyz",
			Path:     "/",
			Domain:   "localhost",
			HttpOnly: true,
		})

		http.SetCookie(w, &http.Cookie{
			Name:   "user_pref",
			Value:  "dark_mode",
			Path:   "/",
			Domain: "localhost",
		})

		http.SetCookie(w, &http.Cookie{
			Name:   "analytics",
			Value:  "enabled",
			Path:   "/analytics",
			Domain: "localhost",
		})

		// Also add raw Set-Cookie headers to be absolutely sure they appear
		w.Header().Add("Set-Cookie", "test_cookie=test_value; Path=/; Domain=localhost")
		w.Header().Add("Set-Cookie", "another_cookie=another_value; Path=/; Domain=localhost")

		// Return HTML content
		html := `<!DOCTYPE html>
<html>
<head>
    <title>Test Page with Cookies</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .cookie-info { background: #f0f0f0; padding: 20px; margin: 20px 0; }
        #status { color: green; font-weight: bold; }
    </style>
</head>
<body>
    <h1>Cookie Test Page</h1>
    <div class="cookie-info">
        <h2>Cookies Set</h2>
        <ul>
            <li>session_id=abc123xyz (HttpOnly, Domain=localhost, Path=/)</li>
            <li>user_pref=dark_mode (Domain=localhost, Path=/)</li>
            <li>analytics=enabled (Domain=localhost, Path=/analytics)</li>
        </ul>
    </div>
    <div id="status">✓ Page loaded successfully with cookies</div>
    <script>
        // Add some dynamic content to verify JavaScript execution
        document.addEventListener('DOMContentLoaded', function() {
            const status = document.getElementById('status');
            status.textContent = '✓ JavaScript executed and cookies set';
        });
    </script>
</body>
</html>`

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, html)
	})

	// Start server in goroutine
	server := &http.Server{Handler: mux}
	go func() {
		server.Serve(listener)
	}()

	// Return cleanup function
	cleanup := func() {
		server.Close()
		listener.Close()
	}

	return baseURL, cleanup
}

func TestMCPServerHTMLToScreenshot(t *testing.T) {
	// Create test HTTP server
	serverURL, cleanup := createTestHTTPServer(t)
	defer cleanup()

	// Wait briefly for server to be ready
	time.Sleep(100 * time.Millisecond)

	// Create HTML content that references our test server (simulating external resource)
	htmlContent := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>HTML Screenshot Test</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; background: #f5f5f5; }
        .test-container { background: white; padding: 30px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .server-info { background: #e3f2fd; padding: 15px; margin: 15px 0; border-radius: 4px; }
    </style>
</head>
<body>
    <h1>MCP HTML Screenshot Test</h1>
    <p>Test server running at: %s</p>
    <p>Current timestamp: <span id="timestamp"><strong>TO BE REPLACED BY JAVASCRIPT</strong></span></p>
    <script>
        document.getElementById('timestamp').textContent = new Date().toISOString();
    </script>
</body>
</html>`, serverURL)

	// Setup MCP server for testing
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	server := setupTestServer()

	// Run server in background
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		err := server.Run(ctx, serverTransport)
		if err != nil && err != context.Canceled {
			t.Errorf("Server run error: %v", err)
		}
	}()

	// Create client and connect to server
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("Failed to connect client to server: %v", err)
	}
	defer session.Close()

	// render the HTML to screenshot using default context
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "capture_screenshot_from_html",
		Arguments: map[string]interface{}{
			"html_content": htmlContent,
		},
	})

	if err != nil {
		t.Fatalf("capture_screenshot_from_html tool call failed: %v", err)
	}

	// Verify the response structure
	if len(result.Content) == 0 {
		t.Fatal("Expected response content, got empty")
	}

	// Check that we got an image content response
	content := result.Content[0]
	if imageContent, ok := content.(*mcp.ImageContent); ok {
		// Validate that the result contains a valid image with proper dimensions
		validateImageContent(t, imageContent)
	} else {
		t.Fatal("Expected ImageContent response from screenshot_html")
	}

	testContextName := "default" // screenshot_html used default context
	context, exists := configManager.GetContext(testContextName)
	if !exists {
		t.Fatal("Expected test context to exist after screenshot_html call")
	}

	// Verify context has request history
	if context.LastRequestID == "" {
		t.Error("Expected LastRequestID to be set after screenshot_html call")
	}

	if len(context.RequestHistory) == 0 {
		t.Error("Expected request history to be populated")
	}

	// Verify the stored request contains the expected data
	lastRequest, exists := requestManager.GetRequest(context.LastRequestID)

	if !exists {
		t.Fatal("Expected to find stored request in request manager")
	}

	if lastRequest.RequestType != "screenshot_html" {
		t.Errorf("Expected request type 'screenshot_html', got %s", lastRequest.RequestType)
	}

	if lastRequest.ContextName != testContextName {
		t.Errorf("Expected context name %s, got %s", testContextName, lastRequest.ContextName)
	}

	// Check for HTML content in stored request
	if lastRequest.Response == nil || lastRequest.Response.HTML == nil || *lastRequest.Response.HTML == "" {
		t.Error("Expected HTML content to be stored in request")
	}

	// Verify screenshot data is stored
	if lastRequest.Response == nil || len(lastRequest.Response.Screenshot) == 0 {
		t.Error("Expected screenshot data to be stored in request")
	} else {
		// Save screenshot to disk if environment variable is set
		saveTestScreenshot(t, lastRequest.Response.Screenshot)
	}

	// Stop server
	cancel()
	wg.Wait()
}

// TestMCPServerCookieUpdates tests the update_cookies functionality with actual HTTP requests
func TestMCPServerCookieUpdates(t *testing.T) {
	// Create test HTTP server that sets cookies
	serverURL, cleanup := createTestHTTPServer(t)
	defer cleanup()

	// Wait briefly for server to be ready
	time.Sleep(100 * time.Millisecond)

	// Setup MCP server for testing
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	server := setupTestServer()

	// Run server in background
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		err := server.Run(ctx, serverTransport)
		if err != nil && err != context.Canceled {
			t.Errorf("Server run error: %v", err)
		}
	}()

	// Create client and connect to server
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("Failed to connect client to server: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "capture_screenshot_from_url",
		Arguments: map[string]interface{}{
			"url":            serverURL,
			"update_cookies": true,
		},
	})

	if err != nil {
		t.Fatalf("capture_screenshot_from_url tool call failed: %v", err)
	}

	// Verify successful response
	if len(result.Content) == 0 {
		t.Fatal("Expected response content, got empty")
	}

	if imageContent, ok := result.Content[0].(*mcp.ImageContent); ok {
		// Validate that the result contains a valid image with proper dimensions
		validateImageContent(t, imageContent)
	} else {
		t.Fatal("Expected ImageContent response from screenshot_url")
	}

	// Verify that cookies were updated in the context
	contextName := "default"
	context, exists := configManager.GetContext(contextName)

	if !exists {
		t.Fatal("missing default context")
	}

	// Verify the last request object contains the cookies
	lastRequest, exists := requestManager.GetRequest(context.LastRequestID)
	if !exists {
		t.Fatal("Expected to find stored request in request manager")
	}

	if lastRequest.Response == nil {
		t.Fatal("Expected response data in stored request")
	}

	// Verify the captured cookies from the browser response
	// Note: Only cookies with path "/" or that match the current path will be captured
	// The analytics cookie with path="/analytics" won't be included since we're visiting "/"
	expectedCookies := []struct {
		name     string
		value    string
		domain   string
		path     string
		httpOnly bool
	}{
		{"session_id", "abc123xyz", "localhost", "/", true},
		{"user_pref", "dark_mode", "localhost", "/", false},
		{"test_cookie", "test_value", "localhost", "/", false},
		{"another_cookie", "another_value", "localhost", "/", false},
	}

	if len(lastRequest.Response.Cookies) != len(expectedCookies) {
		t.Errorf("Expected %d cookies in response, got %d", len(expectedCookies), len(lastRequest.Response.Cookies))
	}

	// Verify each expected cookie is present in the response
	cookieMap := make(map[string]*proto.NetworkCookie)
	for _, cookie := range lastRequest.Response.Cookies {
		cookieMap[cookie.Name] = cookie
	}

	for _, expected := range expectedCookies {
		cookie, found := cookieMap[expected.name]
		if !found {
			t.Errorf("Expected cookie %s not found in response", expected.name)
			continue
		}

		if cookie.Value != expected.value {
			t.Errorf("Cookie %s: expected value %s, got %s", expected.name, expected.value, cookie.Value)
		}

		if cookie.Domain != expected.domain {
			t.Errorf("Cookie %s: expected domain %s, got %s", expected.name, expected.domain, cookie.Domain)
		}

		if cookie.Path != expected.path {
			t.Errorf("Cookie %s: expected path %s, got %s", expected.name, expected.path, cookie.Path)
		}

		if cookie.HTTPOnly != expected.httpOnly {
			t.Errorf("Cookie %s: expected httpOnly %v, got %v", expected.name, expected.httpOnly, cookie.HTTPOnly)
		}
	}

	// Verify context.Cookies has been updated with the resulting cookies
	if len(context.Cookies) == 0 {
		t.Fatal("Expected context cookies to be updated, but got empty slice")
	}

	// Create a map of context cookies for easier verification
	contextCookieMap := make(map[string]*proto.NetworkCookieParam)
	for _, cookie := range context.Cookies {
		contextCookieMap[cookie.Name] = cookie
	}

	// Verify each expected cookie is present in the context
	for _, expected := range expectedCookies {
		contextCookie, found := contextCookieMap[expected.name]
		if !found {
			t.Errorf("Expected cookie %s not found in context cookies", expected.name)
			continue
		}

		if contextCookie.Value != expected.value {
			t.Errorf("Context cookie %s: expected value %s, got %s", expected.name, expected.value, contextCookie.Value)
		}

		if contextCookie.Domain != expected.domain {
			t.Errorf("Context cookie %s: expected domain %s, got %s", expected.name, expected.domain, contextCookie.Domain)
		}

		if contextCookie.Path != expected.path {
			t.Errorf("Context cookie %s: expected path %s, got %s", expected.name, expected.path, contextCookie.Path)
		}

		if contextCookie.HTTPOnly != expected.httpOnly {
			t.Errorf("Context cookie %s: expected httpOnly %v, got %v", expected.name, expected.httpOnly, contextCookie.HTTPOnly)
		}
	}

	// Stop server
	cancel()
	wg.Wait()
}

// TestMCPServerContextCookieTransmission tests that context cookies are properly sent to the test server
func TestMCPServerContextCookieTransmission(t *testing.T) {
	// Create test HTTP server that can display received cookies
	serverURL, cleanup := createTestHTTPServer(t)
	defer cleanup()

	// Wait briefly for server to be ready
	time.Sleep(100 * time.Millisecond)

	// Setup MCP server for testing
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	server := setupTestServer()

	// Run server in background
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		err := server.Run(ctx, serverTransport)
		if err != nil && err != context.Canceled {
			t.Errorf("Server run error: %v", err)
		}
	}()

	// Create client and connect to server
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("Failed to connect client to server: %v", err)
	}
	defer session.Close()

	// Configure a test context with predefined cookies
	testContextName := "cookie_test_context"
	configArgs := map[string]interface{}{
		"context_name": testContextName,
		"viewport":     "1280x720",
		"timeout":      20,
		"cookies": []map[string]interface{}{
			{
				"name":   "auth_token",
				"value":  "xyz123",
				"domain": "localhost",
				"path":   "/",
			},
			{
				"name":   "user_id",
				"value":  "456",
				"domain": "localhost",
				"path":   "/",
			},
			{
				"name":   "session",
				"value":  "active",
				"domain": "localhost",
				"path":   "/",
			},
		},
	}

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "configure_browser_context",
		Arguments: configArgs,
	})
	if err != nil {
		t.Fatalf("configure_browser_context tool call failed: %v", err)
	}

	// Make a screenshot request to the /cookies endpoint using the configured context
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "capture_screenshot_from_url",
		Arguments: map[string]interface{}{
			"url":          serverURL + "/cookies",
			"context_name": testContextName,
		},
	})

	if err != nil {
		t.Fatalf("capture_screenshot_from_url tool call failed: %v", err)
	}

	// Verify successful response
	if len(result.Content) == 0 {
		t.Fatal("Expected response content, got empty")
	}

	if imageContent, ok := result.Content[0].(*mcp.ImageContent); ok {
		// Validate that the result contains a valid image with proper dimensions
		validateImageContent(t, imageContent)
	} else {
		t.Fatal("Expected ImageContent response from screenshot_url")
	}

	// Get the context and verify the request was made
	context, exists := configManager.GetContext(testContextName)
	if !exists {
		t.Fatal("Expected test context to exist")
	}

	// Get the last request to verify HTML content
	lastRequest, exists := requestManager.GetRequest(context.LastRequestID)
	if !exists {
		t.Fatal("Expected to find stored request in request manager")
	}

	if lastRequest.Response == nil || lastRequest.Response.HTML == nil {
		t.Fatal("Expected HTML content in response")
	}

	// Parse the HTML from the /cookies endpoint to verify our cookies were sent
	htmlContent := *lastRequest.Response.HTML

	// Expected cookies that should appear in the HTML
	expectedCookies := []struct {
		name  string
		value string
	}{
		{"auth_token", "xyz123"},
		{"user_id", "456"},
		{"session", "active"},
	}

	// Verify each expected cookie appears in the HTML
	for _, expected := range expectedCookies {
		expectedPattern := fmt.Sprintf("<li>%s: %s</li>", expected.name, expected.value)
		if !strings.Contains(htmlContent, expectedPattern) {
			t.Errorf("Expected cookie pattern %q not found in HTML content", expectedPattern)
		}
	}

	// Also verify that the page shows the expected structure
	if !strings.Contains(htmlContent, "<h1>Cookies</h1>") {
		t.Error("Expected cookies page header not found")
	}

	// Stop server
	cancel()
	wg.Wait()
}

// TestMCPServerConfigureContextPartialUpdate tests the new fetch-or-create logic for context configuration
func TestMCPServerConfigureContextPartialUpdate(t *testing.T) {
	// Setup MCP server for testing
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	server := setupTestServer()

	// Run server in background
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		err := server.Run(ctx, serverTransport)
		if err != nil && err != context.Canceled {
			t.Errorf("Server run error: %v", err)
		}
	}()

	// Create client and connect to server
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("Failed to connect client to server: %v", err)
	}
	defer session.Close()

	testContextName := "partial_update_test"

	// Step 1: Create new context with only viewport specified
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "configure_browser_context",
		Arguments: map[string]interface{}{
			"context_name": testContextName,
			"viewport":     "1920x1080",
			// timeout, domains, cookies, headers are nil - should use defaults
		},
	})
	if err != nil {
		t.Fatalf("First configure_browser_context call failed: %v", err)
	}

	// Verify context was created with defaults for unspecified fields
	config1, exists := configManager.GetContext(testContextName)
	if !exists {
		t.Fatal("Expected context to be created")
	}

	// Check viewport was set
	if config1.DefaultViewport.Width != 1920 || config1.DefaultViewport.Height != 1080 {
		t.Errorf("Expected viewport 1920x1080, got %dx%d", config1.DefaultViewport.Width, config1.DefaultViewport.Height)
	}

	// Check defaults were used for unspecified fields
	defaultConfig := DefaultBrowserContextConfig()
	if config1.DefaultTimeout != defaultConfig.DefaultTimeout {
		t.Errorf("Expected default timeout %d, got %d", defaultConfig.DefaultTimeout, config1.DefaultTimeout)
	}

	// Step 2: Update same context with only timeout specified
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "configure_browser_context",
		Arguments: map[string]interface{}{
			"context_name": testContextName,
			"timeout":      60,
			// viewport, domains, cookies, headers are nil - should preserve existing values
		},
	})
	if err != nil {
		t.Fatalf("Second configure_browser_context call failed: %v", err)
	}

	// Verify context was updated
	config2, exists := configManager.GetContext(testContextName)
	if !exists {
		t.Fatal("Expected context to still exist")
	}

	// Check that timeout was updated
	if config2.DefaultTimeout != 60 {
		t.Errorf("Expected timeout to be updated to 60, got %d", config2.DefaultTimeout)
	}

	// Check that viewport was preserved
	if config2.DefaultViewport.Width != 1920 || config2.DefaultViewport.Height != 1080 {
		t.Errorf("Expected viewport to be preserved as 1920x1080, got %dx%d", config2.DefaultViewport.Width, config2.DefaultViewport.Height)
	}

	// Step 3: Test with empty context name (should use "default")
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "configure_browser_context",
		Arguments: map[string]interface{}{
			"viewport": "800x600",
		},
	})
	if err != nil {
		t.Fatalf("Third configure_browser_context call failed: %v", err)
	}

	// Verify default context was updated
	defaultContext, exists := configManager.GetContext("default")
	if !exists {
		t.Fatal("Expected default context to exist")
	}
	if defaultContext.DefaultViewport.Width != 800 || defaultContext.DefaultViewport.Height != 600 {
		t.Errorf("Expected default context viewport 800x600, got %dx%d", defaultContext.DefaultViewport.Width, defaultContext.DefaultViewport.Height)
	}

	// Stop server
	cancel()
	wg.Wait()
}

// TestMCPServerConfigureContextNullableFields tests individual nullable field handling
func TestMCPServerConfigureContextNullableFields(t *testing.T) {
	// Setup MCP server for testing
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	server := setupTestServer()

	// Run server in background
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		err := server.Run(ctx, serverTransport)
		if err != nil && err != context.Canceled {
			t.Errorf("Server run error: %v", err)
		}
	}()

	// Create client and connect to server
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("Failed to connect client to server: %v", err)
	}
	defer session.Close()

	testContextName := "nullable_fields_test"

	// Create initial context with all fields
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "configure_browser_context",
		Arguments: map[string]interface{}{
			"context_name": testContextName,
			"viewport":     "1024x768",
			"timeout":      45,
			"domains":      "initial.com",
		},
	})
	if err != nil {
		t.Fatalf("Initial configure_browser_context call failed: %v", err)
	}

	// Test: Call with no nullable fields specified - should preserve all existing values
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "configure_browser_context",
		Arguments: map[string]interface{}{
			"context_name": testContextName,
			// All nullable fields omitted
		},
	})
	if err != nil {
		t.Fatalf("Preserve test configure_browser_context call failed: %v", err)
	}

	preservedConfig, _ := configManager.GetContext(testContextName)
	if preservedConfig.DefaultViewport.Width != 1024 || preservedConfig.DefaultViewport.Height != 768 {
		t.Errorf("Expected preserved viewport 1024x768, got %dx%d", preservedConfig.DefaultViewport.Width, preservedConfig.DefaultViewport.Height)
	}
	if preservedConfig.DefaultTimeout != 45 {
		t.Errorf("Expected preserved timeout 45, got %d", preservedConfig.DefaultTimeout)
	}
	if len(preservedConfig.DomainWhitelist) != 1 || preservedConfig.DomainWhitelist[0] != "initial.com" {
		t.Errorf("Expected preserved domains [initial.com], got %v", preservedConfig.DomainWhitelist)
	}

	// Stop server
	cancel()
	wg.Wait()
}

// TestMCPServerConfigureClearVsPreserveSemantics tests the distinction between clearing and preserving fields
func TestMCPServerConfigureClearVsPreserveSemantics(t *testing.T) {
	// Setup MCP server for testing
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	server := setupTestServer()

	// Run server in background
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		err := server.Run(ctx, serverTransport)
		if err != nil && err != context.Canceled {
			t.Errorf("Server run error: %v", err)
		}
	}()

	// Create client and connect to server
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("Failed to connect client to server: %v", err)
	}
	defer session.Close()

	testContextName := "clear_vs_preserve_test"

	// Step 1: Create context with all fields populated
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "configure_browser_context",
		Arguments: map[string]interface{}{
			"context_name": testContextName,
			"viewport":     "1024x768",
			"timeout":      45,
			"domains":      "example.com,test.com",
			"cookies": []interface{}{
				map[string]interface{}{
					"name":  "test_cookie",
					"value": "test_value",
				},
			},
			"headers": map[string]interface{}{
				"Authorization": "Bearer token",
				"Custom-Header": "custom-value",
			},
		},
	})
	if err != nil {
		t.Fatalf("Initial setup configure_browser_context call failed: %v", err)
	}

	// Verify initial state
	_, exists := configManager.GetContext(testContextName)
	if !exists {
		t.Fatal("Expected context to exist after creation")
	}

	// Step 2: Test clearing cookies with empty slice
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "configure_browser_context",
		Arguments: map[string]interface{}{
			"context_name": testContextName,
			"cookies":      []interface{}{}, // Empty slice should clear cookies
		},
	})
	if err != nil {
		t.Fatalf("Clear cookies configure_browser_context call failed: %v", err)
	}

	configAfterClearCookies, _ := configManager.GetContext(testContextName)
	if len(configAfterClearCookies.Cookies) > 0 {
		t.Errorf("Expected cookies to be cleared (nil), got %v", configAfterClearCookies.Cookies)
	}

	// Verify other fields were preserved
	if configAfterClearCookies.DefaultTimeout != 45 {
		t.Errorf("Expected timeout to be preserved as 45, got %d", configAfterClearCookies.DefaultTimeout)
	}
	if len(configAfterClearCookies.DomainWhitelist) != 2 {
		t.Errorf("Expected domains to be preserved as 2 entries, got %d", len(configAfterClearCookies.DomainWhitelist))
	}

	// Step 3: Test clearing domains with empty string
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "configure_browser_context",
		Arguments: map[string]interface{}{
			"context_name": testContextName,
			"domains":      "", // Empty string should clear domains
		},
	})
	if err != nil {
		t.Fatalf("Clear domains configure_browser_context call failed: %v", err)
	}

	configAfterClearDomains, _ := configManager.GetContext(testContextName)
	if configAfterClearDomains.DomainWhitelist != nil {
		t.Errorf("Expected domains to be cleared (nil), got %v", configAfterClearDomains.DomainWhitelist)
	}

	// Verify timeout was still preserved
	if configAfterClearDomains.DefaultTimeout != 45 {
		t.Errorf("Expected timeout to still be preserved as 45, got %d", configAfterClearDomains.DefaultTimeout)
	}

	// Step 4: Test clearing headers with empty map
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "configure_browser_context",
		Arguments: map[string]interface{}{
			"context_name": testContextName,
			"headers":      map[string]interface{}{}, // Empty map should clear headers
		},
	})
	if err != nil {
		t.Fatalf("Clear headers configure_browser_context call failed: %v", err)
	}

	configAfterClearHeaders, _ := configManager.GetContext(testContextName)
	if len(configAfterClearHeaders.Headers) != 0 {
		t.Errorf("Expected headers to be cleared (empty map), got %v", configAfterClearHeaders.Headers)
	}

	// Step 5: Test preserving existing values by omitting fields entirely
	// First, repopulate some fields
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "configure_browser_context",
		Arguments: map[string]interface{}{
			"context_name": testContextName,
			"domains":      "newdomain.com",
			"cookies": []interface{}{
				map[string]interface{}{
					"name":  "new_cookie",
					"value": "new_value",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Repopulate configure_browser_context call failed: %v", err)
	}

	// Now test preservation by omitting all nullable fields
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "configure_browser_context",
		Arguments: map[string]interface{}{
			"context_name": testContextName,
			// All nullable fields omitted - should preserve existing values
		},
	})
	if err != nil {
		t.Fatalf("Preserve test configure_browser_context call failed: %v", err)
	}

	finalConfig, _ := configManager.GetContext(testContextName)

	// Verify all values were preserved
	if len(finalConfig.DomainWhitelist) != 1 || finalConfig.DomainWhitelist[0] != "newdomain.com" {
		t.Errorf("Expected domains to be preserved as [newdomain.com], got %v", finalConfig.DomainWhitelist)
	}
	if finalConfig.Cookies == nil || len(finalConfig.Cookies) != 1 {
		t.Errorf("Expected cookies to be preserved with 1 entry, got %v", finalConfig.Cookies)
	}
	if finalConfig.DefaultTimeout != 45 {
		t.Errorf("Expected timeout to be preserved as 45, got %d", finalConfig.DefaultTimeout)
	}

	// Stop server
	cancel()
	wg.Wait()
}
