# Secret Santa — Technical Design Document

**Status:** Current  
**Product language:** Russian in the bot, English in the operator panel  
**Stack:** Go 1.23, PostgreSQL 16, Telegram Bot API

This file is the gathered list of requirements and the design choices that are in force. It replaces conflicting statements in the earlier drafts.

| Document | Role now |
|---|---|
| [tdd.md](tdd.md) | Requirements and decisions. Start here. |
| [design.md](design.md) | Original product draft, v0.3. Still useful for scenarios that are not built yet. Where it disagrees with this file, this file wins. |
| [backend-design.md](backend-design.md) | Original Go plan. Same rule: this file wins on conflict. |
| [admin.md](admin.md) | Operator panel, including screens and later improvements. |
| [deploy-action.md](deploy-action.md) | How the GitHub Action installs the stack on the server. |

## 1. Product

Secret Santa runs inside one Telegram bot. People are grouped into **internal groups**. Those groups are not Telegram chats. Telegram is the interface, the login (Telegram user id), and the channel for private messages.

One bot serves many groups. One person can be in several groups. A group can run many games over time, but only one of them is current.

Joining a group and joining a game are different. Membership survives from one game to the next.

## 2. Requirements

Status is against the code in this repository.

| # | Requirement | Status |
|---|---|---|
| 1 | One bot serves at least two independent internal groups. | Done |
| 2 | One person can belong to several groups. | Done |
| 3 | A deeplink adds the person to the internal group, not to a Telegram chat. | Done |
| 4 | The first join through an invite, while game registration is open, confirms that person in the current game. | Done |
| 5 | People who were already in the group must confirm each new game themselves. Creating a game does not confirm them. | Done |
| 6 | Opening an invite again does not change a confirmation or a refusal that was already recorded. | Done for an active member. Leave and rejoin are not built. |
| 7 | The person who created the group sees every member. Everyone else sees the owner's name and the number of people, not the other names. | Done. This replaces the older rule that every member, and every admin, sees the full roster. |
| 8 | The draw is one random cycle. A second request does not create a second draw. | Not built |
| 9 | A participant sees only their own receiver. | Not built |
| 10 | Failed deliveries are stored and retried. | Not built. The worker process exists and waits. |
| 11 | A person who joins after the draw does not change assignments until an admin confirms it. | Not built |
| 12 | An admin can insert one waiting person into the cycle, or redraw everyone, as a new draw version. | Not built |

Also required, and done:

- First private `/start` stores the user and shows **Мои группы**, **Создать группу**, **Помощь**.
- Creating a group makes the creator `OWNER` and issues one invite.
- **Пригласить** copies the invite link to the clipboard. The link is also written on the group card.
- Owner or admin can create a game: name, optional gift note, registration deadline, exchange date. The game opens in `REGISTRATION`.
- While registration is open, a participant can confirm or decline, and can change that answer.
- The operator panel can create groups, issue and revoke deeplinks, archive a group, and list registered users. It does not run games.

Explicitly out of the MVP, unchanged from the original draft: wishlists as a separate feature, anonymous questions, excluded pairs, reminders, gift-ready confirmation, a Mini App, more than one current game per group, and any language besides Russian in the bot.

The gift note on a game is free text. It is not a separate wishlist.

## 3. Decisions in force

These are the choices that drifted away from [design.md](design.md) or [backend-design.md](backend-design.md), plus the ones that still hold.

