package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// Version information set by ldflags at build time
var (
	buildDate = "unknown"
	commitHash = "unknown"
)

type RequestConfig struct {
	ViewportWidth   int
	ViewportHeight  int
	TimeoutSeconds  int
	WaitSeconds     int
	DomainWhitelist []string
	ResizeParam     string
	FullHeight      bool
	CustomHeaders   map[string]string
	Cookies         []*proto.NetworkCookieParam // Cookies to set before navigation
	Debug           bool

	CaptureCookies    bool // Enable cookie capture after navigation
	CaptureScreenshot bool // Enable screenshot capture
	CaptureHTML       bool // Enable HTML content capture
	CaptureNetwork    bool // Enable network request capture
	CaptureLogs       bool // Enable console log capture
}

type CapturedNetworkRequest struct {
	URL             string            `json:"url"`
	Method          string            `json:"method"`
	StatusCode      int               `json:"status_code"`
	RequestHeaders  map[string]string `json:"request_headers"`
	ResponseHeaders map[string]string `json:"response_headers"`
	Duration        int64             `json:"duration_ms"`
	Timestamp       time.Time         `json:"timestamp"`
	Failed          bool              `json:"failed"`
	ErrorText       string            `json:"error_text,omitempty"`
}

type CapturedConsoleLog struct {
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source,omitempty"`
	Line      int       `json:"line,omitempty"`
	Column    int       `json:"column,omitempty"`
}

type BrowserResponse struct {
	Cookies         []*proto.NetworkCookie   // Captured cookies from browser
	HTML            *string                  // Rendered HTML content (nil if not captured)
	Screenshot      []byte                   // Screenshot image data (nil if not captured)
	ContentType     string                   // Content type of screenshot (e.g., "image/png", "image/jpeg")
	NetworkRequests []CapturedNetworkRequest // Captured network requests (nil if not captured)
	ConsoleLogs     []CapturedConsoleLog     // Captured console logs (nil if not captured)
}

type JSONOutput struct {
	HTML            *string                  `json:"html,omitempty"`
	Cookies         []*proto.NetworkCookie   `json:"cookies,omitempty"`
	Screenshot      *string                  `json:"screenshot,omitempty"` // base64 encoded screenshot
	ContentType     string                   `json:"content_type,omitempty"`
	NetworkRequests []CapturedNetworkRequest `json:"network_requests,omitempty"`
	ConsoleLogs     []CapturedConsoleLog     `json:"console_logs,omitempty"`
}

var globalDebug bool
var globalCustomHeaders map[string]string
var globalViewport string
var globalTimeout int
var globalWait int
var globalDomains string
var globalFullHeight bool

func convertToJSONOutput(response *BrowserResponse) *JSONOutput {
	output := &JSONOutput{
		HTML:            response.HTML,
		Cookies:         response.Cookies,
		ContentType:     response.ContentType,
		NetworkRequests: response.NetworkRequests,
		ConsoleLogs:     response.ConsoleLogs,
	}

	if response.Screenshot != nil {
		encoded := base64.StdEncoding.EncodeToString(response.Screenshot)
		output.Screenshot = &encoded
	}

	return output
}

func parseCustomHeaders(headersJSON string) (map[string]string, error) {
	if headersJSON == "" {
		return nil, nil
	}

	var headers map[string]string
	err := json.Unmarshal([]byte(headersJSON), &headers)
	if err != nil {
		return nil, fmt.Errorf("invalid JSON format: %v", err)
	}

	return headers, nil
}

