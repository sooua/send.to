# extras

Deployment helpers for running send.to outside Docker.

## systemd

```bash
# 1. Binary
sudo install -m 0755 ./send.to /usr/local/bin/sendto

# 2. Service account
sudo useradd --system --no-create-home --shell /usr/sbin/nologin sendto

# 3. Configuration
sudo install -d -m 0755 /etc/sendto
sudo install -m 0640 -o root -g sendto extras/systemd/sendto.env /etc/sendto/sendto.env
sudo editor /etc/sendto/sendto.env

# 4. Unit
sudo install -m 0644 extras/systemd/sendto.service /etc/systemd/system/sendto.service
sudo systemctl daemon-reload
sudo systemctl enable --now sendto

# 5. Check
systemctl status sendto
curl -s http://127.0.0.1:18080/health
```

`StateDirectory=sendto` creates `/var/lib/sendto` with the right ownership on
first start; the shipped `sendto.env` points `BASEDIR` and `TEMP_PATH` inside
it. Both directories are created at startup if missing.

The unit is hardened (`ProtectSystem=strict`, empty capability bounding set,
`SystemCallFilter=@system-service`) and expects to run behind a reverse proxy
on loopback. To bind a privileged port directly, uncomment the
`CAP_NET_BIND_SERVICE` lines.

### Logs

```bash
journalctl -u sendto -f
```

Output is structured JSON. Share and deletion tokens are masked, so log
shipping does not hand out working download links.

### Upgrading

```bash
sudo install -m 0755 ./send.to /usr/local/bin/sendto
sudo systemctl restart sendto
```

Restart is graceful: in-flight uploads finish within `SHUTDOWN_TIMEOUT`.

## clamd

The ClamAV prescan talks to an existing `clamd`. On Debian/Ubuntu:

```bash
sudo apt install clamav-daemon
sudo systemctl enable --now clamav-daemon
```

Then set in `/etc/sendto/sendto.env`:

```
CLAMAV_HOST=tcp://127.0.0.1:3310
PERFORM_CLAMAV_PRESCAN=true
```

`clamd` must listen on TCP — the default Debian configuration uses a Unix
socket only. Add to `/etc/clamav/clamd.conf`:

```
TCPSocket 3310
TCPAddr 127.0.0.1
```

Note that the prescan buffers each upload to `TEMP_PATH` before scanning, so
size that filesystem for your `MAX_UPLOAD_SIZE`.
