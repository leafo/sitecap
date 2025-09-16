package main

import (
	"context"
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
	ContextName string            `json:"context_name,omitempty" jsonschema:"name of the browser context (default: 'default')"`
	Viewport    *string           `json:"viewport,omitempty" jsonschema:"viewport dimensions like '1920x1080' (default: '1920x1080')"`
	Timeout     *int              `json:"timeout,omitempty" jsonschema:"timeout in seconds for page loads (default: 30)"`
	Domains     *string           `json:"domains,omitempty" jsonschema:"comma-separated list of allowed domains for request filtering"`
	Cookies     []CookieInput     `json:"cookies,omitempty" jsonschema:"array of cookie objects to set in the browser context"`
	Headers     map[string]string `json:"headers,omitempty" jsonschema:"default HTTP headers to send with all requests"`
}

type ScreenshotArgs struct {
	URL           string `json:"url" jsonschema:"URL to capture screenshot from"`
	ContextName   string `json:"context_name,omitempty" jsonschema:"browser context to use (default: 'default')"`
	Resize        string `json:"resize,omitempty" jsonschema:"resize parameters like '800x600', '800x600!' for exact size, or '50%x50%' for percentage"`
	FullHeight    bool   `json:"full_height,omitempty" jsonschema:"capture full page height up to 10x the viewport height"`
	UpdateCookies bool   `json:"update_cookies,omitempty" jsonschema:"automatically apply set-cookie headers from response to context"`
}

type ScreenshotHTMLArgs struct {
	HTMLContent   string `json:"html_content" jsonschema:"HTML content to render and screenshot"`
	ContextName   string `json:"context_name,omitempty" jsonschema:"browser context to use (default: 'default')"`
	Resize        string `json:"resize,omitempty" jsonschema:"resize parameters like '800x600', '800x600!' for exact size, or '50%x50%' for percentage"`
	FullHeight    bool   `json:"full_height,omitempty" jsonschema:"capture full page height up to 10x the viewport height"`
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
	ContentType string `json:"content_type"`
	URL         string `json:"url"`
	Duration    int64  `json:"duration_ms"`
}

// Helper functions

func newErrorResult[T any](err error) (*mcp.CallToolResult, T, error) {
	var zeroValue T
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: err.Error()},
		},
		IsError: true,
	}, zeroValue, err
}

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
	// Set default context name
	contextName := args.ContextName
	if contextName == "" {
		contextName = "default"
	}

	// Fetch existing context or create new default
	config, exists := configManager.GetContext(contextName)
	if !exists {
		config = DefaultBrowserContextConfig()
		config.Name = contextName
	}

	// Conditionally update viewport if provided
	if args.Viewport != nil {
		viewportWidth, viewportHeight, err := ParseViewportString(*args.Viewport)
		if err != nil {
			return newErrorResult[ConfigureContextResult](fmt.Errorf("invalid viewport: %v", err))
		}
		config.DefaultViewport = ViewportConfig{
			Width:  viewportWidth,
			Height: viewportHeight,
		}
	}

	// Conditionally update timeout if provided
	if args.Timeout != nil {
		config.DefaultTimeout = *args.Timeout
	}

	// Conditionally update domain whitelist if provided
	if args.Domains != nil {
		domainWhitelist, err := ParseDomainWhitelist(*args.Domains)
		if err != nil {
			return newErrorResult[ConfigureContextResult](fmt.Errorf("invalid domains: %v", err))
		}
		config.DomainWhitelist = domainWhitelist
	}

	// Conditionally update cookies if provided
	if args.Cookies != nil {
		cookies := convertCookieInputs(args.Cookies)
		config.Cookies = cookies
	}

	// Conditionally update headers if provided
	if args.Headers != nil {
		config.Headers = args.Headers
	}

	// Store the updated context
	configManager.CreateOrUpdateContext(contextName, config)

	// Build result configuration for response
	resultConfig := map[string]interface{}{
		"viewport": fmt.Sprintf("%dx%d", config.DefaultViewport.Width, config.DefaultViewport.Height),
		"timeout":  config.DefaultTimeout,
		"domains":  config.DomainWhitelist,
		"cookies":  config.Cookies,
		"headers":  config.Headers,
	}

	result := ConfigureContextResult{
		Success:     true,
		ContextName: contextName,
		Message:     "Context configured successfully",
		Config:      resultConfig,
	}

	return &mcp.CallToolResult{}, result, nil
}