| Topic | Decision |
|---|---|
| Groups | Internal rows. Not Telegram chats. Private chats only. Updates from groups and channels are consumed and ignored. |
| Game budget | There is no ruble field. The organizer writes an optional description, up to 1000 characters. The prompt is: «Можешь написать здесь пожелания к подаркам. Например бюджет, цвет и т.д.» A dash stores no description. |
| Dates | Calendar days use Moscow time, fixed UTC+3, no daylight saving. A registration date includes that whole day. It is stored as the exclusive start of the next Moscow day. The exchange date is a `date`. A registration date that is already over is rejected in the wizard. A dash skips either date. |
| Game start | A new game is inserted as `REGISTRATION`. `DRAFT` exists in the check constraint and is not used. |
| One current game | At most one row per group in `DRAFT`, `REGISTRATION`, `LOCKED`, or `DRAWN`. `COMPLETED` and `CANCELLED` free the group. Enforced by a partial unique index. |
| Who creates a game | `OWNER` and `ADMIN`. A member does not see the button, and the service rejects the call. |
| Who sees members | Only the active `OWNER` gets the names. Other members, including admins, get the owner's display name and the active member count. The SQL query enforces this, not only the button. |
| Invite secret | 16 random bytes, base64 raw URL, 22 characters. The row stores both the raw token and the SHA-256 of that string. The raw token is stored so the link can be shown again. Do not go back to showing it once. |
| Invite use | Opening the link does not increment `used_count`. Accepting does, and only when the person is not already an active member. A second accept is a success and does not consume the invite again. |
| Deeplink | `https://t.me/<TELEGRAM_BOT_USERNAME>?start=join_<token>`. The join button carries the invite id (`inv:accept:<id>`), not the token. |
| Copy button | **Пригласить** is a Telegram `copy_text` button. It has no `callback_data`. |
| Draw | One cycle: shuffle with `crypto/rand`, edge `i → i+1`, last → first. At least 3 confirmed participants. Not built yet. |
| Assignments | Admins see statuses, never giver → receiver pairs. Assignment text is not written to logs or audit rows. |
| Queries | Hand-written SQL in the service packages. `sqlc` is not used. `make sqlc` does not generate code. |
| Processes | One image, four commands: `bot`, `worker`, `admin`, `migrate`. Code lives under `backend/`. |
| Bot transport | Long polling. Webhook mode is rejected at startup. |
| Migrations | Goose SQL in `backend/db/migrations`. Applied by `cmd/migrate`, not by the bot on startup. |
| IDs | `bigint GENERATED ALWAYS AS IDENTITY`. |
| Logging | `log/slog` JSON to stdout. Never log the bot token, the admin password, an invite token, or an assignment payload. |

## 4. People, groups, and invites

### User

Created or updated on every private update. `telegram_id` is unique. The display name is first name plus last name, otherwise the username, otherwise «Участник», at most 128 characters. A username that contains whitespace is dropped. `dm_started_at` is set the first time the person messages the bot in private. The bot cannot message someone who has never started it.

### Group

Name is trimmed, 1–80 characters. Names may repeat. Status is `ACTIVE` or `ARCHIVED`.

A group created in the bot has `owner_id` set and one `OWNER` membership. A group created in the operator panel has no owner. The first person who joins an operator-created group stays `MEMBER`.

### Membership

Role is `OWNER`, `ADMIN`, or `MEMBER`. Status is `ACTIVE` or `LEFT`. One active owner per group, enforced by a partial unique index. The owner cannot be demoted or removed until ownership is transferred. Transfer, grant, revoke, rename, archive, and leave are specified and not built in the bot.

### Invite

Belongs to the group, not to a game. Usable when the group is active, the invite is not revoked, it is not past `expires_at`, and `used_count` is below `max_uses` when a limit is set. The check and the increment happen in one transaction under a row lock.

The operator panel can set a use limit (1–100000) and an expiry at 23:59:59 UTC on the chosen day. The bot's first invite has neither limit.

### Join while a game exists

Evaluated only when the person is not already an active member. An active member who accepts again gets a success and the participant row is left alone.

| Current game | Membership | Participation |
|---|---|---|
| None, or `COMPLETED` / `CANCELLED` | `ACTIVE` / `MEMBER` | none |
| `DRAFT` | create member | none |
| `REGISTRATION`, deadline not passed | create member | `CONFIRMED`, `NOT_APPLICABLE` |
| `REGISTRATION`, deadline passed | create member | none |
| `LOCKED` | create member | none |
| `DRAWN` | create member | `CONFIRMED`, `AWAITING_ASSIGNMENT` |

The `DRAWN` row is the required rule. The code enrolls only the open-registration case. A later join must not change an existing participant row: `ON CONFLICT DO NOTHING`.

If the new member was auto-confirmed, the bot says they were registered in that game and offers **Отказаться**.

## 5. Games

### Status

```text
DRAFT → REGISTRATION → LOCKED → DRAWN → COMPLETED
                        ↘ CANCELLED
```

`LOCKED → REGISTRATION` is allowed in the plan, as a reopen before any draw. It is not built.

A game in `DRAFT`, `REGISTRATION`, `LOCKED`, or `DRAWN` is current.

### What the organizer enters

1. Name, 1–80 characters.
2. Description, optional, 1–1000 characters, or a dash.
3. Registration deadline, `ДД.ММ.ГГГГ`, or a dash.
4. Exchange date, `ДД.ММ.ГГГГ`, or a dash.

The wizard state is `game_name`, then `game_description`, then `game_deadline`, then `game_exchange`. `/start`, **Отмена**, and any button clear the wizard. A session older than `SESSION_TTL` (default 30 minutes) is deleted.

