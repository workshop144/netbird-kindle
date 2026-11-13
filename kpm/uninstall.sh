#!/bin/sh
# KPM runs this before it deletes the package folder.
HERE=$(cd "$(dirname "$0")" && pwd)

# An update keeps everything: the new version's installer replaces the programs.
[ "${1:-}" = upgrade ] && exit 0
# Called to clean up after an install that failed: there is nothing of ours to undo.
[ -e "$HERE/install-failed" ] && exit 0

if [ -f /var/local/netbird-kindle/uninstall.sh ]; then
    sh /var/local/netbird-kindle/uninstall.sh
fi
exit 0
