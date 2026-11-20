# GitHub Actions deploy

**Workflow:** [.github/workflows/deploy.yml](../.github/workflows/deploy.yml)  
**Stack:** [backend/deploy/docker-compose.yml](../backend/deploy/docker-compose.yml)  
**Panel proxy:** [backend/deploy/nginx.conf](../backend/deploy/nginx.conf)

A push to `main` rebuilds the whole Compose stack on one Ubuntu server over SSH. Postgres data stays in the named volume `secret-santa_postgres_data`. The bot, worker, admin panel, and migration container are recreated. The same job can be started by hand with **Run workflow**.

The action does not receive the bot token or the admin password. Those stay in `/opt/secret-santa/backend/.env` on the server.

## 1. What the job does

1. Check out the repo on the GitHub runner.
2. Write `SSH_PRIVATE_KEY` and `SSH_KNOWN_HOSTS` into the runner's SSH config.
3. `rsync` `backend/` to `/opt/secret-santa/backend/`. The server `.env` is excluded, so `--delete` does not remove it.
4. From that directory, run:

```bash
docker compose -p secret-santa --project-directory . -f deploy/docker-compose.yml up -d --build --wait --force-recreate
```

5. Fail the job unless `http://127.0.0.1:8080/login` returns successfully.

`--force-recreate` is required. A plain `up -d` leaves the old `migrate` container in the "already exited" state and skips new SQL. Do not run `docker compose down -v`. That deletes the database volume.

Keep the Compose project name `secret-santa` and the directory `/opt/secret-santa`. The volume name is derived from the project name. Changing either one starts an empty database.

`concurrency` is `deploy` with `cancel-in-progress: false`, so a second push waits instead of killing a half-recreated stack.

## 2. GitHub secrets

In the repo, add these Actions secrets. No other secrets are required.

| Secret | Value |
|---|---|
| `SSH_HOST` | Server name or IP, the same string used with `ssh-keyscan` |
| `SSH_USER` | `deploy` |
| `SSH_PRIVATE_KEY` | The private deploy key, including the `BEGIN` and `END` lines |
| `SSH_KNOWN_HOSTS` | Every non-comment line from `ssh-keyscan -H` |

## 3. Deploy key

On your machine, create a key used only for this deploy. Do not reuse a personal key. Leave the passphrase empty so the runner can use it.

```bash
ssh-keygen -t ed25519 -f ~/.ssh/secret-santa-deploy -C "secret-santa-deploy" -N ""
chmod 600 ~/.ssh/secret-santa-deploy
```

- `~/.ssh/secret-santa-deploy` is the private key. Paste the whole file into `SSH_PRIVATE_KEY`.
- `~/.ssh/secret-santa-deploy.pub` is the public key. It goes into the server's `authorized_keys`.

If the public file is missing, print the public line from the private key:

```bash
ssh-keygen -y -f ~/.ssh/secret-santa-deploy
```

## 4. Server host keys

On your machine:

```bash
ssh-keyscan -H YOUR_SERVER
```

The output has one line per host-key type (`ssh-ed25519`, `ecdsa-sha2-nistp256`, `ssh-rsa`) and a `#` comment. Skip the comment. Put every other line into `SSH_KNOWN_HOSTS`.

`SSH_HOST` must be the name you scanned. A hash made for a hostname does not match a later connection to the IP address, and the reverse is also true.

## 5. Install Docker on Ubuntu

Run this on the server. It installs Docker Engine and the Compose v2 plugin. The workflow calls `docker compose`, not the older `docker-compose` binary.

```bash
sudo apt update
sudo apt install ca-certificates curl
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc

sudo tee /etc/apt/sources.list.d/docker.sources <<EOF
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: $(. /etc/os-release && echo "${UBUNTU_CODENAME:-$VERSION_CODENAME}")
Components: stable
Architectures: $(dpkg --print-architecture)
Signed-By: /etc/apt/keyrings/docker.asc
EOF

sudo apt update
sudo apt install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo docker run --rm hello-world
```

Published container ports can bypass ufw. The Compose file binds the panel to `127.0.0.1:8080` and Postgres to `127.0.0.1:54329`, so those ports are not reachable from the network.

## 6. Create the deploy user

On the server, after Docker is installed:

