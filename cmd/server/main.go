package main

import (
	"log"
	"net"
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