### Participation

| Status | Meaning |
|---|---|
| `PENDING` | Already in the group when the game was created. Has not answered. |
| `CONFIRMED` | Confirmed by hand, or auto-confirmed on first join. |
| `DECLINED` | Refused. Still a member of the group. |

`assignment_state` is `NOT_APPLICABLE`, `AWAITING_ASSIGNMENT`, or `ASSIGNED`. It changes only in the same transaction as the draw. Rows created at game start, and auto-confirmed joins during registration, are `NOT_APPLICABLE`.

While status is `REGISTRATION` and the deadline is unset or still in the future:

```text
PENDING   → CONFIRMED | DECLINED
CONFIRMED → DECLINED
DECLINED  → CONFIRMED
```

After `LOCKED` or `DRAWN`, the person cannot change this themselves. That lock is not built; the deadline check is.

The game card shows the name, status, confirmed count against all participant rows, the description, the deadline, the exchange date, and the caller's own answer. It does not list other people's names. The grouped lists (confirmed, pending, declined, waiting) are still required and not built.

### Draw, notify, late join

Specified, not built. Keep these rules when they are built:

- Minimum 3 `CONFIRMED` participants. The admin confirms the draw with a second button.
- Draw, assignments, and outbox rows commit together. `games.active_draw_id` points at the active draw. One active draw per game.
- The worker sends after commit. Retry temporary errors. A blocked user becomes `FAILED`. A new draw version cancels unsent messages for the old draw.
- Each person can read only their own receiver from the database.
- Insert one waiting person by replacing a single edge. Warn that one existing giver's receiver will change, without naming receivers. A full redraw is a separate confirmation and notifies everyone.
- Leave before the draw marks the member `LEFT` and the participant `DECLINED`. Leave after the draw is rejected until an admin fixes the cycle.

## 6. Bot interface

Main menu:

```text
🎄 Secret Santa
[Мои группы]
[Создать группу]
[Помощь]
```

Group card for the owner:

```text
👥 Друзья
Участников: 8
Ты: владелец
Текущая игра: Тайный Санта 2026

Ссылка для приглашения:
https://t.me/<bot>?start=join_<token>
[Участники]
[Текущая игра]          or [Создать игру] when there is no current game
[Пригласить]            copies the link
[Мои группы]
```

For anyone who is not the owner, the card shows «Владелец: <name>» and the count. It has no **Участники** button. **Пригласить** stays, because any member who can open the card may share the link.

Game card:

```text
🎁 Тайный Санта 2026
Статус: Регистрация
Подтвердили: 1 / 4
Описание: до 2000 ₽, красный
Регистрация до: 01.01.2030
Обмен: 25.12.2026

Ты ещё не подтвердил участие.
[Подтвердить участие]
[Отказаться]
[К группе]
```

Buttons that are still required and not on the screen: leave group, rename, archive, roles, close registration, start draw, waiting list, redraw, delivery status, and the participant lists by status. There is no separate «Управление группой» screen yet. **Создать игру** and **Текущая игра** live on the group card.

Callback data is at most 64 bytes, `verb:action` or `verb:action:<id>`. Unknown callbacks say «Это действие больше недоступно.» and change nothing.

## 7. Data model

Built in migrations `00001`–`00006`:

| Table | Purpose |
|---|---|
| `users` | Telegram identity and display name. |
| `groups` | Internal group. `owner_id` nullable. |
| `group_members` | Role and `ACTIVE` / `LEFT`. |
| `group_invites` | `token_hash` unique, `token` nullable for rows created before the column existed. |
| `games` | Name, status, `description`, `registration_deadline`, `exchange_on`, `active_draw_id` with no foreign key yet. |
| `game_participants` | Status, assignment state, unique `(game_id, user_id)`. |
| `admin_sessions` | SHA-256 of the session token. `flash` is unused. |
| `processed_updates` | One row per Telegram `update_id`. |
| `bot_offsets` | Single polling cursor, id = 1. |
| `bot_sessions` | Wizard state and JSON context, one row per user. |

Not created yet: `draws`, `assignments`, `notification_outbox`, `audit_logs`.

`active_draw_id` stays a plain nullable bigint until `draws` exists. When draws are added, an assignment must belong to the same game as its draw, and exactly one draw per game may be `ACTIVE`.

## 8. Processes and layout

```text
Telegram private chat
        |
        v
   cmd/bot          long poll, handle, reply
        |
        v
   PostgreSQL 16
        ^
        |
   cmd/admin        operator HTTP
   cmd/worker       idle until outbox exists
   cmd/migrate      goose up, then exit
```

