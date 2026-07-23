package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
)

type Player struct {
	Name      string
	Conn      net.Conn
	Room      string
	Inventory []string
}


type Room struct {
	Name        string
	Description string
	Exits       map[string]string
	Items       []string // item IDs currently in this room
}

var world = map[string]*Room{
	"start": {
		Name:        "Village Square",
		Description: "A bustling square with cobblestone paths.",
		Exits:       map[string]string{"north": "tavern", "east": "shop"},
		Items:       []string{"herbs"},
	},
	"tavern": {
		Name:        "The Prancing Pony",
		Description: "A cozy tavern filled with warmth and laughter.",
		Exits:       map[string]string{"south": "start"},
		Items:       []string{"ale"},
	},
	"shop": {
		Name:        "General Store",
		Description: "Shelves lined with various goods and supplies.",
		Exits:       map[string]string{"west": "start"},
		Items:       []string{},
	},
}

var (
	players   = make(map[string]*Player)
	playersMu sync.Mutex // protects `players`
)

func main() {
	listener, err := net.Listen("tcp", ":5050")
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
	fmt.Println("TAP server listening on :5050")

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("accept error:", err)
			continue
		}
		go handleClient(conn)
	}
}


func removeItem(items []string, target string) ([]string, bool) {
	for i, item := range items {
		if item == target {
			return append(items[:i], items[i+1:]...), true
		}
	}
	return items, false
}

func handleClient(conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)

	var playerName string // remembers who this connection belongs to, once known

	for scanner.Scan() {
		line := scanner.Text()
		playerName = handleLine(conn, line, playerName)
	}

	// cleanup on disconnect
	if playerName != "" {
		playersMu.Lock()
		delete(players, playerName)
		playersMu.Unlock()
		fmt.Println(playerName, "disconnected")
	}
}

func broadcastToRoom(roomID string, message string, exclude string) {
	playersMu.Lock()
	defer playersMu.Unlock()

	for name, p := range players {
		if name == exclude {
			continue
		}
		if p.Room == roomID {
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

func handleLine(conn net.Conn, line string, playerName string) string {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		fmt.Fprintln(conn, "ERR empty command")
		return playerName
	}

	cmd := parts[0]

	switch cmd {
	case "CONNECT":
		if len(parts) != 2 {
			fmt.Fprintln(conn, "ERR usage: CONNECT <name>")
			return playerName
		}
		name := parts[1]

		playersMu.Lock()
		defer playersMu.Unlock()
		players[name] = &Player{Name: name, Conn: conn, Room: "start"}

		fmt.Fprintf(conn, "OK connected as %s\n", name)
		return name

	case "WHO":
		playersMu.Lock()
		defer playersMu.Unlock()
		fmt.Fprintf(conn, "OK online: %d\n", len(players))
		for n := range players {
			fmt.Fprintln(conn, "-", n)
		}
		return playerName

	case "CHAT":
		if len(parts) < 3 {
			fmt.Fprintln(conn, "ERR usage: CHAT <GLOBAL|ROOM> <message>")
			return playerName
		}
		scope := parts[1]
		message := strings.Join(parts[2:], " ") // rejoin the rest of the words into one string

		playersMu.Lock()
		p, ok := players[playerName]
		playersMu.Unlock()
		if !ok {
			fmt.Fprintln(conn, "ERR not connected")
			return playerName
		}

		fmt.Fprintln(conn, "OK")

		switch scope {
		case "GLOBAL":
			broadcastGlobal(fmt.Sprintf("EVT GLOBAL CHAT %s %s", playerName, message))
		case "ROOM":
			broadcastToRoom(p.Room, fmt.Sprintf("EVT ROOM CHAT %s %s", playerName, message), "")
		default:
			fmt.Fprintf(conn, "ERR unknown scope %s\n", scope)
		}
		return playerName
	case "TAKE":
		if len(parts) < 2 {
			fmt.Fprintln(conn, "ERR usage: TAKE <item>")
			return playerName
		}
		itemName := strings.Join(parts[1:], " ")

		playersMu.Lock()
		p, ok := players[playerName]
		if !ok {
			playersMu.Unlock()
			fmt.Fprintln(conn, "ERR not connected")
			return playerName
		}

		room := world[p.Room]
		newItems, found := removeItem(room.Items, itemName)
		if !found {
			playersMu.Unlock()
			fmt.Fprintf(conn, "ERR no such item %s\n", itemName)
			return playerName
		}

		room.Items = newItems
		p.Inventory = append(p.Inventory, itemName)
		playersMu.Unlock()

		fmt.Fprintf(conn, "OK taken=%s\n", itemName)
		return playerName

	case "LOOK":
		playersMu.Lock()
		defer playersMu.Unlock()
		p, ok := players[playerName]
		if !ok {
			fmt.Fprintln(conn, "ERR not connected")
			return playerName
		}

		room := world[p.Room]
		fmt.Fprintf(conn, "OK %s - %s\n", room.Name, room.Description)
		fmt.Fprintf(conn, "Items: %v\n", room.Items)
		fmt.Fprint(conn, "Exits:")
		for dir := range room.Exits {
			fmt.Fprintf(conn, " %s", dir)
		}
		fmt.Fprintln(conn)
		return playerName
	case "INVENTORY":
		playersMu.Lock()
		p, ok := players[playerName]
		playersMu.Unlock()
		if !ok {
			fmt.Fprintln(conn, "ERR not connected")
			return playerName
		}
		fmt.Fprintf(conn, "OK %v\n", p.Inventory)
		return playerName
	case "DROP":
		if len(parts) < 2 {
			fmt.Fprintln(conn, "ERR usage: DROP <item>")
			return playerName
		}
		itemName := strings.Join(parts[1:], " ")

		playersMu.Lock()
		p, ok := players[playerName]
		if !ok {
			playersMu.Unlock()
			fmt.Fprintln(conn, "ERR not connected")
			return playerName
		}

		newInventory, found := removeItem(p.Inventory, itemName)
		if !found {
			playersMu.Unlock()
			fmt.Fprintf(conn, "ERR you don't have %s\n", itemName)
			return playerName
		}

		p.Inventory = newInventory
		room := world[p.Room]
		room.Items = append(room.Items, itemName)
		playersMu.Unlock()

		fmt.Fprintf(conn, "OK dropped=%s\n", itemName)
		return playerName

	case "MOVE":
		if len(parts) != 2 {
			fmt.Fprintln(conn, "ERR usage: MOVE <direction>")
			return playerName
		}
		direction := parts[1]

		playersMu.Lock()
		p, ok := players[playerName]
		if !ok {
			playersMu.Unlock()
			fmt.Fprintln(conn, "ERR not connected")
			return playerName
		}

		room := world[p.Room]
		nextRoomID, ok := room.Exits[direction]
		if !ok {
			playersMu.Unlock()
			fmt.Fprintf(conn, "ERR no exit %s\n", direction)
			return playerName
		}

		oldRoomID := p.Room
		p.Room = nextRoomID
		playersMu.Unlock()

		fmt.Fprintf(conn, "OK room=%s\n", nextRoomID)

		broadcastToRoom(oldRoomID, fmt.Sprintf("EVT ROOM PRESENCE LEAVE %s", playerName), "")
		broadcastToRoom(nextRoomID, fmt.Sprintf("EVT ROOM PRESENCE ENTER %s", playerName), "")

		return playerName
	default:
		fmt.Fprintf(conn, "ERR unknown command %s\n", cmd)
		return playerName
	}
}