```bash
sudo adduser --disabled-password --gecos "" deploy
sudo usermod -aG docker deploy
sudo mkdir -p /home/deploy/.ssh /opt/secret-santa
sudo chmod 700 /home/deploy/.ssh
sudo chown deploy:deploy /opt/secret-santa
echo 'ssh-ed25519 AAAA... secret-santa-deploy' | sudo tee /home/deploy/.ssh/authorized_keys
sudo chmod 600 /home/deploy/.ssh/authorized_keys
sudo chown -R deploy:deploy /home/deploy/.ssh
```

Replace the `echo` line with the real public key. From your machine:

```bash
ssh -i ~/.ssh/secret-santa-deploy deploy@YOUR_SERVER
docker compose version
```

The group membership applies to a new SSH session. `docker compose version` must print a v2 version.

## 7. Server environment

Create `/opt/secret-santa/backend/.env` on the server, mode `600`, owned by `deploy`. Compose reads this file because the project directory is `backend/`. Do not commit it.

```bash
sudo mkdir -p /opt/secret-santa/backend
sudo cp /path/to/.env.example /opt/secret-santa/backend/.env
sudo chown -R deploy:deploy /opt/secret-santa
sudo chmod 600 /opt/secret-santa/backend/.env
```

Fill in:

| Variable | Server value |
|---|---|
| `TELEGRAM_BOT_TOKEN` | Token from BotFather |
| `TELEGRAM_BOT_USERNAME` | Bot username without `@` |
| `ADMIN_USERNAME` | Operator login |
| `ADMIN_PASSWORD` | 8–200 characters |
| `ADMIN_COOKIE_SECURE` | `true` once the hostname is served over HTTPS. On plain HTTP the panel ignores it, because a Secure cookie would be dropped and login would fail. |

`DATABASE_URL` in that file is not used by the containers. Compose points them at `postgres:5432`. The localhost URL is only for `make run-admin` on a development machine.

The Postgres user and password inside Compose are `secretsanta` / `secretsanta`. That password is stored in the volume on the first boot and is not changed by later edits. Set a different `POSTGRES_PASSWORD` in the Compose file before the first production `up` if this volume does not exist yet.

Stop any local `make stack` bot before the server bot starts. Telegram allows one polling consumer per token.

## 8. Firewall and nginx

Allow SSH and the web ports. Leave 8080 and 54329 closed.

```bash
sudo ufw allow OpenSSH
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable
```

Point a DNS name at the server. In `backend/deploy/nginx.conf`, replace `admin.example.com` with that name. Copy it to the host nginx once, before certificates exist:

```bash
sudo apt install nginx certbot python3-certbot-nginx
sudo cp /opt/secret-santa/backend/deploy/nginx.conf /etc/nginx/sites-available/secret-santa
sudo ln -s /etc/nginx/sites-available/secret-santa /etc/nginx/sites-enabled/secret-santa
sudo nginx -t && sudo systemctl reload nginx
sudo certbot --nginx -d admin.example.com
```

Certbot adds the HTTPS server and the HTTP-to-HTTPS redirect by editing `/etc/nginx/sites-available/secret-santa`. Do not copy the repo file over that path again. The next deploy syncs `backend/` only and does not touch `/etc/nginx`.

The site proxies to `http://127.0.0.1:8080`. The deploy check uses that same local URL, so it still passes after TLS is enabled.

## 9. First deploy by hand

Confirm SSH as `deploy`, then run the same command the action runs:

```bash
cd /opt/secret-santa/backend
docker compose -p secret-santa --project-directory . -f deploy/docker-compose.yml up -d --build --wait --force-recreate
curl -fsS -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8080/login
docker compose -p secret-santa --project-directory . -f deploy/docker-compose.yml ps
```

Expected result:

- `postgres` is healthy
- `admin` is healthy and the login URL returns 200
- `bot` and `worker` are up
- `migrate` has exited 0
- `https://<your-domain>/login` loads in a browser

The bot log should show `telegram getMe` and then `update loop is not started in this slice`. The worker log should show `database ready` and then `outbox delivery is not started in this slice`. Neither log should contain the bot token or the admin password.

After this succeeds, a push to `main` is the deploy.

## 10. When a deploy fails

Redeploy the previous commit with the same Compose command. That rolls the containers back. It does not roll the database back. Goose migrations in this repo only move forward, so a bad migration needs a new migration, not `goose down`.

The panel is down while the image builds. That is the current tradeoff. A later step is to build the image in Actions, push it to the GitHub Container Registry, and have the server only pull and recreate.
