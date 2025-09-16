# sitecap

Tool for taking screenshots of websites using Chrome CDP, available in command line, HTTP service, and Model Context Protocol (MCP) server modes.

## Installation

```bash
go install github.com/leafo/sitecap@latest
```

## Usage

### Command Line Mode

```bash
sitecap [--viewport WxH] [--resize WxH] [--timeout N] [--domains list] [--debug] <URL> > screenshot.png
sitecap [options] - < input.html > screenshot.png
sitecap [options] --html <URL> > page.html
sitecap --mcp
sitecap --http --mcp
```

Examples:
```bash
# Basic screenshot
sitecap https://example.com > example.png

# Set browser viewport to 1920x1080 before taking screenshot
sitecap --viewport 1920x1080 https://example.com > desktop.png

# Mobile viewport
sitecap --viewport 375x667 https://example.com > mobile.png

# Set 30 second timeout for slow-loading pages
sitecap --timeout 30 https://slow-site.com > slow.png

# Only load resources from specific domains
sitecap --domains "example.com,*.cloudfront.net" https://example.com > filtered.png

# Enable debug logging to see all network requests
sitecap --debug https://example.com > debug.png 2>requests.log

# Full example with all parameters
sitecap --viewport 1920x1080 --resize 800x600 --timeout 15 --domains "example.com,cdn.example.com" --debug https://example.com > complete.png

# Force exact dimensions (ignore aspect ratio)
sitecap --resize 800x600! https://example.com > stretched.png

# Resize and center crop to exact dimensions
sitecap --resize 800x600# https://example.com > cropped.png

# Resize by percentage
sitecap --resize 50%x50% https://example.com > half-size.png

# Crop manually with offset
sitecap --resize 200x200+100+50 https://example.com > cropped-offset.png

# Render HTML from stdin instead of a URL
echo "<html><body><h1>Hello World</h1></body></html>" | sitecap --viewport 800x600 - > html-screenshot.png

# Process HTML file with domain filtering
sitecap --viewport 1200x800 --resize 600x400 --domains "example.com,cdn.example.com" - < my-page.html > processed.png

# Get HTML content from a live website
sitecap --html https://example.com > example.html

# Get HTML content with viewport and domain filtering
sitecap --html --viewport 1920x1080 --domains "example.com,*.cdn.com" https://example.com > filtered.html
```

### HTML from Stdin

Instead of capturing a live website, you can render HTML content directly from stdin by using `-` as the URL:

```bash
# Render HTML string
echo "<html><body><h1>Hello World</h1></body></html>" | sitecap - > output.png

# Process HTML file with options
sitecap --viewport 1200x800 --resize 600x400 - < my-page.html > screenshot.png

# Filter external resources when rendering HTML
sitecap --domains "cdn.example.com,fonts.googleapis.com" --debug - < my-page.html > filtered.png
```

**Use cases:**
- Generate screenshots from dynamic HTML content
- Test HTML layouts without deploying to a web server
- Process HTML templates with custom data
- Create images from programmatically generated HTML

### HTML Output Mode

The `--html` flag captures the rendered HTML content from a URL instead of
taking a screenshot. This can be useful to access the HTML content of a website
that primarily renders through JavaScript.

```bash
# Get final HTML after JavaScript execution
sitecap --html https://spa-app.com > rendered.html

# Get HTML with custom viewport
sitecap --html --viewport 375x667 https://responsive-site.com > mobile-html.html

# Get HTML with domain filtering
sitecap --html --domains "site.com,*.cdn.com" https://site.com > clean.html
```

### HTTP Server Mode

Start the HTTP server:
```bash
sitecap --http --listen localhost:8080
```

Or listen on all interfaces (use with caution):
```bash
sitecap --http --listen 0.0.0.0:8080
```

Take screenshots via HTTP requests:
```bash
# Basic screenshot
curl "http://localhost:8080/?url=https://example.com" > screenshot.png

# Set browser viewport
curl "http://localhost:8080/?url=https://example.com&viewport=1920x1080" > desktop.png

# Mobile viewport
curl "http://localhost:8080/?url=https://example.com&viewport=375x667" > mobile.png

# Set 30 second timeout for slow pages
curl "http://localhost:8080/?url=https://slow-site.com&timeout=30" > slow.png

# Whitelist specific domains only
curl "http://localhost:8080/?url=https://example.com&domains=example.com,*.cloudfront.net" > filtered.png

# Full example with all parameters
curl "http://localhost:8080/?url=https://example.com&viewport=1920x1080&resize=800x600&timeout=15&domains=example.com,cdn.example.com" > full.png

# Get HTML content instead of screenshot
curl "http://localhost:8080/html?url=https://example.com" > example.html

# Get HTML with viewport and domain filtering
curl "http://localhost:8080/html?url=https://example.com&viewport=1920x1080&domains=example.com,*.cdn.com" > filtered.html
```

**Debug Mode**: Start the server with `--debug` flag to see all network requests in the server logs:
```bash
sitecap --debug --http --listen localhost:8080
```

