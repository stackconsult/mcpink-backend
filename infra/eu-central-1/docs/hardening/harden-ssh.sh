#!/bin/bash
# SSH hardening script for run nodes
# Disables password authentication, requires key-based auth only
set -e

echo "Hardening SSH..."

# Backup original config
cp /etc/ssh/sshd_config /etc/ssh/sshd_config.bak

# Disable password authentication
sed -i 's/#PasswordAuthentication yes/PasswordAuthentication no/' /etc/ssh/sshd_config
sed -i 's/PasswordAuthentication yes/PasswordAuthentication no/' /etc/ssh/sshd_config

# Disable root password login (key only)
sed -i 's/#PermitRootLogin yes/PermitRootLogin prohibit-password/' /etc/ssh/sshd_config
sed -i 's/PermitRootLogin yes/PermitRootLogin prohibit-password/' /etc/ssh/sshd_config

# Disable empty passwords
sed -i 's/#PermitEmptyPasswords no/PermitEmptyPasswords no/' /etc/ssh/sshd_config
sed -i 's/PermitEmptyPasswords yes/PermitEmptyPasswords no/' /etc/ssh/sshd_config

# Restart SSH
systemctl restart sshd

echo ""
echo "SSH hardened successfully!"
echo ""
echo "IMPORTANT: Test SSH access in a SEPARATE terminal before disconnecting!"
echo "  ssh root@<server-ip>"
echo ""
echo "If locked out, use Hetzner console to restore:"
echo "  cp /etc/ssh/sshd_config.bak /etc/ssh/sshd_config && systemctl restart sshd"
