# TAP (The Answer Protocol) — Go Roadmap

Solo build, learning Go along the way. Check items off as you go.

## Phase 0 — Go Fundamentals (1–1.5 weeks)
- [ ] Go tour: syntax, structs, interfaces, error handling, slices/maps, pointers
- [ ] Goroutines, channels, `select`, `sync.Mutex`, `sync.WaitGroup`
- [ ] Mini exercise: worker pool
- [ ] Mini exercise: TCP echo server (goroutine per connection)
- [ ] Mini exercise: JSON marshal/unmarshal with structs
- **Exit criteria:** write a goroutine-per-connection TCP echo server without looking up syntax

## Phase 1 — Protocol + Server Skeleton (1 week)
- [ ] Finalize message format on top of RFC 42TAP
- [ ] Accept loop: goroutine per client
- [ ] "Hub" goroutine owning world state, driven by channels (avoids mutex hell)
- [ ] Implement: CONNECT, LOOK, CHAT, MOVE, QUIT
- **Exit criteria:** two `nc`/`telnet` sessions can chat globally and move between two hardcoded rooms with correct broadcasts

## Phase 2 — World Data + Items (0.5–1 week)
- [ ] Define JSON world schema (rooms, exits, items, NPCs)
- [ ] Loader with `encoding/json`, validate exits/references on load
- [ ] TAKE / DROP / INVENTORY with instance tracking (no duplication)
- [ ] Build real world: 8+ rooms, loop + branch, 4+ items (2+ obtainable)

## Phase 3 — NPCs, Combat, Quests (1.5–2 weeks) ⚠️ biggest design chunk
- [ ] NPC roles: dialogue, quest-giver, enemy + TALK command
- [ ] Combat: HP, ATTACK/STATUS, damage formula, turn order, DEFEND/FLEE, respawn
- [ ] Quests: fetch/defeat/deliver, progress tracking, completion + rewards
- [ ] Write README notes on design decisions as you go

## Phase 4 — Logging (0.5 week)
- [ ] Structured JSON logging (`log/slog`): connections, commands, responses, world changes, quest events
- [ ] Abuse-pattern detection: command flooding, rapid reconnects
- [ ] Log levels: INFO / WARN / ERROR

## Phase 5 — CLI Client (0.5 week)
- [ ] Non-blocking read from stdin + socket (goroutines + channel merge)
- [ ] Decide: raw protocol passthrough vs. friendlier translated commands (document choice)

## Phase 6 — GUI Client (2–2.5 weeks) ⚠️ most likely to overrun
- [ ] Learn Fyne basics in isolation (2-3 days) before wiring to server
- [ ] Room view with live updates
- [ ] Chat panes: Global / Room / Group, separated from log view
- [ ] Inventory + TAKE/DROP buttons
- [ ] Action buttons: LOOK, MOVE, TAKE, DROP, TALK, ATTACK, STATUS, QUEST, QUESTS, WHO, GROUP, QUIT
- [ ] Player-in-room / server player counters
- [ ] NPC dialogue display

## Phase 7 — Integration, Testing, README (1 week)
- [ ] Multi-client stress test (combat, quests, disconnects, concurrent clients)
- [ ] Test disconnect-mid-broadcast handling explicitly
- [ ] Full README: Description, Instructions, Resources (+ AI usage disclosure)
- [ ] Required sections: Architecture, Protocol Implementation, Combat System, Quest System, World Design, Server Logging, Group Contributions*, Building and Running, Testing
- [ ] `go vet` / `golangci-lint`, cleanup
- [ ] Makefile or documented `go run`/`go build` targets: install, run-server, run-client, run-client-gui, lint, clean

*Note: confirm with your evaluator whether solo submission is acceptable for a project spec'd for 2–3 people.

---

**Total estimate:** ~8–10 weeks at 15–20 hrs/week. Protect Phase 3 and Phase 6 from schedule slip — everything else compresses more predictably.
