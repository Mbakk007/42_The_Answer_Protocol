package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// ── types ─────────────────────────────────────────────────────────────────────

type Player struct {
	Name      string
	Conn      net.Conn
	Room      string
	Inventory []string
	HP        int
	MaxHP     int
	InCombat  bool
	Target    string
	Group     string
	Quests    map[string]*QuestState
	Defeated  map[string]bool
}

type QuestState struct {
	ID     string
	Status string // "inactive" | "active" | "completed"
}

type Quest struct {
	ID              string
	Title           string
	Description     string
	GiverNPC        string
	ObjectiveType   string // "fetch" | "defeat"
	ObjectiveTarget string
	Reward          string
	RewardHP        int
}

type Room struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Exits       map[string]string `json:"exits"`
	Items       []string          `json:"items"`
}

type NPC struct {
	Name     string   `json:"name"`
	Dialogue []string `json:"dialogue"`
	HP       int      `json:"hp"`
	MaxHP    int      `json:"-"`
	Room     string   `json:"room"`
	Damage   int      `json:"damage"`
	Role     string   `json:"role"` // "enemy" | "dialogue" | "quest_giver"
}

type WorldData struct {
	Rooms map[string]*Room `json:"rooms"`
	NPCs  map[string]*NPC  `json:"npcs"`
}

// ── JSON response structs (RFC 42TAP) ─────────────────────────────────────────

type RoomJSON struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Exits       map[string]string `json:"exits"`
}

type LookResponse struct {
	Room    RoomJSON `json:"room"`
	Players []string `json:"players"`
	Items   []string `json:"items"`
	NPCs    []string `json:"npcs"`
}

type StatusResponse struct {
	HP     int    `json:"hp"`
	MaxHP  int    `json:"max_hp"`
	Status string `json:"status"`
}

type AttackResponse struct {
	AttackerHP int    `json:"attacker_hp"`
	TargetHP   int    `json:"target_hp"`
	Damage     int    `json:"damage"`
	Status     string `json:"status"`
}

type QuestResponse struct {
	QuestID     string `json:"quest_id"`
	Description string `json:"description"`
	Reward      string `json:"reward"`
	Status      string `json:"status"`
}

type QuestListItem struct {
	QuestID  string `json:"quest_id"`
	Status   string `json:"status"`
	Progress string `json:"progress"`
}

// ── quest definitions ─────────────────────────────────────────────────────────

var questDefs = map[string]*Quest{
	"herb_gathering": {
		ID:              "herb_gathering",
		Title:           "Herb Gathering",
		Description:     "The elder needs herbs. Bring herbs from the forest.",
		GiverNPC:        "elder",
		ObjectiveType:   "fetch",
		ObjectiveTarget: "herbs",
		Reward:          "30 HP restored",
		RewardHP:        30,
	},
	"bandit_problem": {
		ID:              "bandit_problem",
		Title:           "Bandit Problem",
		Description:     "The guard wants you to defeat the bandit in the alley.",
		GiverNPC:        "guard",
		ObjectiveType:   "defeat",
		ObjectiveTarget: "bandit",
		Reward:          "50 HP restored",
		RewardHP:        50,
	},
}

// ── globals ───────────────────────────────────────────────────────────────────

var (
	world       map[string]*Room
	npcs        map[string]*NPC
	players     = make(map[string]*Player)
	playersMu   sync.Mutex
	npcsMu      sync.Mutex
	connTimes   = make(map[string][]time.Time)
	connTimesMu sync.Mutex
)

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	var err error
	world, npcs, err = loadWorld("world/world.json")
	if err != nil {
		slog.Error("failed to load world", "err", err)
		os.Exit(1)
	}
	slog.Info("world loaded", "rooms", len(world), "npcs", len(npcs))

	listener, err := net.Listen("tcp", ":5050")
	if err != nil {
		slog.Error("failed to listen", "err", err)
		os.Exit(1)
	}
	defer listener.Close()
	slog.Info("server started", "addr", ":5050")

	for {
		conn, err := listener.Accept()
		if err != nil {
			slog.Error("accept error", "err", err)
			continue
		}
		go handleClient(conn)
	}
}

// ── world loading ─────────────────────────────────────────────────────────────