func parseRequestConfig(viewportParam, resizeParam, timeoutParam, waitParam, domainsParam string, fullHeight bool) (*RequestConfig, error) {
	config := &RequestConfig{}

	// Parse viewport dimensions
	viewportWidth, viewportHeight, err := ParseViewportString(viewportParam)
	if err != nil {
		return nil, fmt.Errorf("invalid viewport parameters: %v", err)
	}
	config.ViewportWidth = viewportWidth
	config.ViewportHeight = viewportHeight

	// Parse timeout
	timeoutSeconds, err := parseTimeoutString(timeoutParam)
	if err != nil {
		return nil, fmt.Errorf("invalid timeout parameter: %v", err)
	}
	config.TimeoutSeconds = timeoutSeconds

	// Parse wait time
	waitSeconds, err := parseTimeoutString(waitParam)
	if err != nil {
		return nil, fmt.Errorf("invalid wait parameter: %v", err)
	}
	config.WaitSeconds = waitSeconds

	// Parse domain whitelist
	domainWhitelist, err := ParseDomainWhitelist(domainsParam)
	if err != nil {
		return nil, fmt.Errorf("invalid domain whitelist: %v", err)
	}
	config.DomainWhitelist = domainWhitelist

	config.ResizeParam = resizeParam
	config.CustomHeaders = globalCustomHeaders
	config.Debug = globalDebug
	config.FullHeight = fullHeight

	return config, nil
}

func adjustViewportForFullHeight(page *rod.Page, config *RequestConfig) error {
	if config == nil {
		return nil
	}

	metrics, err := proto.PageGetLayoutMetrics{}.Call(page)
	if err != nil {
		return fmt.Errorf("failed to get layout metrics: %w", err)
	}

	var contentHeight float64
	if metrics.CSSContentSize != nil {
		contentHeight = metrics.CSSContentSize.Height
	} else if metrics.ContentSize != nil {
		contentHeight = metrics.ContentSize.Height
	}

	targetHeight := int(math.Ceil(contentHeight))

	// Determine the minimum height baseline
	minHeight := config.ViewportHeight
	if minHeight <= 0 && metrics.CSSVisualViewport != nil {
		minHeight = int(math.Ceil(metrics.CSSVisualViewport.ClientHeight))
	}
	if minHeight <= 0 && metrics.CSSLayoutViewport != nil {
		minHeight = metrics.CSSLayoutViewport.ClientHeight
	}
	if minHeight <= 0 {
		minHeight = targetHeight
	}
	if minHeight <= 0 {
		minHeight = 1
	}

	if targetHeight <= 0 {
		targetHeight = minHeight
	}

	if targetHeight < minHeight {
		targetHeight = minHeight
	}

	maxHeight := minHeight
	if minHeight > 0 {
		if minHeight > math.MaxInt/10 {
			maxHeight = math.MaxInt
		} else {
			maxHeight = minHeight * 10
		}
	}

	if maxHeight > 0 && targetHeight > maxHeight {
		targetHeight = maxHeight
	}

	if targetHeight <= 0 {
		return nil
	}

	width := config.ViewportWidth
	if width <= 0 && metrics.CSSVisualViewport != nil {
		width = int(math.Ceil(metrics.CSSVisualViewport.ClientWidth))
	}
	if width <= 0 && metrics.CSSLayoutViewport != nil {
		width = metrics.CSSLayoutViewport.ClientWidth
	}
	if width <= 0 && metrics.CSSContentSize != nil {
		width = int(math.Ceil(metrics.CSSContentSize.Width))
	}
	if width <= 0 {
		width = 1024
	}

	if targetHeight == config.ViewportHeight && width == config.ViewportWidth {
		return nil
	}

	err = page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:             width,
		Height:            targetHeight,
		DeviceScaleFactor: 1.0,
		Mobile:            false,
	})
	if err != nil {
		return fmt.Errorf("failed to set viewport for full height: %w", err)
	}

	config.ViewportWidth = width
	config.ViewportHeight = targetHeight

	return nil
}

type HijackConfig struct {
	MainURL            string
	DomainWhitelist    []string
	CustomHeaders      map[string]string
	Debug              bool
	PermitFirstRequest bool // Always permit the first request regardless of authorized domains
	CaptureNetwork     bool // Enable network request capture
	CaptureLogs        bool // Enable console log capture
}

