#!/usr/bin/env bash
# Installs the decrivo git hooks. Safe to run more than once.
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
SRC="$ROOT/.claude/skills/commit-blocks/scripts/commit-msg"
DEST_DIR="$(git rev-parse --git-path hooks)"

mkdir -p "$DEST_DIR"
cp "$SRC" "$DEST_DIR/commit-msg"
chmod +x "$DEST_DIR/commit-msg"
git config commit.template .gitmessage

echo "installed: $DEST_DIR/commit-msg"
echo "configured: commit.template = .gitmessage"
