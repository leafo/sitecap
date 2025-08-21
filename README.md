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

Or listen on all interfaces (use with caution):
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