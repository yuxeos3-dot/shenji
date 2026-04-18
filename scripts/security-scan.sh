#!/usr/bin/env bash
# security-scan.sh — run all recommended security scanners.
#
# Prerequisites:
#   - Go toolchain (for govulncheck)
#   - Node.js / npm  (for npm audit)
#   - gitleaks       (for git history secret scanning)
#
# Usage:
#   ./scripts/security-scan.sh
#
# Each section can also be run independently by copy-pasting the
# relevant commands.

set -euo pipefail
cd "$(dirname "$0")/.."

echo "========================================"
echo " 1. Go dependency vulnerability scan"
echo "========================================"
echo ""
echo "Install govulncheck if not present:"
echo "  go install golang.org/x/vuln/cmd/govulncheck@latest"
echo ""
if command -v govulncheck &>/dev/null; then
  govulncheck ./...
else
  echo "[SKIP] govulncheck not found — install with:"
  echo "  go install golang.org/x/vuln/cmd/govulncheck@latest"
fi

echo ""
echo "========================================"
echo " 2. npm dependency audit"
echo "========================================"
echo ""
if [ -d "frontend" ] && command -v npm &>/dev/null; then
  (cd frontend && npm audit --production 2>&1) || true
else
  echo "[SKIP] npm not found or frontend/ missing"
  echo "  cd frontend && npm audit --production"
fi

echo ""
echo "========================================"
echo " 3. Git history secret scan (#100)"
echo "========================================"
echo ""
echo "Install gitleaks if not present:"
echo "  brew install gitleaks  # macOS"
echo "  go install github.com/gitleaks/gitleaks/v8@latest  # or via Go"
echo ""
if command -v gitleaks &>/dev/null; then
  gitleaks detect --source . --verbose
else
  echo "[SKIP] gitleaks not found — install and run:"
  echo "  gitleaks detect --source . --verbose"
fi

echo ""
echo "========================================"
echo " Done"
echo "========================================"