More examples:
```bash
# Force exact dimensions
curl "http://localhost:8080/?url=https://example.com&resize=800x600!" > stretched.png

# Resize and center crop (URL-safe)
curl "http://localhost:8080/?url=https://example.com&resize=800x600^" > cropped.png

# Manual crop with offset (URL-safe)
curl "http://localhost:8080/?url=https://example.com&resize=200x200_100_50" > crop-offset.png
```

### Model Context Protocol (MCP) Support

Sitecap can act as an MCP server, exposing the same capture tools described above to MCP clients.

#### Stdio Transport

Launch Sitecap as an MCP server over stdio (ideal when a client spawns the process directly):

```bash
sitecap --mcp
```

#### Streamable HTTP Transport

Combine the HTTP mode with the MCP server to serve the streamable MCP transport from `/mcp` alongside the REST endpoints:

```bash
sitecap --http --mcp --listen localhost:8080
# MCP clients should connect to http://localhost:8080/mcp
```

Both modes use the same tool definitions and configuration managers, ensuring consistent behavior regardless of transport.

#### Available Tools

- `configure_browser_context` – configure viewport, timeout, cookies, and headers for a named browsing context.
- `list_browser_contexts` – list configured contexts and their active settings.
- `capture_screenshot_from_url` – capture a screenshot by navigating to a URL.
- `capture_screenshot_from_html` – render arbitrary HTML and capture a screenshot.
- `extract_html_content` – retrieve the fully rendered HTML after JavaScript execution.
- `get_last_browser_request` – fetch the most recent request details, including network and console data.

#### Default Configuration via Flags

When launching the MCP server, the standard command line flags (`--viewport`, `--timeout`, `--domains`, `--headers`, `--debug`, etc.) are applied as defaults for every browsing context. Use these flags to preconfigure the screenshot environment before clients connect:

```bash
# Start MCP server with a 1920x1080 viewport and 30s timeout
sitecap --mcp --viewport 1920x1080 --timeout 30

# Streamable HTTP MCP server with domain restrictions and extra headers
sitecap --http --mcp --domains "example.com,*.cdn.com" --headers '{"X-Token":"secret"}'
```

Clients can still override or extend these defaults by calling `configure_browser_context`, but the initial values come from the flags provided at startup.

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

## Viewport Parameters

Control the browser viewport size before capturing the screenshot:

- `--viewport WxH` - Set browser viewport dimensions (e.g. `1920x1080`)
- `?viewport=WxH` - HTTP query parameter for viewport size

Common viewport sizes:
- Desktop: `1920x1080`, `1366x768`, `1280x1024`
- Tablet: `768x1024`, `1024x768`
- Mobile: `375x667` (iPhone), `414x896` (iPhone XR), `360x640` (Android)

**Note**: Viewport affects how the webpage renders before screenshot capture. This is different from resize, which processes the image after capture.

## Timeout Parameters

Control how long to wait for page loading and screenshot generation:

- `--timeout N` - CLI flag to set timeout in seconds
- `?timeout=N` - HTTP query parameter for timeout

**Timeout behavior:**
- `0` (default) - No timeout, wait indefinitely
- `1-300` - Timeout in seconds (max 5 minutes)
- Applies to both page loading and screenshot generation

**Use cases:**
- Slow-loading websites: `--timeout 30`
- Quick captures: `--timeout 5`
- Heavy JavaScript sites: `--timeout 60`

## Domain Whitelisting

Control which domains can load resources to improve performance and reduce bandwidth:

- `--domains "list"` - CLI flag for comma-separated domain patterns
- `?domains=list` - HTTP query parameter for domain filtering

**Domain patterns supported:**
- Exact domains: `example.com`
- Wildcards: `*.cloudfront.net` matches `abc.cloudfront.net`
- Subdomain matching: `.example.com` matches `sub.example.com` and `example.com`
- Multiple domains: `example.com,cdn.example.com,*.amazonaws.com`

**Benefits:**
- **Faster loading**: Block ads, trackers, and unnecessary resources
- **Reduced bandwidth**: Only load essential resources
- **Cleaner screenshots**: Remove third-party content
- **Better performance**: Fewer network requests

**Use cases:**
- Remove ads: `--domains "example.com"`
- CDN only: `--domains "example.com,*.cloudfront.net"`
- Multiple services: `--domains "site.com,api.site.com,*.cdn.com"`

## Resize Parameters

Sitecap supports powerful image resizing with the following syntax:

- `WxH` - Resize maintaining aspect ratio to fit within dimensions (e.g. `800x600`)
- `WxH!` - Force exact dimensions, ignoring aspect ratio (e.g. `800x600!`)
- `WxH#` or `WxH^` - Resize and center crop to exact dimensions (e.g. `800x600#` or `800x600^`)
- `P%xP%` - Resize by percentage (e.g. `50%x50%` for half size)
- `WxH+X+Y` or `WxH_X_Y` - Manual crop to WxH starting at offset X,Y (e.g. `200x200+100+50` or `200x200_100_50`)

You can also specify only width or height:
- `800x` - Resize to width 800, height auto-calculated
- `x600` - Resize to height 600, width auto-calculated

### URL-Safe Alternatives

For HTTP requests, use these URL-safe alternatives:
- Use `^` instead of `#` for center crop: `800x600^`
- Use `_` instead of `+` for crop offsets: `200x200_100_50`

