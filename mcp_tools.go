package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod/lib/proto"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool argument structures with JSON schema tags

type CookieInput struct {
	Name     string `json:"name" jsonschema:"cookie name"`
	Value    string `json:"value" jsonschema:"cookie value"`
	Domain   string `json:"domain,omitempty" jsonschema:"cookie domain"`
	Path     string `json:"path,omitempty" jsonschema:"cookie path (default: '/')"`
	Expires  int64  `json:"expires,omitempty" jsonschema:"expiration timestamp"`
	HTTPOnly bool   `json:"httpOnly,omitempty" jsonschema:"HTTP-only flag"`
	Secure   bool   `json:"secure,omitempty" jsonschema:"secure flag"`
	SameSite string `json:"sameSite,omitempty" jsonschema:"SameSite attribute: 'strict', 'lax', or 'none'"`
}

type ConfigureContextArgs struct {
	ContextName       string            `json:"context_name,omitempty" jsonschema:"name of the browser context (default: 'default')"`
	Viewport          string            `json:"viewport,omitempty" jsonschema:"viewport dimensions like '1920x1080' (default: '1920x1080')"`
	Timeout           int               `json:"timeout,omitempty" jsonschema:"timeout in seconds for page loads (default: 30)"`
	Domains           string            `json:"domains,omitempty" jsonschema:"comma-separated list of allowed domains for request filtering"`
	Cookies           []CookieInput     `json:"cookies,omitempty" jsonschema:"array of cookie objects to set in the browser context"`
	Headers           map[string]string `json:"headers,omitempty" jsonschema:"default HTTP headers to send with all requests"`
	UserAgent         string            `json:"user_agent,omitempty" jsonschema:"custom user agent string to use"`
	JavaScriptEnabled *bool             `json:"javascript_enabled,omitempty" jsonschema:"enable or disable JavaScript execution (default: true)"`
}

type ScreenshotArgs struct {
	URL           string `json:"url" jsonschema:"URL to capture screenshot from"`
	ContextName   string `json:"context_name,omitempty" jsonschema:"browser context to use (default: 'default')"`
	Resize        string `json:"resize,omitempty" jsonschema:"resize parameters like '800x600', '800x600!' for exact size, or '50%x50%' for percentage"`
	UpdateCookies bool   `json:"update_cookies,omitempty" jsonschema:"automatically apply set-cookie headers from response to context"`
}

type ScreenshotHTMLArgs struct {
	HTMLContent   string `json:"html_content" jsonschema:"HTML content to render and screenshot"`
	ContextName   string `json:"context_name,omitempty" jsonschema:"browser context to use (default: 'default')"`
	Resize        string `json:"resize,omitempty" jsonschema:"resize parameters like '800x600', '800x600!' for exact size, or '50%x50%' for percentage"`
	UpdateCookies bool   `json:"update_cookies,omitempty" jsonschema:"automatically apply set-cookie headers from response to context"`
}

type GetHTMLArgs struct {
	URL           string `json:"url" jsonschema:"URL to get rendered HTML content from"`
	ContextName   string `json:"context_name,omitempty" jsonschema:"browser context to use (default: 'default')"`
	UpdateCookies bool   `json:"update_cookies,omitempty" jsonschema:"automatically apply set-cookie headers from response to context"`
}

type ListContextsArgs struct{}

type GetLastRequestArgs struct {
	ContextName    string `json:"context_name,omitempty" jsonschema:"browser context to get last request from (default: 'default')"`
	IncludeHTML    bool   `json:"include_html,omitempty" jsonschema:"include HTML content in response (default: false)"`
	IncludeNetwork bool   `json:"include_network,omitempty" jsonschema:"include network request details (default: false)"`
	IncludeConsole bool   `json:"include_console,omitempty" jsonschema:"include console log messages (default: false)"`
}

