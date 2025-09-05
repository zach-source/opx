package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zach-source/opx/internal/backend"
	"github.com/zach-source/opx/internal/cache"
	"github.com/zach-source/opx/internal/server"
)

func main() {
	var ttlSec int
	var sock string
	var verbose bool
	var backendName string

	flag.IntVar(&ttlSec, "ttl", 120, "cache TTL seconds")
	flag.StringVar(&sock, "sock", "", "unix socket path (default: ~/.op-authd/socket.sock)")
	flag.BoolVar(&verbose, "verbose", true, "verbose logging")
	flag.StringVar(&backendName, "backend", "opcli", "backend: opcli|fake")
	flag.Parse()

	var be backend.Backend
	switch backendName {
	case "opcli":
		be = backend.OpCLI{}
	case "fake":
		be = backend.Fake{}
	default:
		log.Fatalf("unknown backend: %s", backendName)
	}

	srv := &server.Server{
		SockPath: sock,
		Backend:  be,
		Cache:    cache.New(time.Duration(ttlSec) * time.Second),
		Verbose:  verbose,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := srv.Serve(ctx); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
