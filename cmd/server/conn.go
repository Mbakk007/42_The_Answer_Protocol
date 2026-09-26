package main

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"

	"tap/world"
)

// TODO: move player + clients + mu into internal/game

type player struct {
	name            string // "" until CONNECT
	room            string // current location id
	hp              int
	group           string // "" when not in a group
	inventory       []string
	activeQuests    []string
	completedQuests []string
}

var (
	mu        sync.Mutex
	clients   = make(map[net.Conn]*player)
	gameWorld *world.World
)

func handleConn(raw net.Conn) {
	conn := newLoggedConn(raw) // every outbound line is logged
	// defer executes when function returns
	defer conn.Close() // release socket when client is done
	addr := conn.RemoteAddr().String()
	logger.Info("client connected", "addr", addr)

	mu.Lock()
	clients[conn] = &player{room: "loc.frostmere_gate", hp: 100} // TODO: starting room hardcoded
	mu.Unlock()

	fmt.Fprintf(conn, "OK hello proto=1\n") // RFC 3.2

	var flood floodWindow

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

		mu.Lock()
		pname := ""
		if p := clients[conn]; p != nil {
			pname = p.name
		}
		mu.Unlock()

		logger.Info("command", "addr", addr, "player", pname, "verb", verb, "args", rest)
		if flood.hit() {
			logger.Warn("command flooding", "addr", addr, "player", pname)
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
		logger.Error("scan error", "addr", addr, "err", err.Error())
	}

	mu.Lock()
	name := ""
	if p := clients[conn]; p != nil {
		name = p.name
	}
	delete(clients, conn)
	mu.Unlock()
	logger.Info("client disconnected", "addr", addr, "player", name)
}

// TODO: subject requires "broadcasts without interruption if a client disconnects mid-send".
// need to implement a queue
func broadcast(msg string) {
	// broadcast msgs to other clients
	mu.Lock()
	defer mu.Unlock()
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

func broadcastGroup(groupID string, msg string) {
	// broadcast msgs to a specific group
	if groupID == "" {
		return // "" means no group; would hit every ungrouped player
	}
	mu.Lock()
	defer mu.Unlock()
	for c, p := range clients {
		if p.group == groupID {
			fmt.Fprintf(c, "%s\n", msg)
		}
	}
}