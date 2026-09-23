package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"

	"tap/internal/world"
)

// TODO: move player + clients + mu into internal/game

type player struct {
	name string // "" until CONNECT
	room string // current location id
	hp   int
	inventory []string
}

var (
	mu        sync.Mutex
	clients   = make(map[net.Conn]*player)
	gameWorld *world.World
)

func handleConn(conn net.Conn) {
	// defer executes when function returns
	defer conn.Close()                             // release socket when client is done
	log.Printf("connected: %s", conn.RemoteAddr()) // print clients ip and port

	mu.Lock()
	clients[conn] = &player{room: "loc.frostmere_gate", hp: 100} // TODO: starting room still hardcoded fix later
	mu.Unlock()

	fmt.Fprintf(conn, "OK hello proto=1\n") // RFC 3.2

	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text()) // also strips \r from CRLF clients
		if line == "" {
			continue // ignore empty lines
		}
		parts := strings.SplitN(line, " ", 2)
		verb := strings.ToUpper(parts[0])
		rest := ""
		if len(parts) > 1 {
			rest = parts[1]
		}

		switch verb {
		case "CONNECT":
			handleConnect(conn, rest)
		case "LOOK":
			handleLook(conn, rest)
		case "MOVE":
			handleMove(conn, rest)
		case "CHAT":
			handleChat(conn, rest)
		case "TAKE":
			handleTake(conn, rest)
		case "DROP":
			handleDrop(conn, rest)
		case "INVENTORY":
			handleInventory(conn, rest)
		case "TALK":
			handleTalk(conn, rest)
		case "ATTACK":
			handleAttack(conn, rest)
		case "STATUS":
			handleStatus(conn, rest)
		case "QUEST":
			handleQuest(conn, rest)
		case "QUESTS":
			handleQuests(conn, rest)
		case "WHO":
			handleWho(conn, rest)
		case "GROUP":
			handleGroup(conn, rest)
		case "QUIT":
			handleQuit(conn, rest)
		default:
			fmt.Fprintf(conn, "ERR 400 UNKNOWN_COMMAND\n")
		}
	}
	if err := sc.Err(); err != nil {
		log.Printf("scan error: %v", err)
	}

	mu.Lock()
	delete(clients, conn)
	mu.Unlock()
	log.Printf("disconnected: %s", conn.RemoteAddr())
}

// TODO: subject requires "broadcasts without interruption if a client disconnects mid-send".
// need to implement a queue
func broadcast(msg string) {
	// broadcast msgs to other clients (all rooms)
	mu.Lock()
	defer mu.Unlock()
	log.Printf("broadcast %q to %d clients", msg, len(clients))
	for c := range clients {
		fmt.Fprintf(c, "%s\n", msg)
	}
}

func broadcastRoom(roomID string, msg string) {
	// broadcast msgs to a specific room
	mu.Lock()
	defer mu.Unlock()
	for c, p := range clients {
		if p.room == roomID {
			fmt.Fprintf(c, "%s\n", msg)
		}
	}
}