// Tool result structures
type ConfigureContextResult struct {
	Success     bool                   `json:"success"`
	ContextName string                 `json:"context_name"`
	Message     string                 `json:"message"`
	Config      map[string]interface{} `json:"configuration"`
}

type ScreenshotResult struct {
	Success     bool   `json:"success"`
	RequestID   string `json:"request_id"`
	Screenshot  string `json:"screenshot"`
	ContentType string `json:"content_type"`
	URL         string `json:"url"`
	StatusCode  int    `json:"status_code"`
	Duration    int64  `json:"duration"`
}

// Helper functions

func convertCookieInputs(inputs []CookieInput) []*proto.NetworkCookieParam {
	cookies := make([]*proto.NetworkCookieParam, len(inputs))
	for i, input := range inputs {
		cookie := &proto.NetworkCookieParam{
			Name:     input.Name,
			Value:    input.Value,
			Domain:   input.Domain,
			Path:     input.Path,
			HTTPOnly: input.HTTPOnly,
			Secure:   input.Secure,
		}

		if cookie.Path == "" {
			cookie.Path = "/"
		}

		if input.Expires > 0 {
			cookie.Expires = proto.TimeSinceEpoch(input.Expires)
		}

		switch strings.ToLower(input.SameSite) {
		case "strict":
			cookie.SameSite = proto.NetworkCookieSameSiteStrict
		case "lax":
			cookie.SameSite = proto.NetworkCookieSameSiteLax
		case "none":
			cookie.SameSite = proto.NetworkCookieSameSiteNone
		}

		cookies[i] = cookie
	}
	return cookies
}

// convertRodCookiesToParams converts Rod's NetworkCookie to NetworkCookieParam format
func convertRodCookiesToParams(rodCookies []*proto.NetworkCookie) []*proto.NetworkCookieParam {
	cookies := make([]*proto.NetworkCookieParam, len(rodCookies))
	for i, rodCookie := range rodCookies {
		cookie := &proto.NetworkCookieParam{
			Name:     rodCookie.Name,
			Value:    rodCookie.Value,
			Domain:   rodCookie.Domain,
			Path:     rodCookie.Path,
			Expires:  rodCookie.Expires,
			HTTPOnly: rodCookie.HTTPOnly,
			Secure:   rodCookie.Secure,
			SameSite: rodCookie.SameSite,
		}
		cookies[i] = cookie
	}
	return cookies
}

// Tool handlers with proper MCP signatures

func handleConfigureContext(ctx context.Context, request *mcp.CallToolRequest, args ConfigureContextArgs) (*mcp.CallToolResult, ConfigureContextResult, error) {
	// Set defaults
	if args.ContextName == "" {
		args.ContextName = "default"
	}
	if args.Timeout == 0 {
		args.Timeout = 30
	}
	if args.Viewport == "" {
		args.Viewport = "1920x1080"
	}

	// Parse viewport
	viewportWidth, viewportHeight, err := ParseViewportString(args.Viewport)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Invalid viewport: %v", err)},
			},
			IsError: true,
		}, ConfigureContextResult{}, fmt.Errorf("invalid viewport: %v", err)
	}

	// Parse domain whitelist
	domainWhitelist, err := ParseDomainWhitelist(args.Domains)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Invalid domains: %v", err)},
			},
			IsError: true,
		}, ConfigureContextResult{}, fmt.Errorf("invalid domains: %v", err)
	}

	// Parse cookies
	var cookies []*proto.NetworkCookieParam
	if len(args.Cookies) > 0 {
		cookies = convertCookieInputs(args.Cookies)
	}

	// Create context configuration
	config := &BrowserContextConfig{
		Name: args.ContextName,
		DefaultViewport: ViewportConfig{
			Width:  viewportWidth,
			Height: viewportHeight,
		},
		DefaultTimeout:    args.Timeout,
		DomainWhitelist:   domainWhitelist,
		Cookies:           cookies,
		Headers:           args.Headers,
		UserAgent:         args.UserAgent,
		JavaScriptEnabled: args.JavaScriptEnabled == nil || *args.JavaScriptEnabled, // Default to true
	}

	// Set default headers if none provided
	if config.Headers == nil {
		config.Headers = make(map[string]string)
	}

	configManager.CreateOrUpdateContext(args.ContextName, config)

	result := ConfigureContextResult{
		Success:     true,
		ContextName: args.ContextName,
		Message:     "Context configured successfully",
		Config: map[string]interface{}{
			"viewport":           args.Viewport,
			"timeout":            args.Timeout,
			"domains":            len(domainWhitelist),
			"cookies":            len(cookies),
			"headers":            len(config.Headers),
			"javascript_enabled": args.JavaScriptEnabled,
		},
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "Context configured successfully"},
		},
	}, result, nil
}

