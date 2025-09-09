package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

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

type RequestConfig struct {
	ViewportWidth   int
	ViewportHeight  int
	TimeoutSeconds  int
	DomainWhitelist []string
	ResizeParam     string
	CustomHeaders   map[string]string
	Debug           bool

	CaptureCookies    bool // Enable cookie capture after navigation
	CaptureScreenshot bool // Enable screenshot capture
	CaptureHTML       bool // Enable HTML content capture
	CaptureNetwork    bool // Enable network request capture
	JSONOutput        bool // Enable JSON output mode
}

type BrowserResponse struct {
	Cookies         []*proto.NetworkCookie   // Captured cookies from browser
	HTML            *string                  // Rendered HTML content (nil if not captured)
	Screenshot      []byte                   // Screenshot image data (nil if not captured)
	ContentType     string                   // Content type of screenshot (e.g., "image/png", "image/jpeg")
	NetworkRequests []CapturedNetworkRequest // Captured network requests (nil if not captured)
}

type JSONOutput struct {
	HTML            *string                  `json:"html,omitempty"`
	Cookies         []*proto.NetworkCookie   `json:"cookies,omitempty"`
	Screenshot      *string                  `json:"screenshot,omitempty"` // base64 encoded screenshot
	ContentType     string                   `json:"content_type,omitempty"`
	NetworkRequests []CapturedNetworkRequest `json:"network_requests,omitempty"`
}

var globalDebug bool
var globalCustomHeaders map[string]string

func convertToJSONOutput(response *BrowserResponse) *JSONOutput {
	output := &JSONOutput{
		HTML:            response.HTML,
		Cookies:         response.Cookies,
		ContentType:     response.ContentType,
		NetworkRequests: response.NetworkRequests,
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

func parseRequestConfig(viewportParam, resizeParam, timeoutParam, domainsParam string) (*RequestConfig, error) {
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

	// Parse domain whitelist
	domainWhitelist, err := ParseDomainWhitelist(domainsParam)
	if err != nil {
		return nil, fmt.Errorf("invalid domain whitelist: %v", err)
	}
	config.DomainWhitelist = domainWhitelist

	config.ResizeParam = resizeParam
	config.CustomHeaders = globalCustomHeaders
	config.Debug = globalDebug

	return config, nil
}

type HijackConfig struct {
	MainURL            string
	DomainWhitelist    []string
	CustomHeaders      map[string]string
	Debug              bool
	PermitFirstRequest bool // Always permit the first request regardless of authorized domains
	CaptureNetwork     bool // Enable network request capture
}

type HijackResult struct {
	NetworkRequests []CapturedNetworkRequest // Captured network requests during hijacking
}

func setupRequestHijacking(page *rod.Page, config *HijackConfig) *HijackResult {
	result := &HijackResult{
		NetworkRequests: make([]CapturedNetworkRequest, 0),
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
	}
	hijackResult := setupRequestHijacking(page, hijackConfig)

	// Set timeout if specified
	if config.TimeoutSeconds > 0 {
		page = page.Timeout(time.Duration(config.TimeoutSeconds) * time.Second)
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
	timeout := flag.Int("timeout", 0, "Timeout in seconds for page load and screenshot (0 = no timeout)")
	domains := flag.String("domains", "", "Comma-separated list of allowed domains (e.g. example.com,*.cdn.com)")
	headers := flag.String("headers", "", "JSON string of custom headers to add to the initial request (e.g. '{\"Authorization\":\"Bearer token\",\"Custom-Header\":\"value\"}')")
	debug := flag.Bool("debug", false, "Enable debug logging of all network requests")
	flag.Parse()

	// Set global debug flag
	globalDebug = *debug

	// Parse and set global custom headers
	var err error
	globalCustomHeaders, err = parseCustomHeaders(*headers)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing headers: %v\n", err)
		os.Exit(1)
	}

	if *mcpMode {
		StartMCPServer()
		return
	}

	if *httpMode {
		StartHTTPServer(*listen, *debug)
		return
	}

	if len(flag.Args()) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s [--viewport WxH] [--resize WxH] [--timeout N] [--domains list] [--headers JSON] [--debug] [--http] [--mcp] [--html] [--json] <URL>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "       %s [options] - < input.html   (use '-' to read HTML from stdin)\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "       %s --mcp   (start MCP server for Model Context Protocol)\n", os.Args[0])
		os.Exit(1)
	}

	resizeParam := *resize
	if *htmlMode || *jsonMode {
		resizeParam = ""
	}

	config, err := parseRequestConfig(*viewport, resizeParam, strconv.Itoa(*timeout), *domains)
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