func loadWorld(path string) (map[string]*Room, map[string]*NPC, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading world file: %w", err)
	}

	var wd WorldData
	if err := json.Unmarshal(data, &wd); err != nil {
		return nil, nil, fmt.Errorf("parsing world file: %w", err)
	}

	for roomID, room := range wd.Rooms {
		for dir, dest := range room.Exits {
			if _, ok := wd.Rooms[dest]; !ok {
				return nil, nil, fmt.Errorf("room %q exit %q points to unknown room %q", roomID, dir, dest)
			}
		}
	}

	for npcID, npc := range wd.NPCs {
		if _, ok := wd.Rooms[npc.Room]; !ok {
			return nil, nil, fmt.Errorf("npc %q placed in unknown room %q", npcID, npc.Room)
		}
		npc.MaxHP = npc.HP
	}

	return wd.Rooms, wd.NPCs, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func jsonOK(conn net.Conn, v any) {
	data, _ := json.Marshal(v)
	fmt.Fprintf(conn, "OK %s\n", data)
}

func newPlayer(name string, conn net.Conn) *Player {
	p := &Player{
		Name:      name,
		Conn:      conn,
		Room:      "start",
		HP:        100,
		MaxHP:     100,
		Inventory: []string{},
		Quests:    make(map[string]*QuestState),
		Defeated:  make(map[string]bool),
	}
	for id := range questDefs {
		p.Quests[id] = &QuestState{ID: id, Status: "inactive"}
	}
	return p
}

func removeItem(items []string, target string) ([]string, bool) {
	for i, item := range items {
		if item == target {
			return append(items[:i], items[i+1:]...), true
		}
	}
	return items, false
}

