#!/usr/bin/env bash
# Install or upgrade Packrat as a systemd service on Debian 13.
#
# Usage (as root):
#   deploy/install.sh                  # build with the local Go toolchain
#   deploy/install.sh dist/packrat    # install a prebuilt binary (e.g. from `make dist`)
#
# Environment:
#   PACKRAT_SKIP_PACKAGES=1    do not apt-get install anything (PostgreSQL is
#                              then your business)
#
# What it does:
#   1. installs PostgreSQL from Debian's repositories, unless already present;
#   2. creates the system user "packrat", and a PostgreSQL role and database
#      of the same name (the service logs in over the local socket by peer
#      authentication, so there is no database password);
#   3. installs the binary, the systemd unit and /etc/packrat/packrat.env;
#   4. starts the service, which creates or upgrades the schema itself.
#
# Idempotent: re-running upgrades the binary and unit and restarts the
# service; /etc/packrat/packrat.env and the database are left untouched.
set -euo pipefail

SVC_USER=packrat
DB_NAME=packrat
BIN_DST=/usr/local/bin/packrat
UNIT_DST=/etc/systemd/system/packrat.service
ENV_DIR=/etc/packrat
ENV_DST=$ENV_DIR/packrat.env
PACKAGES=(postgresql)

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEPLOY_DIR="$REPO_DIR/deploy"

die() { echo "install.sh: $*" >&2; exit 1; }
log() { echo "==> $*"; }

[[ $EUID -eq 0 ]] || die "must be run as root (try: sudo $0 $*)"
command -v systemctl >/dev/null || die "systemctl not found; this script targets systemd (Debian 13)"
[[ -f /etc/debian_version ]] || echo "install.sh: warning: not a Debian system; continuing anyway" >&2
[[ -f "$DEPLOY_DIR/packrat.service" && -f "$DEPLOY_DIR/packrat.env.example" ]] \
    || die "deploy files not found in $DEPLOY_DIR"

# --- 1. Obtain the binary ---------------------------------------------------
# First, so a build failure leaves the machine untouched.
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

