package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

// CLI client for TAP (RFC 42TAP compliant).
// Two goroutines run concurrently:
//   - one reads server events and prints them
//   - one reads stdin and sends commands to the server
//
// Usage:
//
//	./client-cli [host:port]   default: localhost:5050

func main() {
	addr := "localhost:5050"
	if len(os.Args) == 2 {
		addr = os.Args[1]
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to %s: %v\n", addr, err)
		os.Exit(1)
	}
	defer conn.Close()

	fmt.Printf("connected to TAP server at %s\n", addr)
	fmt.Println("commands: CONNECT LOOK MOVE CHAT TAKE DROP INVENTORY TALK ATTACK DEFEND FLEE STATUS QUEST QUESTS WHO GROUP QUIT")
	fmt.Println("---")

	// channel closed when server disconnects
	done := make(chan struct{})

	// goroutine: read from server, print to stdout
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			fmt.Println("<", scanner.Text())
		}
		fmt.Println("--- server disconnected ---")
	}()

	// main: read stdin, send to server
	stdin := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")

		// check if server disconnected before blocking on stdin
		select {
		case <-done:
			return
		default:
		}

		if !stdin.Scan() {
			break
		}

		line := strings.TrimSpace(stdin.Text())
		if line == "" {
			continue
		}

		fmt.Fprintf(conn, "%s\n", line)

		// exit after QUIT
		fields := strings.Fields(line)
		if len(fields) > 0 && strings.ToUpper(fields[0]) == "QUIT" {
			<-done
			return
		}
	}

	<-done
}