func handleMCPScreenshot(ctx context.Context, request *mcp.CallToolRequest, args ScreenshotArgs) (*mcp.CallToolResult, ScreenshotResult, error) {
	if args.URL == "" {
		return newErrorResult[ScreenshotResult](fmt.Errorf("URL is required"))
	}

	// Get context configuration
	contextName := args.ContextName
	if contextName == "" {
		contextName = "default"
	}

	config, exists := configManager.GetContext(contextName)
	if !exists {
		return newErrorResult[ScreenshotResult](fmt.Errorf("context not found: %s", contextName))
	}

	startTime := time.Now()

	// Create request config
	requestConfig := &RequestConfig{
		ViewportWidth:   config.DefaultViewport.Width,
		ViewportHeight:  config.DefaultViewport.Height,
		TimeoutSeconds:  config.DefaultTimeout,
		DomainWhitelist: config.DomainWhitelist,
		ResizeParam:     args.Resize,
		FullHeight:      args.FullHeight,
		CustomHeaders:   config.Headers,
		Cookies:         config.Cookies,
		Debug:           globalDebug,

		// capture everything
		CaptureCookies:    true,
		CaptureScreenshot: true,
		CaptureHTML:       true,
		CaptureNetwork:    true,
		CaptureLogs:       true,
	}

	response, err := executeBrowserRequest(args.URL, "", requestConfig)

	entry := NewRequestHistoryEntry(contextName, args.URL, "", "screenshot", requestConfig, response, startTime, err)

	if err != nil {
		requestManager.StoreRequest(entry)
		config.AddRequestToHistory(entry.ID)

		return newErrorResult[ScreenshotResult](fmt.Errorf("screenshot failed: %v", err))
	}

	// Handle cookie updates if requested
	if args.UpdateCookies && len(response.Cookies) > 0 {
		cookieParams := convertRodCookiesToParams(response.Cookies)
		config.UpdateCookies(cookieParams, true)
	}

	// Store request and update history
	requestManager.StoreRequest(entry)
	config.AddRequestToHistory(entry.ID)

	result := ScreenshotResult{
		Success:     true,
		RequestID:   entry.ID,
		ContentType: response.ContentType,
		URL:         args.URL,
		Duration:    entry.Duration.Milliseconds(),
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.ImageContent{
				Data:     []byte(response.Screenshot),
				MIMEType: response.ContentType,
			},
		},
	}, result, nil
}

func handleMCPScreenshotHTML(ctx context.Context, request *mcp.CallToolRequest, args ScreenshotHTMLArgs) (*mcp.CallToolResult, ScreenshotResult, error) {
	if args.HTMLContent == "" {
		return newErrorResult[ScreenshotResult](fmt.Errorf("HTML content is required"))
	}

	// Get context configuration
	contextName := args.ContextName
	if contextName == "" {
		contextName = "default"
	}

	config, exists := configManager.GetContext(contextName)
	if !exists {
		return newErrorResult[ScreenshotResult](fmt.Errorf("context not found: %s", contextName))
	}

	startTime := time.Now()

	// Create request config
	requestConfig := &RequestConfig{
		ViewportWidth:   config.DefaultViewport.Width,
		ViewportHeight:  config.DefaultViewport.Height,
		TimeoutSeconds:  config.DefaultTimeout,
		DomainWhitelist: config.DomainWhitelist,
		ResizeParam:     args.Resize,
		FullHeight:      args.FullHeight,
		CustomHeaders:   config.Headers,
		Cookies:         config.Cookies,
		Debug:           globalDebug,

		// capture everything
		CaptureCookies:    true,
		CaptureScreenshot: true,
		CaptureHTML:       true,
		CaptureNetwork:    true,
		CaptureLogs:       true,
	}

	response, err := executeBrowserRequest("", args.HTMLContent, requestConfig)

	entry := NewRequestHistoryEntry(contextName, "", args.HTMLContent, "screenshot_html", requestConfig, response, startTime, err)

	if err != nil {
		requestManager.StoreRequest(entry)
		config.AddRequestToHistory(entry.ID)

		return newErrorResult[ScreenshotResult](fmt.Errorf("HTML screenshot failed: %v", err))
	}

	// Handle cookie updates if requested (though less relevant for HTML content)
	if args.UpdateCookies && len(response.Cookies) > 0 {
		cookieParams := convertRodCookiesToParams(response.Cookies)
		config.UpdateCookies(cookieParams, true)
	}

	// Store request and update history
	requestManager.StoreRequest(entry)
	config.AddRequestToHistory(entry.ID)

	result := ScreenshotResult{
		Success:     true,
		RequestID:   entry.ID,
		ContentType: response.ContentType,
		URL:         "(HTML content)",
		Duration:    entry.Duration.Milliseconds(),
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.ImageContent{
				Data:     []byte(response.Screenshot),
				MIMEType: response.ContentType,
			},
		},
	}, result, nil
}

