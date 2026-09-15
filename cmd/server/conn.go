package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
)

var (
	mu      sync.Mutex
	clients = make(map[net.Conn]string)
)

func handleConn(conn net.Conn) {
	// defer executes when function returns
	defer conn.Close()                             // release socket when client is done
	log.Printf("connected: %s", conn.RemoteAddr()) // print clients ip and port

	mu.Lock()
	clients[conn] = ""
	mu.Unlock()

	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		parts := strings.SplitN(sc.Text(), " ", 2)
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
			fmt.Fprintf(conn, "ERR: unknown command\n")
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
	// broadcast msgs to other clients
	mu.Lock()
	defer mu.Unlock()
	log.Printf("broadcast %q to %d clients", msg, len(clients))
	for c := range clients {
		fmt.Fprintf(c, "%s\n", msg)
	}
}