package main

import (
	"flag"
	"fmt"
	"os"
)

const helpText = `sitecap - Website screenshot and content capture tool

DESCRIPTION
    Sitecap captures screenshots and extracts HTML content from websites using
    Chrome DevTools Protocol (CDP). It operates in three modes: command-line,
    HTTP server, and MCP (Model Context Protocol) server.

MODES
    Command Line (default)
        Capture a single screenshot or extract content from a URL.
        Output is written to stdout (PNG for screenshots, text for HTML/JSON).

    HTTP Server (--http)
        Run as a web service accepting requests via HTTP API.
        Endpoints: / (screenshot), /metrics (Prometheus metrics)
        Query parameters mirror CLI flags.

    MCP Server (--mcp)
        Run as a Model Context Protocol server for AI assistant integration.
        Supports stdio transport (default) or HTTP transport (with --http).

USAGE
    sitecap [options] <URL>                     Screenshot URL to stdout
    sitecap [options] - < input.html            Screenshot HTML from stdin
    sitecap --html [options] <URL>              Rendered HTML to stdout
    sitecap --json [options] <URL>              JSON with all captured data
    sitecap --http [--listen addr]              Start HTTP server
    sitecap --mcp                               Start MCP server (stdio)
    sitecap --http --mcp [--listen addr]        Start MCP server (HTTP)

OPTIONS

  Mode Selection:
    --http              Start HTTP server mode
    --mcp               Start MCP server mode (can combine with --http)
    --html              Output rendered HTML instead of screenshot
    --json              Output JSON with HTML, cookies, network, and console data

  Browser Configuration:
    --viewport WxH      Set browser viewport dimensions (e.g., 1920x1080)
                        Common: 1920x1080 (desktop), 375x667 (mobile)
    --full-height       Capture full page height (up to 10x viewport height)
    --timeout N         Timeout in seconds for page load (0 = no timeout)
    --wait N            Wait N seconds after page load before capture

  Image Processing:
    --resize SPEC       Resize the captured screenshot (see RESIZE SYNTAX)

  Network Control:
    --domains LIST      Comma-separated whitelist of allowed domains
                        Supports wildcards (see DOMAIN FILTERING)
    --headers JSON      Custom HTTP headers as JSON object
                        Example: '{"Authorization":"Bearer token"}'

  Server Options:
    --listen ADDR       Address for HTTP server (default: localhost:8080)

  Other:
    --debug             Log all network requests to stderr
    --version           Print version information and exit

RESIZE SYNTAX
    Resize the captured screenshot using these formats:

    Basic (maintain aspect ratio):
        WxH             Fit within dimensions       --resize 800x600
        Wx              Set width, auto height      --resize 800x
        xH              Set height, auto width      --resize x600

    Exact dimensions:
        WxH!            Force exact size            --resize 800x600!

    Center crop:
        WxH#            Resize and center crop      --resize 800x600#
        WxH^            Same (URL-safe alternative) --resize 800x600^

    Percentage:
        P%xP%           Scale by percentage         --resize 50%x50%

    Manual crop with offset:
        WxH+X+Y         Crop WxH at position X,Y    --resize 200x200+100+50
        WxH_X_Y         Same (URL-safe alternative) --resize 200x200_100_50

DOMAIN FILTERING
    Control which domains can load resources with --domains:

    Patterns:
        example.com         Exact domain match
        *.cdn.com           Wildcard subdomain match
        .example.com        Match domain and all subdomains

    Example: --domains "example.com,*.cloudfront.net,cdn.example.com"

    Benefits: Faster loading, reduced bandwidth, cleaner screenshots by
    blocking ads, trackers, and unnecessary third-party resources.

EXAMPLES

  Basic Screenshots:
    sitecap https://example.com > screenshot.png
    sitecap --viewport 1920x1080 https://example.com > desktop.png
    sitecap --viewport 375x667 https://example.com > mobile.png

  With Timeout and Wait:
    sitecap --timeout 30 https://slow-site.com > slow.png
    sitecap --wait 5 https://example.com > delayed.png

  Resize Operations:
    sitecap --resize 800x600 https://example.com > resized.png
    sitecap --resize 50%x50% https://example.com > half.png
    sitecap --resize 800x600# https://example.com > cropped.png

  HTML and JSON Output:
    sitecap --html https://example.com > page.html
    sitecap --json https://example.com > data.json

  From Stdin:
    echo "<h1>Hello</h1>" | sitecap - > hello.png
    sitecap --viewport 800x600 - < template.html > output.png

  HTTP Server:
    sitecap --http --listen :8080
    curl "http://localhost:8080/?url=https://example.com" > shot.png
    curl "http://localhost:8080/?url=https://example.com&viewport=1920x1080" > desktop.png
    curl "http://localhost:8080/?url=https://example.com&resize=800x600" > resized.png

  MCP Server:
    sitecap --mcp
    sitecap --http --mcp --listen :8080

  Advanced:
    sitecap --viewport 1920x1080 --resize 800x600 --timeout 30 \
            --wait 2 --domains "site.com,*.cdn.com" \
            --headers '{"X-Token":"secret"}' --debug \
            https://example.com > screenshot.png

HTTP API
    When running with --http, use query parameters:

    Screenshot (GET /):
        url             Required. URL to capture
        viewport        Browser viewport (e.g., 1920x1080)
        resize          Resize parameters (see RESIZE SYNTAX)
        full_height     Capture full page (true/false)
        timeout         Timeout in seconds
        wait            Wait time in seconds
        domains         Domain whitelist (comma-separated)
        html            Set to "true" for HTML output instead of PNG
        json            Set to "true" for JSON output with all data

    Metrics (GET /metrics):
        Prometheus-compatible metrics endpoint

MCP TOOLS
    When running with --mcp, these tools are available to MCP clients:

    configure_browser_context
        Set viewport, timeout, wait, cookies, headers for a named context

    list_browser_contexts
        List all configured contexts and their settings

    capture_screenshot_from_url
        Capture screenshot by navigating to a URL

    capture_screenshot_from_html
        Render arbitrary HTML and capture screenshot

    extract_html_content
        Get fully rendered HTML after JavaScript execution

    get_last_browser_request
        Retrieve details of the most recent request including network
        and console data

    CLI flags (--viewport, --timeout, --wait, --domains, --headers) set
    defaults for MCP contexts. Clients can override via configure_browser_context.

EXIT CODES
    0   Success
    1   Error (invalid parameters, capture failed, network error, etc.)

ENVIRONMENT
    ROD_BROWSER     Path to Chrome/Chromium executable
    ROD_HEADLESS    Set to "false" to show browser window (debugging)
`

func init() {
	flag.Usage = printUsage
}

func printUsage() {
	fmt.Fprint(os.Stderr, helpText)
}
