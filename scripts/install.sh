#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
#  The Hive — Installer
#  https://github.com/Aether-Labs-Studio/the-hive
#
#  Supported platforms: macOS (darwin), Linux (linux)
# ─────────────────────────────────────────────────────────────────────────────
set -euo pipefail

BINARY="hive"
REPO="Aether-Labs-Studio/the-hive"
HIVE_DIR="$HOME/.hive_data"
INSTALL_DIR="/usr/local/bin"
YES="${YES:-0}"

# ── Colors ────────────────────────────────────────────────────────────────────
if [ -t 1 ]; then
  GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'
  BLUE='\033[0;34m'; BOLD='\033[1m'; NC='\033[0m'
else
  GREEN=''; YELLOW=''; RED=''; BLUE=''; BOLD=''; NC=''
fi

info()    { printf "  ${BLUE}→${NC} %s\n" "$*"; }
success() { printf "  ${GREEN}✓${NC} %s\n" "$*"; }
warn()    { printf "  ${YELLOW}!${NC} %s\n" "$*" >&2; }
fatal()   { printf "  ${RED}✗${NC} %s\n" "$*" >&2; exit 1; }

# ── Platform detection ────────────────────────────────────────────────────────
detect_platform() {
  OS=$(uname -s | tr '[:upper:]' '[:lower:]')
  ARCH=$(uname -m)

  case "$OS" in
    linux)  OS="linux"  ;;
    darwin) OS="darwin" ;;
    *)      fatal "Unsupported OS: $OS." ;;
  esac

  case "$ARCH" in
    x86_64|amd64)  ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *)             fatal "Unsupported architecture: $ARCH." ;;
  esac
}

# ── Check for Go ──────────────────────────────────────────────────────────────
check_go() {
  if ! command -v go >/dev/null 2>&1; then
    fatal "Go is not installed. Please install Go 1.25+ to compile The Hive."
  fi
  GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
  # Simple version check (assumes 1.x)
  MAJOR=$(echo "$GO_VERSION" | cut -d. -f1)
  MINOR=$(echo "$GO_VERSION" | cut -d. -f2)
  if [ "$MAJOR" -lt 1 ] || { [ "$MAJOR" -eq 1 ] && [ "$MINOR" -lt 25 ]; }; then
    warn "The Hive recommends Go 1.25+. Found: $GO_VERSION. Compilation might fail."
  fi
}

# ── Compile binary ────────────────────────────────────────────────────────────
compile() {
  info "Compiling The Hive..."
  go build -o "bin/$BINARY" ./cmd/hive
  success "Compilation complete: bin/$BINARY"
}

# ── Install binary ────────────────────────────────────────────────────────────
install_binary() {
  if [ -w "$INSTALL_DIR" ]; then
    mv "bin/$BINARY" "$INSTALL_DIR/$BINARY"
  else
    info "Installing to $INSTALL_DIR (requires sudo)..."
    sudo mv "bin/$BINARY" "$INSTALL_DIR/$BINARY"
  fi
  chmod +x "$INSTALL_DIR/$BINARY"
  success "Binary installed → $INSTALL_DIR/$BINARY"
}

# ── Setup ~/.hive_data ───────────────────────────────────────────────────────
setup_data_dir() {
  mkdir -p "$HIVE_DIR"
  success "Data directory created → $HIVE_DIR"
}

# ── MCP client auto-configuration ────────────────────────────────────────────
confirm() {
  local msg="$1"
  [ "$YES" = "1" ] && return 0
  if [ -e /dev/tty ]; then
    printf "  %s [y/N] " "$msg" >/dev/tty
    read -r REPLY </dev/tty
  else
    return 1
  fi
  case "$REPLY" in [yY]*) return 0 ;; *) return 1 ;; esac
}

# Advanced JSON patcher using Python 3
# rc=2: Already configured correctly
# rc=3: Configured with a different command
patch_json_mcp() {
  local file="$1" key="$2" value_str="$3"
  
  if ! command -v python3 >/dev/null 2>&1; then
    return 1
  fi

  python3 - "$file" "$key" "$value_str" <<'PYEOF'
import sys, json, os, re

def load_jsonc(path):
    """Load JSON that may contain trailing commas."""
    with open(path) as f:
        content = f.read()
    content = re.sub(r',(\s*[}\]])', r'\1', content)
    return json.loads(content)

path, key, value_str = sys.argv[1], sys.argv[2], sys.argv[3]
value = json.loads(value_str)

try:
    if os.path.exists(path):
        data = load_jsonc(path)
    else:
        data = {}
except (json.JSONDecodeError, OSError):
    sys.exit(1)

# Handle different config structures
mcp_key = "mcpServers"
if "servers" in data: mcp_key = "servers"
if "mcp" in data: mcp_key = "mcp"

existing = data.get(mcp_key, {}).get(key)
if existing is not None:
    # Compare commands for equality
    if existing.get("command") == value.get("command"):
        sys.exit(2)
    else:
        sys.exit(3)

data.setdefault(mcp_key, {})[key] = value

with open(path, "w") as f:
    json.dump(data, f, indent=2)
    f.write("\n")
PYEOF
}

handle_mcp_rc() {
  local rc="$1" name="$2" path="$3"
  case "$rc" in
    0) success "$name configured → $path" ;;
    2) info "$name is already configured." ;;
    3) warn "$name is configured with a different path. Manual update may be needed." ;;
    *) warn "Could not auto-configure $name. Please add the entry manually to $path" ;;
  esac
}

