package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/jnsoft/jnq/src/httphelper"
)

type WorkerClient struct {
	ServerURL string
	Interval  time.Duration
	APIKey    string
}

func NewWorkerClient(serverURL string, interval time.Duration, apiKey string) *WorkerClient {
	return &WorkerClient{
		ServerURL: serverURL,
		Interval:  interval,
		APIKey:    apiKey,
	}
}

func (wc *WorkerClient) StartPoster(wg *sync.WaitGroup) {
	defer wg.Done()
	ticker := time.NewTicker(wc.Interval)
	defer ticker.Stop()

	fmt.Printf("Poster Worker started. Posting to %s every %v\n", wc.ServerURL, wc.Interval)

	for range ticker.C {
		err := wc.postToQueue()
		if err != nil {
			fmt.Printf("Error posting to queue: %v\n", err)
		} else {
			fmt.Println("Successfully posted to queue.")
		}
	}
}

func (wc *WorkerClient) StartReader(wg *sync.WaitGroup) {
	defer wg.Done()
	ticker := time.NewTicker(wc.Interval)
	defer ticker.Stop()

	fmt.Printf("Reader Worker started. Reading from %s every %v\n", wc.ServerURL, wc.Interval)

	for range ticker.C {
		err := wc.readFromQueue()
		if err != nil {
			fmt.Printf("Error reading from queue: %v\n", err)
		} else {
			fmt.Println("Successfully read from queue.")
		}
	}
}

func (wc *WorkerClient) postToQueue() error {
	item := fmt.Sprintf(`{"value": "Worker item at %s"}`, time.Now().Format(time.RFC3339))
	headers := [][2]string{
		{"Content-Type", "application/json"},
		{"Authorization", wc.APIKey},
	}

	_, code, err := httphelper.PostString(wc.ServerURL+"/enqueue", item, headers...)
	if err != nil {
		return fmt.Errorf("failed to post to queue: %w", err)
	}

	if code != http.StatusOK {
		return fmt.Errorf("unexpected response status: %d", code)
	}

	return nil
}

func (wc *WorkerClient) readFromQueue() error {
	headers := [][2]string{
		{"Authorization", wc.APIKey},
	}

	value, code, err := httphelper.GetString(wc.ServerURL+"/dequeue?channel=1", headers...)
	if err != nil {
		return fmt.Errorf("failed to read from queue: %w", err)
	}

	if code == http.StatusNoContent {
		fmt.Println("Queue is empty.")
		return nil
	}

	if code != http.StatusOK {
		return fmt.Errorf("unexpected response status: %d", code)
	}

	fmt.Printf("Read item from queue: %s\n", value)
	return nil
}

func main() {
	if len(os.Args) < 6 {
		fmt.Println("Usage: workerclient <server_url> <interval_seconds> <api_key> <num_posters> <num_readers>")
		os.Exit(1)
	}

	serverURL := os.Args[1]
	intervalSeconds, err := strconv.Atoi(os.Args[2])
	if err != nil || intervalSeconds <= 0 {
		fmt.Println("Invalid interval. Please provide a positive integer.")
		os.Exit(1)
	}
	apiKey := os.Args[3]
	numPosters, err := strconv.Atoi(os.Args[4])
	if err != nil || numPosters < 0 {
		fmt.Println("Invalid number of posters. Please provide a non-negative integer.")
		os.Exit(1)
	}
	numReaders, err := strconv.Atoi(os.Args[5])
	if err != nil || numReaders < 0 {
		fmt.Println("Invalid number of readers. Please provide a non-negative integer.")
		os.Exit(1)
	}

	worker := NewWorkerClient(serverURL, time.Duration(intervalSeconds)*time.Second, apiKey)

	var wg sync.WaitGroup

	// context with a 2-minute timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Start poster workers
	//for i := 0; i < numPosters; i++ {
	//	wg.Add(1)
	//	go worker.StartPoster(&wg)
	//}
	for i := 0; i < numPosters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(worker.Interval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					fmt.Println("Poster worker stopped due to timeout.")
					return
				case <-ticker.C:
					err := worker.postToQueue()
					if err != nil {
						fmt.Printf("Error posting to queue: %v\n", err)
					} else {
						fmt.Println("Successfully posted to queue.")
					}
				}
			}
		}()
	}

	// Start reader workers
	//for i := 0; i < numReaders; i++ {
	//	wg.Add(1)
	//	go worker.StartReader(&wg)
	//}
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(worker.Interval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					fmt.Println("Reader worker stopped due to timeout.")
					return
				case <-ticker.C:
					err := worker.readFromQueue()
					if err != nil {
						fmt.Printf("Error reading from queue: %v\n", err)
					} else {
						fmt.Println("Successfully read from queue.")
					}
				}
			}
		}()
	}

	wg.Wait()
	fmt.Println("All workers stopped.")
}
