package main

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
)

func handleConnect(conn net.Conn, rest string) {
	if rest == "" {
		fmt.Fprintf(conn, "ERR 400 BAD_REQUEST\n")
		return
	}
	mu.Lock()
	p := clients[conn]
	if p == nil {
		mu.Unlock()
		return
	}
	for _, other := range clients {
		if other.name == rest {
			mu.Unlock()
			fmt.Fprintf(conn, "ERR 201 NAME_IN_USE\n")
			return
		}
	}
	p.name = rest
	mu.Unlock()
	fmt.Fprintf(conn, "OK connected\n")
}

func handleChat(conn net.Conn, rest string) {
	cp := strings.SplitN(rest, " ", 2)
	if len(cp) < 2 {
		fmt.Fprintf(conn, "ERR 400 BAD_REQUEST\n")
		return
	}
	scope, msg := strings.ToUpper(cp[0]), cp[1]
	switch scope {
	case "GLOBAL", "ROOM", "GROUP":
	default:
		fmt.Fprintf(conn, "ERR 400 BAD_REQUEST\n")
		return
	}
	p := authPlayer(conn)
	if p == nil {
		return
	}
	name := p.name
	room := p.room
	group := p.group
	mu.Unlock()

	evt := fmt.Sprintf("EVT %s CHAT %s %s", scope, name, msg)

	switch scope {
	case "GLOBAL":
		fmt.Fprintf(conn, "OK\n")
		broadcast(evt)
	case "ROOM":
		fmt.Fprintf(conn, "OK\n")
		broadcastRoom(room, evt)
	case "GROUP":
		if group == "" {
			fmt.Fprintf(conn, "ERR 401 NOT_IN_GROUP\n")
			return
		}
		fmt.Fprintf(conn, "OK\n")
		broadcastGroup(group, evt)
	}
}

func handleQuit(conn net.Conn, rest string) {
	fmt.Fprintf(conn, "OK bye\n")
	conn.Close()
}

func handleWho(conn net.Conn, rest string) {
	// RFC 5.2.2: WHO returns only a total count, not a player list
	mu.Lock()
	count := 0
	for _, p := range clients {
		if p.name != "" { //skip clients that havent used CONNECT cmd yet
			count++
		}
	}
	mu.Unlock()
	fmt.Fprintf(conn, "OK players=%d\n", count)
}

func handleLook(conn net.Conn, rest string) {
	p := authPlayer(conn)
	if p == nil {
		return
	}
	roomID := p.room
	players := []string{}
	for _, other := range clients {
		if other.name != "" && other.room == roomID {
			players = append(players, other.name)
		}
	}
	mu.Unlock()

	loc, ok := gameWorld.Locations[roomID]
	if !ok {
		fmt.Fprintf(conn, "ERR 901 SEND_FAILED\n")
		return
	}

	exits, _ := json.Marshal(loc.Exits)
	pl, _ := json.Marshal(players)
	items, _ := json.Marshal(loc.Items)
	npcs, _ := json.Marshal(loc.NPCs)

	fmt.Fprintf(conn, `OK {"room":{"id":"%s","name":"%s","description":"%s","exits":%s},"players":%s,"items":%s,"npcs":%s}`+"\n",
		loc.ID, loc.Name, loc.Description, exits, pl, items, npcs)
}

func handleMove(conn net.Conn, rest string) {
	p := authPlayer(conn)
	if p == nil {
		return
	}
	loc, ok := gameWorld.Locations[p.room]
	if !ok {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 901 SEND_FAILED\n")
		return
	}
	dest, ok := loc.Exits[strings.ToLower(rest)]
	if !ok {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 301 NO_EXIT\n")
		return
	}
	oldroom := p.room
	oldname := p.name
	p.room = dest
	mu.Unlock()
	fmt.Fprintf(conn, "OK room=%s\n", dest)
	broadcastRoom(oldroom, "EVT ROOM PRESENCE LEAVE "+oldname)
	broadcastRoom(dest, "EVT ROOM PRESENCE ENTER "+oldname)
}

