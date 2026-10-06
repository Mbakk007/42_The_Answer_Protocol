*This project has been created as part of the 42 curriculum by ael-bakk, pearredo.*

# TAP — The Answer Protocol

## Description

TAP is a small multi-user dungeon (MUD): a shared, text-based world that
several players explore at the same time over TCP. The server implements
RFC 42TAP, a line-oriented protocol where every message is a single
UTF-8 line terminated by `\n`.

The repository contains three programs:

- **server** — holds the world, accepts TCP connections, and implements
  every command and event in RFC 42TAP.
- **cli** — a command-line client that speaks the raw protocol.
- **gui** — a graphical client built with Fyne, showing the room, the
  player's inventory, chat channels and a protocol log.

The world is a Nordic settlement of eight rooms with NPCs to talk to,
enemies to fight, items to pick up and carry, and two quests. World data
is static JSON, loaded and validated at startup. No state is persisted:
restarting the server resets the world.

## Instructions

Requirements:

- Go 1.22 or newer
- A C toolchain for Fyne's native window (on macOS: `xcode-select --install`;
  on Debian/Ubuntu: `libgl1-mesa-dev` and `xorg-dev`)

```sh
make deps            # download Go modules
make run-server      # start the server on :4040
make run-client      # start the CLI client
make run-client-gui  # start the GUI client
```

Run the server from the repository root: the world file is loaded from
the relative path `world/world.json`.

## Architecture

**Dispatcher.** `cmd/server/conn.go` reads one line per command, splits
it into a verb and the remainder of the line, and routes the verb through
an explicit `switch` to a handler in `handlers.go`. A `switch` was chosen
over a handler map so that every supported verb is visible in one place;
the cost is one extra line per command, which is acceptable for a fixed
set of fifteen.

**Line parsing.** Commands are split with `strings.SplitN(line, " ", 2)`
rather than by whitespace, so the final argument keeps its spaces. This
is what allows `CHAT GLOBAL hello everyone` and `TAKE Frost Herbs` to
work. Each handler splits its own remainder further only as far as it
needs to. Verbs are case-insensitive (RFC 4.2); empty lines are ignored.

**Concurrency.** One goroutine per connection, started by the accept loop
in `main.go`. All shared state — the `clients` map, each `player` struct,
and the loaded world — is guarded by a single `sync.Mutex`. Handlers take
the lock, perform their state changes, copy out whatever they still need,
release the lock, and only then write to the network or broadcast. This
ordering is required because `sync.Mutex` is not reentrant and the
broadcast helpers take the same lock, and because reading a `player`
field after unlocking would be a data race.

**Broadcasts and slow clients.** Broadcasting writes directly to each
client socket while the lock is held. On its own this would mean that a
client which has stopped reading — a suspended process, a sleeping
laptop, a dropped connection the kernel has not yet noticed — fills its
send buffer, blocks the write, and stalls every other goroutine behind
the mutex. `broadcastWrite` therefore sets a 100 ms write deadline before
each broadcast write and clears it afterwards, so a stuck socket fails
fast, is logged at `WARN`, and the broadcast continues to everyone else.
The deadline applies only to broadcasts; ordinary command replies are
unaffected.

**State.** Static world data and runtime state share the same structures:
`Location.Items` is loaded from JSON and then mutated as players take and
drop things, and `NPC.CurrentHP` is seeded from `NPC.HP` at load time and
decremented in combat. This keeps a single source of truth at the cost of
the loaded world no longer being read-only. Since the subject requires no
persistence, restarting the server restores the world file exactly.

## Protocol Implementation

The server follows RFC 42TAP. Deviations and additions are listed here.

**Error codes not in the RFC.** The RFC's table (8.2) has no code for a
malformed command, an unknown verb, acting before `CONNECT`, connecting
twice, an item that exists but cannot be picked up, an NPC with no
dialogue, or an unknown player name. The following were added, following
the RFC's 3-digit convention and reusing the nearest existing band:

