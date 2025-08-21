#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Sitecap Installation Script${NC}"
echo "================================"

# Check if running as root
if [[ $EUID -eq 0 ]]; then
   echo -e "${RED}This script should not be run as root${NC}"
   echo "Please run as a regular user with sudo privileges"
   exit 1
fi

# Check if binary exists
if [ ! -f "./sitecap" ]; then
    echo -e "${YELLOW}Binary not found. Building...${NC}"
    if command -v make >/dev/null 2>&1; then
        make build
    elif command -v go >/dev/null 2>&1; then
        go build -o sitecap
    else
        echo -e "${RED}Neither make nor go found. Please build the binary first.${NC}"
        exit 1
    fi
fi

# Get listen address from user
echo ""
read -p "Enter listen address (default: localhost:8080): " LISTEN_ADDR
LISTEN_ADDR=${LISTEN_ADDR:-"localhost:8080"}

echo -e "${YELLOW}Installing sitecap service...${NC}"

# Create user and directories
echo "Creating sitecap user and directories..."
sudo useradd --system --shell /bin/false --home /var/lib/sitecap sitecap 2>/dev/null || true
sudo mkdir -p /var/lib/sitecap
sudo chown sitecap:sitecap /var/lib/sitecap

# Install binary
echo "Installing binary to /usr/local/bin/..."
sudo cp sitecap /usr/local/bin/
sudo chmod +x /usr/local/bin/sitecap

# Create customized service file
echo "Creating systemd service file..."
sed "s|--listen localhost:8080|--listen $LISTEN_ADDR|g" sitecap.service > /tmp/sitecap.service
sudo mv /tmp/sitecap.service /etc/systemd/system/

# Enable and start service
echo "Enabling and starting service..."
sudo systemctl daemon-reload
sudo systemctl enable sitecap
sudo systemctl start sitecap

# Check status
sleep 2
if sudo systemctl is-active --quiet sitecap; then
    echo -e "${GREEN}✓ Sitecap service installed and started successfully!${NC}"
    echo ""
    echo "Service is running on: http://$LISTEN_ADDR"
    echo "Metrics available at: http://$LISTEN_ADDR/metrics"
    echo ""
    echo "Useful commands:"
    echo "  sudo systemctl status sitecap    # Check service status"
    echo "  sudo journalctl -u sitecap -f    # View logs"
    echo "  sudo systemctl restart sitecap   # Restart service"
else
    echo -e "${RED}✗ Service failed to start. Check logs with: sudo journalctl -u sitecap${NC}"
    exit 1
fi