// resolveItem maps "item.frost_herbs" or "Frost Herbs" to an item id.
// Returns "" if nothing matches.
func resolveItem(s string) string {
	if _, ok := gameWorld.Items[s]; ok {
		return s
	}
	for id, item := range gameWorld.Items {
		if strings.EqualFold(item.Name, s) {
			return id
		}
	}
	return ""
}

func handleTake(conn net.Conn, rest string) {
	p := authPlayer(conn)
	if p == nil {
		return
	}
	id := resolveItem(rest)
	if id == "" {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 404 ITEM_NOT_FOUND\n")
		return
	}
	loc, ok := gameWorld.Locations[p.room]
	if !ok {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 901 SEND_FAILED\n")
		return
	}
	idx := -1
	for i, it := range loc.Items {
		if it == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 404 ITEM_NOT_FOUND\n")
		return
	}
	if !gameWorld.Items[id].Obtainable {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 405 ITEM_NOT_OBTAINABLE\n")
		return
	}
	loc.Items = append(loc.Items[:idx], loc.Items[idx+1:]...)
	p.inventory = append(p.inventory, id)
	mu.Unlock()
	fmt.Fprintf(conn, "OK taken=%s\n", id)
}

func handleDrop(conn net.Conn, rest string) {
	p := authPlayer(conn)
	if p == nil {
		return
	}
	id := resolveItem(rest)
	if id == "" {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 404 ITEM_NOT_FOUND\n")
		return
	}
	loc, ok := gameWorld.Locations[p.room]
	if !ok {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 901 SEND_FAILED\n")
		return
	}
	idx := -1
	for i, it := range p.inventory {
		if it == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 404 ITEM_NOT_IN_INVENTORY\n")
		return
	}
	p.inventory = append(p.inventory[:idx], p.inventory[idx+1:]...)
	loc.Items = append(loc.Items, id)
	mu.Unlock()
	fmt.Fprintf(conn, "OK dropped=%s\n", id)
}

func handleInventory(conn net.Conn, rest string) {
	p := authPlayer(conn)
	if p == nil {
		return
	}
	inv := p.inventory
	if inv == nil {
		inv = []string{}
	}
	b, _ := json.Marshal(inv)
	mu.Unlock()
	fmt.Fprintf(conn, "OK %s\n", b)
}

// helper func to check if player is authenticated (used CONNECT cmd)
func authPlayer(conn net.Conn) *player {
	mu.Lock()
	p := clients[conn]
	if p == nil || p.name == "" {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 403 NOT_AUTHENTICATED\n")
		return nil
	}
	return p
}

func resolveNPC(s string) string {
	if _, ok := gameWorld.NPCs[s]; ok {
		return s
	}
	for id, NPC := range gameWorld.NPCs {
		if strings.EqualFold(NPC.Name, s) {
			return id
		}
	}
	return ""
}

func handleTalk(conn net.Conn, rest string) {
	p := authPlayer(conn)
	if p == nil {
		return
	}
	loc, ok := gameWorld.Locations[p.room]
	if !ok {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 901 SEND_FAILED\n")
		return
	}
	id := resolveNPC(rest)
	if id == "" {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 404 NPC_NOT_FOUND\n")
		return
	}
	found := false
	for _, n := range loc.NPCs {
		if n == id {
			found = true
			break
		}
	}
	if !found {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 404 NPC_NOT_FOUND\n")
		return
	}
	npc := gameWorld.NPCs[id]
	if len(npc.Dialogue) == 0 {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 405 NPC_NOT_TALKATIVE\n")
		return
	}
	mu.Unlock()
	fmt.Fprintf(conn, "OK %s\n", npc.Dialogue[0])
}

