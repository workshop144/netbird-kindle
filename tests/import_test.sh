#!/bin/sh
# Tests for `nbk import`: taking settings and a setup key from the folder a computer can see.
# Runs anywhere with a POSIX shell: sh tests/import_test.sh
# shellcheck disable=SC2034 # out and rc are read inside the strings handed to check
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
fails=0

# A fresh copy of nbk with its fixed paths pointed into a scratch folder.
fresh() {
    rm -rf "$WORK/nbk" "$WORK/drop" "$WORK/run"
    mkdir -p "$WORK/nbk" "$WORK/drop"
    sed -e "s|^RUN=.*|RUN=$WORK/run|" -e "s|^DROP=.*|DROP=$WORK/drop|" "$HERE/../kindle/nbk" > "$WORK/nbk/nbk"
    chmod 755 "$WORK/nbk/nbk"
    NBK="$WORK/nbk/nbk"; CONF="$WORK/nbk/netbird-kindle.conf"; DROPPED="$WORK/drop/netbird-kindle.conf"
}
ok() { echo "ok   $1"; }
bad() { echo "FAIL $1"; fails=$((fails + 1)); }
check() { if eval "$2"; then ok "$1"; else bad "$1"; fi; }
good_conf() { printf 'PEER_NAME="reader"\nFORWARDS="10.20.30.40:443"\nHOSTS="10.20.30.40 library.example"\n'; }

fresh
out=$("$NBK" import 2>&1); rc=$?
check "nothing dropped: says so and succeeds" '[ $rc -eq 0 ] && echo "$out" | grep -q "nothing to take in"'
check "nothing dropped: no settings appear" '[ ! -e "$CONF" ]'

fresh
printf '\357\273\277# my settings\r\n\r\nPEER_NAME="reader"\r\nFORWARDS="10.20.30.40:443 10.20.30.41:8080"\r\nHOSTS="10.20.30.40 library.example; 10.20.30.41 other.example"\r\nSOCKS_PORT=1081\r\nMANAGEMENT_URL="https://nb.example.org"\r\n' > "$DROPPED"
out=$("$NBK" import 2>&1); rc=$?
check "good file from a Windows editor: taken" '[ $rc -eq 0 ] && echo "$out" | grep -q "took the new settings"'
check "good file: removed from the visible folder" '[ ! -e "$DROPPED" ]'
check "good file: settings are private to root" '[ "$(ls -l "$CONF" | cut -c1-10)" = "-rw-------" ]'
check "good file: values arrive intact" 'grep -q "^FORWARDS=\"10.20.30.40:443 10.20.30.41:8080\"$" "$CONF" && grep -q "^SOCKS_PORT=1081$" "$CONF"'
check "good file: comments and stray bytes are not kept" '! grep -q "my settings" "$CONF" && ! grep -q "$(printf "\r")" "$CONF"'
check "good file: nbk check accepts the result" '"$NBK" check > /dev/null 2>&1'

fresh
good_conf > "$WORK/drop/netbird-kindle.conf.txt"
"$NBK" import > /dev/null 2>&1
check "a .txt ending added by an editor is accepted" '[ -s "$CONF" ] && [ ! -e "$WORK/drop/netbird-kindle.conf.txt" ]'

for attack in 'FORWARDS="10.20.30.40:443"; touch '"$WORK"'/pwned' 'PEER_NAME="$(touch '"$WORK"'/pwned)"' 'PEER_NAME="`touch '"$WORK"'/pwned`"' 'touch '"$WORK"'/pwned' 'HOSTS="10.20.30.40 a" FORWARDS="1.2.3.4:1"' 'PEER_NAME=a\ b' 'PEER_NAME=~root' 'PATH="/mnt/us"'; do
    fresh
    good_conf > "$CONF"; cp "$CONF" "$WORK/before"
    { good_conf; printf '%s\n' "$attack"; } > "$DROPPED"
    out=$("$NBK" import 2>&1); rc=$?
    check "refused: $attack" '[ $rc -ne 0 ] && echo "$out" | grep -q "line 4" && [ ! -e "$WORK/pwned" ] && cmp -s "$CONF" "$WORK/before" && [ -e "$DROPPED" ]'
    rm -f "$WORK/pwned"
done

fresh
good_conf > "$CONF"; cp "$CONF" "$WORK/before"
printf 'FORWARDS="127.0.0.1:443"\n' > "$DROPPED"
out=$("$NBK" import 2>&1); rc=$?
check "well-formed but wrong settings: refused, old ones kept" '[ $rc -ne 0 ] && echo "$out" | grep -q "FORWARDS" && cmp -s "$CONF" "$WORK/before" && [ -e "$DROPPED" ]'
check "a refusal leaves no half-written file behind" '[ -z "$(ls "$WORK/nbk" | grep -v -e "^nbk$" -e "^netbird-kindle.conf$")" ]'

fresh
good_conf > "$CONF"
printf '  ABCD-1234-key \r\n' > "$WORK/drop/setup-key.txt"
out=$("$NBK" import 2>&1); rc=$?
check "setup key: taken and trimmed" '[ $rc -eq 0 ] && [ "$(cat "$WORK/nbk/setup-key")" = "ABCD-1234-key" ] && echo "$out" | grep -q "took the setup key"'
check "setup key: removed from the visible folder" '[ ! -e "$WORK/drop/setup-key.txt" ]'
check "setup key: private to root" '[ "$(ls -l "$WORK/nbk/setup-key" | cut -c1-10)" = "-rw-------" ]'

fresh
if out=$("$NBK" check 2>&1); then rc=0; else rc=$?; fi
check "no settings at all: check says where to put them" '[ $rc -ne 0 ] && echo "$out" | grep -q "netbird-kindle folder"'

[ "$fails" -eq 0 ] && echo "all passed" || { echo "$fails failed"; exit 1; }