| Code | Message | Meaning |
|------|---------|---------|
| 400 | `BAD_REQUEST` | Malformed or incomplete command |
| 400 | `UNKNOWN_COMMAND` | Verb not in the protocol |
| 402 | `ALREADY_CONNECTED` | `CONNECT` sent twice on one connection |
| 403 | `NOT_AUTHENTICATED` | Command sent before `CONNECT` |
| 404 | `PLAYER_NOT_FOUND` | `GROUP INVITE` names an unknown player |
| 405 | `ITEM_NOT_OBTAINABLE` | Item is scenery and cannot be taken |
| 405 | `NPC_NOT_TALKATIVE` | NPC has no dialogue (enemies) |

402 is reused for `ALREADY_CONNECTED` because it matches the sense of the
RFC's own `ALREADY_IN_GROUP`: the client is asking for a state it already
holds. 405 is reused for the two "present but not valid for this action"
cases, matching the sense of `NPC_NOT_HOSTILE`.

**`WHO`.** The RFC (5.2.2) specifies `OK players=<count>`; the subject's
examples show a richer JSON object with a room roster. The RFC was
followed, since the subject states protocol compliance is mandatory. The
per-room roster is available through `LOOK`, which carries a `players`
array, and the GUI uses both to show its two counters.

**`TALK`.** The RFC (5.4.4) returns the dialogue as plain text after `OK`;
the subject's example shows a JSON object. The RFC was followed.

**`EVT ROOM COMBAT`.** The RFC's event table (6.2.1) defines only
`PRESENCE ENTER`, `PRESENCE LEAVE` and `CHAT` for room scope, but the
subject requires combat results to be broadcast to relevant players. A
`COMBAT` room event was added for kills and defeats.

**Usernames.** `CONNECT` takes the remainder of the line, so usernames may
contain spaces. The RFC leaves `username` undefined. Uniqueness among
active connections is enforced as required (3.3), and a second `CONNECT`
on an already-authenticated connection is rejected rather than treated as
a rename.

**Line length.** The RFC recommends a 1024-byte limit (9.4). The server
uses `bufio.Scanner`, whose default maximum token size is 64 KB; longer
lines cause a scan error and the connection is dropped. The recommended
limit is therefore not enforced exactly.

## Combat System

**Turn model.** One `ATTACK` command is one complete exchange: the player
strikes first, and the NPC counterattacks if it survives. The turn
boundary is the command boundary. Initiative is therefore fixed — the
attacker always acts first — which suits a request/response protocol with
no server-driven timers, and means a player is never damaged by a command
they did not send.

**Damage.** Fixed values, no randomness. The player deals 10 damage per
attack. Each enemy deals its own `damage` value from the world file: the
Snow Wolf 8, the Forge Wraith 12. Deterministic damage makes combat
testable and puts the difficulty curve in the data rather than in the
code.

**Health.** Players start at 100 HP, the maximum. Enemy HP varies by type:
the Snow Wolf has 40, the Forge Wraith 60. `STATUS` reports `hp`, `max_hp`
and a status word: `healthy` at 50 HP or above, `wounded` below.
`ATTACK` replies with `attacker_hp`, `target_hp`, `damage` and a status of
`combat`, `victory` or `defeated`.

**Death.** A player reduced to 0 HP or below respawns at the world's start
room with 50 HP — half the maximum — which is the subject's "safe location
with reduced health". A defeated player's death and re-entry are broadcast
to the rooms concerned. A killed NPC is removed from its room and its
`drops` fall to the floor, where any player may pick them up. NPCs do not
respawn; restarting the server restores them.

**Omitted commands.** `DEFEND`, `FLEE` and `USE_ITEM` are listed by the
RFC (6.1.1) as optional. They were not implemented, to keep the combat
model small enough to specify completely: with fixed damage and no
initiative order, a `DEFEND` command would have no meaningful effect
without also introducing defence values and multi-round state.

## Quest System

**Data.** Quests live in the world file with a `type` (`fetch` or `kill`),
a `target`, an optional `proof` item and a `reward_hp`. The Shrine Keeper
in the Old Shrine gives both:

