package server

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/jnsoft/jngo/pqueue"
	"github.com/jnsoft/jnq/src/httphelper"
	"github.com/jnsoft/jnq/src/mempqueue"
	"github.com/jnsoft/jnq/src/priorityqueue"
)

const (
	DEFAULT_PRIO    = 0
	DEFAULT_CHANNEL = 0
	API_KEY_HEADER  = "X-API-Key"
	API_KEY         = "api-key"
)

//go:embed swagger.json
var swaggerJSON []byte

type (
	Server struct {
		pq      priorityqueue.IPriorityQueue
		mu      sync.Mutex
		api_key string
		verbose bool
		server  *http.Server
	}
)

func NewServer(pq priorityqueue.IPriorityQueue, apikey string, verbose bool) *Server {
	if apikey == "" {
		apikey = API_KEY
	}
	return &Server{pq: pq, api_key: apikey, verbose: verbose}
}

// Middleware to check API key
func (s *Server) apiKeyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get(API_KEY_HEADER)
		if apiKey != s.api_key {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// EnqueueHandler handles the enqueue requests
// @Summary Enqueue an item
// @Description Enqueue an item to the priority queue. The item is provided in the request body as a string (which can be a JSON object).
// Query parameters are used to specify the priority, channel, and notbefore timestamp.
// @Accept  plain
// @Produce  plain
// @Param  prio  query  float  false  "Priority of the item"
// @Param  channel  query  int  false  "Channel to enqueue the item to"
// @Param  notbefore  query  timestamp  false  "Timestamp in RFC3339 format specifying when the item becomes valid"
// @Param  item  body  string  true  "Item to enqueue (string or JSON object)"
// @Success 200 "Item enqueued"
// @Failure 400 "Bad Request"
// @Failure 403 "Forbidden"
// @Failure 405 "Method Not Allowed"
// @Failure 500 "Internal Server Error"
// @Router /enqueue [post]
// @Method post
func (s *Server) EnqueueHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// query parameters
	channelStr := r.URL.Query().Get("channel")
	channel, err := strconv.Atoi(channelStr)
	if err != nil || channel < 0 || channel > 100 {
		channel = DEFAULT_CHANNEL
	}

	prioStr := r.URL.Query().Get("prio")
	priority, err := strconv.ParseFloat(prioStr, 64)
	if err != nil {
		priority = DEFAULT_PRIO
	}

	notBeforeStr := r.URL.Query().Get("notbefore")
	var notBefore time.Time
	if notBeforeStr != "" {
		notBefore, err = time.Parse(time.RFC3339, notBeforeStr)
		if err != nil {
			http.Error(w, "Invalid notbefore timestamp", http.StatusBadRequest)
			return
		}
		notBefore = notBefore.UTC() // Ensure the timestamp is in UTC
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	item := string(bodyBytes)

	if item == "" {
		http.Error(w, "Request body is required", http.StatusBadRequest)
		return
	}

	err = s.pq.Enqueue(item, priority, channel, notBefore)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var resStr string
	if s.verbose {
		resStr = fmt.Sprintf("EnqueueHandler: enqueued item: %s with priority: %f, channel: %d, notbefore: %s", item, priority, channel, notBefore.Format(time.RFC3339))
	} else {
		resStr = "Item enqueued"
	}

	w.Write([]byte(resStr))

	if s.verbose {
		log.Println(resStr)
	}
}

// DequeueHandler handles the dequeue requests
// @Summary Dequeue an item
// @Description Dequeue an item from the priority queue
// @Produce  json
// @Param  channel  query  int  false  "Channel to dequeue from"
// @Success 200 "Dequeued item: {value}" json
// @Failure 204 "No Content"
// @Failure 400 "Bad Request"
// @Failure 403 "Forbidden"
// @Failure 405 "Method Not Allowed"
// @Failure 500 "Internal Server Error"
// @Router /dequeue [get]
// @Method get
func (s *Server) DequeueHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	channelStr := r.URL.Query().Get("channel")
	channel, err := strconv.Atoi(channelStr)
	if err != nil {
		channel = DEFAULT_CHANNEL
	}

	if channel < 0 || channel > mempqueue.MAX_CHANNEL {
		http.Error(w, "Channel must be between 0 and 100", http.StatusBadRequest)
		return
	}

	value, err := s.pq.Dequeue(channel)
	if err != nil {
		if err.Error() == pqueue.EMPTY_QUEUE {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the dequeued item as a JSON object
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(value))

	if s.verbose {
		log.Printf("DequeueHandler: dequeued item: %s\n", value)
	}
}

// DequeueWithReservationHandler handles dequeue requests with reservation
// @Summary Dequeue an item with reservation
// @Description Dequeue an item from the priority queue with a reservation ID
// @Produce  json
// @Param  channel  query  int  false  "Channel to dequeue from"
// @Success 200 {object} map[string]string "Dequeued item and reservation ID"
// @Failure 204 "No Content"
// @Failure 400 "Bad Request"
// @Failure 403 "Forbidden"
// @Failure 405 "Method Not Allowed"
// @Failure 500 "Internal Server Error"
// @Router /reserve [get]
// @Method get
func (s *Server) DequeueWithReservationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	channelStr := r.URL.Query().Get("channel")
	channel, err := strconv.Atoi(channelStr)
	if err != nil {
		channel = DEFAULT_CHANNEL
	}

	if channel < 0 || channel > mempqueue.MAX_CHANNEL {
		http.Error(w, "Channel must be between 0 and 100", http.StatusBadRequest)
		return
	}

	value, reservationId, err := s.pq.DequeueWithReservation(channel)
	if err != nil {
		if err.Error() == pqueue.EMPTY_QUEUE {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the dequeued item and reservation ID as a JSON object
	//response := map[string]string{
	//		"value":          value,
	//		"reservation_id": reservationId,
	//	}
	//	w.Header().Set("Content-Type", "application/json")
	//	json.NewEncoder(w).Encode(response)

	var raw json.RawMessage
	if err := json.Unmarshal([]byte(value), &raw); err == nil {
		response := map[string]any{
			"value":          raw,
			"reservation_id": reservationId,
		}
		json.NewEncoder(w).Encode(response)
	} else {
		// Value is not valid JSON, return as string
		response := map[string]interface{}{
			"value":          value,
			"reservation_id": reservationId,
		}
		json.NewEncoder(w).Encode(response)
	}

	if s.verbose {
		log.Printf("DequeueWithReservationHandler: dequeued item: %s with reservation ID: %s\n", value, reservationId)
	}
}

// ConfirmReservationHandler handles requests to confirm a reservation
// @Summary Confirm a reservation
// @Description Confirm a reservation by providing the reservation Id as a path parameter
// @Accept  json
// @Produce  json
// @Param  reservation_id  path string true "Reservation Id to confirm"
// @Success 200 "Reservation confirmed"
// @Failure 400 "Bad Request"
// @Failure 403 "Forbidden"
// @Failure 405 "Method Not Allowed"
// @Failure 500 "Internal Server Error"
// @Router /confirm/{reservation_id} [post]
// @Method post
func (s *Server) ConfirmReservationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	parts := httphelper.SplitPath(r.URL.Path)
	if len(parts) != 2 || parts[0] != "confirm" || parts[1] == "" {
		http.Error(w, "Missing or invalid reservation_id in path", http.StatusBadRequest)
		return
	}
	reservationId := parts[1]

	if _, err := s.pq.ConfirmReservation(reservationId); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	if s.verbose {
		log.Printf("ConfirmReservationHandler: confirmed reservation Id: %s\n", reservationId)
	}
}

// SizeHandler handles requests to get the current size of the queue
// @Summary Get the size of the queue
// @Description Returns the number of items in the queue for a specified channel
// @Produce json
// @Param channel query int false "Channel to get the size of"
// @Success 200 {object} map[string]int "Queue size"
// @Failure 400 "Bad Request"
// @Failure 403 "Forbidden"
// @Failure 500 "Internal Server Error"
// @Router /size [get]
// @Method get
func (s *Server) SizeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	channelStr := r.URL.Query().Get("channel")
	channel, err := strconv.Atoi(channelStr)
	if err != nil || channel < 0 || channel > mempqueue.MAX_CHANNEL {
		http.Error(w, "Invalid channel. Must be between 0 and 100.", http.StatusBadRequest)
		return
	}

	size, err := s.pq.Size(channel)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get queue size: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]int{"size": size}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}

	if s.verbose {
		log.Printf("SizeHandler: channel %d has size %d\n", channel, size)
	}
}

