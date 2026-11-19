# Secret Santa — Admin Panel

**Status:** Implemented  
**Product source:** [design.md](design.md)  
**Backend plan:** [backend-design.md](backend-design.md)  
**Code:** `backend/cmd/admin`, `backend/internal/admin`

The panel is an operator tool. It creates internal groups, issues Telegram deeplinks, and shows who has joined and which links are still usable. It is not the bot, and it does not run games or draws.

## 1. What it is

One extra Go process, `cmd/admin`, talks to the same PostgreSQL database as the bot. Pages are Go `html/template` files with [Bootstrap 5.3.3](https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/css/bootstrap.min.css) loaded from the CDN. There is no Node build and no frontend framework. The browser is not allowed to run scripts (`script-src 'none'`).

Start it from `backend/`:

```bash
make migrate
make run-admin
```

Default address: `http://127.0.0.1:8080`.

## 2. Sign-in

There is one operator. The username and password come from the environment. They are not stored in the database.

| Variable | Required | Default | Rule |
|---|---|---|---|
| `DATABASE_URL` | yes | | Postgres URL |
| `TELEGRAM_BOT_USERNAME` | yes | | Bot username without `@`. Used only to build `t.me` links. The bot token is not required. |
| `ADMIN_USERNAME` | yes | | 1–64 characters, no spaces |
| `ADMIN_PASSWORD` | yes | | 8–200 characters |
| `ADMIN_HTTP_ADDR` | no | `:8080` | Listen address |
| `ADMIN_COOKIE_SECURE` | no | `false` | Set `true` behind HTTPS so cookies are `Secure` |
| `SESSION_TTL` | no | `30m` | Session lifetime |
| `LOG_LEVEL` | no | `info` | `debug`, `info`, `warn`, or `error` |

A `.env` file in `backend/` fills any of these that are not already set. See `backend/.env.example`.

Comparison hashes both sides with SHA-256 and uses `subtle.ConstantTimeCompare`. A failed sign-in always says "Invalid username or password." Logs record `admin login failed` or `admin login succeeded` and do not include the password or the username.

### Session

| Cookie | Purpose |
|---|---|
| `admin_csrf` | Set on the login page. The login form must send the same value. |
| `admin_session` | Set after a successful login. Holds the raw session token. |

Both cookies are `HttpOnly`, `SameSite=Lax`, and `Path=/`. The database stores only the SHA-256 of the session token, plus a CSRF value for later forms, in `admin_sessions`. Expired rows are deleted when a session is looked up.

Every state-changing form posts a `csrf` field that must match the session. A mismatch returns `400`. Unauthenticated requests to any other page redirect to `/login`. Logout deletes the session row and clears the cookie.

