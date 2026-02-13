#!/bin/bash
# gVisor installation script for run nodes
set -e

echo "Installing gVisor..."

ARCH=$(uname -m)
URL="https://storage.googleapis.com/gvisor/releases/release/latest/${ARCH}"

echo "Downloading gVisor for ${ARCH}..."
wget "${URL}/runsc" -O /usr/local/bin/runsc
wget "${URL}/containerd-shim-runsc-v1" -O /usr/local/bin/containerd-shim-runsc-v1

chmod +x /usr/local/bin/runsc /usr/local/bin/containerd-shim-runsc-v1

echo ""
echo "gVisor installed successfully!"
runsc --version

echo ""
echo "Next steps:"
echo "1. Update /etc/docker/daemon.json with gVisor runtime config"
echo "2. Run: systemctl restart docker"
echo "3. Test: docker run --runtime=runsc --rm hello-world"