func hasItem(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func findQuestByGiver(npcID string) *Quest {
	for _, q := range questDefs {
		if q.GiverNPC == npcID {
			return q
		}
	}
	return nil
}

func checkObjective(p *Player, q *Quest) bool {
	switch q.ObjectiveType {
	case "fetch":
		return hasItem(p.Inventory, q.ObjectiveTarget)
	case "defeat":
		return p.Defeated[q.ObjectiveTarget]
	}
	return false
}

func questProgress(p *Player, q *Quest) string {
	switch q.ObjectiveType {
	case "fetch":
		if hasItem(p.Inventory, q.ObjectiveTarget) {
			return "1/1"
		}
		return "0/1"
	case "defeat":
		if p.Defeated[q.ObjectiveTarget] {
			return "1/1"
		}
		return "0/1"
	}
	return "0/1"
}

func combatStatus(p *Player) string {
	if p.HP <= 0 {
		return "dead"
	}
	if p.InCombat {
		return "combat"
	}
	if p.HP < 30 {
		return "injured"
	}
	return "healthy"
}

func respawnNPC(npcID string, delay int) {
	time.Sleep(time.Duration(delay) * time.Second)
	npcsMu.Lock()
	npc := npcs[npcID]
	npc.HP = npc.MaxHP
	npcsMu.Unlock()
	slog.Info("npc respawned", "npc", npcID, "hp", npc.HP)
}

func playerDeath(p *Player, killedBy string) {
	p.HP = 50
	p.Room = "start"
	p.InCombat = false
	p.Target = ""
	slog.Warn("player died", "player", p.Name, "killed_by", killedBy, "respawn", "start")
}

func broadcastPlayerCount() {
	playersMu.Lock()
	count := len(players)
	playersMu.Unlock()
	broadcastGlobal(fmt.Sprintf("EVT STATS players=%d", count))
}

// ── broadcast helpers ─────────────────────────────────────────────────────────

func broadcastToRoom(roomID, message, exclude string) {
	playersMu.Lock()
	defer playersMu.Unlock()
	for name, p := range players {
		if name != exclude && p.Room == roomID {
			fmt.Fprintln(p.Conn, message)
		}
	}
}

func broadcastGlobal(message string) {
	playersMu.Lock()
	defer playersMu.Unlock()
	for _, p := range players {
		fmt.Fprintln(p.Conn, message)
	}
}

func broadcastGroup(groupName, message string) {
	playersMu.Lock()
	defer playersMu.Unlock()
	for _, p := range players {
		if p.Group == groupName {
			fmt.Fprintln(p.Conn, message)
		}
	}
}

// ── client handler ────────────────────────────────────────────────────────────

func handleClient(conn net.Conn) {
	defer conn.Close()

	addr := conn.RemoteAddr().String()
	ip := strings.Split(addr, ":")[0]

	// rapid reconnect detection
	connTimesMu.Lock()
	now := time.Now()
	var recent []time.Time
	for _, t := range connTimes[ip] {
		if now.Sub(t) < 30*time.Second {
			recent = append(recent, t)
		}
	}
	recent = append(recent, now)
	connTimes[ip] = recent
	connTimesMu.Unlock()
	if len(recent) > 5 {
		slog.Warn("rapid reconnect detected", "ip", ip, "connects_in_30s", len(recent))
	}

	slog.Info("client connected", "addr", addr)

	// RFC 42TAP: server MUST send greeting on connect
	fmt.Fprintln(conn, "OK hello proto=1")

	scanner := bufio.NewScanner(conn)
	var playerName string

	// flood detection
	var cmdCount int
	windowStart := time.Now()
	const maxCmds = 20
	const windowDur = 5 * time.Second

	for scanner.Scan() {
		if time.Since(windowStart) > windowDur {
			cmdCount = 0
			windowStart = time.Now()
		}
		cmdCount++
		if cmdCount > maxCmds {
			slog.Warn("command flood detected", "player", playerName, "addr", addr, "count", cmdCount)
			fmt.Fprintln(conn, "ERR 900 CONNECTION_FAILED slow down")
			continue
		}

		line := scanner.Text()
		playerName = handleLine(conn, line, playerName)
	}

	// cleanup on disconnect
	if playerName != "" {
		playersMu.Lock()
		p, ok := players[playerName]
		if ok {
			roomID := p.Room
			delete(players, playerName)
			playersMu.Unlock()
			broadcastToRoom(roomID, fmt.Sprintf("EVT ROOM PRESENCE LEAVE %s", playerName), "")
		} else {
			playersMu.Unlock()
		}
		slog.Info("player disconnected", "player", playerName)
		broadcastPlayerCount()
	}
}

// ── command dispatcher ────────────────────────────────────────────────────────

func handleLine(conn net.Conn, line, playerName string) string {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		fmt.Fprintln(conn, "ERR 400 BAD_REQUEST empty command")
		return playerName
	}

	cmd := strings.ToUpper(parts[0])
	slog.Debug("command received", "player", playerName, "cmd", cmd)

	switch cmd {

	// ── CONNECT ──────────────────────────────────────────────────────────────
	case "CONNECT":
		if len(parts) != 2 {
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST usage: CONNECT <name>")
			return playerName
		}
		name := parts[1]

		playersMu.Lock()
		if _, exists := players[name]; exists {
			playersMu.Unlock()
			fmt.Fprintln(conn, "ERR 201 NAME_IN_USE")
			return playerName
		}
		players[name] = newPlayer(name, conn)
		playersMu.Unlock()

		slog.Info("player connected", "player", name, "addr", conn.RemoteAddr())
		fmt.Fprintln(conn, "OK connected")
		broadcastToRoom("start", fmt.Sprintf("EVT ROOM PRESENCE ENTER %s", name), name)
		broadcastPlayerCount()
		return name

	// ── LOOK ─────────────────────────────────────────────────────────────────
	case "LOOK":
		playersMu.Lock()
		p, ok := players[playerName]
		if !ok {
			playersMu.Unlock()
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST not connected")
			return playerName
		}
		roomID := p.Room
		room := world[roomID]

		var playersHere []string
		for name, other := range players {
			if other.Room == roomID && name != playerName {
				playersHere = append(playersHere, name)
			}
		}
		playersMu.Unlock()

		npcsMu.Lock()
		var npcsHere []string
		for id, npc := range npcs {
			if npc.Room == roomID {
				npcsHere = append(npcsHere, id)
			}
		}
		npcsMu.Unlock()

		items := room.Items
		if items == nil {
			items = []string{}
		}
		if playersHere == nil {
			playersHere = []string{}
		}
		if npcsHere == nil {
			npcsHere = []string{}
		}

		jsonOK(conn, LookResponse{
			Room: RoomJSON{
				ID:          roomID,
				Name:        room.Name,
				Description: room.Description,
				Exits:       room.Exits,
			},
			Players: playersHere,
			Items:   items,
			NPCs:    npcsHere,
		})
		return playerName

	// ── WHO ──────────────────────────────────────────────────────────────────
	case "WHO":
		playersMu.Lock()
		count := len(players)
		var roomCount int
		p, ok := players[playerName]
		if ok {
			for _, other := range players {
				if other.Room == p.Room {
					roomCount++
				}
			}
		}
		playersMu.Unlock()
		fmt.Fprintf(conn, "OK players=%d room=%d\n", count, roomCount)
		return playerName

	// ── CHAT ─────────────────────────────────────────────────────────────────
	case "CHAT":
		if len(parts) < 3 {
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST usage: CHAT <GLOBAL|ROOM|GROUP> <message>")
			return playerName
		}
		scope := strings.ToUpper(parts[1])
		message := strings.Join(parts[2:], " ")

		playersMu.Lock()
		p, ok := players[playerName]
		playersMu.Unlock()
		if !ok {
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST not connected")
			return playerName
		}

		fmt.Fprintln(conn, "OK")
		switch scope {
		case "GLOBAL":
			broadcastGlobal(fmt.Sprintf("EVT GLOBAL CHAT %s %s", playerName, message))
		case "ROOM":
			broadcastToRoom(p.Room, fmt.Sprintf("EVT ROOM CHAT %s %s", playerName, message), "")
		case "GROUP":
			if p.Group == "" {
				fmt.Fprintln(conn, "ERR 401 NOT_IN_GROUP")
				return playerName
			}
			broadcastGroup(p.Group, fmt.Sprintf("EVT GROUP CHAT %s %s", playerName, message))
		default:
			fmt.Fprintf(conn, "ERR 400 BAD_REQUEST unknown scope: %s\n", scope)
		}
		return playerName

	// ── MOVE ─────────────────────────────────────────────────────────────────
	case "MOVE":
		if len(parts) != 2 {
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST usage: MOVE <direction>")
			return playerName
		}
		direction := strings.ToLower(parts[1])

		playersMu.Lock()
		p, ok := players[playerName]
		if !ok {
			playersMu.Unlock()
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST not connected")
			return playerName
		}
		room := world[p.Room]
		nextRoomID, ok := room.Exits[direction]
		if !ok {
			playersMu.Unlock()
			fmt.Fprintln(conn, "ERR 301 NO_EXIT")
			return playerName
		}
		oldRoomID := p.Room
		p.Room = nextRoomID
		playersMu.Unlock()

		slog.Info("player moved", "player", playerName, "from", oldRoomID, "to", nextRoomID)
		fmt.Fprintf(conn, "OK room=%s\n", nextRoomID)
		broadcastToRoom(oldRoomID, fmt.Sprintf("EVT ROOM PRESENCE LEAVE %s", playerName), "")
		broadcastToRoom(nextRoomID, fmt.Sprintf("EVT ROOM PRESENCE ENTER %s", playerName), "")
		return playerName

	// ── TAKE ─────────────────────────────────────────────────────────────────
	case "TAKE":
		if len(parts) < 2 {
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST usage: TAKE <item>")
			return playerName
		}
		itemName := strings.Join(parts[1:], " ")

		playersMu.Lock()
		p, ok := players[playerName]
		if !ok {
			playersMu.Unlock()
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST not connected")
			return playerName
		}
		room := world[p.Room]
		newItems, found := removeItem(room.Items, itemName)
		if !found {
			playersMu.Unlock()
			fmt.Fprintln(conn, "ERR 404 ITEM_NOT_FOUND")
			return playerName
		}
		room.Items = newItems
		p.Inventory = append(p.Inventory, itemName)
		roomID := p.Room
		playersMu.Unlock()

		slog.Info("item taken", "player", playerName, "item", itemName, "room", roomID)
		fmt.Fprintf(conn, "OK taken=%s\n", itemName)
		broadcastToRoom(roomID, fmt.Sprintf("EVT ROOM ITEM TAKE %s %s", playerName, itemName), playerName)
		return playerName

	// ── DROP ─────────────────────────────────────────────────────────────────
	case "DROP":
		if len(parts) < 2 {
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST usage: DROP <item>")
			return playerName
		}
		itemName := strings.Join(parts[1:], " ")

		playersMu.Lock()
		p, ok := players[playerName]
		if !ok {
			playersMu.Unlock()
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST not connected")
			return playerName
		}
		newInv, found := removeItem(p.Inventory, itemName)
		if !found {
			playersMu.Unlock()
			fmt.Fprintln(conn, "ERR 404 ITEM_NOT_IN_INVENTORY")
			return playerName
		}
		p.Inventory = newInv
		room := world[p.Room]
		room.Items = append(room.Items, itemName)
		roomID := p.Room
		playersMu.Unlock()

		slog.Info("item dropped", "player", playerName, "item", itemName, "room", roomID)
		fmt.Fprintf(conn, "OK dropped=%s\n", itemName)
		broadcastToRoom(roomID, fmt.Sprintf("EVT ROOM ITEM DROP %s %s", playerName, itemName), playerName)
		return playerName

	// ── INVENTORY ────────────────────────────────────────────────────────────
	case "INVENTORY":
		playersMu.Lock()
		p, ok := players[playerName]
		playersMu.Unlock()
		if !ok {
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST not connected")
			return playerName
		}
		inv := p.Inventory
		if inv == nil {
			inv = []string{}
		}
		data, _ := json.Marshal(inv)
		fmt.Fprintf(conn, "OK %s\n", data)
		return playerName

	// ── TALK ─────────────────────────────────────────────────────────────────
	case "TALK":
		if len(parts) != 2 {
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST usage: TALK <npc>")
			return playerName
		}
		npcID := strings.ToLower(parts[1])

		playersMu.Lock()
		p, ok := players[playerName]
		playersMu.Unlock()
		if !ok {
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST not connected")
			return playerName
		}

		npcsMu.Lock()
		npc, ok := npcs[npcID]
		npcsMu.Unlock()
		if !ok || npc.Room != p.Room {
			fmt.Fprintln(conn, "ERR 404 NPC_NOT_FOUND")
			return playerName
		}

		fmt.Fprintf(conn, "OK %s\n", npc.Dialogue[0])
		if q := findQuestByGiver(npcID); q != nil {
			playersMu.Lock()
			qs := p.Quests[q.ID]
			playersMu.Unlock()
			if qs.Status == "inactive" {
				fmt.Fprintf(conn, "HINT use QUEST %s to accept: %s\n", npcID, q.Title)
			}
		}
		return playerName

	// ── STATUS ───────────────────────────────────────────────────────────────
	case "STATUS":
		playersMu.Lock()
		p, ok := players[playerName]
		playersMu.Unlock()
		if !ok {
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST not connected")
			return playerName
		}
		jsonOK(conn, StatusResponse{
			HP:     p.HP,
			MaxHP:  p.MaxHP,
			Status: combatStatus(p),
		})
		return playerName

	// ── ATTACK ───────────────────────────────────────────────────────────────
	case "ATTACK":
		if len(parts) != 2 {
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST usage: ATTACK <npc>")
			return playerName
		}
		npcID := strings.ToLower(parts[1])

		playersMu.Lock()
		p, ok := players[playerName]
		playersMu.Unlock()
		if !ok {
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST not connected")
			return playerName
		}

		npcsMu.Lock()
		npc, ok := npcs[npcID]
		npcsMu.Unlock()
		if !ok || npc.Room != p.Room {
			fmt.Fprintln(conn, "ERR 404 NPC_NOT_FOUND")
			return playerName
		}
		if npc.Role != "enemy" {
			fmt.Fprintln(conn, "ERR 405 NPC_NOT_HOSTILE")
			return playerName
		}
		if npc.HP <= 0 {
			fmt.Fprintln(conn, "ERR 404 NPC_NOT_FOUND")
			return playerName
		}

		playerDamage := 5 + rand.Intn(11)
		npcsMu.Lock()
		npc.HP -= playerDamage
		npcDead := npc.HP <= 0
		if npcDead {
			npc.HP = 0
		}
		npcHP := npc.HP
		npcsMu.Unlock()

		slog.Info("combat attack", "player", playerName, "npc", npcID, "damage", playerDamage, "npc_hp", npcHP)
		broadcastToRoom(p.Room, fmt.Sprintf("EVT COMBAT ATTACK %s %s %d", playerName, npcID, playerDamage), playerName)

		if npcDead {
			slog.Info("npc defeated", "player", playerName, "npc", npcID)
			broadcastToRoom(p.Room, fmt.Sprintf("EVT COMBAT DEFEAT %s %s", playerName, npcID), "")
			go respawnNPC(npcID, 30)

			playersMu.Lock()
			p.InCombat = false
			p.Target = ""
			p.Defeated[npcID] = true
			playersMu.Unlock()

			jsonOK(conn, AttackResponse{
				AttackerHP: p.HP,
				TargetHP:   0,
				Damage:     playerDamage,
				Status:     "victory",
			})
			return playerName
		}

		// npc counterattacks
		npcDamage := npc.Damage
		playersMu.Lock()
		p.HP -= npcDamage
		p.InCombat = true
		p.Target = npcID
		died := p.HP <= 0
		if died {
			playerDeath(p, npcID)
		}
		playerHP := p.HP
		playersMu.Unlock()

		slog.Info("combat counter", "npc", npcID, "player", playerName, "damage", npcDamage, "player_hp", playerHP)

		status := "combat"
		if died {
			status = "dead"
			broadcastGlobal(fmt.Sprintf("EVT COMBAT DEATH %s %s", playerName, npcID))
		}

		jsonOK(conn, AttackResponse{
			AttackerHP: playerHP,
			TargetHP:   npcHP,
			Damage:     playerDamage,
			Status:     status,
		})

		if died {
			fmt.Fprintln(conn, "OK you died and respawned at Village Square with 50 HP")
		}
		return playerName

	// ── DEFEND ───────────────────────────────────────────────────────────────
	case "DEFEND":
		if len(parts) != 2 {
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST usage: DEFEND <npc>")
			return playerName
		}
		npcID := strings.ToLower(parts[1])

		playersMu.Lock()
		p, ok := players[playerName]
		playersMu.Unlock()
		if !ok {
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST not connected")
			return playerName
		}

		npcsMu.Lock()
		npc, ok := npcs[npcID]
		npcsMu.Unlock()
		if !ok || npc.Room != p.Room || npc.Role != "enemy" || npc.HP <= 0 {
			fmt.Fprintln(conn, "ERR 404 NPC_NOT_FOUND")
			return playerName
		}

		npcDamage := npc.Damage / 2
		playersMu.Lock()
		p.HP -= npcDamage
		p.InCombat = true
		p.Target = npcID
		died := p.HP <= 0
		if died {
			playerDeath(p, npcID)
		}
		playerHP := p.HP
		playersMu.Unlock()

		slog.Info("combat defend", "player", playerName, "npc", npcID, "damage", npcDamage, "player_hp", playerHP)
		fmt.Fprintf(conn, "OK defended, took %d damage (your hp=%d)\n", npcDamage, playerHP)

		if died {
			fmt.Fprintln(conn, "OK you died and respawned at Village Square with 50 HP")
			broadcastGlobal(fmt.Sprintf("EVT COMBAT DEATH %s %s", playerName, npcID))
		}
		return playerName

	// ── FLEE ─────────────────────────────────────────────────────────────────
	case "FLEE":
		playersMu.Lock()
		p, ok := players[playerName]
		if !ok {
			playersMu.Unlock()
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST not connected")
			return playerName
		}
		target := p.Target
		playersMu.Unlock()

		if target == "" {
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST not in combat")
			return playerName
		}

		if rand.Intn(100) < 70 {
			playersMu.Lock()
			p.InCombat = false
			p.Target = ""
			playersMu.Unlock()
			slog.Info("combat flee", "player", playerName, "npc", target, "success", true)
			fmt.Fprintln(conn, "OK you flee successfully")
			return playerName
		}

		npcsMu.Lock()
		npc := npcs[target]
		npcsMu.Unlock()

		npcDamage := npc.Damage
		playersMu.Lock()
		p.HP -= npcDamage
		died := p.HP <= 0
		if died {
			playerDeath(p, target)
		}
		playerHP := p.HP
		playersMu.Unlock()

		slog.Info("combat flee", "player", playerName, "npc", target, "success", false, "damage", npcDamage, "player_hp", playerHP)
		fmt.Fprintf(conn, "OK flee failed, %s hits you for %d (your hp=%d)\n", npc.Name, npcDamage, playerHP)

		if died {
			fmt.Fprintln(conn, "OK you died and respawned at Village Square with 50 HP")
			broadcastGlobal(fmt.Sprintf("EVT COMBAT DEATH %s %s", playerName, target))
		}
		return playerName

	// ── QUEST ────────────────────────────────────────────────────────────────
	case "QUEST":
		if len(parts) != 2 {
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST usage: QUEST <npc>")
			return playerName
		}
		npcID := strings.ToLower(parts[1])

		playersMu.Lock()
		p, ok := players[playerName]
		playersMu.Unlock()
		if !ok {
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST not connected")
			return playerName
		}

		q := findQuestByGiver(npcID)
		if q == nil {
			fmt.Fprintln(conn, "ERR 406 NO_QUEST_AVAILABLE")
			return playerName
		}

		npcsMu.Lock()
		npc, ok := npcs[npcID]
		npcsMu.Unlock()
		if !ok || npc.Room != p.Room {
			fmt.Fprintln(conn, "ERR 404 NPC_NOT_FOUND")
			return playerName
		}

		playersMu.Lock()
		qs := p.Quests[q.ID]
		switch qs.Status {
		case "inactive":
			qs.Status = "active"
			playersMu.Unlock()
			slog.Info("quest accepted", "player", playerName, "quest", q.ID)
			jsonOK(conn, QuestResponse{
				QuestID:     q.ID,
				Description: q.Description,
				Reward:      q.Reward,
				Status:      "active",
			})

		case "active":
			done := checkObjective(p, q)
			if !done {
				playersMu.Unlock()
				jsonOK(conn, QuestResponse{
					QuestID:     q.ID,
					Description: q.Description,
					Reward:      q.Reward,
					Status:      "in_progress",
				})
				return playerName
			}
			qs.Status = "completed"
			oldHP := p.HP
			p.HP += q.RewardHP
			if p.HP > p.MaxHP {
				p.HP = p.MaxHP
			}
			playersMu.Unlock()
			slog.Info("quest completed", "player", playerName, "quest", q.ID, "reward_hp", q.RewardHP)
			fmt.Fprintf(conn, "OK quest completed: %s reward: +%d HP (%d -> %d)\n", q.Title, q.RewardHP, oldHP, p.HP)
			broadcastGlobal(fmt.Sprintf("EVT QUEST COMPLETE %s %s", playerName, q.Title))

		case "completed":
			playersMu.Unlock()
			fmt.Fprintln(conn, "ERR 406 NO_QUEST_AVAILABLE")
		}
		return playerName

	// ── QUESTS ───────────────────────────────────────────────────────────────
	case "QUESTS":
		playersMu.Lock()
		p, ok := players[playerName]
		if !ok {
			playersMu.Unlock()
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST not connected")
			return playerName
		}
		var list []QuestListItem
		for id, q := range questDefs {
			qs := p.Quests[id]
			list = append(list, QuestListItem{
				QuestID:  id,
				Status:   qs.Status,
				Progress: questProgress(p, q),
			})
		}
		playersMu.Unlock()
		data, _ := json.Marshal(list)
		fmt.Fprintf(conn, "OK %s\n", data)
		return playerName

	// ── GROUP ────────────────────────────────────────────────────────────────
	case "GROUP":
		if len(parts) < 2 {
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST usage: GROUP <CREATE|JOIN|INVITE|LEAVE|WHO|CHAT>")
			return playerName
		}

		playersMu.Lock()
		p, ok := players[playerName]
		playersMu.Unlock()
		if !ok {
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST not connected")
			return playerName
		}

		switch strings.ToUpper(parts[1]) {
		case "CREATE":
			if len(parts) < 3 {
				fmt.Fprintln(conn, "ERR 400 BAD_REQUEST usage: GROUP CREATE <name>")
				return playerName
			}
			if p.Group != "" {
				fmt.Fprintln(conn, "ERR 402 ALREADY_IN_GROUP")
				return playerName
			}
			groupName := parts[2]
			playersMu.Lock()
			p.Group = groupName
			playersMu.Unlock()
			fmt.Fprintf(conn, "OK group=%s\n", groupName)

		case "JOIN":
			if len(parts) < 3 {
				fmt.Fprintln(conn, "ERR 400 BAD_REQUEST usage: GROUP JOIN <name>")
				return playerName
			}
			if p.Group != "" {
				fmt.Fprintln(conn, "ERR 402 ALREADY_IN_GROUP")
				return playerName
			}
			groupName := parts[2]
			playersMu.Lock()
			p.Group = groupName
			playersMu.Unlock()
			fmt.Fprintf(conn, "OK group=%s\n", groupName)
			broadcastGroup(groupName, fmt.Sprintf("EVT GROUP JOIN %s", playerName))

		case "INVITE":
			if len(parts) < 3 {
				fmt.Fprintln(conn, "ERR 400 BAD_REQUEST usage: GROUP INVITE <player>")
				return playerName
			}
			if p.Group == "" {
				fmt.Fprintln(conn, "ERR 401 NOT_IN_GROUP")
				return playerName
			}
			target := parts[2]
			playersMu.Lock()
			tp, ok := players[target]
			playersMu.Unlock()
			if !ok {
				fmt.Fprintf(conn, "ERR 404 PLAYER_NOT_FOUND\n")
				return playerName
			}
			fmt.Fprintln(tp.Conn, fmt.Sprintf("EVT GROUP INVITE %s %s", playerName, p.Group))
			fmt.Fprintln(conn, "OK")

		case "LEAVE":
			if p.Group == "" {
				fmt.Fprintln(conn, "ERR 401 NOT_IN_GROUP")
				return playerName
			}
			oldGroup := p.Group
			playersMu.Lock()
			p.Group = ""
			playersMu.Unlock()
			broadcastGroup(oldGroup, fmt.Sprintf("EVT GROUP LEAVE %s", playerName))
			fmt.Fprintln(conn, "OK")

		case "WHO":
			if p.Group == "" {
				fmt.Fprintln(conn, "ERR 401 NOT_IN_GROUP")
				return playerName
			}
			playersMu.Lock()
			fmt.Fprintln(conn, "OK group members:")
			for _, other := range players {
				if other.Group == p.Group {
					fmt.Fprintln(conn, "-", other.Name)
				}
			}
			playersMu.Unlock()

		case "CHAT":
			if len(parts) < 3 {
				fmt.Fprintln(conn, "ERR 400 BAD_REQUEST usage: GROUP CHAT <message>")
				return playerName
			}
			if p.Group == "" {
				fmt.Fprintln(conn, "ERR 401 NOT_IN_GROUP")
				return playerName
			}
			msg := strings.Join(parts[2:], " ")
			broadcastGroup(p.Group, fmt.Sprintf("EVT GROUP CHAT %s %s", playerName, msg))
			fmt.Fprintln(conn, "OK")

		default:
			fmt.Fprintf(conn, "ERR 400 BAD_REQUEST unknown GROUP subcommand: %s\n", parts[1])
		}
		return playerName

	// ── QUIT ─────────────────────────────────────────────────────────────────
	case "QUIT":
		playersMu.Lock()
		p, ok := players[playerName]
		if !ok {
			playersMu.Unlock()
			fmt.Fprintln(conn, "ERR 400 BAD_REQUEST not connected")
			return playerName
		}
		roomID := p.Room
		delete(players, playerName)
		playersMu.Unlock()

		slog.Info("player quit", "player", playerName)
		fmt.Fprintln(conn, "OK bye")
		broadcastToRoom(roomID, fmt.Sprintf("EVT ROOM PRESENCE LEAVE %s", playerName), "")
		broadcastPlayerCount()
		conn.Close()
		return ""

	// ── DEFAULT ───────────────────────────────────────────────────────────────
	default:
		fmt.Fprintf(conn, "ERR 400 BAD_REQUEST unknown command: %s\n", cmd)
		return playerName
	}
}