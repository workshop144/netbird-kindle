-- netbird-kindle: bring the NetBird link up whenever KOReader starts.
-- Harmless if it is already up. Remove this file to stop doing it.
os.execute("/var/local/netbird-kindle/nbk start >/dev/null 2>&1 &")
