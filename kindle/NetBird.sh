#!/bin/sh
# Name: NetBird link
# Author: netbird-kindle
# Tap to bring the NetBird link up, tap again to take it down. Shows the result on screen.
NBK=/var/local/netbird-kindle/nbk
FBINK=/mnt/us/libkh/bin/fbink
say() { [ -x "$FBINK" ] && "$FBINK" -q -pmh -y -4 "$1" >/dev/null 2>&1; }
if [ ! -x "$NBK" ]; then say "NetBird is not installed"; exit 1; fi
say "NetBird: working..."
out=$("$NBK" toggle 2>&1 | tail -n 1)
say "$out"