configure_clients() {
  local binary_path="$INSTALL_DIR/$BINARY"
  local configured=0
  
  local entry
  entry=$(printf '{"command":"%s","args":["serve"],"env":{}}' "$binary_path")

  printf "\n  ${BOLD}Detecting MCP clients...${NC}\n"

  # ── Claude Code ──────────────────────────────────────────────────────────
  local claude_json="$HOME/.claude.json"
  if [ -f "$claude_json" ] || command -v claude >/dev/null 2>&1; then
    printf "\n  ${BOLD}Claude Code${NC} detected\n"
    if confirm "Configure Hive in Claude Code (~/.claude.json)?"; then
      rc=0; patch_json_mcp "$claude_json" "hive" "$entry" || rc=$?
      handle_mcp_rc "$rc" "Claude Code" "$claude_json"
    fi
  fi

  # ── Gemini CLI ───────────────────────────────────────────────────────────
  local gemini_settings="$HOME/.gemini/settings.json"
  if [ -f "$gemini_settings" ] || [ -d "$HOME/.gemini" ]; then
    printf "\n  ${BOLD}Gemini CLI${NC} detected\n"
    if confirm "Configure Hive in Gemini CLI (~/.gemini/settings.json)?"; then
      mkdir -p "$HOME/.gemini"
      rc=0; patch_json_mcp "$gemini_settings" "hive" "$entry" || rc=$?
      handle_mcp_rc "$rc" "Gemini CLI" "$gemini_settings"
    fi
  fi

  # ── Cursor ───────────────────────────────────────────────────────────────
  local cursor_mcp="$HOME/.cursor/mcp.json"
  if [ -f "$cursor_mcp" ] || [ -d "$HOME/.cursor" ]; then
    printf "\n  ${BOLD}Cursor${NC} detected\n"
    if confirm "Configure Hive in Cursor (~/.cursor/mcp.json)?"; then
      mkdir -p "$HOME/.cursor"
      rc=0; patch_json_mcp "$cursor_mcp" "hive" "$entry" || rc=$?
      handle_mcp_rc "$rc" "Cursor" "$cursor_mcp"
    fi
  fi

  # ── VS Code ──────────────────────────────────────────────────────────────
  local vscode_mcp="$HOME/.vscode/mcp.json"
  if [ -f "$vscode_mcp" ] || command -v code >/dev/null 2>&1 || [ -d "$HOME/.vscode" ]; then
    printf "\n  ${BOLD}VS Code${NC} detected\n"
    if confirm "Configure Hive in VS Code (~/.vscode/mcp.json)?"; then
      mkdir -p "$HOME/.vscode"
      rc=0; patch_json_mcp "$vscode_mcp" "hive" "$entry" || rc=$?
      handle_mcp_rc "$rc" "VS Code" "$vscode_mcp"
    fi
  fi

  # ── Antigravity ──────────────────────────────────────────────────────────
  local antigravity_mcp="$HOME/.gemini/antigravity/mcp_config.json"
  if [ -d "$HOME/.gemini/antigravity" ] || [ -f "$antigravity_mcp" ]; then
    printf "\n  ${BOLD}Antigravity${NC} detected\n"
    if confirm "Configure Hive in Antigravity (~/.gemini/antigravity/mcp_config.json)?"; then
      mkdir -p "$HOME/.gemini/antigravity"
      rc=0; patch_json_mcp "$antigravity_mcp" "hive" "$entry" || rc=$?
      handle_mcp_rc "$rc" "Antigravity" "$antigravity_mcp"
    fi
  fi

  # ── OpenCode ─────────────────────────────────────────────────────────────
  local opencode_config="$HOME/.config/opencode/opencode.json"
  if [ -f "$opencode_config" ] || command -v opencode >/dev/null 2>&1; then
    printf "\n  ${BOLD}OpenCode${NC} detected\n"
    if confirm "Configure Hive in OpenCode (~/.config/opencode/opencode.json)?"; then
      mkdir -p "$HOME/.config/opencode"
      local opencode_entry
      opencode_entry=$(printf '{"type":"local","command":["%s","serve"],"enabled":true}' "$binary_path")
      rc=0; patch_json_mcp "$opencode_config" "hive" "$opencode_entry" || rc=$?
      handle_mcp_rc "$rc" "OpenCode" "$opencode_config"
    fi
  fi

  # ── Codex ────────────────────────────────────────────────────────────────
  local codex_config="$HOME/.codex/config.toml"
  if [ -f "$codex_config" ] || command -v codex >/dev/null 2>&1; then
    printf "\n  ${BOLD}Codex${NC} detected\n"
    if confirm "Configure Hive in Codex (~/.codex/config.toml)?"; then
      mkdir -p "$HOME/.codex"
      if ! grep -q '\[mcp_servers\.hive\]' "$codex_config" 2>/dev/null; then
        printf '\n[mcp_servers.hive]\ncommand = "%s"\nargs = ["serve"]\n' "$binary_path" >> "$codex_config"
        success "Codex configured."
      else
        info "Codex already configured."
      fi
    fi
  fi
}

# ── Main ──────────────────────────────────────────────────────────────────────
main() {
  printf "\n  ${BOLD}🐝 The Hive — Installer${NC}\n\n"

  detect_platform
  check_go
  compile
  install_binary
  setup_data_dir
  configure_clients

  printf "\n  ${GREEN}${BOLD}✅ The Hive configured successfully!${NC}\n\n"
  printf "  Next steps:\n"
  printf "    1. Start a node: ${BOLD}hive serve${NC}\n"
  printf "    2. Open Monitor: ${BOLD}http://localhost:7439${NC}\n\n"
}

main
