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
	"github.com/jnsoft/jnq/src/server"
	"github.com/jnsoft/jnq/src/sqlpqueue"
)

const (
	MAX_PARALLELISM = 10
)

func main() {
	dbFile := flag.String("db", "queue.db", "The name of the database file")
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

	var pq mempqueue.IPriorityQueue

	if mem_queue {
		log.Println("Using in-memory queue")
		pq = mempqueue.NewMemPQueue(true)
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

/*
func runQueueOperations(no_of_items int, enqueue_endpoint, dequeue_endpoint, apikey string, verbose bool) (time.Duration, time.Duration) {
	time.Sleep(3 * time.Second)

	if verbose {
		fmt.Printf("Enqueueing %d items...\n", no_of_items)
	}

	// Enqueue items
	startEnqueue := time.Now()
	enqueueItems(no_of_items, enqueue_endpoint, apikey)
	endEnqueue := time.Now()
	enqueueDuration := endEnqueue.Sub(startEnqueue)
	fmt.Printf("Enqueueing %d items took %v\n", no_of_items, enqueueDuration)

	// Dequeue items
	dequeueDuration := dequeueItems(dequeue_endpoint, apikey, server.API_KEY_HEADER, verbose)
	fmt.Printf("Dequeuing items took %v\n", dequeueDuration)

	return enqueueDuration, dequeueDuration
}

func enqueueItems(no_of_items int, enqueue_endpoint, apikey string) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, int(math.Min(float64(no_of_items), MAX_PARALLELISM)))

	for i := 0; i < no_of_items; i++ {
		wg.Add(1)
		sem <- struct{}{} // Acquire a slot in the semaphore

		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }() // Release the slot in the semaphore

			item := fmt.Sprintf(`{"value": "Item %v", "priority": %d, "channel": %d}`, i, 1, 0)
			url := fmt.Sprintf("%s?prio=%d", enqueue_endpoint, i)
			req, err := http.NewRequest("POST", url, strings.NewReader(item))
			if err != nil {
				fmt.Printf("Failed to create request for item %d: %v\n", i, err)
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(server.API_KEY_HEADER, apikey)

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				fmt.Printf("Failed to enqueue item %d: %v\n", i, err)
				return
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				fmt.Printf("Failed to enqueue item %d: received status code %d\n", i, resp.StatusCode)
			}
		}(i)
	}

	wg.Wait()
}

func dequeueItems(dequeue_endpoint, apikey, apikey_header string, verbose bool) time.Duration {
	var wg sync.WaitGroup
	sem := make(chan struct{}, MAX_PARALLELISM)
	stop := make(chan struct{})
	noMoreItems := make(chan struct{})
	startDequeue := time.Now()

	var noContentCount int32
	var mu sync.Mutex

	worker := func() {
		defer wg.Done()
		for {
			select {
			case sem <- struct{}{}: // Acquire a slot in the semaphore
				apikey_header := [2]string{apikey_header, apikey}
				val, status, err := httphelper.GetJSON[string](dequeue_endpoint, apikey_header)

				<-sem // Release the slot in the semaphore

				if err != nil {
					fmt.Printf("Failed to dequeue item: %v\n", err)
					close(stop) // Signal all threads to stop
					return
				}

				if status == http.StatusNoContent {
					// Increment the counter for threads reporting 204
					mu.Lock()
					noContentCount++
					if noContentCount == int32(MAX_PARALLELISM) {
						close(noMoreItems) // Signal that all threads reported 204
					}
					mu.Unlock()
					return
				} else if status == http.StatusForbidden {
					fmt.Println("Dequeue failed with status 403: Forbidden. Stopping...")
					close(stop) // Signal all threads to stop
					return
				} else if status != http.StatusOK {
					fmt.Printf("Failed to dequeue item: received status code %d\n", status)
					close(stop) // Signal all threads to stop
					return
				} else {
					if verbose {
						fmt.Printf("Dequeued value: %v\n", val)
					}
				}
			case <-stop: // Stop signal received
				return
			}
		}
	}

	for i := 0; i < MAX_PARALLELISM; i++ {
		wg.Add(1)
		go worker()
	}

	select {
	case <-noMoreItems: // All threads reported 204
	case <-stop: // An error occurred
	}

	wg.Wait() // Ensure all goroutines have exited
	endDequeue := time.Now()
	return endDequeue.Sub(startDequeue)

}

*/
