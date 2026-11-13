#!/bin/sh
# Remove netbird-kindle from the Kindle. Run this ON the Kindle, as root.
# It also removes this device's NetBird identity, so afterwards delete the peer in your
# NetBird dashboard.

DEST=/var/local/netbird-kindle
[ "$(id -u)" = 0 ] || { echo "uninstall: run this as root on the Kindle" >&2; exit 1; }

# `nbk stop` needs no settings file, so this works on a half-removed install too.
if [ -x "$DEST/nbk" ]; then
    "$DEST/nbk" stop || echo "uninstall: the link did not stop cleanly; a reboot clears what is left"
fi
rm -f /mnt/us/documents/NetBird.sh
rm -rf /mnt/us/documents/NetBird.sh.sdr
rm -f /mnt/us/koreader/patches/2-netbird-kindle.lua
# the handover folder goes too, unless something of yours is still in it
rm -f /mnt/us/netbird-kindle/netbird-kindle.conf.example
rmdir /mnt/us/netbird-kindle 2> /dev/null
rm -rf "$DEST" /var/run/netbird-kindle
echo "uninstall: removed. Delete this Kindle's peer in your NetBird dashboard."
