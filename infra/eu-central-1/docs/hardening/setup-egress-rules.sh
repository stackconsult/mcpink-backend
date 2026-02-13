#!/bin/bash
# Egress firewall rules for run nodes
# Blocks known malicious/abusive traffic patterns
set -e

echo "Setting up egress firewall rules..."

# === CRITICAL BLOCKS ===

# Cloud metadata (AWS/GCP/Azure credential theft)
echo "Blocking cloud metadata endpoints..."
iptables -I DOCKER-USER -d 169.254.169.254 -j DROP

# SMTP (spam prevention) - blocks all email sending
echo "Blocking SMTP ports (spam prevention)..."
iptables -I DOCKER-USER -p tcp --dport 25 -j DROP
iptables -I DOCKER-USER -p tcp --dport 465 -j DROP
iptables -I DOCKER-USER -p tcp --dport 587 -j DROP

# Common mining pool ports
echo "Blocking common mining pool ports..."
iptables -I DOCKER-USER -p tcp --dport 3333 -j DROP
iptables -I DOCKER-USER -p tcp --dport 4444 -j DROP
iptables -I DOCKER-USER -p tcp --dport 5555 -j DROP
iptables -I DOCKER-USER -p tcp --dport 7777 -j DROP
iptables -I DOCKER-USER -p tcp --dport 9999 -j DROP

# IRC (botnet C&C)
echo "Blocking IRC ports (botnet prevention)..."
iptables -I DOCKER-USER -p tcp --dport 6667 -j DROP
iptables -I DOCKER-USER -p tcp --dport 6697 -j DROP

# Tor directory/relay ports
echo "Blocking Tor ports..."
iptables -I DOCKER-USER -p tcp --dport 9001 -j DROP
iptables -I DOCKER-USER -p tcp --dport 9030 -j DROP

# === PERSIST ===
echo "Installing iptables-persistent..."
DEBIAN_FRONTEND=noninteractive apt-get install -y iptables-persistent

echo "Saving rules..."
netfilter-persistent save

echo "Egress rules configured successfully!"
echo ""
echo "Verify with: iptables -L DOCKER-USER -n"
