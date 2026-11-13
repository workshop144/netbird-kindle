# netbird-kindle

Run [NetBird](https://netbird.io) on a jailbroken Kindle that has no tunnel driver, so the Kindle can reach things on your mesh from anywhere: a self-hosted ebook library, an OPDS catalog, a sync server.

I run a small ebook library at home and wanted my Kindle to fetch from it when I travel. NetBird is how the rest of my devices get home, but the Kindle's kernel was built without TUN, so the normal client cannot create a network interface there. This project is the workaround: the stock NetBird client in userspace mode, plus a small forwarder and a couple of firewall rules, so that programs on the Kindle can use an ordinary `https://` address and never know the difference.

## Is this the right tool for your Kindle?

Check for a tunnel driver first. On the Kindle, as root:

```
ls /dev/net/tun
```

If that file exists, you probably want [netbird.koplugin](https://github.com/denMaier/netbird.koplugin) instead. It runs NetBird with a real tunnel from inside KOReader, signs in with a QR code, and names resolve the way they should.

If it does not exist, or you need the link to work outside KOReader (for a script, `curl`, or the stock software), this is for you.

## How it works

NetBird can run without a network interface. In that mode it offers a SOCKS5 proxy on localhost and nothing else, and there is no mesh DNS. KOReader cannot use a SOCKS5 proxy, and neither can most things on a Kindle. So:

```
any program ── https://library.example ──┐
                                         │  hosts overlay: the name resolves to the mesh address
                                         v
          firewall rule: that address and port are redirected to a local port
                                         v
          nblink: accepts there and opens the real connection through SOCKS5
                                         v
          netbird (upstream client, userspace mode) ── WireGuard ──> your mesh
```

You list the addresses the Kindle should reach. Only those go through the mesh. Everything else on the Kindle behaves as before.

The pieces:

- `nbk`, a shell script that starts and stops the lot, and checks that the link really answers before it says "up".
- `nblink`, a small Go program with no dependencies that carries connections from a local port through the SOCKS5 proxy.
- A home-screen item that toggles the link, and an optional hook that brings it up whenever KOReader starts.
- A [KPM](https://kindlemodding.org/jailbreaking/whats-next/installing-homebrew.html#kpm) package, so the whole thing installs from the Kindle's search bar.

## Tested on

One device: a Kindle Oasis 3 (10th generation) on firmware 5.18.2, with NetBird client 0.82.1 against a self-hosted NetBird server. From a network away from home it lists an OPDS catalog and downloads books through the mesh.

The 64-bit ARM build is there but has not been run on a device. If you try this on another model, an issue saying what happened is welcome, including "it just worked".

## What you need

- A jailbroken Kindle with KPM, the package manager current jailbreaks install. Without KPM you need a root shell instead; see [Installing without KPM](#installing-without-kpm).
- Wi-Fi on while installing, and about 120 MB free in `/var/local`.
- A NetBird account or your own server, and a setup key for the Kindle. Make it a one-off key with a short expiry.
- On the NetBird side, a route to whatever the Kindle should reach and an access policy that allows it.

## Install

1. On the Kindle, with Wi-Fi on, type these into the search bar, one at a time:

   ```
   ;kpm add-repo https://github.com/workshop144/netbird-kindle/releases/latest/download/repo.json
   ;kpm install netbird-kindle
   ```

   The second one downloads the NetBird client (about 15 MB) straight from NetBird's GitHub releases and checks it against a pinned SHA-256. If the checksum does not match, nothing is installed. It adds a "NetBird link" item to the home screen and, if KOReader is installed, a hook that brings the link up when KOReader starts.

2. Plug the Kindle into a computer. There is now a `netbird-kindle` folder at the top level of its storage. In it:

   - copy `netbird-kindle.conf.example` to `netbird-kindle.conf` and edit it (see [Settings](#settings));
   - save your setup key in a file called `setup-key`.

3. Eject, and tap "NetBird link" on the home screen.

The tap checks your settings, moves both files off the storage a computer can see, and starts the link. If there is a mistake in the settings it says what is wrong, leaves the file where it is, and carries on with the settings it had. To change settings later, do step 2 again with just the settings file.

To update, run `;kpm update` and then `;kpm install netbird-kindle` again. Your settings and the Kindle's NetBird identity are kept, and a link that was up comes back up.

### Installing without KPM

This needs a root shell on the Kindle. KOReader's SSH server (Network menu) is the easy way.

1. Download `netbird-kindle.zip` from the [releases page](https://github.com/workshop144/netbird-kindle/releases) and unzip it. `SHA256SUMS` beside it lets you check the download.
2. Plug the Kindle in and copy the `netbird-kindle` folder to the top level of its storage.
3. In that folder, copy `netbird-kindle.conf.example` to `netbird-kindle.conf` and edit it, and save your setup key in a file called `setup-key`.
4. Eject, then on the Kindle, as root:

   ```
   sh /mnt/us/netbird-kindle/install.sh
   /var/local/netbird-kindle/nbk start
   ```

   Add `--no-koreader` to the first line to skip the KOReader hook.

Once the link is up you can delete everything in the `netbird-kindle` folder on the Kindle's storage.

## Settings

`netbird-kindle.conf` is short. The two lines that matter:

```
FORWARDS="10.20.30.40:443"
HOSTS="10.20.30.40 library.example"
```

`FORWARDS` is what the Kindle may reach through the mesh, as IPv4 address and port. `HOSTS` gives those addresses names while the link is up, because userspace mode has no mesh DNS. With the two lines above, `https://library.example` works in KOReader exactly as it does at home, certificate and all.

Set `MANAGEMENT_URL` if you run your own NetBird server.

Every line is `NAME="value"`. A settings file handed over through the Kindle's storage is read as text and never run: a line that is not one of the six settings in the example, or a value with anything but letters, digits, spaces and `. : ; / _ ~ -` in it, gets the whole file refused.

## Using it

`nbk` is at `/var/local/netbird-kindle/nbk`. It is not on the PATH, so call it by its full name.

```
nbk start     bring the link up
nbk stop      take it down and undo every change
nbk restart   stop, then start
nbk status    what is running, and whether the link answers
nbk toggle    the home-screen item runs this
nbk check     check the settings and change nothing
nbk import    take in settings and a setup key waiting in the netbird-kindle folder
```

`nbk start`, `restart` and `toggle` first take in anything waiting in the `netbird-kindle` folder, so `nbk import` by itself is rarely needed.

`nbk start` waits until the first address in `FORWARDS` answers, through the redirect and through the mesh. If it does not within a minute, it says so and keeps the setup key, so a missing policy or a dead Wi-Fi connection does not look like success.

The home-screen item runs `nbk toggle` and prints the result on the screen. It relies on the scriptlet support that current jailbreaks set up.

The link keeps running across sleep. After a short sleep it works again within seconds. After a long one NetBird can take a minute or two to reconnect, and `nbk restart` gets you there in about ten seconds. It does not survive a reboot: tap the home-screen item, or open KOReader if you installed the hook.

## What it changes on your Kindle

Nothing on the read-only system partition. While the link is up there are:

- two firewall chains of its own, `NBK` and `NBK-IN`, holding the redirects and the loopback rules they need;
- a bind mount over `/etc/hosts` that adds your `HOSTS` lines;
- two processes, `netbird` and `nblink`, and a small helper that writes NetBird's log;
- two log files in `/var/run/netbird-kindle`, held in memory and each capped at 256 KB.

`nbk stop` removes all of it except the two log files, which stay until the next start or a reboot so you can read what happened.

The programs, your settings and the Kindle's NetBird identity live in `/var/local/netbird-kindle`, which is not the partition a computer sees over USB. Two small files do sit on that partition: the home-screen item (`documents/NetBird.sh`) and the KOReader hook (`koreader/patches/2-netbird-kindle.lua`). Each only calls `nbk` with a fixed argument. The `netbird-kindle` folder with the example settings is there too, and so is KPM's copy of the package under `kmc/kpm/packages` if you installed that way.

## Security

Read this part before you put a mesh credential on a device that lives in a coat pocket.

- **Everything runs as root**, because on a jailbroken Kindle everything does.
- **The Kindle's NetBird identity is stored unencrypted** in `/var/local/netbird-kindle/state`. A Kindle has no disk encryption. If you lose it, delete its peer in the NetBird dashboard.
- **Give the Kindle as little as it needs.** Put it in its own group and write a policy that allows one address and one port. Do not add it to the group your laptop is in.
- **A jailbroken Kindle runs scripts from its USB storage as root.** That is true with or without this project, but once the Kindle is on your mesh it matters more: a computer you plug it into could plant a script that uses the link. Narrow access is the answer to that too.
- **When the link is down, the redirect is gone.** A private address then means whatever machine has that address on the Wi-Fi you happen to be on. Use HTTPS and never click through a certificate warning.
- The proxy and the forwarder listen on loopback only, a firewall rule drops anything from outside aimed at their ports, and the client is started with inbound connections blocked. Nothing on your mesh or the local network can connect to the Kindle through this.
- The setup key is read from a file, never passed on a command line, and deleted once the link first answers. Until your first tap it sits in the `netbird-kindle` folder where a computer can read it, which is one more reason for a one-off key with a short expiry.
- **Settings can be changed by whoever can write to the Kindle's storage.** That is what makes setup possible without a shell. A settings file from there is never run as code, but it can point the link at a different address or NetBird server. Your NetBird policy still decides what the Kindle may reach, so keep it narrow.

## Limits

- IPv4 and TCP only.
- No mesh DNS. Names are mapped by hand in the settings.
- The link carries traffic only while the Kindle is awake, since a sleeping Kindle turns its Wi-Fi off.
- NetBird takes tens of megabytes of memory on a device that has 512.
- The settings are a text file you edit on a computer. There is no settings screen on the Kindle.

## Uninstall

In the Kindle's search bar:

```
;kpm uninstall netbird-kindle
```

If you installed without KPM, run `sh /var/local/netbird-kindle/uninstall.sh` as root instead. If you no longer have the script, `nbk stop` followed by removing `/var/local/netbird-kindle`, `/mnt/us/documents/NetBird.sh` and `/mnt/us/koreader/patches/2-netbird-kindle.lua` does the same.

Either way, delete the Kindle's peer in your NetBird dashboard afterwards.

## Building from source

You need Go, make, shellcheck, zip and shasum.

```
make test     vet and test nblink, lint and test the scripts
make dist     build the zip, the KPM package (.kpkg) and its index (repo.json) in dist/
```

To try a package on a Kindle before releasing it, `sh tools/kpm-pack.sh v9.9.9 file:///mnt/us/somewhere` writes an index that points at a folder on the Kindle. Copy the `.kpkg` and `repo.json` there and `kpm add-repo file:///mnt/us/somewhere/repo.json`.

To move to another NetBird version, change the version and both checksums in `kindle/versions`. The checksums come from `netbird_<version>_checksums.txt` on NetBird's release page.

## Related projects and thanks

- [netbird.koplugin](https://github.com/denMaier/netbird.koplugin) by denMaier: NetBird as a KOReader plugin. The better choice when your device has a tunnel driver.
- [koreader-tailscale](https://github.com/victoria-riley-barnett/koreader-tailscale) and [tailscale_kual](https://github.com/mitanshu7/tailscale_kual): the same idea for Tailscale, and where the userspace approach on Kindles was worked out.
- [KindleModding](https://kindlemodding.org) for the jailbreak documentation.

This project is not affiliated with or endorsed by NetBird. The NetBird client it downloads is BSD-3-Clause licensed, copyright NetBird GmbH and authors.

## Licence

MIT. See [LICENSE](LICENSE).
