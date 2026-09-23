package main

import (
	"fmt"
	"net"
	"strings"
	"encoding/json"
)

func handleConnect(conn net.Conn, rest string) {
	if rest == "" {
		fmt.Fprintf(conn, "ERR 400 BAD_REQUEST\n")
		return
	}
	mu.Lock()
	for _, p := range clients {
		if p.name == rest {
			mu.Unlock()
			fmt.Fprintf(conn, "ERR 201 NAME_IN_USE\n")
			return
		}
	}
	clients[conn].name = rest
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
	case "GLOBAL", "ROOM", "GROUP": //TODO:implement cases
	default:
		fmt.Fprintf(conn, "ERR 400 BAD_REQUEST\n")
		return
	}
	p := authPlayer(conn)
	if p == nil {
		return
	}
	name := p.name
	mu.Unlock()

	fmt.Fprintf(conn, "OK\n")
	broadcast(fmt.Sprintf("EVT %s CHAT %s %s", scope, name, msg))
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

func handleTalk(conn net.Conn, rest string)      { fmt.Fprintf(conn, "ERR: not implemented\n") }
func handleAttack(conn net.Conn, rest string)    { fmt.Fprintf(conn, "ERR: not implemented\n") }
func handleStatus(conn net.Conn, rest string)    { fmt.Fprintf(conn, "ERR: not implemented\n") }
func handleQuest(conn net.Conn, rest string)     { fmt.Fprintf(conn, "ERR: not implemented\n") }
func handleQuests(conn net.Conn, rest string)    { fmt.Fprintf(conn, "ERR: not implemented\n") }
func handleGroup(conn net.Conn, rest string)     { fmt.Fprintf(conn, "ERR: not implemented\n") }