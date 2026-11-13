#!/bin/sh
# KPM runs this in the unpacked package folder after `kpm install netbird-kindle`.
# The work is done by the same installer the zip release uses.

if sh ./payload/install.sh; then
    rm -f ./install-failed
    exit 0
fi
# KPM runs uninstall.sh after a failed install. This note tells it to leave things alone, so a
# failed update (no Wi-Fi, say) does not remove a setup that was working.
: > ./install-failed
exit 1
