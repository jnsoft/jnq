package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/jnsoft/jngo/randx"
	"github.com/jnsoft/jnq/src/httphelper"
)

var enqueuedCount int64
var mu sync.Mutex

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

/*
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
*/

func (wc *WorkerClient) getQueueSize(channel int) (int, error) {
	headers := [][2]string{
		{"X-API-Key", wc.APIKey},
	}

	url := fmt.Sprintf("%s/size?channel=%d", wc.ServerURL, channel)
	response, code, err := httphelper.GetJSON[map[string]int](url, headers...)
	if err != nil {
		return 0, fmt.Errorf("failed to query queue size: %w", err)
	}

	if code != http.StatusOK {
		return 0, fmt.Errorf("unexpected response status: %d", code)
	}

	size, ok := response["size"]
	if !ok {
		return 0, fmt.Errorf("invalid response format: missing 'size' field")
	}

	return size, nil
}

func (wc *WorkerClient) postToQueue() error {
	qId := randx.Int(1, 10000) // Random channel ID for demonstration
	item := fmt.Sprintf(`{"value": "Worker item %d at %s"}`, qId, time.Now().Format(time.RFC3339))
	headers := [][2]string{
		{"Content-Type", "application/json"},
		{"X-API-Key", wc.APIKey},
	}

	_, code, err := httphelper.PostString(wc.ServerURL+"/enqueue?channel=1", item, headers...)
	if err != nil {
		return fmt.Errorf("failed to post to queue: %w", err)
	}

	if code != http.StatusOK {
		return fmt.Errorf("unexpected response status: %d", code)
	}

	size, err := wc.getQueueSize(1) // Assuming channel 1
	if err != nil {
		return fmt.Errorf("failed to get queue size: %w", err)
	}
	fmt.Printf("Enqueued item. Current queue size: %d\n", size)

	return nil

}

func (wc *WorkerClient) readFromQueue() error {
	headers := [][2]string{
		{"X-API-Key", wc.APIKey},
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

	size, err := wc.getQueueSize(1) // Assuming channel 1
	if err != nil {
		return fmt.Errorf("failed to get queue size: %w", err)
	}
	fmt.Printf("Dequeued item: %s. Current queue size: %d\n", value, size)

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
	fmt.Println("Using API Key:", apiKey)

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
			randomDelay := time.Duration(randx.Int(0, 250)) * time.Millisecond
			ticker := time.NewTicker(worker.Interval + randomDelay)
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
						//fmt.Println("Successfully posted to queue.")
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
			randomDelay := time.Duration(randx.Int(0, 250)) * time.Millisecond
			ticker := time.NewTicker(worker.Interval + randomDelay)
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
						//fmt.Println("Successfully read from queue.")
					}
				}
			}
		}()
	}

	wg.Wait()
	fmt.Println("All workers stopped.")
}
