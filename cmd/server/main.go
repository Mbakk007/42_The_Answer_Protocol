package main
//TODO: subject requires "broadcasts without interruption if a client disconnects mid-send".
// need to implement a queue
import (
	"bufio"
	"fmt"
	"log"
	"net"
	"sync"
)

var (
	mu	sync.Mutex
	clients = make(map[net.Conn]bool)
)

func main() {
	ln, err := net.Listen("tcp", ":4040") // bind tcp socket to port 4040, fail if port is taken
	if err != nil {
		log.Fatal(err)
	}
	log.Println("listening on :4040")

	for { // keep accepting new clients
		conn, err := ln.Accept() // block until a client connects
		if err != nil {
			log.Println("accept:", err)
			continue // continue loop if a connection fails
		}
		go handleConn(conn)
	}
}

func handleConn(conn net.Conn) {
	// defer executes when function returns
	defer conn.Close() // release socket when client is done
	log.Printf("connected: %s", conn.RemoteAddr()) // print clients ip and port

	mu.Lock()
	clients[conn] = true
	mu.Unlock()

	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		log.Printf("[%s] %q", conn.RemoteAddr(), sc.Text())
		broadcast(sc.Text())
	}
	if err := sc.Err(); err != nil {
		log.Printf("scan error: %v", err)
	}

	mu.Lock()
	delete(clients, conn)
	mu.Unlock()
	log.Printf("disconnected: %s", conn.RemoteAddr())
}

func broadcast(msg string) {
	// broadcast msgs to other clients
	mu.Lock()
	defer mu.Unlock()
	log.Printf("broadcast %q to %d clients", msg, len(clients))
	for c := range clients {
		fmt.Fprintf(c, "%s\n", msg)
	}

}