func handleMCPScreenshot(ctx context.Context, request *mcp.CallToolRequest, args ScreenshotArgs) (*mcp.CallToolResult, ScreenshotResult, error) {
	if args.URL == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "URL is required"},
			},
			IsError: true,
		}, ScreenshotResult{}, fmt.Errorf("URL is required")
	}

	// Get context configuration
	contextName := args.ContextName
	if contextName == "" {
		contextName = "default"
	}

	config, exists := configManager.GetContext(contextName)
	if !exists {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Context not found: %s", contextName)},
			},
			IsError: true,
		}, ScreenshotResult{}, fmt.Errorf("context not found: %s", contextName)
	}

	// Generate request ID
	requestID := GenerateRequestID()
	startTime := time.Now()

	// Create request config
	requestConfig := &RequestConfig{
		ViewportWidth:   config.DefaultViewport.Width,
		ViewportHeight:  config.DefaultViewport.Height,
		TimeoutSeconds:  config.DefaultTimeout,
		DomainWhitelist: config.DomainWhitelist,
		ResizeParam:     args.Resize,
		CustomHeaders:   config.Headers,
		Debug:           false,
		CaptureCookies:  args.UpdateCookies, // Enable cookie capture when cookies should be updated
	}

	// Take screenshot using existing sitecap functionality
	requestConfig.CaptureScreenshot = true
	requestConfig.CaptureHTML = true
	response, err := executeBrowserRequest(args.URL, "", requestConfig)

	// Create stored request
	var html string
	if response.HTML != nil {
		html = *response.HTML
	}

	storedRequest := &StoredRequest{
		ID:          requestID,
		ContextName: contextName,
		URL:         args.URL,
		Timestamp:   startTime,
		Duration:    time.Since(startTime),
		RequestType: "screenshot",
		Screenshot:  response.Screenshot,
		HTML:        html,
	}

	// Convert captured cookies from Rod format to storage format
	if len(response.Cookies) > 0 {
		storedRequest.SetCookies = convertRodCookiesToParams(response.Cookies)
	}

	if err != nil {
		storedRequest.Error = err.Error()
		requestManager.StoreRequest(storedRequest)
		configManager.AddRequestToHistory(contextName, requestID)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Screenshot failed: %v", err)},
			},
			IsError: true,
		}, ScreenshotResult{}, fmt.Errorf("screenshot failed: %v", err)
	}

	storedRequest.StatusCode = 200

	// Handle cookie updates if requested
	if args.UpdateCookies && len(storedRequest.SetCookies) > 0 {
		configManager.UpdateCookies(contextName, storedRequest.SetCookies, true)
	}

	// Store request and update history
	requestManager.StoreRequest(storedRequest)
	configManager.AddRequestToHistory(contextName, requestID)

	result := ScreenshotResult{
		Success:     true,
		RequestID:   requestID,
		Screenshot:  base64.StdEncoding.EncodeToString(response.Screenshot),
		ContentType: response.ContentType,
		URL:         args.URL,
		StatusCode:  storedRequest.StatusCode,
		Duration:    storedRequest.Duration.Milliseconds(),
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "Screenshot captured successfully"},
		},
	}, result, nil
}

