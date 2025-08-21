# sitecap

Command line tool for taking screenshots of websites.

## Installation

```bash
go install github.com/leafo/sitecap@latest
```

## Usage

### Command Line Mode

```bash
sitecap <URL> > screenshot.png
```

Example:
```bash
sitecap https://example.com > example.png
```

### HTTP Server Mode

Start the HTTP server:
```bash
sitecap --http --listen localhost:8080
```

Or listen on all interfaces:
```bash
sitecap --http --listen 0.0.0.0:8080
```

Take screenshots via HTTP requests:
```bash
curl "http://localhost:8080/?url=https://example.com" > screenshot.png
```

### Metrics

The HTTP server exposes Prometheus-compatible metrics at `/metrics`:
```bash
curl "http://localhost:8080/metrics"
```

Available metrics:
- `sitecap_requests_total` - Total number of screenshot requests
- `sitecap_requests_success_total` - Number of successful requests  
- `sitecap_requests_failed_total` - Number of failed requests
- `sitecap_duration_seconds_total` - Total time spent taking screenshots