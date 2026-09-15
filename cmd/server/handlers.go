package main

import (
	"fmt"
	"net"
	"strings"
	"encoding/json"
)

func handleConnect(conn net.Conn, rest string) {
	if rest == "" {
		fmt.Fprintf(conn, "ERR: missing name\n")
		return
	}
	mu.Lock()
	clients[conn] = rest
	mu.Unlock()
	fmt.Fprintf(conn, "OK: connected\n")
}

func handleChat(conn net.Conn, rest string) {
	// "GLOBAL"
	cp := strings.SplitN(rest, " ", 2)
	if len(cp) < 2 {
		fmt.Fprintf(conn, "ERR: select channel\n")
		return
	}
	scope, msg := strings.ToUpper(cp[0]), cp[1]

	mu.Lock()
	name := clients[conn]
	mu.Unlock()
	if name == "" {
		fmt.Fprintf(conn, "ERR: not connected\n")
		return
	}

	fmt.Fprintf(conn, "OK\n")
	broadcast(fmt.Sprintf("EVT %s CHAT %s %s", scope, name, msg))
}

func handleQuit(conn net.Conn, rest string) {
	fmt.Fprintf(conn, "OK: bye\n")
	conn.Close()
}

func handleWho(conn net.Conn, rest string) {
	// TODO: who currently lists every connected player. once rooms are implemented it must list only players in the caller's room.
	mu.Lock()
	names := []string{}
	for _, name := range clients {
		if name != "" { //skip clients that havent used CONNECT cmd yet
			names = append(names, name)
		}
	}
	mu.Unlock()
	count := len(names)
	roomJSON, _ := json.Marshal(names)
	fmt.Fprintf(conn, `OK { "room": %s, "server": %d }`+"\n", roomJSON, count)
}


func handleLook(conn net.Conn, rest string)      { fmt.Fprintf(conn, "ERR: not implemented\n") }
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