func handleAttack(conn net.Conn, rest string) {
	p := authPlayer(conn)
	if p == nil {
		return
	}
	loc, ok := gameWorld.Locations[p.room]
	if !ok {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 901 SEND_FAILED\n")
		return
	}
	id := resolveNPC(rest)
	if id == "" {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 404 NPC_NOT_FOUND\n")
		return
	}
	found := false
	for _, n := range loc.NPCs {
		if n == id {
			found = true
			break
		}
	}
	if !found {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 404 NPC_NOT_FOUND\n")
		return
	}
	npc := gameWorld.NPCs[id]
	if npc.Role != "enemy" {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 405 NPC_NOT_HOSTILE\n")
		return
	}
	if npc.CurrentHP <= 0 {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 404 NPC_NOT_FOUND\n")
		return
	}

	fightRoom := p.room // p.room changes if the player dies
	npc.CurrentHP -= 10 // fixed player damage

	npcDied := npc.CurrentHP <= 0
	playerDied := false

	if npcDied {
		// remove the npc from the room
		idx := -1
		for i, n := range loc.NPCs {
			if n == id {
				idx = i
				break
			}
		}
		if idx != -1 {
			loc.NPCs = append(loc.NPCs[:idx], loc.NPCs[idx+1:]...)
		}
		loc.Items = append(loc.Items, npc.Drops...) // drops fall in the room
	} else {
		p.hp -= npc.Damage // counterattack
		if p.hp <= 0 {
			playerDied = true
			p.hp = 50
			p.room = "loc.frostmere_gate"
		}
	}

	name := p.name
	npcName := npc.Name
	respawnRoom := p.room
	playerHP := p.hp
	npcHP := npc.CurrentHP
	mu.Unlock()

	status := "combat"
	if npcDied {
		status = "victory"
	}
	if playerDied {
		status = "defeated"
	}

	fmt.Fprintf(conn, `OK {"attacker_hp": %d, "target_hp": %d, "damage": 10, "status": "%s"}`+"\n",
		playerHP, npcHP, status)
	if npcDied {
		broadcastRoom(fightRoom, fmt.Sprintf("EVT ROOM COMBAT %s killed %s", name, npcName))
	}
	if playerDied {
		broadcastRoom(fightRoom, fmt.Sprintf("EVT ROOM COMBAT %s was defeated by %s", name, npcName))
		broadcastRoom(respawnRoom, "EVT ROOM PRESENCE ENTER "+name)
	}
}

func handleStatus(conn net.Conn, rest string) {
	p := authPlayer(conn)
	if p == nil {
		return
	}
	hp := p.hp
	mu.Unlock()
	status := "healthy"
	if hp < 50 {
		status = "wounded"
	}
	maxHP := 100
	fmt.Fprintf(conn, `OK {"hp": %d, "max_hp": %d, "status": "%s"}`+"\n", hp, maxHP, status)
}

func handleGroup(conn net.Conn, rest string) {
	parts := strings.SplitN(rest, " ", 2)
	sub := strings.ToUpper(parts[0])
	arg := ""
	if len(parts) > 1 {
		arg = parts[1]
	}

	switch sub {
	case "CREATE":
		groupCreate(conn)
	case "INVITE":
		groupInvite(conn, arg)
	case "JOIN":
		groupJoin(conn, arg)
	case "LEAVE":
		groupLeave(conn)
	default:
		fmt.Fprintf(conn, "ERR 400 BAD_REQUEST\n")
	}
}

func groupCreate(conn net.Conn) {
	p := authPlayer(conn)
	if p == nil {
		return
	}
	if p.group != "" {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 402 ALREADY_IN_GROUP\n")
		return
	}
	id := "grp." + p.name
	p.group = id
	mu.Unlock()
	fmt.Fprintf(conn, "OK group=%s\n", id)
}

func groupLeave(conn net.Conn) {
	p := authPlayer(conn)
	if p == nil {
		return
	}
	if p.group == "" {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 401 NOT_IN_GROUP\n")
		return
	}
	oldGroup := p.group
	name := p.name
	p.group = ""
	mu.Unlock()
	fmt.Fprintf(conn, "OK\n")
	broadcastGroup(oldGroup, "EVT GROUP LEAVE "+name)
}

func groupInvite(conn net.Conn, arg string) {
	p := authPlayer(conn)
	if p == nil {
		return
	}
	if p.group == "" {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 401 NOT_IN_GROUP\n")
		return
	}
	// find the invited player by name
	var target net.Conn
	for c, other := range clients {
		if other.name == arg {
			target = c
			break
		}
	}
	group := p.group
	name := p.name
	mu.Unlock()

	if target == nil {
		fmt.Fprintf(conn, "ERR 404 PLAYER_NOT_FOUND\n")
		return
	}
	fmt.Fprintf(conn, "OK\n")
	fmt.Fprintf(target, "EVT GROUP INVITE %s %s\n", name, group)
}

