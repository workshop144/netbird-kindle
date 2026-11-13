# Security

netbird-kindle runs a VPN client as root on a jailbroken device and changes its firewall, so problems here matter.

## Reporting a vulnerability

Please do not open a public issue. Use GitHub's private reporting instead: on this repository, go to the Security tab and choose "Report a vulnerability". Say what you found, how to reproduce it, and which Kindle model and firmware you saw it on.

You should get a reply within a week. Fixes are released as a new version, with credit to the reporter unless you would rather not be named.

## Scope

In scope: `nbk`, `nblink`, the installer and uninstaller, the home-screen item and the KOReader hook.

Out of scope: the NetBird client itself (report that to [NetBird](https://github.com/netbirdio/netbird/security)), the jailbreak, and KOReader.

## What to expect from the design

The README's Security section describes what this project does and does not protect against. In short: everything runs as root, the device's NetBird identity is stored unencrypted, and a jailbroken Kindle will run scripts placed on its USB storage. Give the Kindle's peer the narrowest access that works.
