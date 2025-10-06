package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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

func handleHTML(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/html" {
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
	timeoutParam := r.URL.Query().Get("timeout")
	waitParam := r.URL.Query().Get("wait")
	domainsParam := r.URL.Query().Get("domains")
	fullHeightParam := r.URL.Query().Get("full_height")
	fullHeight := globalFullHeight
	if fullHeightParam != "" {
		parsed, err := strconv.ParseBool(fullHeightParam)
		if err != nil {
			metrics.FailedRequests.Add(1)
			http.Error(w, fmt.Sprintf("Invalid full_height parameter: %v", err), http.StatusBadRequest)
			return
		}
		fullHeight = parsed
	}

	config, err := parseRequestConfig(viewportParam, "", timeoutParam, waitParam, domainsParam, fullHeight)
	if err != nil {
		metrics.FailedRequests.Add(1)
		http.Error(w, fmt.Sprintf("Invalid parameters: %v", err), http.StatusBadRequest)
		return
	}

	config.CaptureHTML = true
	response, err := executeBrowserRequest(url, "", config)
	duration := time.Since(start)

	metrics.TotalDuration.Add(uint64(duration.Nanoseconds()))
	if err != nil {
		metrics.FailedRequests.Add(1)
		http.Error(w, fmt.Sprintf("Error processing HTML: %v", err), http.StatusInternalServerError)
		return
	} else {
		metrics.SuccessRequests.Add(1)
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if response.HTML != nil {
		w.Write([]byte(*response.HTML))
	}
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
	waitParam := r.URL.Query().Get("wait")
	domainsParam := r.URL.Query().Get("domains")
	fullHeightParam := r.URL.Query().Get("full_height")
	fullHeight := globalFullHeight
	if fullHeightParam != "" {
		parsed, err := strconv.ParseBool(fullHeightParam)
		if err != nil {
			metrics.FailedRequests.Add(1)
			http.Error(w, fmt.Sprintf("Invalid full_height parameter: %v", err), http.StatusBadRequest)
			return
		}
		fullHeight = parsed
	}

	config, err := parseRequestConfig(viewportParam, resizeParam, timeoutParam, waitParam, domainsParam, fullHeight)
	if err != nil {
		metrics.FailedRequests.Add(1)
		http.Error(w, fmt.Sprintf("Invalid parameters: %v", err), http.StatusBadRequest)
		return
	}

	config.CaptureScreenshot = true
	response, err := executeBrowserRequest(url, "", config)
	duration := time.Since(start)

	metrics.TotalDuration.Add(uint64(duration.Nanoseconds()))
	if err != nil {
		metrics.FailedRequests.Add(1)
		http.Error(w, fmt.Sprintf("Error processing screenshot: %v", err), http.StatusInternalServerError)
		return
	} else {
		metrics.SuccessRequests.Add(1)
	}

	w.Header().Set("Content-Type", response.ContentType)
	w.Write(response.Screenshot)
}

func StartHTTPServer(listen string, debug bool, enableMCP bool) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleScreenshot)
	mux.HandleFunc("/html", handleHTML)
	mux.Handle("/metrics", &metrics)

	if enableMCP {
		server := newMCPServer()
		handler := mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server {
			return server
		}, nil)
		mux.Handle("/mcp", handler)
		mux.Handle("/mcp/", handler)
	}

	handler := loggingMiddleware(mux)

	fmt.Printf("Starting HTTP server on %s\n", listen)
	fmt.Printf("Screenshot: http://%s/?url=https://leafo.net&viewport=1920x1080&resize=100x200&timeout=30&domains=example.com,*.cdn.com\n", listen)
	fmt.Printf("HTML: http://%s/html?url=https://leafo.net&viewport=1920x1080&timeout=30&domains=example.com,*.cdn.com\n", listen)
	if len(globalCustomHeaders) > 0 {
		fmt.Printf("Custom headers will be applied to all requests: %+v\n", globalCustomHeaders)
	}
	fmt.Printf("Metrics: http://%s/metrics\n", listen)
	if enableMCP {
		fmt.Printf("MCP (streamable): http://%s/mcp\n", listen)
	}
	if debug {
		fmt.Println("Debug mode enabled - all network requests will be logged")
	}
	log.Fatal(http.ListenAndServe(listen, handler))
}
