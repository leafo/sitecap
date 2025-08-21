# sitecap

Command line tool for taking screenshots of websites.

## Installation

```bash
go install github.com/leafo/sitecap@latest
```

## Usage

### Command Line Mode

```bash
sitecap [--resize WxH] <URL> > screenshot.png
```

Examples:
```bash
# Basic screenshot
sitecap https://example.com > example.png

# Resize to 800x600 maintaining aspect ratio
sitecap --resize 800x600 https://example.com > resized.png

# Force exact dimensions (ignore aspect ratio)
sitecap --resize 800x600! https://example.com > stretched.png

# Resize and center crop to exact dimensions
sitecap --resize 800x600# https://example.com > cropped.png

# Resize by percentage
sitecap --resize 50%x50% https://example.com > half-size.png

# Crop manually with offset
sitecap --resize 200x200+100+50 https://example.com > cropped-offset.png
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

# With resize parameter
curl "http://localhost:8080/?url=https://example.com&resize=800x600" > resized.png

# Force exact dimensions
curl "http://localhost:8080/?url=https://example.com&resize=800x600!" > stretched.png

# Resize and center crop (URL-safe)
curl "http://localhost:8080/?url=https://example.com&resize=800x600^" > cropped.png

# Manual crop with offset (URL-safe)
curl "http://localhost:8080/?url=https://example.com&resize=200x200_100_50" > crop-offset.png
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

## Systemd Service Installation

### Quick Install

Use the provided install script for automatic setup:

```bash
./install.sh
```

### Manual Installation

To run sitecap as a systemd service:

### 1. Create the sitecap user and directories

```bash
sudo useradd --system --shell /bin/false --home /var/lib/sitecap sitecap
sudo mkdir -p /var/lib/sitecap
sudo chown sitecap:sitecap /var/lib/sitecap
```

### 2. Install the binary

```bash
# Build and install the binary
make build
sudo cp sitecap /usr/local/bin/
sudo chmod +x /usr/local/bin/sitecap
```

### 3. Install and configure the service

```bash
# Copy the service file
sudo cp sitecap.service /etc/systemd/system/

# Edit the service file to customize listen address/port if needed
sudo nano /etc/systemd/system/sitecap.service

# Reload systemd and enable the service
sudo systemctl daemon-reload
sudo systemctl enable sitecap
sudo systemctl start sitecap
```

### 4. Check service status

```bash
# Check if service is running
sudo systemctl status sitecap

# View logs
sudo journalctl -u sitecap -f
```

### Service Configuration

The default service configuration:
- Runs on `localhost:8080` (edit the service file to change)
- Uses a dedicated `sitecap` user for security
- Includes security hardening options
- Auto-restarts on failure
- Logs to systemd journal

To customize the listen address, edit the `ExecStart` line in `/etc/systemd/system/sitecap.service`.

**Security Note:** The service defaults to `localhost:8080` for security. Only bind to `0.0.0.0` if you need external access and have proper firewall rules in place.