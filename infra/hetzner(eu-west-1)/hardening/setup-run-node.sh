#!/bin/bash
# One-command setup script for run nodes
# Usage: ./setup-run-node.sh <run-node-ip>
set -e

if [ -z "$1" ]; then
    echo "Usage: $0 <run-node-ip>"
    echo ""
    echo "This script applies all baseline security hardening to a run node:"
    echo "  - Egress firewall rules (blocks metadata, SMTP, mining pools, IRC, Tor)"
    echo "  - gVisor sandbox installation (not set as default)"
    echo "  - Docker daemon configuration (live-restore, log limits)"
    echo "  - Miner detection cron job"
    echo ""
    echo "After running, gVisor can be tested with:"
    echo "  docker run --runtime=runsc --rm hello-world"
    exit 1
fi

RUN_NODE_IP="$1"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "=== Setting up run node: ${RUN_NODE_IP} ==="
echo ""

# Verify we can connect
echo "Verifying SSH connectivity..."
if ! ssh -o ConnectTimeout=10 -o BatchMode=yes root@${RUN_NODE_IP} "echo 'SSH OK'" 2>/dev/null; then
    echo "ERROR: Cannot connect to root@${RUN_NODE_IP}"
    echo "Ensure SSH key is configured and server is reachable."
    exit 1
fi

echo ""
echo "Step 1/6: Copying scripts..."
scp -q "${SCRIPT_DIR}/setup-egress-rules.sh" \
    "${SCRIPT_DIR}/install-gvisor.sh" \
    "${SCRIPT_DIR}/detect-miners.sh" \
    "${SCRIPT_DIR}/verify-hardening.sh" \
    root@${RUN_NODE_IP}:/root/

ssh root@${RUN_NODE_IP} "chmod +x /root/*.sh"
echo "  Scripts copied and made executable."

echo ""
echo "Step 2/6: Applying egress firewall rules..."
ssh root@${RUN_NODE_IP} "bash /root/setup-egress-rules.sh"

echo ""
echo "Step 3/6: Installing gVisor..."
ssh root@${RUN_NODE_IP} "bash /root/install-gvisor.sh"

echo ""
echo "Step 4/6: Configuring Docker daemon (baseline - gVisor available but not default)..."
# Check current containers
echo "  Current containers:"
ssh root@${RUN_NODE_IP} "docker ps --format '  - {{.Names}}'"

# Deploy baseline config
scp -q "${SCRIPT_DIR}/daemon-baseline.json" root@${RUN_NODE_IP}:/etc/docker/daemon.json
ssh root@${RUN_NODE_IP} "systemctl restart docker"
echo "  Docker restarted with baseline config."

# Verify containers survived
echo "  Verifying containers after restart..."
ssh root@${RUN_NODE_IP} "docker ps --format '  - {{.Names}}'"

echo ""
echo "Step 5/6: Setting up miner detection cron..."
ssh root@${RUN_NODE_IP} 'crontab -l 2>/dev/null | grep -v "detect-miners" | { cat; echo "*/5 * * * * /root/detect-miners.sh"; } | crontab -'
echo "  Cron job configured to run every 5 minutes."

echo ""
echo "Step 6/6: Verifying setup..."
ssh root@${RUN_NODE_IP} "bash /root/verify-hardening.sh"

echo ""
echo "=========================================="
echo "=== Setup complete for ${RUN_NODE_IP} ==="
echo "=========================================="
echo ""
echo "Next steps:"
echo ""
echo "1. Test gVisor manually:"
echo "   ssh root@${RUN_NODE_IP} 'docker run --runtime=runsc --rm hello-world'"
echo ""
echo "2. Test gVisor with Alpine:"
echo "   ssh root@${RUN_NODE_IP} 'docker run --runtime=runsc --rm alpine echo gVisor works'"
echo ""
echo "3. When ready to make gVisor the default runtime:"
echo "   scp ${SCRIPT_DIR}/daemon.json root@${RUN_NODE_IP}:/etc/docker/daemon.json"
echo "   ssh root@${RUN_NODE_IP} 'systemctl restart docker'"
echo ""
echo "4. Optional - Apply additional hardening:"
echo "   - SSH hardening: scp ${SCRIPT_DIR}/harden-ssh.sh root@${RUN_NODE_IP}:/root/ && ssh root@${RUN_NODE_IP} 'bash /root/harden-ssh.sh'"
echo "   - Host firewall: scp ${SCRIPT_DIR}/setup-host-firewall.sh root@${RUN_NODE_IP}:/root/ && ssh root@${RUN_NODE_IP} 'bash /root/setup-host-firewall.sh'"
echo ""
