package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
)

func main() {
	addr := "localhost:4040"
	if len(os.Args) > 1 { // optional host:port argument
		addr = os.Args[1]
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	go readLoop(conn) // server messages arrive independently of user input

	// forward each typed line to the server
	in := bufio.NewScanner(os.Stdin)
	if err := in.Err(); err != nil {
		log.Printf("input error: %v", err)
	}
	for in.Scan() {
		fmt.Fprintf(conn, "%s\n", in.Text())
	}
}

// readLoop prints everything the server sends until the connection closes.
func readLoop(conn net.Conn) {
	sc := bufio.NewScanner(conn)
	if err := sc.Err(); err != nil {
		log.Printf("read error: %v", err)
	}
	for sc.Scan() {
		fmt.Println(sc.Text())
	}
	fmt.Println("disconnected")
	os.Exit(0)
}