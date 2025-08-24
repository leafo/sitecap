package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

type Metrics struct {
	TotalRequests   atomic.Int64  `metric:"sitecap_requests_total"`
	SuccessRequests atomic.Int64  `metric:"sitecap_requests_success_total"`
	FailedRequests  atomic.Int64  `metric:"sitecap_requests_failed_total"`
	TotalDuration   atomic.Uint64 `metric:"sitecap_duration_seconds_total"`
}

var metrics Metrics
var globalDebug bool
var globalCustomHeaders map[string]string

func (m *Metrics) String() string {
	var sb strings.Builder

	v := reflect.ValueOf(m).Elem()
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		metricName := fieldType.Tag.Get("metric")
		if metricName == "" {
			continue
		}

		var value string
		switch field.Type().String() {
		case "atomic.Int64":
			atomicInt := field.Interface().(atomic.Int64)
			value = strconv.FormatInt(atomicInt.Load(), 10)
		case "atomic.Uint64":
			atomicUint := field.Interface().(atomic.Uint64)
			nanoseconds := atomicUint.Load()
			seconds := float64(nanoseconds) / 1e9
			value = strconv.FormatFloat(seconds, 'f', 6, 64)
		}

		sb.WriteString(metricName + " " + value + "\n")
	}

	return sb.String()
}

type responseWriter struct {
	http.ResponseWriter
	status int
	size   int64
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.status == 0 {
		rw.status = 200
	}
	size, err := rw.ResponseWriter.Write(b)
	rw.size += int64(size)
	return size, err
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &responseWriter{ResponseWriter: w}
		next.ServeHTTP(rw, r)

		remoteAddr := r.RemoteAddr
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			remoteAddr = strings.Split(forwarded, ",")[0]
		}

		timestamp := time.Now().Format("02/Jan/2006:15:04:05 -0700")
		method := r.Method
		uri := r.RequestURI
		proto := r.Proto
		userAgent := r.Header.Get("User-Agent")
		referer := r.Header.Get("Referer")
		if referer == "" {
			referer = "-"
		}
		if userAgent == "" {
			userAgent = "-"
		}

		log.Printf("%s - - [%s] \"%s %s %s\" %d %d \"%s\" \"%s\"",
			remoteAddr, timestamp, method, uri, proto, rw.status, rw.size, referer, userAgent)
	})
}

func (m *Metrics) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, m.String())
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

