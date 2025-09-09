package main

import (
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
}

type BrowserResponse struct {
	Cookies     []*proto.NetworkCookie // Captured cookies from browser
	HTML        *string                // Rendered HTML content (nil if not captured)
	Screenshot  []byte                 // Screenshot image data (nil if not captured)
	ContentType string                 // Content type of screenshot (e.g., "image/png", "image/jpeg")
}

var globalDebug bool
var globalCustomHeaders map[string]string

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
}

func setupRequestHijacking(page *rod.Page, config *HijackConfig) {
	// Set up network event logging for responses when debug is enabled
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

	if config.Debug || len(config.DomainWhitelist) > 0 || len(config.CustomHeaders) > 0 {
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

	// Set up request hijacking for debugging, domain filtering, or custom headers
	hijackConfig := &HijackConfig{
		MainURL:            url,
		DomainWhitelist:    config.DomainWhitelist,
		CustomHeaders:      config.CustomHeaders,
		Debug:              config.Debug,
		PermitFirstRequest: url != "",
	}
	setupRequestHijacking(page, hijackConfig)

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

	return response, nil
}

func main() {
	httpMode := flag.Bool("http", false, "Start HTTP server mode")
	mcpMode := flag.Bool("mcp", false, "Start MCP (Model Context Protocol) server mode")
	htmlMode := flag.Bool("html", false, "Output HTML content instead of screenshot")
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
		fmt.Fprintf(os.Stderr, "Usage: %s [--viewport WxH] [--resize WxH] [--timeout N] [--domains list] [--headers JSON] [--debug] [--http] [--mcp] [--html] <URL>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "       %s [options] - < input.html   (use '-' to read HTML from stdin)\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "       %s --mcp   (start MCP server for Model Context Protocol)\n", os.Args[0])
		os.Exit(1)
	}

	resizeParam := *resize
	if *htmlMode {
		resizeParam = ""
	}

	config, err := parseRequestConfig(*viewport, resizeParam, strconv.Itoa(*timeout), *domains)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing parameters: %v\n", err)
		os.Exit(1)
	}

	url := flag.Args()[0]

	// Check if URL is "-" to read HTML from stdin
	if url == "-" {
		// Read HTML from stdin
		htmlBytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading HTML from stdin: %v\n", err)
			os.Exit(1)
		}

		htmlContent := string(htmlBytes)
		if htmlContent == "" {
			fmt.Fprintf(os.Stderr, "No HTML content provided via stdin\n")
			os.Exit(1)
		}

		if *htmlMode {
			config.CaptureHTML = true
			response, err := executeBrowserRequest("", htmlContent, config)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error processing HTML content: %v\n", err)
				os.Exit(1)
			}

			if response.HTML != nil {
				fmt.Print(*response.HTML)
			}
		} else {
			config.CaptureScreenshot = true
			response, err := executeBrowserRequest("", htmlContent, config)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error processing HTML screenshot: %v\n", err)
				os.Exit(1)
			}

			_, err = os.Stdout.Write(response.Screenshot)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error writing to stdout: %v\n", err)
				os.Exit(1)
			}
		}
		return
	}

	if *htmlMode {
		config.CaptureHTML = true
		response, err := executeBrowserRequest(url, "", config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing HTML content: %v\n", err)
			os.Exit(1)
		}

		if response.HTML != nil {
			fmt.Print(*response.HTML)
		}
	} else {
		config.CaptureScreenshot = true
		response, err := executeBrowserRequest(url, "", config)
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