if [[ $# -ge 1 ]]; then
    BIN_SRC=$1
    [[ -f "$BIN_SRC" ]] || die "prebuilt binary not found: $BIN_SRC"
    [[ -x "$BIN_SRC" ]] || die "prebuilt binary is not executable: $BIN_SRC"
    log "using prebuilt binary $BIN_SRC"
else
    # sudo usually resets PATH, so also look where the Go tarball installs to.
    GO=$(command -v go || true)
    if [[ -z "$GO" && -x /usr/local/go/bin/go ]]; then GO=/usr/local/go/bin/go; fi
    [[ -n "$GO" ]] || die "go not found; either install Go or pass a prebuilt binary: $0 dist/packrat (build it with 'make dist')"
    VERSION=$(cd "$REPO_DIR" && git describe --tags --always --dirty 2>/dev/null || echo dev)
    log "building packrat $VERSION with $("$GO" version)"
    BIN_SRC="$TMP/packrat"
    (cd "$REPO_DIR" && CGO_ENABLED=0 "$GO" build -trimpath -ldflags "-s -w -X main.build=$VERSION" -o "$BIN_SRC" .)
fi

"$BIN_SRC" -version >/dev/null || die "$BIN_SRC does not run on this machine"

# --- 2. Packages -------------------------------------------------------------
if [[ -n "${PACKRAT_SKIP_PACKAGES:-}" ]]; then
    log "PACKRAT_SKIP_PACKAGES is set: not installing packages"
else
    missing=()
    for pkg in "${PACKAGES[@]}"; do
        dpkg-query -W -f='${Status}' "$pkg" 2>/dev/null | grep -q 'install ok installed' || missing+=("$pkg")
    done
    if [[ ${#missing[@]} -eq 0 ]]; then
        log "packages already installed: ${PACKAGES[*]}"
    else
        log "installing packages: ${missing[*]}"
        apt-get update
        DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends "${missing[@]}"
    fi
fi

# --- 3. System user ----------------------------------------------------------
if getent passwd "$SVC_USER" >/dev/null; then
    log "system user $SVC_USER already exists"
else
    log "creating system user $SVC_USER"
    useradd --system --user-group --home-dir /nonexistent --no-create-home \
        --shell /usr/sbin/nologin --comment "packrat service" "$SVC_USER"
fi

# --- 4. Database role and database --------------------------------------------
# Only when the service is configured for the local socket (the default); a
# database on another host is prepared there.
db_url=""
if [[ -f "$ENV_DST" ]]; then
    db_url=$(grep -E '^[[:space:]]*PACKRAT_DATABASE_URL=' "$ENV_DST" | tail -n1 | cut -d= -f2- | tr -d '"'"'" || true)
fi
if [[ -n "$db_url" && "$db_url" != *"host=/var/run/postgresql"* ]]; then
    log "PACKRAT_DATABASE_URL in $ENV_DST points elsewhere: not creating a local database"
else
    command -v psql >/dev/null || die "psql not found: install PostgreSQL (apt install postgresql) or set PACKRAT_DATABASE_URL in $ENV_DST"
    systemctl start postgresql.service
    # Wait for the cluster to accept connections (a fresh install takes a moment).
    for _ in $(seq 1 30); do
        runuser -u postgres -- psql -tAc 'SELECT 1' >/dev/null 2>&1 && break
        sleep 1
    done
    pg() { runuser -u postgres -- psql -v ON_ERROR_STOP=1 -tA "$@"; }
    pg -c 'SELECT 1' >/dev/null || die "cannot connect to PostgreSQL as the postgres user"

    if [[ $(pg -c "SELECT 1 FROM pg_roles WHERE rolname = '$SVC_USER'") == 1 ]]; then
        log "database role $SVC_USER already exists"
    else
        log "creating database role $SVC_USER"
        pg -c "CREATE ROLE $SVC_USER LOGIN"
    fi
    if [[ $(pg -c "SELECT 1 FROM pg_database WHERE datname = '$DB_NAME'") == 1 ]]; then
        log "database $DB_NAME already exists"
    else
        log "creating database $DB_NAME (owner $SVC_USER)"
        pg -c "CREATE DATABASE $DB_NAME OWNER $SVC_USER ENCODING 'UTF8' TEMPLATE template0"
    fi
fi

# --- 5. Binary ---------------------------------------------------------------
# Copy then rename so a running service is never left with a half-written
# or "text file busy" binary.
log "installing binary to $BIN_DST ($("$BIN_SRC" -version))"
install -m 0755 "$BIN_SRC" "$BIN_DST.tmp"
mv -f "$BIN_DST.tmp" "$BIN_DST"

# --- 6. Unit and environment file --------------------------------------------
log "installing unit to $UNIT_DST"
install -m 0644 "$DEPLOY_DIR/packrat.service" "$UNIT_DST"

# The env file can hold a database password (when the database is on another
# host), so only root and the service may read it.
install -d -m 0755 "$ENV_DIR"
install -m 0644 "$DEPLOY_DIR/packrat.env.example" "$ENV_DIR/packrat.env.example"
if [[ -f "$ENV_DST" ]]; then
    log "keeping existing $ENV_DST"
else
    log "creating $ENV_DST from example"
    install -m 0640 -o root -g "$SVC_USER" "$DEPLOY_DIR/packrat.env.example" "$ENV_DST"
fi

# --- 7. Service --------------------------------------------------------------
log "reloading systemd and (re)starting packrat"
systemctl daemon-reload
systemctl enable packrat.service >/dev/null
# restart also starts a stopped service, and picks up a replaced binary.
systemctl restart packrat.service

# Give it a moment to fail if it is going to (bad address, no database...).
sleep 2
if ! systemctl is-active --quiet packrat.service; then
    echo "install.sh: packrat.service is not running; recent log:" >&2
    journalctl -u packrat.service -n 20 --no-pager >&2 || true
    exit 1
fi

# --- 8. Report ---------------------------------------------------------------
addr=$(grep -E '^[[:space:]]*PACKRAT_ADDR=' "$ENV_DST" 2>/dev/null | tail -n1 | cut -d= -f2- | tr -d '"'"'" || true)
addr=${addr:-0.0.0.0:8082}
host=${addr%:*}
port=${addr##*:}
if [[ -z "$host" || "$host" == "0.0.0.0" || "$host" == "[::]" || "$host" == "::" ]]; then
    host=$(hostname -I 2>/dev/null | awk '{print $1}')
    host=${host:-$(hostname)}
fi

echo
echo "$("$BIN_DST" -version) is running."
echo "  URL:       http://$host:$port/"
echo "  Config:    $ENV_DST   (edit, then: systemctl restart packrat)"
echo "  Database:  PostgreSQL database \"$DB_NAME\"   (sudo -u postgres psql $DB_NAME)"
echo "  Backup:    sudo -u postgres pg_dump -Fc $DB_NAME > packrat-\$(date +%F).dump"
echo "  Logs:      journalctl -u packrat -f"