func setupRequestHijacking(page *rod.Page, mainURL string, domainWhitelist []string, debugMode bool, customHeaders map[string]string) {
	if debugMode || len(domainWhitelist) > 0 || len(customHeaders) > 0 {
		router := page.HijackRequests()
		router.MustAdd("*", func(ctx *rod.Hijack) {
			requestURL := ctx.Request.URL().String()

			// Debug logging
			if debugMode {
				log.Printf("[DEBUG] Request: %s", requestURL)
			}

			// Always allow the main requested URL
			if mainURL != "" && requestURL == mainURL {
				if debugMode {
					log.Printf("[DEBUG] Allowed (main URL): %s", requestURL)
				}
				// Apply custom headers to the main request
				if len(customHeaders) > 0 {
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
					for k, v := range customHeaders {
						headers = append(headers, &proto.FetchHeaderEntry{
							Name:  k,
							Value: v,
						})
					}
					if debugMode {
						log.Printf("[DEBUG] Adding custom headers: %+v", customHeaders)
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
			if len(domainWhitelist) > 0 {
				if isDomainWhitelisted(requestURL, domainWhitelist) {
					if debugMode {
						log.Printf("[DEBUG] Allowed: %s", requestURL)
					}
					// Apply custom headers if any
					if len(customHeaders) > 0 {
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
						for k, v := range customHeaders {
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
					if debugMode {
						log.Printf("[DEBUG] Blocked: %s", requestURL)
					}
					ctx.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
				}
			} else {
				// No filtering, just continue with optional custom headers
				if len(customHeaders) > 0 {
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
					for k, v := range customHeaders {
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

func takeScreenshotFromHTML(htmlContent string, viewportWidth, viewportHeight int, timeoutSeconds int, domainWhitelist []string, debugMode bool, customHeaders map[string]string) ([]byte, error) {
	browser := rod.New().MustConnect()
	defer browser.MustClose()

	page := browser.MustPage()

	// Set up request hijacking for debugging, domain filtering, or custom headers
	setupRequestHijacking(page, "", domainWhitelist, debugMode, customHeaders)

	// Set timeout if specified
	if timeoutSeconds > 0 {
		page = page.Timeout(time.Duration(timeoutSeconds) * time.Second)
	}

	// Set viewport if dimensions are specified
	if viewportWidth > 0 && viewportHeight > 0 {
		page.MustSetViewport(viewportWidth, viewportHeight, 1.0, false)
	}

	// Set HTML content directly
	page.MustSetDocumentContent(htmlContent)
	page.MustWaitLoad()

	return page.Screenshot(false, &proto.PageCaptureScreenshot{
		Format:      proto.PageCaptureScreenshotFormatPng,
		FromSurface: true,
	})
}

func takeScreenshot(url string, viewportWidth, viewportHeight int, timeoutSeconds int, domainWhitelist []string, debugMode bool, customHeaders map[string]string) ([]byte, error) {
	browser := rod.New().MustConnect()
	defer browser.MustClose()

	page := browser.MustPage()

	// Set up request hijacking for debugging, domain filtering, or custom headers
	setupRequestHijacking(page, url, domainWhitelist, debugMode, customHeaders)

	// Set timeout if specified
	if timeoutSeconds > 0 {
		page = page.Timeout(time.Duration(timeoutSeconds) * time.Second)
	}

	// Navigate to URL
	page.MustNavigate(url)

	// Set viewport if dimensions are specified
	if viewportWidth > 0 && viewportHeight > 0 {
		page.MustSetViewport(viewportWidth, viewportHeight, 1.0, false)
	}

	page.MustWaitLoad()

	return page.Screenshot(false, &proto.PageCaptureScreenshot{
		Format:      proto.PageCaptureScreenshotFormatPng,
		FromSurface: true,
	})
}

func processScreenshotFromHTML(htmlContent string, viewportParam string, resizeParam string, timeoutSeconds int, allowedDomains string, debugMode bool) ([]byte, string, error) {
	// Parse viewport dimensions
	viewportWidth, viewportHeight, err := parseViewportString(viewportParam)
	if err != nil {
		return nil, "", fmt.Errorf("invalid viewport parameters: %v", err)
	}

	// Parse domain whitelist
	domainWhitelist, err := parseDomainWhitelist(allowedDomains)
	if err != nil {
		return nil, "", fmt.Errorf("invalid domain whitelist: %v", err)
	}

	// Take screenshot from HTML
	img, err := takeScreenshotFromHTML(htmlContent, viewportWidth, viewportHeight, timeoutSeconds, domainWhitelist, debugMode, globalCustomHeaders)
	if err != nil {
		return nil, "", err
	}

	// Apply resizing if specified
	if resizeParam != "" {
		params, err := parseResizeString(resizeParam)
		if err != nil {
			return nil, "", fmt.Errorf("invalid resize parameters: %v", err)
		}

		resized, format, err := resizeImage(img, params)
		if err != nil {
			return nil, "", fmt.Errorf("resize failed: %v", err)
		}

		return resized, getContentType(format), nil
	}

	return img, "image/png", nil
}

func processScreenshot(url string, viewportParam string, resizeParam string, timeoutSeconds int, allowedDomains string, debugMode bool) ([]byte, string, error) {
	// Parse viewport dimensions
	viewportWidth, viewportHeight, err := parseViewportString(viewportParam)
	if err != nil {
		return nil, "", fmt.Errorf("invalid viewport parameters: %v", err)
	}

	// Parse domain whitelist
	domainWhitelist, err := parseDomainWhitelist(allowedDomains)
	if err != nil {
		return nil, "", fmt.Errorf("invalid domain whitelist: %v", err)
	}

	// Take screenshot
	img, err := takeScreenshot(url, viewportWidth, viewportHeight, timeoutSeconds, domainWhitelist, debugMode, globalCustomHeaders)
	if err != nil {
		return nil, "", err
	}

	// Apply resizing if specified
	if resizeParam != "" {
		params, err := parseResizeString(resizeParam)
		if err != nil {
			return nil, "", fmt.Errorf("invalid resize parameters: %v", err)
		}

		resized, format, err := resizeImage(img, params)
		if err != nil {
			return nil, "", fmt.Errorf("resize failed: %v", err)
		}

		return resized, getContentType(format), nil
	}

	return img, "image/png", nil
}

func handleScreenshot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	start := time.Now()

	metrics.TotalRequests.Add(1)

	url := r.URL.Query().Get("url")
	if url == "" {
		metrics.FailedRequests.Add(1)
		http.Error(w, "Missing url parameter", http.StatusBadRequest)
		return
	}

	viewportParam := r.URL.Query().Get("viewport")
	resizeParam := r.URL.Query().Get("resize")
	timeoutParam := r.URL.Query().Get("timeout")
	domainsParam := r.URL.Query().Get("domains")

	timeoutSeconds, err := parseTimeoutString(timeoutParam)
	if err != nil {
		metrics.FailedRequests.Add(1)
		http.Error(w, fmt.Sprintf("Invalid timeout parameter: %v", err), http.StatusBadRequest)
		return
	}

	img, contentType, err := processScreenshot(url, viewportParam, resizeParam, timeoutSeconds, domainsParam, globalDebug)
	duration := time.Since(start)

	metrics.TotalDuration.Add(uint64(duration.Nanoseconds()))
	if err != nil {
		metrics.FailedRequests.Add(1)
		http.Error(w, fmt.Sprintf("Error processing screenshot: %v", err), http.StatusInternalServerError)
		return
	} else {
		metrics.SuccessRequests.Add(1)
	}

	w.Header().Set("Content-Type", contentType)
	w.Write(img)
}

func main() {
	httpMode := flag.Bool("http", false, "Start HTTP server mode")
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


	if *httpMode {
		mux := http.NewServeMux()
		mux.HandleFunc("/", handleScreenshot)
		mux.Handle("/metrics", &metrics)

		handler := loggingMiddleware(mux)

		fmt.Printf("Starting HTTP server on %s\n", *listen)
		fmt.Printf("Usage: http://%s/?url=https://leafo.net&viewport=1920x1080&resize=100x200&timeout=30&domains=example.com,*.cdn.com\n", *listen)
		if len(globalCustomHeaders) > 0 {
			fmt.Printf("Custom headers will be applied to all requests: %+v\n", globalCustomHeaders)
		}
		fmt.Printf("Metrics: http://%s/metrics\n", *listen)
		if *debug {
			fmt.Println("Debug mode enabled - all network requests will be logged")
		}
		log.Fatal(http.ListenAndServe(*listen, handler))
	}

	if len(flag.Args()) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s [--viewport WxH] [--resize WxH] [--timeout N] [--domains list] [--headers JSON] [--debug] [--http] <URL>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "       %s [options] - < input.html   (use '-' to read HTML from stdin)\n", os.Args[0])
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

		img, _, err := processScreenshotFromHTML(htmlContent, *viewport, *resize, *timeout, *domains, *debug)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing HTML screenshot: %v\n", err)
			os.Exit(1)
		}

		_, err = os.Stdout.Write(img)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing to stdout: %v\n", err)
			os.Exit(1)
		}
		return
	}

	img, _, err := processScreenshot(url, *viewport, *resize, *timeout, *domains, *debug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error processing screenshot: %v\n", err)
		os.Exit(1)
	}

	_, err = os.Stdout.Write(img)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing to stdout: %v\n", err)
		os.Exit(1)
	}
}