type HijackResult struct {
	NetworkRequests []CapturedNetworkRequest // Captured network requests during hijacking
	ConsoleLogs     []CapturedConsoleLog     // Captured console logs during hijacking
}

func setupRequestHijacking(page *rod.Page, config *HijackConfig) *HijackResult {
	result := &HijackResult{
		NetworkRequests: make([]CapturedNetworkRequest, 0),
		ConsoleLogs:     make([]CapturedConsoleLog, 0),
	}

	if config.Debug {
		// Track request URLs by ID for failure logging
		var requestURLs sync.Map

		go page.EachEvent(func(e *proto.NetworkRequestWillBeSent) {
			requestURLs.Store(string(e.RequestID), e.Request.URL)
		})()

		go page.EachEvent(func(e *proto.NetworkResponseReceived) {
			response := e.Response
			statusColor := "\033[32m" // Green for 2xx
			if response.Status >= 400 {
				statusColor = "\033[31m" // Red for 4xx/5xx
			} else if response.Status >= 300 {
				statusColor = "\033[33m" // Yellow for 3xx
			}
			log.Printf("\033[36mResponse:\033[0m %s - Status: %s%d\033[0m - Size: %d bytes",
				response.URL, statusColor, response.Status, int64(response.EncodedDataLength))
		})()

		// Log network loading failures (invalid hosts, connection errors, etc.)
		go page.EachEvent(func(e *proto.NetworkLoadingFailed) {
			url := "unknown URL"
			if val, exists := requestURLs.Load(string(e.RequestID)); exists {
				if urlStr, ok := val.(string); ok && urlStr != "" {
					url = urlStr
				}
			}
			log.Printf("\033[31mNetwork Error:\033[0m %s - %s", url, e.ErrorText)
			requestURLs.Delete(string(e.RequestID)) // Clean up
		})()
	}

	if config.CaptureNetwork {
		// Track request details by ID
		var requestTimes sync.Map
		var requestInfo sync.Map
		var networkRequestsMutex sync.Mutex

		go page.EachEvent(func(e *proto.NetworkRequestWillBeSent) {
			requestID := string(e.RequestID)
			requestTimes.Store(requestID, time.Now())

			// Convert headers to map
			requestHeaders := make(map[string]string)
			for key, value := range e.Request.Headers {
				requestHeaders[key] = fmt.Sprintf("%v", value)
			}

			requestInfo.Store(requestID, CapturedNetworkRequest{
				URL:            e.Request.URL,
				Method:         e.Request.Method,
				RequestHeaders: requestHeaders,
				Timestamp:      time.Now(),
			})
		})()

		go page.EachEvent(func(e *proto.NetworkResponseReceived) {
			requestID := string(e.RequestID)

			if reqInfo, exists := requestInfo.Load(requestID); exists {
				if startTime, timeExists := requestTimes.Load(requestID); timeExists {
					req := reqInfo.(CapturedNetworkRequest)
					req.StatusCode = e.Response.Status
					req.Duration = time.Since(startTime.(time.Time)).Milliseconds()

					// Convert response headers to map
					responseHeaders := make(map[string]string)
					for key, value := range e.Response.Headers {
						responseHeaders[key] = fmt.Sprintf("%v", value)
					}
					req.ResponseHeaders = responseHeaders

					networkRequestsMutex.Lock()
					result.NetworkRequests = append(result.NetworkRequests, req)
					networkRequestsMutex.Unlock()

					// Clean up
					requestInfo.Delete(requestID)
					requestTimes.Delete(requestID)
				}
			}
		})()

		go page.EachEvent(func(e *proto.NetworkLoadingFailed) {
			requestID := string(e.RequestID)

			if reqInfo, exists := requestInfo.Load(requestID); exists {
				if startTime, timeExists := requestTimes.Load(requestID); timeExists {
					req := reqInfo.(CapturedNetworkRequest)
					req.Failed = true
					req.ErrorText = e.ErrorText
					req.Duration = time.Since(startTime.(time.Time)).Milliseconds()

					networkRequestsMutex.Lock()
					result.NetworkRequests = append(result.NetworkRequests, req)
					networkRequestsMutex.Unlock()

					// Clean up
					requestInfo.Delete(requestID)
					requestTimes.Delete(requestID)
				}
			}
		})()
	}

	if config.CaptureLogs {
		var consoleLogsMutex sync.Mutex

		// Capture console API calls (console.log, console.warn, console.error, etc.)
		go page.EachEvent(func(e *proto.RuntimeConsoleAPICalled) {
			message := ""
			if len(e.Args) > 0 {
				// Convert all arguments to strings and join them
				args := make([]string, len(e.Args))
				for i, arg := range e.Args {
					if arg.Description != "" {
						args[i] = arg.Description
					} else {
						// Try to extract value as string from JSON
						valueBytes, err := json.Marshal(arg.Value)
						if err == nil && len(valueBytes) > 0 {
							valueStr := string(valueBytes)
							// Remove quotes if it's a quoted string
							if strings.HasPrefix(valueStr, "\"") && strings.HasSuffix(valueStr, "\"") {
								valueStr = strings.Trim(valueStr, "\"")
							}
							args[i] = valueStr
						} else {
							args[i] = fmt.Sprintf("[%s]", arg.Type)
						}
					}
				}
				if len(args) == 1 {
					message = args[0]
				} else if len(args) > 1 {
					// Join all arguments with spaces for console output
					message = fmt.Sprintf("%s", args[0])
					for _, arg := range args[1:] {
						message += " " + arg
					}
				}
			}

			consoleLog := CapturedConsoleLog{
				Level:     string(e.Type),
				Message:   message,
				Timestamp: time.Now(),
			}

			// Add stack trace info if available
			if e.StackTrace != nil && len(e.StackTrace.CallFrames) > 0 {
				frame := e.StackTrace.CallFrames[0]
				consoleLog.Source = frame.URL
				consoleLog.Line = frame.LineNumber
				consoleLog.Column = frame.ColumnNumber
			}

			consoleLogsMutex.Lock()
			result.ConsoleLogs = append(result.ConsoleLogs, consoleLog)
			consoleLogsMutex.Unlock()
		})()

		// Capture uncaught JavaScript exceptions
		go page.EachEvent(func(e *proto.RuntimeExceptionThrown) {
			message := "JavaScript Exception"
			source := ""
			line := 0
			column := 0

			if e.ExceptionDetails != nil {
				if e.ExceptionDetails.Text != "" {
					message = e.ExceptionDetails.Text
				}
				if e.ExceptionDetails.URL != "" {
					source = e.ExceptionDetails.URL
				}
				line = e.ExceptionDetails.LineNumber
				column = e.ExceptionDetails.ColumnNumber

				// Try to get exception message from the exception object
				if e.ExceptionDetails.Exception != nil && e.ExceptionDetails.Exception.Description != "" {
					message = e.ExceptionDetails.Exception.Description
				}
			}

			consoleLog := CapturedConsoleLog{
				Level:     "error",
				Message:   message,
				Timestamp: time.Now(),
				Source:    source,
				Line:      line,
				Column:    column,
			}

			consoleLogsMutex.Lock()
			result.ConsoleLogs = append(result.ConsoleLogs, consoleLog)
			consoleLogsMutex.Unlock()
		})()

		// Capture general log entries
		go page.EachEvent(func(e *proto.LogEntryAdded) {
			if e.Entry != nil {
				message := e.Entry.Text
				source := e.Entry.URL
				line := 0
				if e.Entry.LineNumber != nil {
					line = *e.Entry.LineNumber
				}

				consoleLog := CapturedConsoleLog{
					Level:     string(e.Entry.Level),
					Message:   message,
					Timestamp: time.Now(),
					Source:    source,
					Line:      line,
				}

				consoleLogsMutex.Lock()
				result.ConsoleLogs = append(result.ConsoleLogs, consoleLog)
				consoleLogsMutex.Unlock()
			}
		})()
	}

	if config.Debug || len(config.DomainWhitelist) > 0 || len(config.CustomHeaders) > 0 || config.CaptureNetwork {
		router := page.HijackRequests()
		var firstRequest atomic.Bool
		firstRequest.Store(true)
		router.MustAdd("*", func(ctx *rod.Hijack) {
			requestURL := ctx.Request.URL().String()

			// Debug logging
			if config.Debug {
				log.Printf("\033[34mRequest:\033[0m %s", requestURL)
			}

			// Always allow the very first request regardless of domain
			if config.PermitFirstRequest && firstRequest.CompareAndSwap(true, false) {
				if config.Debug {
					log.Printf("\033[32mAllowed (first request):\033[0m %s", requestURL)
				}
				// Apply custom headers to the first request
				if len(config.CustomHeaders) > 0 {
					var headers []*proto.FetchHeaderEntry
					// First add existing headers
					for name, values := range ctx.Request.Req().Header {
						for _, value := range values {
							headers = append(headers, &proto.FetchHeaderEntry{
								Name:  name,
								Value: value,
							})
						}
					}
					// Then add custom headers (will override existing ones with same name)
					for k, v := range config.CustomHeaders {
						headers = append(headers, &proto.FetchHeaderEntry{
							Name:  k,
							Value: v,
						})
					}
					if config.Debug {
						headersJSON, _ := json.Marshal(config.CustomHeaders)
						log.Printf("\033[35mAdding custom headers:\033[0m %s", headersJSON)
					}
					ctx.ContinueRequest(&proto.FetchContinueRequest{
						Headers: headers,
					})
				} else {
					ctx.ContinueRequest(&proto.FetchContinueRequest{})
				}
				return
			}

			// Domain filtering
			if len(config.DomainWhitelist) > 0 {
				if isDomainWhitelisted(requestURL, config.DomainWhitelist) {
					if config.Debug {
						log.Printf("\033[32mAllowed:\033[0m %s", requestURL)
					}
					// Apply custom headers if any
					if len(config.CustomHeaders) > 0 {
						var headers []*proto.FetchHeaderEntry
						// Add existing headers
						for name, values := range ctx.Request.Req().Header {
							for _, value := range values {
								headers = append(headers, &proto.FetchHeaderEntry{
									Name:  name,
									Value: value,
								})
							}
						}
						// Add custom headers
						for k, v := range config.CustomHeaders {
							headers = append(headers, &proto.FetchHeaderEntry{
								Name:  k,
								Value: v,
							})
						}
						ctx.ContinueRequest(&proto.FetchContinueRequest{
							Headers: headers,
						})
					} else {
						ctx.ContinueRequest(&proto.FetchContinueRequest{})
					}
				} else {
					if config.Debug {
						log.Printf("\033[31mBlocked:\033[0m %s", requestURL)
					}
					ctx.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
				}
			} else {
				// No filtering, just continue with optional custom headers
				if len(config.CustomHeaders) > 0 {
					var headers []*proto.FetchHeaderEntry
					// Add existing headers
					for name, values := range ctx.Request.Req().Header {
						for _, value := range values {
							headers = append(headers, &proto.FetchHeaderEntry{
								Name:  name,
								Value: value,
							})
						}
					}
					// Add custom headers
					for k, v := range config.CustomHeaders {
						headers = append(headers, &proto.FetchHeaderEntry{
							Name:  k,
							Value: v,
						})
					}
					ctx.ContinueRequest(&proto.FetchContinueRequest{
						Headers: headers,
					})
				} else {
					ctx.ContinueRequest(&proto.FetchContinueRequest{})
				}
			}
		})
		go router.Run()
	}

	return result
}