func handleMCPScreenshotHTML(ctx context.Context, request *mcp.CallToolRequest, args ScreenshotHTMLArgs) (*mcp.CallToolResult, ScreenshotResult, error) {
	if args.HTMLContent == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "HTML content is required"},
			},
			IsError: true,
		}, ScreenshotResult{}, fmt.Errorf("HTML content is required")
	}

	// Get context configuration
	contextName := args.ContextName
	if contextName == "" {
		contextName = "default"
	}

	config, exists := configManager.GetContext(contextName)
	if !exists {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Context not found: %s", contextName)},
			},
			IsError: true,
		}, ScreenshotResult{}, fmt.Errorf("context not found: %s", contextName)
	}

	// Generate request ID
	requestID := GenerateRequestID()
	startTime := time.Now()

	// Create request config
	requestConfig := &RequestConfig{
		ViewportWidth:   config.DefaultViewport.Width,
		ViewportHeight:  config.DefaultViewport.Height,
		TimeoutSeconds:  config.DefaultTimeout,
		DomainWhitelist: config.DomainWhitelist,
		ResizeParam:     args.Resize,
		CustomHeaders:   config.Headers,
		Debug:           false,
		CaptureCookies:  args.UpdateCookies, // Enable cookie capture when cookies should be updated
	}

	// Take screenshot from HTML using existing sitecap functionality
	requestConfig.CaptureScreenshot = true
	requestConfig.CaptureHTML = true
	response, err := executeBrowserRequest("", args.HTMLContent, requestConfig)

	// Create stored request
	var renderedHTML string
	if response.HTML != nil {
		renderedHTML = *response.HTML
	}

	storedRequest := &StoredRequest{
		ID:          requestID,
		ContextName: contextName,
		URL:         "(HTML content)",
		Timestamp:   startTime,
		Duration:    time.Since(startTime),
		RequestType: "screenshot_html",
		Screenshot:  response.Screenshot,
		HTML:        renderedHTML, // Store the rendered HTML content (not original input)
	}

	// Convert captured cookies from Rod format to storage format
	if len(response.Cookies) > 0 {
		storedRequest.SetCookies = convertRodCookiesToParams(response.Cookies)
	}

	if err != nil {
		storedRequest.Error = err.Error()
		requestManager.StoreRequest(storedRequest)
		configManager.AddRequestToHistory(contextName, requestID)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("HTML screenshot failed: %v", err)},
			},
			IsError: true,
		}, ScreenshotResult{}, fmt.Errorf("HTML screenshot failed: %v", err)
	}

	storedRequest.StatusCode = 200

	// Handle cookie updates if requested (though less relevant for HTML content)
	if args.UpdateCookies && len(storedRequest.SetCookies) > 0 {
		configManager.UpdateCookies(contextName, storedRequest.SetCookies, true)
	}

	// Store request and update history
	requestManager.StoreRequest(storedRequest)
	configManager.AddRequestToHistory(contextName, requestID)

	result := ScreenshotResult{
		Success:     true,
		RequestID:   requestID,
		Screenshot:  base64.StdEncoding.EncodeToString(response.Screenshot),
		ContentType: response.ContentType,
		URL:         "(HTML content)",
		StatusCode:  storedRequest.StatusCode,
		Duration:    storedRequest.Duration.Milliseconds(),
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "HTML screenshot captured successfully"},
		},
	}, result, nil
}

