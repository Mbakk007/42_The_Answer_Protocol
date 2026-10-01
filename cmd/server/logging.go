package main

import (
	"log/slog"
	"net"
	"os"
	"strings"
	"time"
)

var logger *slog.Logger

// initLogging sets up structured JSON logging on stdout.
func initLogging() {
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

type loggedConn struct {
	net.Conn
}

func newLoggedConn(c net.Conn) *loggedConn {
	return &loggedConn{Conn: c}
}

func (l *loggedConn) Write(b []byte) (int, error) {
	line := strings.TrimRight(string(b), "\n")
	level := slog.LevelInfo
	if strings.HasPrefix(line, "ERR") {
		level = slog.LevelWarn
	}
	logger.Log(nil, level, "response",
		"addr", l.RemoteAddr().String(),
		"line", line,
	)
	return l.Conn.Write(b)
}

type floodWindow struct {
	start time.Time
	count int
}

const (
	floodWindowSize = 5 * time.Second
	floodThreshold  = 40
)

func (f *floodWindow) hit() bool {
	now := time.Now()
	if now.Sub(f.start) > floodWindowSize {
		f.start = now
		f.count = 0
	}
	f.count++
	return f.count > floodThreshold
}