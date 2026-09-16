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
	mu.Lock()
	name := clients[conn].name
	mu.Unlock()
	if name == "" {
		fmt.Fprintf(conn, "ERR 403 NOT_AUTHENTICATED\n")
		return
	}

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
	mu.Lock()
	p := clients[conn]
	if p.name == "" {
		mu.Unlock()
		fmt.Fprintf(conn, "ERR 403 NOT_AUTHENTICATED\n")
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

func handleMove(conn net.Conn, rest string)      { fmt.Fprintf(conn, "ERR: not implemented\n") }
func handleTake(conn net.Conn, rest string)      { fmt.Fprintf(conn, "ERR: not implemented\n") }
func handleDrop(conn net.Conn, rest string)      { fmt.Fprintf(conn, "ERR: not implemented\n") }
func handleInventory(conn net.Conn, rest string) { fmt.Fprintf(conn, "ERR: not implemented\n") }
func handleTalk(conn net.Conn, rest string)      { fmt.Fprintf(conn, "ERR: not implemented\n") }
func handleAttack(conn net.Conn, rest string)    { fmt.Fprintf(conn, "ERR: not implemented\n") }
func handleStatus(conn net.Conn, rest string)    { fmt.Fprintf(conn, "ERR: not implemented\n") }
func handleQuest(conn net.Conn, rest string)     { fmt.Fprintf(conn, "ERR: not implemented\n") }
func handleQuests(conn net.Conn, rest string)    { fmt.Fprintf(conn, "ERR: not implemented\n") }
func handleGroup(conn net.Conn, rest string)     { fmt.Fprintf(conn, "ERR: not implemented\n") }