func handleMCPGetHTML(ctx context.Context, request *mcp.CallToolRequest, args GetHTMLArgs) (*mcp.CallToolResult, map[string]interface{}, error) {
	if args.URL == "" {
		return newErrorResult[map[string]interface{}](fmt.Errorf("URL is required"))
	}

	// Get context configuration
	contextName := args.ContextName
	if contextName == "" {
		contextName = "default"
	}

	config, exists := configManager.GetContext(contextName)
	if !exists {
		return newErrorResult[map[string]interface{}](fmt.Errorf("context not found: %s", contextName))
	}

	startTime := time.Now()

	requestConfig := &RequestConfig{
		ViewportWidth:   config.DefaultViewport.Width,
		ViewportHeight:  config.DefaultViewport.Height,
		TimeoutSeconds:  config.DefaultTimeout,
		DomainWhitelist: config.DomainWhitelist,
		CustomHeaders:   config.Headers,
		Cookies:         config.Cookies,
		Debug:           globalDebug,

		CaptureCookies: true,
		CaptureHTML:    true,
		CaptureNetwork: true,
		CaptureLogs:    true,
	}

	response, err := executeBrowserRequest(args.URL, "", requestConfig)

	entry := NewRequestHistoryEntry(contextName, args.URL, "", "get_html", requestConfig, response, startTime, err)

	if err != nil {
		requestManager.StoreRequest(entry)
		config.AddRequestToHistory(entry.ID)

		return newErrorResult[map[string]interface{}](fmt.Errorf("get HTML failed: %v", err))
	}

	// Handle cookie updates if requested
	if args.UpdateCookies && len(response.Cookies) > 0 {
		cookieParams := convertRodCookiesToParams(response.Cookies)
		config.UpdateCookies(cookieParams, true)
	}

	// Store request and update history
	requestManager.StoreRequest(entry)
	config.AddRequestToHistory(entry.ID)

	var html string
	if response.HTML != nil {
		html = *response.HTML
	}

	result := map[string]interface{}{
		"success":     true,
		"request_id":  entry.ID,
		"html":        html,
		"url":         args.URL,
		"duration_ms": entry.Duration.Milliseconds(),
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: html},
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

	return &mcp.CallToolResult{}, result, nil
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
		return newErrorResult[map[string]interface{}](fmt.Errorf("context not found: %s", contextName))
	}

	// Get last request for the context
	lastRequest, found := requestManager.GetLastRequest(contextName, configManager)
	if !found {
		return newErrorResult[map[string]interface{}](fmt.Errorf("No requests found for context: %s", contextName))
	}

	// Create response with basic fields that are always present
	result := map[string]interface{}{
		"success":      true,
		"id":           lastRequest.ID,
		"context_name": lastRequest.ContextName,
		"url":          lastRequest.URL,
		"timestamp":    lastRequest.Timestamp,
		"duration_ms":  lastRequest.Duration.Milliseconds(),
		"request_type": lastRequest.RequestType,
	}

	// Include input HTML if present
	if lastRequest.InputHTML != "" {
		result["input_html"] = lastRequest.InputHTML
	}

	// Handle error cases
	if lastRequest.Error != "" {
		result["error"] = lastRequest.Error
		result["success"] = false
		return &mcp.CallToolResult{}, result, nil
	}

	// Extract information from the BrowserResponse
	if lastRequest.Response != nil {
		// Convert cookies to expected format
		if len(lastRequest.Response.Cookies) > 0 {
			cookies := make([]map[string]interface{}, len(lastRequest.Response.Cookies))
			for i, cookie := range lastRequest.Response.Cookies {
				cookies[i] = map[string]interface{}{
					"name":     cookie.Name,
					"value":    cookie.Value,
					"domain":   cookie.Domain,
					"path":     cookie.Path,
					"expires":  cookie.Expires,
					"httpOnly": cookie.HTTPOnly,
					"secure":   cookie.Secure,
					"sameSite": cookie.SameSite,
				}
			}
			result["set_cookies"] = cookies
		}

		if args.IncludeHTML && lastRequest.Response.HTML != nil {
			result["html"] = *lastRequest.Response.HTML
		}

		if args.IncludeNetwork {
			// Create a sanitized version of network requests without headers to reduce bloat
			sanitizedRequests := make([]map[string]interface{}, len(lastRequest.Response.NetworkRequests))
			for i, req := range lastRequest.Response.NetworkRequests {
				sanitizedRequests[i] = map[string]interface{}{
					"url":         req.URL,
					"method":      req.Method,
					"status_code": req.StatusCode,
					"duration_ms": req.Duration,
					"timestamp":   req.Timestamp,
					"failed":      req.Failed,
				}
				if req.ErrorText != "" {
					sanitizedRequests[i]["error_text"] = req.ErrorText
				}
			}
			result["network_requests"] = sanitizedRequests
		}

		if args.IncludeConsole {
			result["console_logs"] = lastRequest.Response.ConsoleLogs
		}
	}

	return &mcp.CallToolResult{}, result, nil
}
