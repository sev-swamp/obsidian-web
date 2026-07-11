# Deployment

The server is a single static binary with the frontend embedded; the
only external file is `config.yaml` (see
[config.example.yaml](../config.example.yaml)).

## Local

```bash
make build
./bin/obsidianweb -config config.yaml            # or -vault /path/to/vault
```

## Docker

```bash
docker build -t obsidianweb .
docker run -p 8787:8787 \
  -v /path/to/vault:/vault \
  -v $(pwd)/config:/config:ro \
  obsidianweb
```

Or `docker compose up --build` using [docker-compose.yml](../docker-compose.yml).

## Linux VPS (systemd)

```ini
# /etc/systemd/system/obsidianweb.service
[Unit]
Description=Obsidian Web
After=network.target

[Service]
User=obsidianweb
ExecStart=/usr/local/bin/obsidianweb -config /etc/obsidianweb/config.yaml
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

## Reverse proxy

WebSocket upgrade must be forwarded.

### Caddy

```
notes.example.com {
    reverse_proxy localhost:8787
}
```

### Nginx

```nginx
server {
    server_name notes.example.com;
    location / {
        proxy_pass http://127.0.0.1:8787;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
    }
}
```

## Security checklist for public instances

1. `auth.enabled: true`, strong `jwtSecret` (e.g. `openssl rand -hex 32`).
2. Use `passwordHash` (bcrypt), not plaintext `password`:
   `htpasswd -bnBC 10 "" 'secret' | tr -d ':\n'`.
3. Terminate TLS at the reverse proxy.
4. Mount the vault read-write only if editing from the web is desired.