- `quest.herbs` — a fetch quest: bring Frost Herbs from Icy Creek.
- `quest.wolf` — a kill quest: kill the Snow Wolf in Pinewood and return
  with its pelt.

**Progression.** `QUEST <npc>` is the single verb for the whole lifecycle.
The server walks the NPC's quest list and acts on the first one the player
has not completed:

1. If the player does not have it, it becomes active and is returned with
   status `available`.
2. If the player has it active, this is a turn-in attempt.
3. If every quest the NPC offers is complete, `ERR 406 NO_QUEST_AVAILABLE`.

Because the list is walked in order and the handler stops at the first
incomplete quest, the Shrine Keeper's two quests form a chain: the wolf
quest is only offered once the herbs have been delivered.

**Validation.** Completion is validated at turn-in, from the player's
inventory. A fetch quest requires its `target` item; a kill quest requires
its `proof` item, which only exists because the enemy dropped it. Proving
a kill by carrying its drop means the server does not need to track kill
counts per player, and it keeps both quest types on one code path. If the
required item is absent, the quest is returned with status `active` and
nothing changes.

**Rewards.** On completion the required item is consumed, the quest moves
to the player's completed list, and the player is healed by `reward_hp`
(20 for the herbs, 30 for the wolf), capped at 100. Healing was chosen as
the reward because the world has no currency and no equipment; it is
useful precisely when a player has been fighting.

**`QUESTS`** lists the player's own active and completed quests with a
`progress` field. Both quests have a single objective, so progress is
`0/1` or `1/1`.

## World Design

Eight rooms, six of which form a closed circuit:

```
              Old Shrine
                   |
               Mead Hall
                   |
Frozen Well — Frostmere Gate — Trading Post
     |                               |
 Icy Creek ——— Pinewood ——————— The Forge
```

`Frostmere Gate → Frozen Well → Icy Creek → Pinewood → The Forge →
Trading Post → Frostmere Gate` is walkable in both directions. Mead Hall
and Old Shrine form the optional branch off the hub, with Old Shrine as
the only dead end. Every exit has a matching reverse exit.

The world file's top-level `start` field names the room players spawn in
and respawn at, so the spawn point is world data rather than a constant in
the server, and it is checked by the validator like any other reference.

**NPCs** cover the three required roles:

| NPC | Room | Role |
|-----|------|------|
| Innkeeper | Mead Hall | dialogue |
| Fur Trader | Trading Post | dialogue |
| Shrine Keeper | Old Shrine | quest-giver |
| Snow Wolf | Pinewood | enemy (40 HP, 8 damage) |
| Forge Wraith | The Forge | enemy (60 HP, 12 damage) |

**Items.** Five, of which two are placed in the world and obtainable
(Iron Bucket at the Frozen Well, Frost Herbs at Icy Creek). The Anvil at
the Forge is scenery and refuses `TAKE`. The Wolf Pelt and Ember Shard
exist only as enemy drops and enter the world when their enemy dies.

Items are unique instances: `TAKE` removes an item from the room and adds
it to the player's inventory, `DROP` does the reverse, and two players
racing for the same item means exactly one of them gets it. Items can be
referenced by canonical id (`item.frost_herbs`) or display name
(`Frost Herbs`, case-insensitive), including multi-word names.

**Validation.** `world.Validate()` runs at startup and reports every
dangling reference at once: a missing or unknown `start` room, exits
pointing at rooms that do not exist, rooms listing unknown items or NPCs,
NPCs dropping unknown items or offering unknown quests, and quests naming
an unknown giver, target or proof. The server refuses to start if the
world is invalid, so a typo in the data file fails immediately instead of
at the moment a player walks into it.

## Server Logging

Logging uses Go's standard `log/slog` with a JSON handler writing to
stdout, so the log can be piped to a file, `jq`, or any log collector
without a third-party dependency.

**Format.** One JSON object per line, with an RFC 3339 timestamp to
microsecond precision, a level, a message and structured fields:

