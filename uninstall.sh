#!/usr/bin/env bash
#
# Ai Arbuz Provider Api — one-command uninstaller.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/juushimatsu/ai-arbuz-provider-api/master/uninstall.sh | bash
#
#   # non-interactive full wipe (containers + image + SQLite data + secrets):
#   curl -fsSL https://raw.githubusercontent.com/juushimatsu/ai-arbuz-provider-api/master/uninstall.sh | bash -s -- --purge
#
# By default it stops & removes the containers and the Docker image, then ASKS
# [y/N] before deleting the SQLite database (your keys + logs) and the .env
# secrets. --purge answers "yes" automatically. --keep-data forces "no".
#
set -euo pipefail

# ---- settings -------------------------------------------------------------
REPO_NAME="ai-arbuz-provider-api"
INSTALL_DIR="${ARBUZ_DIR:-${HOME}/${REPO_NAME}}"

PURGE=0
KEEP_DATA=0
for a in "$@"; do
  case "$a" in
    --purge)     PURGE=1 ;;
    --keep-data) KEEP_DATA=1 ;;
    *) ;;
  esac
done

# ---- helpers --------------------------------------------------------------
c_g="\033[1;32m"; c_y="\033[1;33m"; c_r="\033[1;31m"; c_0="\033[0m"
log()  { printf "${c_g}==>${c_0} %s\n" "$*"; }
warn() { printf "${c_y}!! ${c_0} %s\n" "$*"; }
die()  { printf "${c_r}xx ${c_0} %s\n" "$*" >&2; exit 1; }

SUDO=""
if [ "$(id -u)" -ne 0 ] && ! docker info >/dev/null 2>&1; then
  command -v sudo >/dev/null 2>&1 && SUDO="sudo"
fi

# choose how to run docker in THIS session
if docker info >/dev/null 2>&1; then
  RUN() { bash -c "$*"; }
elif command -v sg >/dev/null 2>&1 && sg docker -c "docker info" >/dev/null 2>&1; then
  RUN() { sg docker -c "$*"; }
else
  RUN() { $SUDO bash -c "$*"; }
fi

# ---- 1. stop & remove containers + image ----------------------------------
if [ -f "$INSTALL_DIR/docker-compose.yml" ] || [ -f "$INSTALL_DIR/compose.yml" ]; then
  log "Stopping containers and removing the image in $INSTALL_DIR..."
  RUN "cd '$INSTALL_DIR' && docker compose down --rmi all --remove-orphans" || \
    warn "docker compose down reported an error — continuing."
else
  warn "No compose file in $INSTALL_DIR — nothing to stop. (Already removed?)"
fi

# ---- 2. decide whether to delete data -------------------------------------
DELETE_DATA=0
if [ "$PURGE" = "1" ]; then
  DELETE_DATA=1
elif [ "$KEEP_DATA" = "1" ]; then
  DELETE_DATA=0
else
  ans="n"
  if [ -r /dev/tty ]; then
    printf "${c_y}?? ${c_0} Delete ALL data (SQLite DB with keys & logs) and secrets (.env) in %s? [y/N] " "$INSTALL_DIR"
    read -r ans < /dev/tty || ans="n"
  else
    warn "No interactive terminal; keeping data. Re-run with --purge to wipe everything."
  fi
  case "$ans" in [yY]|[yY][eE][sS]) DELETE_DATA=1 ;; *) DELETE_DATA=0 ;; esac
fi

# ---- 3. remove files ------------------------------------------------------
if [ "$DELETE_DATA" = "1" ]; then
  if [ -d "$INSTALL_DIR" ]; then
    log "Deleting install directory (code + SQLite data + .env): $INSTALL_DIR"
    # data dir is owned by in-container uid 10001 — may need sudo to remove
    rm -rf "$INSTALL_DIR" 2>/dev/null || $SUDO rm -rf "$INSTALL_DIR" || \
      die "Could not remove $INSTALL_DIR — remove it manually (try sudo)."
  fi
  log "Ai Arbuz Provider Api fully removed (including data)."
else
  log "Containers and image removed. Data kept in: $INSTALL_DIR/data and $INSTALL_DIR/.env"
  log "To finish wiping later: rm -rf '$INSTALL_DIR'  (or re-run with --purge)."
fi
