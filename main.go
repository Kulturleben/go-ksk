package main

import (
	"io"
	"log"
	"net/http"
	"regexp"
	"sync"
	"time"
)

const (
	baseUpstream = "https://calman02.barrierefrei.berlin/calendar/api/v1"
	cacheTTL     = 5 * time.Minute
)

type cacheEntry struct {
	data  []byte
	until time.Time
}

var (
	cache      = map[string]cacheEntry{}
	cacheMutex sync.RWMutex

	httpClient = &http.Client{
		Timeout: 10 * time.Second,
	}

	eventIDRegex = regexp.MustCompile(`^[0-9]+$`)
)

func main() {
	mux := http.NewServeMux()

	// Static endpoints
	mux.HandleFunc("/api/calendar/events", proxyStatic("/events?show_past=true"))
	mux.HandleFunc("/api/calendar/genres", proxyStatic("/genres"))

	// Dynamic endpoint
	mux.HandleFunc("/api/calendar/event/", eventByIDHandler)

	server := &http.Server{
		Addr:         ":3000",
		Handler:      withCORS(mux),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	log.Println("Calendar API Gateway running on :3000")
	log.Fatal(server.ListenAndServe())
}

func proxyStatic(path string) http.HandlerFunc {
	upstream := baseUpstream + path

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		serveCached(w, upstream)
	}
}

func eventByIDHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Path[len("/api/calendar/event/"):]

	// Strict validation
	if !eventIDRegex.MatchString(id) {
		http.Error(w, "Invalid event id", http.StatusBadRequest)
		return
	}

	upstream := baseUpstream + "/event/" + id
	serveCached(w, upstream)
}

func serveCached(w http.ResponseWriter, upstream string) {
	// Cache lookup
	cacheMutex.RLock()
	entry, ok := cache[upstream]
	if ok && time.Now().Before(entry.until) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		w.Write(entry.data)
		cacheMutex.RUnlock()
		return
	}
	cacheMutex.RUnlock()

	resp, err := httpClient.Get(upstream)
	if err != nil {
		http.Error(w, "Upstream unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, "Upstream error", http.StatusBadGateway)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read upstream response", http.StatusBadGateway)
		return
	}

	cacheMutex.Lock()
	cache[upstream] = cacheEntry{
		data:  body,
		until: time.Now().Add(cacheTTL),
	}
	cacheMutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.Write(body)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
