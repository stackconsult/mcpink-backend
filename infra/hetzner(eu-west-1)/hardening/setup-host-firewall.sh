#!/bin/bash
# Host firewall setup using UFW for run nodes
# Allows only SSH, HTTP, and HTTPS traffic
set -e

echo "Setting up host firewall..."

# Install UFW if not present
apt-get update && apt-get install -y ufw

# Reset to defaults
ufw --force reset

# Default policies
ufw default deny incoming
ufw default allow outgoing

# Allow essential services
ufw allow 22/tcp comment 'SSH'
ufw allow 80/tcp comment 'HTTP'
ufw allow 443/tcp comment 'HTTPS'

# Enable firewall (non-interactive)
ufw --force enable

echo ""
echo "Host firewall configured successfully!"
echo ""
ufw status verbose