Module `github.com/alekseymakarkin/secret-santa`. Packages that exist: `config`, `platform/postgres`, `platform/logger`, `users`, `groups`, `games`, `telegram`, `admin`. `telegram` calls `groups` and `games`. `groups` calls `games.EnrollNewMember` inside the join transaction. `games` does not import `groups` or `telegram`.

The bot claims `update_id` in the same transaction as the user upsert. A duplicate update does not send a second message. The offset advances only after a successful handle. A database error stops the batch so the offset is not skipped. A Telegram send error is logged and the offset still advances.

`cmd/worker` pings the database, logs that the outbox is not started, and waits for `SIGINT` or `SIGTERM`.

Configuration comes from the environment. A `backend/.env` fills variables that are not already set. The bot requires `DATABASE_URL`, `TELEGRAM_BOT_TOKEN`, and `TELEGRAM_BOT_USERNAME`. The panel does not need the bot token. It needs the username to build links, plus `ADMIN_USERNAME` and `ADMIN_PASSWORD` (8–200 characters).

## 9. Operator panel

One operator. The username and password are environment variables, not database rows. A failed login always says "Invalid username or password."

Cookies `admin_session` and `admin_csrf` are `HttpOnly` and `SameSite=Lax`. The database stores the SHA-256 of the session token. Login compares the CSRF cookie with the form field. `ADMIN_COOKIE_SECURE=true` marks cookies `Secure` only when the request is HTTPS (`TLS` or `X-Forwarded-Proto: https`). On plain HTTP the flag must not be forced, or the browser drops the CSRF cookie and login returns "Form expired."

Pages are Go templates and Bootstrap from the CDN. Content-Security-Policy sets `script-src 'none'`. There is no copy button in the panel.

Routes: `/login`, `/groups`, `/groups/new`, `/groups/{id}` (members and deeplinks), archive, create invite, revoke invite, `/users`. `/users` lists people who have a row: name, username, Telegram id, active group count, when they first opened the chat, and when they were registered. The panel does not create Telegram users.

Details and the later panel work are in [admin.md](admin.md).

## 10. Privacy and abuse

- Do not put a receiver's name in a group message, a member list, an admin page, a log line, or an audit row.
- Check the caller's user id and role on every mutation. Hiding a button is not the check.
- Redact `join_<token>` from error logs.
- The raw invite token in `group_invites.token` means a database copy contains live links. Encrypting that column is a later option. Deleting it after the first view is not.
- Deduplicate `update_id`. Unique membership and unique participation stop double joins and double registration.

## 11. Running and deploy

Local host URL, for `make run-bot` and `make run-admin`:

```text
postgres://secretsanta:secretsanta@localhost:54329/secretsanta?sslmode=disable
```

Compose overrides that with `postgres:5432` inside the network. Published ports are bound to `127.0.0.1`. The Compose project name is `secret-santa`. The volume is `secret-santa_postgres_data`. Do not rename the project or the directory, and do not run `docker compose down -v`.

`make up` starts Postgres only. `make stack` builds and starts the whole stack. New SQL runs only when the migrate container is recreated (`--force-recreate`). The Postgres password is set when the volume is first created. Changing it in the file later does not change the volume.

GitHub Actions, on push to `main`, rsyncs `backend/` to `/opt/secret-santa/backend/` except `.env`, then recreates the stack. Secrets in the workflow are SSH only. The bot token and the admin password stay in the server `.env`. The directory must be owned by `deploy`. Nginx on the server proxies the panel and sets `X-Forwarded-Proto`. Steps are in [deploy-action.md](deploy-action.md).

Only one process may poll a given bot token.

## 12. Tests

`go test ./...` does not need Postgres. Integration tests use `//go:build integration` and `DATABASE_URL`. They apply migrations themselves. They must not move `bot_offsets` to a high value, because that cursor is shared with a real bot on the same database.

A test must not print the bot token, the admin password, or an invite token.

## 13. Still to build

In the order of the original milestones, after what is already done:

1. Finish games: close and reopen registration, cancel, lists by participant status, leave group.
2. Draw and the private "my receiver" view.
3. Outbox worker, Russian assignment message, retry, and delivery status without pairs.
4. Late join as `AWAITING_ASSIGNMENT`, single-edge insert, full redraw.
5. Audit log without assignment payloads. Webhook mode only if polling is no longer enough.

Group admin actions that are still only specified: rename, archive, transfer ownership, grant and revoke admin, revoke and rotate invites from the bot.
