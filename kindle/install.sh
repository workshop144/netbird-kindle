#!/bin/sh
# Install netbird-kindle. Run this ON the Kindle, as root, from the unpacked release folder:
#
#   sh install.sh                 install or update
#   sh install.sh --no-koreader   skip the KOReader start hook
#
# It downloads the NetBird client named in ./versions from NetBird's GitHub releases, checks
# its SHA-256 against the value pinned there, and puts everything in /var/local/netbird-kindle.
# Your settings and the device's NetBird identity are kept when updating.

set -u
SRC=$(cd "$(dirname "$0")" && pwd)
DEST=/var/local/netbird-kindle
KOREADER=/mnt/us/koreader
DOCS=/mnt/us/documents
hook=yes
[ "${1:-}" = "--no-koreader" ] && hook=no

die() { echo "install: $*" >&2; exit 1; }

[ "$(id -u)" = 0 ] || die "run this as root on the Kindle"
[ -r "$SRC/versions" ] || die "no versions file beside this script"
# shellcheck disable=SC1091
. "$SRC/versions"
: "${NETBIRD_VERSION:?versions does not set NETBIRD_VERSION}"
: "${NETBIRD_SHA256_armv6:?versions does not set NETBIRD_SHA256_armv6}"
: "${NETBIRD_SHA256_arm64:?versions does not set NETBIRD_SHA256_arm64}"

case "$(uname -m)" in
    armv6* | armv7* | armv8l) arch=armv6; want=$NETBIRD_SHA256_armv6; link=nblink-arm ;;
    aarch64 | arm64) arch=arm64; want=$NETBIRD_SHA256_arm64; link=nblink-arm64 ;;
    *) die "unsupported processor: $(uname -m)" ;;
esac
[ -f "$SRC/$link" ] || die "no $link in this folder (use a release archive, or run 'make dist')"

if [ -c /dev/net/tun ]; then
    echo "install: this Kindle has a tunnel driver (/dev/net/tun)."
    echo "         netbird-kindle still works, but a plugin that uses the tunnel, such as"
    echo "         netbird.koplugin, gives you more: names resolve and nothing needs redirecting."
fi

sum() {
    if command -v sha256sum > /dev/null 2>&1; then
        sha256sum "$1" | cut -d' ' -f1
    elif command -v openssl > /dev/null 2>&1; then
        openssl dgst -sha256 "$1" | sed 's/.* //'
    fi
}

mkdir -p "$DEST" || die "cannot create $DEST"
chmod 700 "$DEST"

# stop a running link before replacing what it runs
if [ -x "$DEST/nbk" ]; then
    "$DEST/nbk" stop > /dev/null 2>&1 || die "could not stop the running link; stop it and run this again"
fi

# The download, the unpacked client and the client being replaced sit side by side for a moment.
free_kb=$(df -k "$DEST" 2> /dev/null | awk 'NR==2 {print $4}')
if [ -n "$free_kb" ] && [ "$free_kb" -lt 122880 ]; then
    die "less than 120 MB free in /var/local ($((free_kb / 1024)) MB): not enough room to install safely"
fi

have=$("$DEST/netbird" version 2> /dev/null)
if [ "$have" = "$NETBIRD_VERSION" ]; then
    echo "install: NetBird $NETBIRD_VERSION is already here"
else
    url="https://github.com/netbirdio/netbird/releases/download/v${NETBIRD_VERSION}/netbird_${NETBIRD_VERSION}_linux_${arch}.tar.gz"
    tmp="$DEST/download.tmp"
    rm -rf "$tmp" && mkdir "$tmp" || die "cannot create $tmp"
    echo "install: downloading NetBird $NETBIRD_VERSION ($arch), about 15 MB"
    curl -fsSL -m 600 -o "$tmp/netbird.tar.gz" "$url" || { rm -rf "$tmp"; die "download failed (is Wi-Fi on?)"; }
    got=$(sum "$tmp/netbird.tar.gz")
    if [ -z "$got" ] || [ "$got" != "$want" ]; then
        rm -rf "$tmp"
        die "checksum mismatch for the NetBird archive: nothing was installed"
    fi
    tar -xzf "$tmp/netbird.tar.gz" -C "$tmp" netbird || { rm -rf "$tmp"; die "could not unpack the archive"; }
    mv "$tmp/netbird" "$DEST/netbird" && chmod 755 "$DEST/netbird" || { rm -rf "$tmp"; die "could not install the NetBird client"; }
    rm -rf "$tmp"
    echo "install: NetBird $("$DEST/netbird" version 2> /dev/null) installed, checksum verified"
fi

cp "$SRC/$link" "$DEST/nblink" && cp "$SRC/nbk" "$DEST/nbk" && cp "$SRC/uninstall.sh" "$DEST/uninstall.sh" &&
    chmod 755 "$DEST/nblink" "$DEST/nbk" "$DEST/uninstall.sh" ||
    die "could not copy nbk, nblink and uninstall.sh"
[ -r "$SRC/VERSION" ] && cp "$SRC/VERSION" "$DEST/VERSION"
cp "$SRC/netbird-kindle.conf.example" "$DEST/netbird-kindle.conf.example"
if [ ! -e "$DEST/netbird-kindle.conf" ]; then
    cp "$SRC/netbird-kindle.conf.example" "$DEST/netbird-kindle.conf"
    echo "install: wrote $DEST/netbird-kindle.conf from the example: edit it before starting"
fi
chmod 600 "$DEST/netbird-kindle.conf"

# A setup key left beside this script (on USB-visible storage) is moved to where only root on
# the device can read it.
if [ -s "$SRC/setup-key" ]; then
    ( umask 077; tr -d ' \r\n' < "$SRC/setup-key" > "$DEST/setup-key" )
    rm -f "$SRC/setup-key"
    echo "install: moved the setup key into $DEST"
fi

if [ -d "$DOCS" ]; then
    cp "$SRC/NetBird.sh" "$DOCS/NetBird.sh" && echo "install: added the home-screen item 'NetBird link'"
fi
if [ "$hook" = yes ] && [ -d "$KOREADER" ]; then
    mkdir -p "$KOREADER/patches" &&
        cp "$SRC/koreader-patch/2-netbird-kindle.lua" "$KOREADER/patches/" &&
        echo "install: KOReader will bring the link up when it starts (restart KOReader once)"
fi

echo "install: done. Next: edit $DEST/netbird-kindle.conf, then run $DEST/nbk start"
