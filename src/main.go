package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jnsoft/jnq/src/mempqueue"
	"github.com/jnsoft/jnq/src/priorityqueue"
	"github.com/jnsoft/jnq/src/server"
	"github.com/jnsoft/jnq/src/sqlpqueue"
)

const (
	MAX_PARALLELISM             = 10
	RESERVATION_TIMEOUT_SECONDS = 30
)

func main() {
	dbFile := flag.String("db", "queue.db", "The name of the database file")
	persistantFile := flag.String("f", "", "The name of the persistant storage files <name>.wal and <name>.sav")
	tableName := flag.String("table", "QueueItems", "The name of the table used for the queue")
	mem_queue := false
	flag.BoolVar(&mem_queue, "m", false, "Use in-memory queue")
	verbose := flag.Bool("v", false, "Enable verbose logging")
	port := flag.Int("p", 8080, "Port of the server")
	apiKey := flag.String("key", "", "API key for authentication")
	flag.Parse()

	log.SetFlags(0)
	log.SetPrefix(time.Now().Format(time.RFC3339) + " ")

	if *apiKey == "" {
		*apiKey = os.Getenv("API_KEY")
	}

	if *apiKey == "" {
		log.Fatal("API key is required (use -key or set API_KEY environment variable)")
	}

	var pq priorityqueue.IPriorityQueue

	if mem_queue {
		if *persistantFile == "" {
			log.Println("Using in-memory queue without persistence")
			pq = mempqueue.NewMemPQueue(true)
		} else {
			log.Printf("Using %s.wal/sav for persistent in-memory queue\n", *persistantFile)
			pq = mempqueue.NewMemPQueuePersistent(true, *persistantFile+".sav", *persistantFile+".wal")
		}
	} else {
		if dbFile == nil || *dbFile == "" {
			log.Fatal("Database file is required (use -db)")
		}
		log.Printf("Using SQLite queue with database file: %s and table name: %s\n", *dbFile, *tableName)
		pq = sqlpqueue.NewSqLitePQueue(*dbFile, *tableName, true)
	}

	// Set up server
	log.Printf("Starting server on port %d\n", *port)
	srv := server.NewServer(pq, *apiKey, *verbose)

	ready := make(chan struct{})
	go func() {
		err := srv.Start(":"+strconv.Itoa(*port), ready)
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v\n", err)
		}
	}()
	<-ready
	time.Sleep(1 * time.Second)
	log.Println("Server is up and running")

	reservationTimeout := RESERVATION_TIMEOUT_SECONDS * time.Second
	srv.StartRequeueTask(reservationTimeout)

	// Wait for SIGINT or SIGTERM
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	log.Println("Received shutdown signal, shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown failed: %v\n", err)
	} else {
		log.Println("Server shutdown complete")
	}
}
