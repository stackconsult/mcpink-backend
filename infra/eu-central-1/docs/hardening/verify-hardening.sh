#!/bin/bash
# Security verification script for run nodes
# Checks all hardening measures are in place

echo "=== Security Verification ==="
echo ""

# Track overall status
PASS=0
FAIL=0
INFO=0

check_pass() {
    echo "  [OK] $1"
    ((PASS++))
}

check_fail() {
    echo "  [FAIL] $1"
    ((FAIL++))
}

check_info() {
    echo "  [INFO] $1"
    ((INFO++))
}

# 1. gVisor
echo "1. gVisor:"
if runsc --version &>/dev/null; then
    check_pass "gVisor installed ($(runsc --version 2>&1 | head -1))"
else
    check_fail "gVisor not installed"
fi

if docker info 2>/dev/null | grep -q "Default Runtime: runsc"; then
    check_pass "gVisor is default runtime"
else
    check_info "gVisor not default runtime (may be intentional)"
fi

# 2. Egress Rules
echo ""
echo "2. Egress Rules:"
if iptables -L DOCKER-USER -n 2>/dev/null | grep -q "169.254.169.254"; then
    check_pass "Cloud metadata endpoint blocked"
else
    check_fail "Cloud metadata endpoint NOT blocked"
fi

if iptables -L DOCKER-USER -n 2>/dev/null | grep -q "dpt:25"; then
    check_pass "SMTP port 25 blocked"
else
    check_fail "SMTP port 25 NOT blocked"
fi

if iptables -L DOCKER-USER -n 2>/dev/null | grep -q "dpt:3333"; then
    check_pass "Mining pool port 3333 blocked"
else
    check_fail "Mining pool port 3333 NOT blocked"
fi

if iptables -L DOCKER-USER -n 2>/dev/null | grep -q "dpt:6667"; then
    check_pass "IRC port 6667 blocked"
else
    check_fail "IRC port 6667 NOT blocked"
fi

# 3. Miner Detection
echo ""
echo "3. Miner Detection:"
if crontab -l 2>/dev/null | grep -q "detect-miners"; then
    check_pass "Miner detection cron configured"
else
    check_fail "Miner detection cron NOT configured"
fi

if [ -x /root/detect-miners.sh ]; then
    check_pass "detect-miners.sh is executable"
else
    check_fail "detect-miners.sh not found or not executable"
fi

# 4. Docker Configuration
echo ""
echo "4. Docker Configuration:"
if docker info 2>/dev/null | grep -q "Live Restore Enabled: true"; then
    check_pass "Docker live-restore enabled"
else
    check_fail "Docker live-restore NOT enabled"
fi

if [ -f /etc/docker/daemon.json ]; then
    check_pass "Docker daemon.json exists"
else
    check_fail "Docker daemon.json missing"
fi

# 5. System Containers
echo ""
echo "5. System Containers:"
if docker ps --format '{{.Names}}' 2>/dev/null | grep -qE 'traefik|coolify-proxy'; then
    check_pass "Reverse proxy (Traefik/coolify-proxy) is running"
else
    check_fail "Reverse proxy is NOT running"
fi

if docker ps --format '{{.Names}}' 2>/dev/null | grep -q coolify; then
    check_pass "Coolify services running"
else
    check_info "Coolify services not detected (may be Factory-only)"
fi

# 6. SSH Hardening
echo ""
echo "6. SSH Hardening:"
if grep -q "^PasswordAuthentication no" /etc/ssh/sshd_config 2>/dev/null; then
    check_pass "SSH password auth disabled"
else
    check_info "SSH password auth may be enabled"
fi

if grep -q "^PermitRootLogin prohibit-password" /etc/ssh/sshd_config 2>/dev/null; then
    check_pass "SSH root login key-only"
else
    check_info "SSH root login may allow passwords"
fi

# 7. Host Firewall
echo ""
echo "7. Host Firewall:"
if command -v ufw &>/dev/null && ufw status 2>/dev/null | grep -q "Status: active"; then
    check_pass "UFW firewall is active"
else
    check_info "UFW firewall not active"
fi

# 8. Network Tests
echo ""
echo "8. Network Tests:"
echo "   Testing SMTP block (should timeout)..."
if timeout 3 nc -zv smtp.gmail.com 25 &>/dev/null; then
    check_fail "SMTP is reachable (should be blocked)"
else
    check_pass "SMTP is blocked"
fi

# Summary
echo ""
echo "=== Summary ==="
echo "  Passed: ${PASS}"
echo "  Failed: ${FAIL}"
echo "  Info:   ${INFO}"
echo ""

if [ "$FAIL" -gt 0 ]; then
    echo "WARNING: Some security checks failed. Review above output."
    exit 1
else
    echo "All critical security checks passed."
    exit 0
fi
