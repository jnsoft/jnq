package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jnsoft/jnq/src/httphelper"
	"github.com/jnsoft/jnq/src/mempqueue"
	"github.com/jnsoft/jnq/src/server"
	"github.com/jnsoft/jnq/src/sqlpqueue"
	. "github.com/jnsoft/jnq/src/testhelper"
)

func TestJnqEndToEnd(t *testing.T) {

	const (
		PORT             = 8080
		PRIO             = 0.1
		CHANNEL          = 1
		API_KEY          = "api-key"
		API_BASE_URL     = "http://localhost"
		ENQUEUE_ENDPOINT = "/enqueue"
		DEQUEUE_ENDPOINT = "/dequeue"
	)

	t.Run("End to end test in memory", func(t *testing.T) {

		pq := mempqueue.NewMemPQueue(true)

		// Start the server
		srv := server.NewServer(pq, API_KEY, true)
		ready := make(chan struct{})
		go func() {
			err := srv.Start(":"+strconv.Itoa(PORT), ready)
			if err != nil && err != http.ErrServerClosed {
				fmt.Printf("Failed to start server: %v\n", err)
			}
		}()

		<-ready
		time.Sleep(1 * time.Second)

		// Dequeue an item from empty queue
		url := fmt.Sprintf("%s:%d%s?channel=%d", API_BASE_URL, PORT, DEQUEUE_ENDPOINT, CHANNEL)
		_, code, err := httphelper.GetString(url, [2]string{server.API_KEY_HEADER, API_KEY})
		AssertNoError(t, err)
		AssertEqual(t, code, http.StatusNoContent)

		item := getItem(42)

		// Enqueue an item without API key
		url = fmt.Sprintf("%s:%d%s?prio=%f&channel=%d&notbefore=%s", API_BASE_URL, PORT, ENQUEUE_ENDPOINT, PRIO, CHANNEL, time.Now().Format(time.RFC3339))
		_, code, err = httphelper.PostString(url, item)
		AssertNoError(t, err)
		AssertEqual(t, code, http.StatusForbidden)

		// Enqueue an item
		url = fmt.Sprintf("%s:%d%s?prio=%f&channel=%d&notbefore=%s", API_BASE_URL, PORT, ENQUEUE_ENDPOINT, PRIO, CHANNEL, time.Now().Format(time.RFC3339))
		_, code, err = httphelper.PostString(url, item, [2]string{server.API_KEY_HEADER, API_KEY})
		AssertNoError(t, err)
		AssertEqual(t, code, http.StatusOK)

		time.Sleep(500 * time.Millisecond)

		// Dequeue the item
		url = fmt.Sprintf("%s:%d%s?channel=%d", API_BASE_URL, PORT, DEQUEUE_ENDPOINT, CHANNEL)
		value, code, err := httphelper.GetString(url, [2]string{server.API_KEY_HEADER, API_KEY})
		AssertNoError(t, err)
		AssertEqual(t, code, http.StatusOK)

		// Normalize JSON strings for comparison
		var expected, actual map[string]any
		err = json.Unmarshal([]byte(item), &expected)
		AssertNoError(t, err)
		err = json.Unmarshal([]byte(value), &actual)
		AssertNoError(t, err)

		AssertEqual(t, expected["value"], actual["value"])

		// Dequeue an item from empty queue
		url = fmt.Sprintf("%s:%d%s?channel=%d", API_BASE_URL, PORT, DEQUEUE_ENDPOINT, CHANNEL)
		_, code, err = httphelper.GetString(url, [2]string{server.API_KEY_HEADER, API_KEY})
		AssertNoError(t, err)
		AssertEqual(t, code, 204)

		// shutdown the server
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		err = srv.Shutdown(ctx)
		AssertNoError(t, err)
	})

	t.Run("End to end test db", func(t *testing.T) {

		tempFile, err := getTempFile()
		AssertNoError(t, err)
		defer os.Remove(tempFile.Name())

		pq := sqlpqueue.NewSqLitePQueue(tempFile.Name(), "test", true)

		// Start the server
		srv := server.NewServer(pq, API_KEY, true)
		ready := make(chan struct{})
		go func() {
			err := srv.Start(":"+strconv.Itoa(PORT+2), ready)
			if err != nil && err != http.ErrServerClosed {
				fmt.Printf("Failed to start server: %v\n", err)
			}
		}()

		<-ready
		time.Sleep(1 * time.Second)

		// Dequeue an item from empty queue
		url := fmt.Sprintf("%s:%d%s?channel=%d", API_BASE_URL, PORT+2, DEQUEUE_ENDPOINT, CHANNEL)
		_, code, err := httphelper.GetString(url, [2]string{server.API_KEY_HEADER, API_KEY})
		AssertNoError(t, err)
		AssertEqual(t, code, http.StatusNoContent)

		item := getItem(42)

		// Enqueue an item without API key
		url = fmt.Sprintf("%s:%d%s?prio=%f&channel=%d&notbefore=%s", API_BASE_URL, PORT+2, ENQUEUE_ENDPOINT, PRIO, CHANNEL, time.Now().Format(time.RFC3339))
		_, code, err = httphelper.PostString(url, item)
		AssertNoError(t, err)
		AssertEqual(t, code, http.StatusForbidden)

		// Enqueue an item
		url = fmt.Sprintf("%s:%d%s?prio=%f&channel=%d&notbefore=%s", API_BASE_URL, PORT+2, ENQUEUE_ENDPOINT, PRIO, CHANNEL, time.Now().Format(time.RFC3339))
		_, code, err = httphelper.PostString(url, item, [2]string{server.API_KEY_HEADER, API_KEY})
		AssertNoError(t, err)
		AssertEqual(t, code, http.StatusOK)

		time.Sleep(500 * time.Millisecond)

		// Dequeue the item
		url = fmt.Sprintf("%s:%d%s?channel=%d", API_BASE_URL, PORT+2, DEQUEUE_ENDPOINT, CHANNEL)
		value, code, err := httphelper.GetString(url, [2]string{server.API_KEY_HEADER, API_KEY})
		AssertNoError(t, err)
		AssertEqual(t, code, http.StatusOK)

		// Normalize JSON strings for comparison
		var expected, actual map[string]any
		err = json.Unmarshal([]byte(item), &expected)
		AssertNoError(t, err)
		err = json.Unmarshal([]byte(value), &actual)
		AssertNoError(t, err)

		AssertEqual(t, expected["value"], actual["value"])

		// Dequeue an item from empty queue
		url = fmt.Sprintf("%s:%d%s?channel=%d", API_BASE_URL, PORT+2, DEQUEUE_ENDPOINT, CHANNEL)
		_, code, err = httphelper.GetString(url, [2]string{server.API_KEY_HEADER, API_KEY})
		AssertNoError(t, err)
		AssertEqual(t, code, 204)

		// shutdown the server
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		err = srv.Shutdown(ctx)
		AssertNoError(t, err)
	})

	t.Run("Performance test", func(t *testing.T) {
		const NO_OF_ITEMS = 100
		const MAX_PARALLELISM = 20
		const VERBOSE = false

		tempFile, err := getTempFile()
		AssertNoError(t, err)
		defer os.Remove(tempFile.Name())

		pq := sqlpqueue.NewSqLitePQueue(tempFile.Name(), "test", VERBOSE)
		//pq := mempqueue.NewMemPQueue(true)

		srv := server.NewServer(pq, API_KEY, VERBOSE)
		ready := make(chan struct{})
		go func() {
			err := srv.Start(":"+strconv.Itoa(PORT+1), ready)
			if err != nil && err != http.ErrServerClosed {
				fmt.Printf("Failed to start server: %v\n", err)
			}
		}()

		<-ready
		time.Sleep(1 * time.Second)

		startTime := time.Now()
		enqueueURL := fmt.Sprintf("%s:%d%s", API_BASE_URL, PORT+1, ENQUEUE_ENDPOINT)
		dequeueURL := fmt.Sprintf("%s:%d%s", API_BASE_URL, PORT+1, DEQUEUE_ENDPOINT)

		runQueueOperations(NO_OF_ITEMS, enqueueURL, dequeueURL, API_KEY, MAX_PARALLELISM)

		duration := time.Since(startTime)
		t.Logf("Performance test completed: %d items processed in %v", NO_OF_ITEMS, duration)
		fmt.Printf("Performance test completed: %d items processed in %v\n", NO_OF_ITEMS, duration)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		err = srv.Shutdown(ctx)
		AssertNoError(t, err)
	})
}