func groupJoin(conn net.Conn, arg string) {
	p := authPlayer(conn)
	if p == nil {
		return
	}
	if p.group != "" {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 402 ALREADY_IN_GROUP\n")
		return
	}
	// the group must already have at least one member
	exists := false
	for _, other := range clients {
		if other.group == arg {
			exists = true
			break
		}
	}
	if !exists {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 401 NOT_IN_GROUP\n")
		return
	}
	p.group = arg
	name := p.name
	mu.Unlock()

	fmt.Fprintf(conn, "OK group=%s\n", arg)
	broadcastGroup(arg, "EVT GROUP JOIN "+name)
}

// Helper: hasString reports whether s is present in list.
func hasString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// Helper: removeString returns list with the first occurrence of s removed.
func removeString(list []string, s string) []string {
	for i, v := range list {
		if v == s {
			return append(list[:i], list[i+1:]...)
		}
	}
	return list // not found, unchanged
}

func handleQuest(conn net.Conn, rest string) {
	p := authPlayer(conn)
	if p == nil {
		return
	}
	loc, ok := gameWorld.Locations[p.room]
	if !ok {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 901 SEND_FAILED\n")
		return
	}
	id := resolveNPC(rest)
	if id == "" {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 404 NPC_NOT_FOUND\n")
		return
	}
	found := false
	for _, n := range loc.NPCs {
		if n == id {
			found = true
			break
		}
	}
	if !found {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 404 NPC_NOT_FOUND\n")
		return
	}

	npc := gameWorld.NPCs[id]
	if len(npc.Quests) == 0 {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 406 NO_QUEST_AVAILABLE\n")
		return
	}
	for _, qid := range npc.Quests {
		if hasString(p.completedQuests, qid) {
			continue // already done, look at the next one
		}
		q := gameWorld.Quests[qid]

		if hasString(p.activeQuests, qid) {
			// turn-in attempt: which item proves it?
			need := q.Target
			if q.Type == "kill" {
				need = q.Proof
			}
			if !hasString(p.inventory, need) {
				mu.Unlock()
				fmt.Fprintf(conn, `OK {"quest_id": "%s", "description": "%s", "reward": "%d hp", "status": "active"}`+"\n",
					qid, q.Description, q.RewardHP)
				return
			}
			// requirement met: consume the item, complete, reward
			idx := -1
			for i, it := range p.inventory {
				if it == need {
					idx = i
					break
				}
			}
			p.inventory = append(p.inventory[:idx], p.inventory[idx+1:]...)
			p.activeQuests = removeString(p.activeQuests, qid)
			p.completedQuests = append(p.completedQuests, qid)
			p.hp += q.RewardHP
			if p.hp > 100 {
				p.hp = 100
			}
			mu.Unlock()
			fmt.Fprintf(conn, `OK {"quest_id": "%s", "description": "%s", "reward": "%d hp", "status": "completed"}`+"\n",
				qid, q.Description, q.RewardHP)
			return
		}

		// not started yet
		p.activeQuests = append(p.activeQuests, qid)
		mu.Unlock()
		fmt.Fprintf(conn, `OK {"quest_id": "%s", "description": "%s", "reward": "%d hp", "status": "available"}`+"\n",
			qid, q.Description, q.RewardHP)
		return
	}

	mu.Unlock()
	fmt.Fprintf(conn, "ERR 406 NO_QUEST_AVAILABLE\n")
}

func handleQuests(conn net.Conn, rest string) {
	p := authPlayer(conn)
	if p == nil {
		return
	}
	parts := []string{}
	for _, qid := range p.activeQuests {
		parts = append(parts, fmt.Sprintf(`{"quest_id": "%s", "status": "active", "progress": "0/1"}`, qid))
	}
	for _, qid := range p.completedQuests {
		parts = append(parts, fmt.Sprintf(`{"quest_id": "%s", "status": "completed", "progress": "1/1"}`, qid))
	}
	mu.Unlock()

	fmt.Fprintf(conn, "OK [%s]\n", strings.Join(parts, ", "))
}
