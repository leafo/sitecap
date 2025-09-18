# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

```
Usage of ./sitecap:
  -debug
    	Enable debug logging of all network requests
  -domains string
    	Comma-separated list of allowed domains (e.g. example.com,*.cdn.com)
  -headers string
    	JSON string of custom headers to add to the initial request (e.g. '{"Authorization":"Bearer token","Custom-Header":"value"}')
  -html
    	Output HTML content instead of screenshot
  -http
    	Start HTTP server mode
  -json
    	Output JSON with HTML, cookies, and other request information
  -listen string
    	Address to listen on for HTTP server (default "localhost:8080")
  -mcp
    	Start MCP (Model Context Protocol) server mode
  -resize string
    	Resize parameters (e.g. 100x200, 100x200!, 100x200#)
  -rod string
    	Set the default value of options used by rod.
  -timeout int
    	Timeout in seconds for page load and screenshot (0 = no timeout)
  -viewport string
    	Viewport dimensions for the browser (e.g. 1920x1080)
```

## Project Overview

Sitecap is a Go-based website screenshot tool that uses Chrome DevTools Protocol (CDP) via the go-rod library. It operates in three distinct modes:

1. **CLI Mode**: Command-line screenshot capture with multiple output formats
2. **HTTP Server Mode**: Web service for screenshot generation via HTTP API
3. **MCP Server Mode**: Model Context Protocol server for AI assistant integration

Key features include viewport control, image resizing, domain filtering, custom headers, cookie capture, HTML extraction, and comprehensive debug logging.

## Development Commands

```bash
go build -o sitecap
go test -v
```
## Architecture

### Core Components

- **main.go**: Entry point with mode selection, CLI argument parsing, and core browser automation logic
- **http.go**: HTTP server implementation with logging middleware and request handling
- **mcp_server.go**: Model Context Protocol server implementation
- **mcp_tools.go**: MCP tool definitions and handlers
- **mcp_context_config.go**: Browser context configuration management for MCP
- **mcp_request_history.go**: Request history tracking for MCP mode
- **domains.go**: Domain whitelisting logic with wildcard pattern matching support
- **resize.go**: Image processing using libvips with complex resize parameter parsing
- **viewport.go**: Browser viewport dimension parsing and validation
- **timeout.go**: Request timeout parsing and validation
- **metrics.go**: Prometheus metrics collection

### Key Dependencies

- `github.com/go-rod/rod`: Chrome DevTools Protocol automation
- `github.com/cshum/vipsgen/vips`: High-performance image processing
- `github.com/modelcontextprotocol/go-sdk`: Model Context Protocol server implementation

### Request Flow

1. Mode selection (CLI, HTTP server, or MCP server)
2. Configuration parsing (CLI args, HTTP params, or MCP context)
3. Browser instance created via Rod with request hijacking
4. Custom headers, domain filtering, and debug logging setup
5. Page navigation with viewport/timeout settings
6. Content capture (screenshot, HTML, cookies) based on request type
7. Optional image processing via libvips
8. Result delivery (stdout, HTTP response, or MCP response)

### HTTP Server Architecture

- Main handler at `/` for screenshots and content capture
- Metrics endpoint at `/metrics` (Prometheus format)
- Access logging middleware with detailed request information
- Support for all CLI parameters via query string

### MCP Server Architecture

- Browsing context management with persistent configurations
- Request history tracking across sessions
- Tool-based interface for screenshot capture, HTML extraction, and context management
- Cookie and state management across requests

## Usage Patterns

### Command Line - Screenshot
```bash
sitecap --viewport 1920x1080 --resize 800x600 --timeout 30 --domains "example.com,*.cdn.com" --debug https://example.com > screenshot.png
```

### Command Line - HTML Output
```bash
sitecap --html --viewport 1920x1080 --timeout 30 https://example.com > page.html
```

### Command Line - JSON Output (HTML + Cookies)
```bash
sitecap --json --viewport 1920x1080 --timeout 30 https://example.com > response.json
```

### Command Line - HTML from Stdin
```bash
echo "<html><body><h1>Test</h1></body></html>" | sitecap --debug - > screenshot.png
```

### HTTP Server
```bash
# Start server
sitecap --http --listen localhost:9191

# Screenshot request
curl "http://localhost:9191/?url=https://example.com&viewport=1920x1080&resize=800x600&timeout=30&domains=example.com,*.cdn.com" > screenshot.png

# HTML extraction
curl "http://localhost:9191/?url=https://example.com&html=true" > page.html

# JSON response with all data
curl "http://localhost:9191/?url=https://example.com&json=true" > response.json
```

### MCP Server
```bash
# Start MCP server
sitecap --mcp

# Test with MCP inspector
make test_mcp
```

### Custom Headers
```bash
sitecap --headers '{"Authorization":"Bearer token","User-Agent":"Custom Agent"}' https://example.com > screenshot.png
```

### Resize Parameter Syntax
- `WxH` - Aspect ratio maintained
- `WxH!` - Force exact dimensions
- `WxH#` or `WxH^` - Center crop to exact dimensions
- `P%xP%` - Percentage scaling
- `WxH+X+Y` or `WxH_X_Y` - Manual crop with offset