```json
{"time":"2026-09-24T16:59:09.588089+02:00","level":"INFO","msg":"command","addr":"[::1]:49625","player":"alice","verb":"LOOK","args":""}
```

**Levels.** `INFO` for normal activity, `WARN` for protocol errors sent to
clients, failed broadcast writes and suspected abuse, `ERROR` for
connection read failures.

**Events logged:**

| Event | Fields |
|-------|--------|
| `world loaded` | room, item, NPC and quest counts |
| `listening` | bind address |
| `client connected` / `client disconnected` | address, player name |
| `command` | address, player, verb, arguments |
| `response` | address, the exact line sent |
| `broadcast write failed` (WARN) | address, error |
| `command flooding` (WARN) | address, player |
| `scan error` (ERROR) | address, error |

**How responses are captured.** Rather than a log call in each of the
fifteen handlers, `loggedConn` in `logging.go` embeds `net.Conn` and
overrides `Write` to log each line before passing it on. Because handlers
take a `net.Conn` interface and the `clients` map stores the wrapper,
every reply *and* every broadcast event is logged with no handler changes,
and it is impossible to add a new handler that silently skips logging.
Lines beginning with `ERR` are logged at `WARN`, so error codes can be
filtered out directly.

World state changes are therefore recorded through the pair of entries
that produced them: the `command` entry carries the player and arguments,
and the `response` entry immediately after carries the outcome — for
example `OK taken=item.frost_herbs` for an item movement, or the combat
JSON for an attack, with the matching `EVT ROOM COMBAT` broadcast logged
as its own response line.

**Abuse detection.** Each connection keeps a `floodWindow`: a rolling
5-second counter that emits a `WARN` past 40 commands. Rapid reconnection
from one address is visible in the log as repeated `client connected`
entries but is not currently aggregated or rate-limited.

**Performance.** `slog`'s JSON handler is allocation-light and writes are
buffered by the OS. Logging happens outside the mutex in the command path
and inside it during broadcasts, which is the same cost as the network
write it accompanies.

**Monitoring.**

```sh
make run-server > server.log 2>&1 &
jq 'select(.level == "WARN")' server.log       # protocol errors and abuse
jq 'select(.verb == "ATTACK")' server.log      # combat activity
jq -r 'select(.msg == "command") | .player' server.log | sort | uniq -c
```

## Group Contributions

**ael-bakk** — TCP server and the full RFC 42TAP command set, the line
parser and dispatcher, the concurrency and locking model, the broadcast
write deadline, world data design and the `world` package with its loader
and validator, the combat and quest systems, structured logging, the CLI
client, and the Makefile.

**pearredo** — the Fyne GUI client: layout, the separation of the chat
channels from the protocol log, parsing of server replies into the room,
inventory and counter panels, and the action buttons.

## Building and Running

The build tool is a `Makefile` wrapping the Go toolchain.

| Target | Effect |
|--------|--------|
| `make deps` | `go mod download` and `go mod tidy` |
| `make build` | build all three binaries into `bin/` |
| `make run-server` | run the server on `:4040` |
| `make run-client` | run the CLI client |
| `make run-client-gui` | run the GUI client |
| `make lint` | `gofmt -l .` and `go vet ./...` |
| `make fmt` | `gofmt -w .` |
| `make clean` | remove `bin/` and run `go clean` |

`make lint` uses `gofmt` and `go vet` from the standard toolchain rather
than an external linter, so it needs no installation beyond Go itself.

The CLI client sends what you type straight to the server as a raw
protocol line — option (1) in the subject. No translation layer was added,
so what the client sends is exactly what RFC 42TAP specifies, which makes
the client usable for testing any conforming server. The GUI client offers
buttons for the commands and also keeps a raw input box, so the protocol
can be driven directly from either client.

## Testing

**Manual multiplayer.** Start the server, then two or more clients (any
mix of CLI and GUI). `nc localhost 4040` also works and is useful for
testing malformed input.

Checks worth running:

- Movement: walk the full circuit in both directions; `MOVE up` should
  give `ERR 301 NO_EXIT`.