func executeBrowserRequest(url, htmlContent string, config *RequestConfig) (*BrowserResponse, error) {
	browser := rod.New()

	err := browser.Connect()

	if err != nil {
		return nil, err
	}

	defer func() {
		if err := browser.Close(); err != nil {
			log.Printf("Error closing browser: %v", err)
		}
	}()

	page := browser.MustPage()

	// Set up request hijacking for debugging, domain filtering, custom headers, or network capture
	hijackConfig := &HijackConfig{
		MainURL:            url,
		DomainWhitelist:    config.DomainWhitelist,
		CustomHeaders:      config.CustomHeaders,
		Debug:              config.Debug,
		PermitFirstRequest: url != "",
		CaptureNetwork:     config.CaptureNetwork,
		CaptureLogs:        config.CaptureLogs,
	}
	hijackResult := setupRequestHijacking(page, hijackConfig)

	// Set timeout if specified
	if config.TimeoutSeconds > 0 {
		page = page.Timeout(time.Duration(config.TimeoutSeconds) * time.Second)
	}

	// Set cookies before navigation if specified
	if len(config.Cookies) > 0 {
		err = page.SetCookies(config.Cookies)
		if err != nil {
			return nil, fmt.Errorf("failed to set cookies: %v", err)
		}
	}

	// Load content (URL or HTML)
	if htmlContent != "" {
		err = page.SetDocumentContent(htmlContent)
		if err != nil {
			return nil, err
		}
	} else {
		err = page.Navigate(url)
		if err != nil {
			return nil, err
		}
	}

	// Set viewport if dimensions are specified
	if config.ViewportWidth > 0 && config.ViewportHeight > 0 {
		page.MustSetViewport(config.ViewportWidth, config.ViewportHeight, 1.0, false)
	}

	err = page.WaitLoad()
	if err != nil {
		return nil, err
	}

	// Wait additional time if specified
	if config.WaitSeconds > 0 {
		time.Sleep(time.Duration(config.WaitSeconds) * time.Second)
	}

	if config.FullHeight {
		if err := adjustViewportForFullHeight(page, config); err != nil {
			return nil, err
		}
	}

	response := &BrowserResponse{}

	if config.CaptureCookies {
		if url != "" {
			// For URL-based requests, get cookies for that specific URL
			cookies, err := page.Cookies([]string{url})
			if err == nil {
				response.Cookies = cookies
			}
		} else {
			// For HTML content, get all cookies
			cookies, err := page.Cookies([]string{})
			if err == nil {
				response.Cookies = cookies
			}
		}
	}

	if config.CaptureScreenshot {
		screenshot, err := page.Screenshot(false, &proto.PageCaptureScreenshot{
			Format:      proto.PageCaptureScreenshotFormatPng,
			FromSurface: true,
		})
		if err != nil {
			return nil, err
		}
		response.Screenshot = screenshot
		response.ContentType = "image/png" // Default content type

		// Apply resizing if specified
		if config.ResizeParam != "" {
			params, err := parseResizeString(config.ResizeParam)
			if err != nil {
				return nil, fmt.Errorf("invalid resize parameters: %v", err)
			}

			resized, format, err := resizeImage(response.Screenshot, params)
			if err != nil {
				return nil, fmt.Errorf("resize failed: %v", err)
			}

			response.Screenshot = resized
			response.ContentType = getContentType(format)
		}
	}

	if config.CaptureHTML {
		html, err := page.HTML()
		if err != nil {
			return nil, err
		}
		response.HTML = &html
	}

	if config.CaptureNetwork {
		response.NetworkRequests = hijackResult.NetworkRequests
	}

	if config.CaptureLogs {
		response.ConsoleLogs = hijackResult.ConsoleLogs
	}

	return response, nil
}

