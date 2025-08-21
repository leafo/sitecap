package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/davidbyttow/govips/v2/vips"
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

func takeScreenshot(url string) ([]byte, error) {
	browser := rod.New().MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(url).MustWaitLoad()

	return page.Screenshot(false, &proto.PageCaptureScreenshot{
		Format:      proto.PageCaptureScreenshotFormatPng,
		FromSurface: true,
	})
}

func processScreenshot(url string, resizeParam string) ([]byte, string, error) {
	// Take screenshot
	img, err := takeScreenshot(url)
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

	resizeParam := r.URL.Query().Get("resize")
	img, contentType, err := processScreenshot(url, resizeParam)
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
	resize := flag.String("resize", "", "Resize parameters (e.g. 100x200, 100x200!, 100x200#)")
	flag.Parse()

	vips.Startup(&vips.Config{
		ConcurrencyLevel: 1,
		MaxCacheFiles:    0,
		MaxCacheMem:      0,
		MaxCacheSize:     0,
	})
	defer vips.Shutdown()

	if *httpMode {
		mux := http.NewServeMux()
		mux.HandleFunc("/", handleScreenshot)
		mux.Handle("/metrics", &metrics)

		handler := loggingMiddleware(mux)

		fmt.Printf("Starting HTTP server on %s\n", *listen)
		fmt.Printf("Usage: http://%s/?url=https://leafo.net&resize=100x200\n", *listen)
		fmt.Printf("Metrics: http://%s/metrics\n", *listen)
		log.Fatal(http.ListenAndServe(*listen, handler))
	}

	if len(flag.Args()) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s [--resize WxH] [--http] <URL>\n", os.Args[0])
		os.Exit(1)
	}

	url := flag.Args()[0]

	img, _, err := processScreenshot(url, *resize)
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