func handleMCPGetHTML(ctx context.Context, request *mcp.CallToolRequest, args GetHTMLArgs) (*mcp.CallToolResult, map[string]interface{}, error) {
	if args.URL == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "URL is required"},
			},
			IsError: true,
		}, nil, fmt.Errorf("URL is required")
	}

	// Get context configuration
	contextName := args.ContextName
	if contextName == "" {
		contextName = "default"
	}

	config, exists := configManager.GetContext(contextName)
	if !exists {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Context not found: %s", contextName)},
			},
			IsError: true,
		}, nil, fmt.Errorf("context not found: %s", contextName)
	}

	// Generate request ID and get HTML
	requestID := GenerateRequestID()
	startTime := time.Now()

	requestConfig := &RequestConfig{
		ViewportWidth:   config.DefaultViewport.Width,
		ViewportHeight:  config.DefaultViewport.Height,
		TimeoutSeconds:  config.DefaultTimeout,
		DomainWhitelist: config.DomainWhitelist,
		CustomHeaders:   config.Headers,
		Debug:           false,
		CaptureCookies:  args.UpdateCookies, // Enable cookie capture when cookies should be updated
	}

	requestConfig.CaptureHTML = true
	response, err := executeBrowserRequest(args.URL, "", requestConfig)

	var html string
	if response.HTML != nil {
		html = *response.HTML
	}

	storedRequest := &StoredRequest{
		ID:          requestID,
		ContextName: contextName,
		URL:         args.URL,
		Timestamp:   startTime,
		Duration:    time.Since(startTime),
		RequestType: "get_html",
		HTML:        html,
	}

	// Convert captured cookies from Rod format to storage format
	if len(response.Cookies) > 0 {
		storedRequest.SetCookies = convertRodCookiesToParams(response.Cookies)
	}

	if err != nil {
		storedRequest.Error = err.Error()
		requestManager.StoreRequest(storedRequest)
		configManager.AddRequestToHistory(contextName, requestID)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Get HTML failed: %v", err)},
			},
			IsError: true,
		}, nil, fmt.Errorf("get HTML failed: %v", err)
	}

	storedRequest.StatusCode = 200

	// Handle cookie updates if requested
	if args.UpdateCookies && len(storedRequest.SetCookies) > 0 {
		configManager.UpdateCookies(contextName, storedRequest.SetCookies, true)
	}

	// Store request and update history
	requestManager.StoreRequest(storedRequest)
	configManager.AddRequestToHistory(contextName, requestID)

	result := map[string]interface{}{
		"success":     true,
		"request_id":  requestID,
		"html":        html,
		"url":         args.URL,
		"status_code": storedRequest.StatusCode,
		"duration":    storedRequest.Duration.Milliseconds(),
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "HTML extracted successfully"},
		},
	}, result, nil
}

func handleListContexts(ctx context.Context, request *mcp.CallToolRequest, args ListContextsArgs) (*mcp.CallToolResult, map[string]interface{}, error) {
	contexts := configManager.ListContexts()

	result := map[string]interface{}{
		"success":  true,
		"contexts": contexts,
		"count":    len(contexts),
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Found %d contexts", len(contexts))},
		},
	}, result, nil
}

func handleGetLastRequest(ctx context.Context, request *mcp.CallToolRequest, args GetLastRequestArgs) (*mcp.CallToolResult, map[string]interface{}, error) {
	// Set default context name
	contextName := args.ContextName
	if contextName == "" {
		contextName = "default"
	}

	// Check if context exists
	_, exists := configManager.GetContext(contextName)
	if !exists {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Context not found: %s", contextName)},
			},
			IsError: true,
		}, nil, fmt.Errorf("context not found: %s", contextName)
	}

	// Get last request for the context
	lastRequest, found := requestManager.GetLastRequest(contextName, configManager)
	if !found {
		return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("No requests found for context: %s", contextName)},
				},
			}, map[string]interface{}{
				"success": false,
				"message": fmt.Sprintf("No requests found for context: %s", contextName),
			}, nil
	}

	// Create response with requested details
	result := requestManager.CreateRequestResponse(lastRequest, args.IncludeHTML, args.IncludeNetwork, args.IncludeConsole)
	result["success"] = true

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Last request for context '%s': %s (%s)", contextName, lastRequest.URL, lastRequest.RequestType)},
		},
	}, result, nil
}