func main() {
	httpMode := flag.Bool("http", false, "Start HTTP server mode")
	mcpMode := flag.Bool("mcp", false, "Start MCP (Model Context Protocol) server mode")
	htmlMode := flag.Bool("html", false, "Output HTML content instead of screenshot")
	jsonMode := flag.Bool("json", false, "Output JSON with HTML, cookies, and other request information")
	listen := flag.String("listen", "localhost:8080", "Address to listen on for HTTP server")
	viewport := flag.String("viewport", "", "Viewport dimensions for the browser (e.g. 1920x1080)")
	resize := flag.String("resize", "", "Resize parameters (e.g. 100x200, 100x200!, 100x200#)")
	fullHeight := flag.Bool("full-height", false, "Capture the full page height up to 10x the viewport height")
	timeout := flag.Int("timeout", 0, "Timeout in seconds for page load and screenshot (0 = no timeout)")
	wait := flag.Int("wait", 0, "Wait time in seconds after page load before taking screenshot (0 = no wait)")
	domains := flag.String("domains", "", "Comma-separated list of allowed domains (e.g. example.com,*.cdn.com)")
	headers := flag.String("headers", "", "JSON string of custom headers to add to the initial request (e.g. '{\"Authorization\":\"Bearer token\",\"Custom-Header\":\"value\"}')")
	debug := flag.Bool("debug", false, "Enable debug logging of all network requests")
	version := flag.Bool("version", false, "Print version information and exit")
	flag.Parse()

	if *version {
		fmt.Printf("sitecap\n  build date: %s\n  commit: %s\n", buildDate, commitHash)
		return
	}

	// Set global flags
	globalDebug = *debug
	globalViewport = *viewport
	globalTimeout = *timeout
	globalWait = *wait
	globalDomains = *domains
	globalFullHeight = *fullHeight

	// Parse and set global custom headers
	var err error
	globalCustomHeaders, err = parseCustomHeaders(*headers)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing headers: %v\n", err)
		os.Exit(1)
	}

	if *httpMode {
		StartHTTPServer(*listen, *debug, *mcpMode)
		return
	}

	if *mcpMode {
		StartMCPServer()
		return
	}

	if len(flag.Args()) != 1 {
		flag.Usage()
		os.Exit(1)
	}

	resizeParam := *resize
	if *htmlMode || *jsonMode {
		resizeParam = ""
	}

	config, err := parseRequestConfig(*viewport, resizeParam, strconv.Itoa(*timeout), strconv.Itoa(*wait), *domains, *fullHeight)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing parameters: %v\n", err)
		os.Exit(1)
	}

	url := flag.Args()[0]
	var htmlContent string

	// Check if URL is "-" to read HTML from stdin
	if url == "-" {
		// Read HTML from stdin
		htmlBytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading HTML from stdin: %v\n", err)
			os.Exit(1)
		}

		htmlContent = string(htmlBytes)
		if htmlContent == "" {
			fmt.Fprintf(os.Stderr, "No HTML content provided via stdin\n")
			os.Exit(1)
		}
		url = "" // Clear URL when using stdin
	}

	// Process request based on mode
	if *jsonMode {
		config.CaptureHTML = true
		config.CaptureCookies = true
		config.CaptureNetwork = true
		config.CaptureLogs = true
		response, err := executeBrowserRequest(url, htmlContent, config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing request: %v\n", err)
			os.Exit(1)
		}

		jsonOutput := convertToJSONOutput(response)
		jsonBytes, err := json.MarshalIndent(jsonOutput, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(string(jsonBytes))
	} else if *htmlMode {
		config.CaptureHTML = true
		response, err := executeBrowserRequest(url, htmlContent, config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing HTML content: %v\n", err)
			os.Exit(1)
		}

		if response.HTML != nil {
			fmt.Print(*response.HTML)
		}
	} else {
		config.CaptureScreenshot = true
		response, err := executeBrowserRequest(url, htmlContent, config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing screenshot: %v\n", err)
			os.Exit(1)
		}

		_, err = os.Stdout.Write(response.Screenshot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing to stdout: %v\n", err)
			os.Exit(1)
		}
	}
}