func (s *Server) ServeSwagger(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(swaggerJSON)
}

// ResetHandler handles the reset requests
// @Summary Reset the queue
// @Description Reset the priority queue
// @Accept  json
// @Produce json
// @Success 200 "Queue reset successfully"
// @Failure 403 "Forbidden"
// @Failure 405 "Method Not Allowed"
// @Failure 500 "Internal Server Error"
// @Router /reset [post]
// @Method post
func (s *Server) ResetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.pq.ResetQueue()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// Serve the Swagger UI index.html with the correct URL for the swagger.json file
func (s *Server) ServeSwaggerUi(w http.ResponseWriter, r *http.Request) {
	swaggerUIPath := filepath.Join("swagger-ui")
	if r.URL.Path == "/swagger-ui" {
		http.ServeFile(w, r, filepath.Join(swaggerUIPath, "index.html"))
	} else {
		http.StripPrefix("/swagger-ui", http.FileServer(http.Dir(swaggerUIPath))).ServeHTTP(w, r)
	}
}

func (s *Server) StartRequeueTask(timeout time.Duration) {
	ticker := time.NewTicker(timeout / 2)
	go func() {
		for range ticker.C {
			requeued, err := s.pq.RequeueExpiredReservations(timeout)
			if err != nil {
				log.Printf("Error requeuing expired reservations: %v\n", err)
				continue
			}
			if s.verbose {
				log.Printf("Requeued %d expired reservations", requeued)
			}
		}
	}()
}

func (s *Server) Start(addr string, ready chan<- struct{}) error {
	mux := http.NewServeMux() // custom ServeMux

	mux.Handle("/enqueue", s.apiKeyMiddleware(http.HandlerFunc(s.EnqueueHandler)))
	mux.Handle("/dequeue", s.apiKeyMiddleware(http.HandlerFunc(s.DequeueHandler)))
	mux.Handle("/reserve", s.apiKeyMiddleware(http.HandlerFunc(s.DequeueWithReservationHandler)))
	mux.Handle("/confirm/", s.apiKeyMiddleware(http.HandlerFunc(s.ConfirmReservationHandler)))
	mux.Handle("/reset", s.apiKeyMiddleware(http.HandlerFunc(s.ResetHandler)))
	mux.Handle("/size", s.apiKeyMiddleware(http.HandlerFunc(s.SizeHandler)))
	mux.HandleFunc("/swagger.json", s.ServeSwagger)
	mux.HandleFunc("/swagger-ui/", s.ServeSwaggerUi)

	s.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Signal ready
	close(ready)

	err := s.server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed to start server: %w", err)
	}

	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// @Success 200 {value} QueueItem "Dequeued item: {value}"
