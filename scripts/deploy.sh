#!/usr/bin/env bash
# Deploy rigctlweb to a Linux host over SSH as a systemd user service.
# Usage: scripts/deploy.sh user@host [arm64|amd64]
# Needs SSH key auth (ssh-copy-id user@host). The access token is printed once on first start.
set -euo pipefail
# Extra ssh/scp options, e.g. SSH_OPTS="-i ~/.ssh/mykey -o IdentitiesOnly=yes"
SSH_OPTS="${SSH_OPTS:-}"
ssh() { command ssh $SSH_OPTS "$@"; }
scp() { command scp $SSH_OPTS "$@"; }
TARGET="${1:?usage: deploy.sh user@host [arm64|amd64]}"
ARCH="${2:-$(ssh "$TARGET" uname -m | sed 's/aarch64/arm64/; s/x86_64/amd64/')}"
cd "$(dirname "$0")/.."

mise run web
GOOS=linux GOARCH="$ARCH" CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(git describe --tags --always --dirty)" \
  -o "bin/rigctlweb-linux-$ARCH" ./cmd/rigctlweb

ssh "$TARGET" 'mkdir -p ~/.local/bin ~/.config/systemd/user ~/.config/rigctlweb'
scp "bin/rigctlweb-linux-$ARCH" "$TARGET:.local/bin/rigctlweb.new"
scp packaging/systemd/rigctlweb.service "$TARGET:.config/systemd/user/rigctlweb.service"
ssh "$TARGET" 'chmod +x ~/.local/bin/rigctlweb.new && mv ~/.local/bin/rigctlweb.new ~/.local/bin/rigctlweb \
  && systemctl --user daemon-reload && systemctl --user enable --now rigctlweb \
  && systemctl --user restart rigctlweb && sleep 1 && systemctl --user --no-pager status rigctlweb | head -5'
echo "Token (in config): ssh $TARGET \"grep '^token' ~/.config/rigctlweb/config.toml\""
echo "Remember: loginctl enable-linger \$USER  (keeps the service running when logged out)"