Other response headers: `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, and a Content-Security-Policy that allows styles only from the Bootstrap CDN. POST bodies are limited to 64 KiB.

## 3. Screens and routes

`GET /` redirects to `/groups`.

| Method | Path | Who | What it does |
|---|---|---|---|
| GET | `/login` | public | Sign-in form. Already signed-in operators go to `/groups`. |
| POST | `/login` | public | Check username and password. |
| POST | `/logout` | operator | End the session. |
| GET | `/groups` | operator | All groups: name, Active/Archived, active member count, active link count, created time. |
| GET | `/groups/new` | operator | Name form. |
| POST | `/groups` | operator | Create an `ACTIVE` group. Name is trimmed, 1–80 characters. |
| GET | `/groups/{id}` | operator | Group page: deeplinks and members. |
| POST | `/groups/{id}/archive` | operator | Set status to `ARCHIVED`. Idempotent if it is already archived. |
| POST | `/groups/{id}/invites` | operator | Create a deeplink. Rejected when the group is archived. |
| POST | `/invites/{id}/revoke` | operator | Set `revoked_at`. A second revoke is a no-op and still returns to the group. |

Times on the page are UTC, formatted as `02 Jan 2006 15:04 UTC`.

The panel cannot rename a group, unarchive it, delete it, or change member roles. It does not create games.

## 4. Groups and members

`groups.owner_id` is nullable. The panel does not set an owner, because the operator is not a Telegram user. The first Telegram owner is assigned later by the bot.

The member table is read-only. It lists `display_name`, `@username`, role (`OWNER`, `ADMIN`, `MEMBER`), status (`ACTIVE`, `LEFT`), and `joined_at`. Nobody is inserted here by the panel. Until the bot accepts deeplinks, the list stays empty and the page says so.

Active member count on the list is `status = 'ACTIVE'` only.

## 5. Deeplinks

A new link is:

```text
https://t.me/<TELEGRAM_BOT_USERNAME>?start=join_<token>
```

The token is 16 random bytes, encoded with base64 raw URL (22 characters). The row stores:

- `token` — the raw token, so the page can show the link again
- `token_hash` — SHA-256 of that token, unique, for the bot to look up later without comparing the raw value

The link is a column on the group page. It stays there after reload and after revoke. Links created before `token` existed have a hash only; the page shows "Unavailable" for those, because the raw token cannot be recovered.

Optional limits on the create form:

| Field | Empty | Set |
|---|---|---|
| Use limit | unlimited | integer from 1 to 100000 |
| Expiry date | never | end of that calendar day, `23:59:59 UTC`. A date that is already over is rejected. |

Shown status, in this order:

1. **Revoked** — `revoked_at` is set
2. **Expired** — `expires_at` is in the past
3. **Used up** — `used_count` has reached `max_uses`
4. **Active** — otherwise

The list's "Active links" count uses the same rule in SQL. `used_count` is stored and displayed (`0 / 4`, or `0 / unlimited`). Nothing in the panel increments it. That happens when the bot accepts a join.

Archiving a group hides the "new deeplink" form and refuses new links. Existing links remain on the page. Revoke is offered only while the status is Active.

## 6. Data the panel uses

Migrations `00002_groups.sql` and `00003_invite_token.sql`.

| Table | Panel use |
|---|---|
| `users` | Read when listing members. The panel does not insert users. |
| `groups` | Create and archive. |
| `group_members` | Read. One active owner per group is enforced by a partial unique index, for the bot. |
| `group_invites` | Create, list, revoke. |
| `admin_sessions` | Sign-in sessions. `flash` is unused; the link is no longer a one-time message. |

Group names may repeat. Identity is the numeric id.

## 7. Process

`cmd/admin` loads config, opens the pgx pool, and serves until `SIGINT` or `SIGTERM`. Shutdown waits up to 5 seconds. HTTP timeouts: header 5s, read 10s, write 15s, idle 60s.

Startup logs the listen address only.

The bot and the worker are separate commands. They do not serve this UI. If the bot later binds a webhook on `:8080`, set `ADMIN_HTTP_ADDR` to another port.

## 8. Tests

- `go test ./internal/admin/` covers sign-in, CSRF, group create, a deeplink that is still present on reload and after revoke, and archive.
- `go test -tags=integration ./internal/admin/` applies migrations and checks the token, its hash, member counts, expiry, revoke, and the archived-group refusal.

The password and the invite token must not appear in log lines. The handler tests check the login failure log.

## 9. Later improvements

These are not built. Order is the suggested one.

### 9.1. Bot consumes the links

The panel already shows members and `used_count`, but both stay at zero until the bot handles `join_<token>`.

When that lands, the bot should hash the token and look up `token_hash`, then in one transaction check revoked, expired, and `max_uses`, increment `used_count`, and create the `group_members` row. The panel should not grow a second copy of that logic.

Joining still follows [design.md](design.md): first join during open registration confirms the current game; a join after the draw waits for an admin of the game. The operator panel does not do that confirmation.

### 9.2. Games on the group page

After games exist, the group page should show the current game: status, confirmed / pending / declined counts, and people waiting to be added after a draw. It should not show giver-to-receiver pairs. Delivery counts can be added the same way as the bot's admin view: status and error class, not the message payload.

Creating a game can stay in the bot, or move here later. Either way there is still one current game per group.

### 9.3. Group edits the panel still lacks

- Rename.
- Unarchive, only when the operator explicitly asks.
- Transfer of `owner_id` once a Telegram owner exists.
- Role changes for members (`ADMIN` / `MEMBER`). The owner row stays protected the same way as in the backend plan.

### 9.4. Secrets and access

The raw invite token is in `group_invites.token` on purpose, so the operator can copy it later. A database copy therefore contains live join links.

A later step can encrypt `token` with a key that only the admin process has, and keep `token_hash` for the bot. The page would still show the decrypted link. Do not go back to showing the link once and throwing away the secret.

Other access work, when more than one operator exists:

- Store a password hash (not the env string) and support more than one account.
- Rate-limit failed sign-ins.
- Write an audit row for create, archive, invite, and revoke, with no token in the audit payload.
- Drop the unused `admin_sessions.flash` column.
- Set `ADMIN_COOKIE_SECURE=true` in any HTTPS deployment.

### 9.5. Small UI

A copy control needs a script, which the current CSP forbids. If it is added, use a per-response nonce rather than `unsafe-inline`. Filtering the group list and paging it can wait until there are more than a handful of groups.