func runQueueOperations(noOfItems int, enqueueURL, dequeueURL, apiKey string, max_parallel int) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, max_parallel)

	// Enqueue items
	for i := 0; i < noOfItems; i++ {
		wg.Add(1)
		sem <- struct{}{} // Acquire a slot in the semaphore
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }() // Release the slot in the semaphore
			item := getItem(i)
			_, code, err := httphelper.PostString(
				fmt.Sprintf("%s?prio=0.1&channel=1&notbefore=%s", enqueueURL, time.Now().Format(time.RFC3339)),
				item,
				[2]string{server.API_KEY_HEADER, apiKey},
			)
			if err != nil || code != http.StatusOK {
				panic(fmt.Sprintf("Failed to enqueue item %d: %v (HTTP %d)", i, err, code))
			}
			if i%100 == 0 {
				fmt.Printf("Enqueued %d items\n", i)
			}
		}(i)
	}
	wg.Wait()

	// Dequeue items
	for i := 0; i < noOfItems; i++ {
		wg.Add(1)
		sem <- struct{}{} // Acquire a slot
		go func(itemID int) {
			defer wg.Done()
			defer func() { <-sem }() // Release the slot

			_, code, err := httphelper.GetString(
				fmt.Sprintf("%s?channel=1", dequeueURL),
				[2]string{server.API_KEY_HEADER, apiKey},
			)
			if err != nil || code != http.StatusOK {
				panic(fmt.Sprintf("Failed to dequeue item %d: %v (HTTP %d)", itemID, err, code))
			}
			if itemID%100 == 0 {
				fmt.Printf("Dequeued %d items\n", itemID)
			}
		}(i)
	}
	wg.Wait()
}

func getItem(itemId int) string {
	return fmt.Sprintf(`{
"value": "Item %d",
"metadata": {
	"created_at": "2023-01-01T12:00:00Z",
	"tags": ["tag1", "tag2"],
	"attributes": {
		"key1": "value1",
		"key2": "value2"
	}
},
"priority": 0.1,
"channel": 1
}`, itemId)
}

func getTempFile() (*os.File, error) {
	// Create a temporary file in the default temp directory
	tempFile, err := os.CreateTemp("", "prefix-*.db")
	if err != nil {
		return nil, err
	}
	return tempFile, nil
}
