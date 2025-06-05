package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
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

/*

t.Run("End to end test DB", func(t *testing.T) {
		pq := sqlpqueue.NewSqLitePQueue("test.db", "test", true)
		srv := server.NewServer(pq, "", false)
		go func() {
			if err := srv.Start(":" + strconv.Itoa(8080)); err != nil {
				fmt.Printf("Failed to start server: %v\n", err)
			}
		}()

		time.Sleep(3 * time.Second)
		enqueueItems(NO_OF_ITEMS, "http://localhost:8080/enqueue", server.API_KEY)
		dequeueItems("http://localhost:8080/dequeue", server.API_KEY, server.API_KEY_HEADER, false)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		err := srv.Shutdown(ctx)
		AssertNoError(t, err)

	})


*/