- Presence: with two clients in the same room, one moves away and the
  other receives `EVT ROOM PRESENCE LEAVE`.
- Chat: `CHAT GLOBAL` reaches everyone, `CHAT ROOM` only players in the
  same room, `CHAT GROUP` only group members and `ERR 401 NOT_IN_GROUP`
  when ungrouped.
- Groups: `GROUP CREATE`, then a second client `GROUP JOIN grp.<name>`;
  the first receives `EVT GROUP JOIN`. `GROUP CREATE` twice gives
  `ERR 402 ALREADY_IN_GROUP`, `GROUP LEAVE` when ungrouped gives
  `ERR 401 NOT_IN_GROUP`.
- Item uniqueness: put two clients at the Frozen Well and have both send
  `TAKE Iron Bucket`. Exactly one succeeds; the other gets
  `ERR 404 ITEM_NOT_FOUND`.
- Name resolution: `TAKE item.frost_herbs` and `TAKE frost herbs` behave
  identically.
- Scenery: `TAKE Anvil` gives `ERR 405 ITEM_NOT_OBTAINABLE`.

**Combat.** In Pinewood, `ATTACK Snow Wolf` four times. Each reply shows
the wolf losing 10 HP and the player losing 8 until the wolf dies, then
`LOOK` shows it gone and the Wolf Pelt on the floor. `ATTACK Innkeeper`
gives `ERR 405 NPC_NOT_HOSTILE`. To exercise the respawn path, lower the
starting HP or raise an enemy's `damage` in the world file — with the
shipped values a player cannot lose.

**Quests.** `QUEST Shrine Keeper` to start the herb quest, collect the
herbs at Icy Creek, return and `QUEST Shrine Keeper` again: the status
becomes `completed`, `INVENTORY` no longer lists the herbs, and `STATUS`
shows the healing. Asking again offers the wolf quest. `QUESTS` lists
both with their status.

**World validation.** Change an exit target in `world/world.json` to a
room that does not exist, or the `start` field to an unknown room, and
start the server: it refuses to start and names the bad reference. Restore
the file afterwards.

**Slow client.** Open a client that never drains its socket and suspend
it, then flood the server with broadcast traffic from another connection:

```sh
nc localhost 4040 > /dev/null     # CONNECT, then Ctrl-Z to suspend
(echo "CONNECT flood"; for i in $(seq 1 20000); do echo "CHAT GLOBAL $i"; done) \
    | nc localhost 4040
```

The server logs `broadcast write failed` for the suspended client and
keeps serving everyone else.

**Protocol conformance.** Using `nc`, send a command before `CONNECT`
(`ERR 403`), an unknown verb (`ERR 400`), a second `CONNECT` on the same
connection (`ERR 402`), a duplicate name from a second connection
(`ERR 201 NAME_IN_USE`), and an empty line (ignored).

**Static checks.** `make lint` must be silent. `go run -race ./cmd/server`
with several clients checks the server's locking.

## Resources

- RFC 42TAP, the protocol specification for this project
- [RFC 2119](https://www.rfc-editor.org/rfc/rfc2119) — requirement keywords
- [RFC 5234](https://www.rfc-editor.org/rfc/rfc5234) — ABNF
- Go documentation: [`net`](https://pkg.go.dev/net),
  [`bufio`](https://pkg.go.dev/bufio),
  [`encoding/json`](https://pkg.go.dev/encoding/json),
  [`sync`](https://pkg.go.dev/sync),
  [`log/slog`](https://pkg.go.dev/log/slog)
- [Effective Go](https://go.dev/doc/effective_go) and
  [Go by Example](https://gobyexample.com/)
- [Fyne documentation](https://docs.fyne.io/) for the GUI client
- Background on MUDs and their line-oriented protocols, for the shape of
  the world and the command set

### Use of AI

AI was used to explain Go concepts
that were new to us, goroutines, `sync.Mutex`, struct tags and JSON
unmarshalling, interface embedding.
To review handlers for bugs before
testing, to suggest the shape of the world data, and to help draft this
README.