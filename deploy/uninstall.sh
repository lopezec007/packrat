#!/usr/bin/env bash
# Remove the Packrat service from this machine.
#
# Usage (as root):
#   deploy/uninstall.sh            # stop and remove the service, unit and binary;
#                                  # KEEP the database, /etc/packrat and the system user
#   deploy/uninstall.sh --purge    # also DROP the database and its role, and remove
#                                  # /etc/packrat and the system user (asks first)
#
# PostgreSQL itself is left installed either way.
set -euo pipefail

SVC_USER=packrat
DB_NAME=packrat
PURGE=0
[[ "${1:-}" == "--purge" ]] && PURGE=1

die() { echo "uninstall.sh: $*" >&2; exit 1; }
log() { echo "==> $*"; }

[[ $EUID -eq 0 ]] || die "must be run as root (try: sudo $0 $*)"

if systemctl list-unit-files packrat.service >/dev/null 2>&1; then
    log "stopping and disabling packrat.service"
    systemctl disable --now packrat.service >/dev/null 2>&1 || true
fi
rm -f /etc/systemd/system/packrat.service
systemctl daemon-reload
log "removing /usr/local/bin/packrat"
rm -f /usr/local/bin/packrat

if [[ $PURGE -eq 0 ]]; then
    echo
    echo "Service removed. Kept: the PostgreSQL database \"$DB_NAME\", /etc/packrat and the system user."
    echo "Re-run with --purge to delete those too."
    exit 0
fi

echo
echo "--purge will permanently DELETE the PostgreSQL database \"$DB_NAME\": every"
echo "list, the whole item library and the statuses. There is no undo."
read -r -p "Type the database name to confirm: " answer
[[ "$answer" == "$DB_NAME" ]] || die "not confirmed; nothing was deleted"

if command -v psql >/dev/null && runuser -u postgres -- psql -tAc 'SELECT 1' >/dev/null 2>&1; then
    log "dropping database $DB_NAME and role $SVC_USER"
    runuser -u postgres -- psql -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $DB_NAME"
    runuser -u postgres -- psql -v ON_ERROR_STOP=1 -c "DROP ROLE IF EXISTS $SVC_USER"
else
    echo "uninstall.sh: warning: PostgreSQL is not reachable; database not dropped" >&2
fi
log "removing /etc/packrat"
rm -rf /etc/packrat
if getent passwd "$SVC_USER" >/dev/null; then
    log "removing system user $SVC_USER"
    userdel "$SVC_USER" || true
fi
echo "Purged."
