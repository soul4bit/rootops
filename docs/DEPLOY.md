# RootOPS Deploy

RootOPS is deployed as a Go binary with file-backed content.

The GitHub Actions workflow:

1. runs tests;
2. validates `content/`;
3. builds a Linux binary;
4. uploads a release to the server;
5. switches `current` to the new release;
6. restarts a systemd service.

## Required GitHub Secrets

```text
DEPLOY_HOST=<server-host>
DEPLOY_USER=<ssh-user>
DEPLOY_PORT=22
DEPLOY_PATH=/opt/rootops
DEPLOY_SSH_KEY=<private-deploy-key>
ROOTOPS_SERVICE=rootops
```

`ROOTOPS_SERVICE` is optional in the workflow and defaults to `rootops`.

Optional repository variable:

```text
DEPLOY_GOARCH=amd64
```

## Server Layout

```text
/opt/rootops/
  current -> /opt/rootops/releases/<commit-sha>
  releases/
    <commit-sha>/
      rootops
      assets/
      content/
      docs/
      README.md
      REVISION
```

## systemd Service

Example `/etc/systemd/system/rootops.service`:

```ini
[Unit]
Description=RootOPS
After=network.target

[Service]
Type=simple
User=www-data
WorkingDirectory=/opt/rootops/current
ExecStart=/opt/rootops/current/rootops serve -addr 127.0.0.1:8080 -content /opt/rootops/current/content
Restart=always
RestartSec=3
Environment=ROOTOPS_HOST=127.0.0.1
Environment=ROOTOPS_PORT=8080

[Install]
WantedBy=multi-user.target
```

Enable it once on the server:

```bash
sudo systemctl daemon-reload
sudo systemctl enable rootops
sudo systemctl start rootops
```

The deploy user must be able to restart the service without an interactive
password prompt. Create a narrow sudoers file on the server:

```bash
sudo visudo -f /etc/sudoers.d/rootops-deploy
```

Add this line, replacing `deploy` with the real `DEPLOY_USER` if needed:

```sudoers
deploy ALL=(root) NOPASSWD: /usr/bin/systemctl restart rootops, /usr/bin/systemctl is-active rootops, /bin/systemctl restart rootops, /bin/systemctl is-active rootops
```

Then check it from the deploy user:

```bash
sudo -n systemctl restart rootops
sudo -n systemctl is-active rootops
```

The `-n` flag is important: it makes sudo fail instead of asking GitHub Actions
for a password it cannot type.

## Caddy Reverse Proxy

```caddyfile
rootops.su {
  encode gzip zstd
  reverse_proxy 127.0.0.1:8080
}

www.rootops.su {
  redir https://rootops.su{uri} permanent
}
```
