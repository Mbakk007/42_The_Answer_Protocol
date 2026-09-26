package main

import (
	"log"
	"net"

	"tap/world"
)

func main() {
	initLogging()

	w, err := world.Load("world/world.json") // load world data
	if err != nil {
		log.Fatal(err) // no world file
	}
	logger.Info("world loaded",
		"rooms", len(w.Locations),
		"items", len(w.Items),
		"npcs", len(w.NPCs),
		"quests", len(w.Quests))

	gameWorld = w // make the world visible to the handlers

	ln, err := net.Listen("tcp", ":4040") // bind tcp socket to port 4040, fail if port is taken
	if err != nil {
		log.Fatal(err)
	}
	logger.Info("listening", "addr", ":4040")

	for { // keep accepting new clients
		conn, err := ln.Accept() // block until a client connects
		if err != nil {
			logger.Error("accept failed", "err", err.Error())
			continue // continue loop if a connection fails
		}
		go handleConn(conn)
	}
}