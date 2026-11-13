#!/bin/sh
# Name: NetBird link
# Author: netbird-kindle
# DontUseFBInk
# Tap to bring the NetBird link up, tap again to take it down. Says what happened on screen.
NBK=/var/local/netbird-kindle/nbk
FBINK=/mnt/us/libkh/bin/fbink
[ -x "$FBINK" ] || FBINK=/var/local/kmc/bin/fbink
say() { [ -x "$FBINK" ] && "$FBINK" -q -pmh -y -4 "$1" >/dev/null 2>&1; }
if [ ! -x "$NBK" ]; then say "NetBird is not installed"; exit 1; fi
say "NetBird link: one moment..."
out=$("$NBK" toggle 2>&1 | tail -n 1)
case "$out" in
    "nbk: up") say "NetBird link is up" ;;
    "nbk: down") say "NetBird link is down" ;;
    *) say "NetBird link: ${out#nbk: }" ;;
esac
