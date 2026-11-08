# Remote use

The Explorer reaches an IED behind a gateway or an engineering host with the daemon and a browser. Nothing is installed on your workstation, and the desktop application is not involved (it has no remote mode).

```
workstation                        gateway                          IED
browser ── ssh -L (encrypted) ──►  iec61850-explorerd ── MMS ──►  relay, bay controller
           127.0.0.1:8765          127.0.0.1:8765         TCP 102, Observe mode
```

## Three steps

1. **On the gateway, start the daemon.**

   ```sh
   iec61850-explorerd
   ```

   It listens on `127.0.0.1:8765` only. Started from a terminal it prints its URL; started as a service or in a container it does not, because the URL carries the access token. To get the URL at any time:

   ```sh
   iec61850-explorerd url
   ```

2. **On your workstation, forward the port.**

   ```sh
   ssh -N -L 8765:127.0.0.1:8765 user@gateway
   ```

   Keep it running. If port 8765 is taken on your workstation, use another local port (`-L 9000:127.0.0.1:8765`) and change the port in the URL to match.

3. **Open the URL in a browser.** Paste the whole URL, including the part after `#`. The page removes the token from the address bar and keeps it in memory only, so after a reload or in a new tab you paste the URL again.

In the page, connect to the IED with an address **as the gateway sees it** (for example `192.168.1.10`, port 102).

## What to expect

- **The tunnel drops.** The daemon keeps its sessions. The page says "connection lost", retries, and when the tunnel is back it continues where it stopped: missed events are replayed.
- **You close the tab.** The session stays open on the daemon, holding its MMS association. Open the URL again and the page picks it up. Disconnect in the page to close it: an IED may accept only a few associations.
- **The token.** It is the only credential: treat the URL like a password. It is created on the first start, kept in a file only the daemon's user can read, and stays the same across restarts. Replace it with `iec61850-explorerd token rotate`: the old URL stops working within a second, without a restart.
- **Which IEDs.** By default the daemon connects only to private and link-local addresses (10/8, 172.16/12, 192.168/16, 169.254/16 and their IPv6 counterparts). Add others with `--allow-target <cidr>`, for example `--allow-target 127.0.0.1` for an IED simulated on the gateway itself.
- **Observe mode.** The daemon sends discovery and read services only; see [`SECURITY.md`](../SECURITY.md).

## Where the daemon keeps its files

By default in a temporary folder, `/tmp/iec61850-explorer-<uid>`: the token, the state file and the connection profiles. That is lost when the system clears `/tmp`, after which the daemon makes a new token. For a gateway that should keep its token, pass `--data-dir` (the systemd unit below does). The directory must belong to the daemon's user and be closed to everyone else (mode 0700); the daemon creates it that way and refuses anything else.

## As a service (systemd)

[`packaging/iec61850-explorerd.service`](../packaging/iec61850-explorerd.service) runs the daemon as its own user with `/var/lib/iec61850-explorer` as data directory:

```sh
sudo install -m 0755 iec61850-explorerd /usr/local/bin/
sudo useradd --system --no-create-home --shell /usr/sbin/nologin iec61850-explorer
sudo install -m 0644 iec61850-explorerd.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now iec61850-explorerd
sudo -u iec61850-explorer iec61850-explorerd url --data-dir /var/lib/iec61850-explorer
```

The URL is not in the journal: as a service the daemon writes one line that names the `url` command instead.

## In a container

The daemon is also a container image (`packaging/Dockerfile`; amd64, arm64, armv7):

```sh
make docker
docker run -d --name explorer -p 127.0.0.1:8765:8765 -v explorer-data:/data iec61850-explorerd:dev
docker exec explorer iec61850-explorerd url
```

Publish the port on `127.0.0.1` only, as above: inside a container the daemon listens on every address for the published port to reach it, and the listener is plain HTTP. The image is distroless, runs as a non-root user, and keeps the token in `/data`.

## Binding to another address

`--listen` accepts only loopback addresses unless you also pass `--allow-remote-bind`. The daemon then warns at start: its listener is plain HTTP, so the token crosses the network unencrypted. The token and the Host check still apply; name the hosts the daemon should answer to with `--allow-host`. Prefer the SSH forward.
