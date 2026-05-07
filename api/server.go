package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/client"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

type Server struct {
	router        *mux.Router
	docker        *client.Client
	upgrader      websocket.Upgrader
	metricsStore  *MetricsStore
	alertStore    *AlertStore
}

func NewServer(dockerClient *client.Client, metricsStore *MetricsStore, alertStore *AlertStore) *Server {
	s := &Server{
		router:       mux.NewRouter(),
		docker:       dockerClient,
		metricsStore: metricsStore,
		alertStore:   alertStore,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // TODO: Implement proper origin check
			},
		},
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	// API routes
	api := s.router.PathPrefix("/api").Subrouter()
	api.Use(s.corsMiddleware)
	api.Use(s.authMiddleware)

	// Container endpoints
	api.HandleFunc("/containers", s.handleListContainers).Methods("GET")
	api.HandleFunc("/containers/{id}", s.handleGetContainer).Methods("GET")
	api.HandleFunc("/containers/{id}/start", s.handleStartContainer).Methods("POST")
	api.HandleFunc("/containers/{id}/stop", s.handleStopContainer).Methods("POST")
	api.HandleFunc("/containers/{id}/restart", s.handleRestartContainer).Methods("POST")
	api.HandleFunc("/containers/{id}/logs", s.handleContainerLogs).Methods("GET")
	api.HandleFunc("/containers/{id}/stats", s.handleContainerStats).Methods("GET")
	api.HandleFunc("/containers/{id}/metrics", s.handleContainerMetrics).Methods("GET")
	
	// Stats endpoint
	api.HandleFunc("/stats", s.handleSystemStats).Methods("GET")
	
	// Metrics endpoints
	api.HandleFunc("/metrics", s.handleAllMetrics).Methods("GET")
	api.HandleFunc("/metrics/export", s.handleExportMetrics).Methods("GET")
	
	// Bulk operations
	api.HandleFunc("/bulk", s.handleBulkAction).Methods("POST")
	
	// Docker Compose endpoints
	api.HandleFunc("/compose/projects", s.handleListComposeProjects).Methods("GET")
	api.HandleFunc("/compose/action", s.handleComposeAction).Methods("POST")
	
	// Alert endpoints
	api.HandleFunc("/alerts/configs", s.handleGetAlertConfigs).Methods("GET")
	api.HandleFunc("/alerts/configs", s.handleSetAlertConfig).Methods("POST")
	api.HandleFunc("/alerts/configs/{id}", s.handleDeleteAlertConfig).Methods("DELETE")
	api.HandleFunc("/alerts/history", s.handleGetAlertHistory).Methods("GET")
	
	// WebSocket endpoint
	api.HandleFunc("/ws", s.handleWebSocket)
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		initData := r.Header.Get("X-Telegram-Init-Data")
		if initData == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"success":false,"error":"Unauthorized"}`))
			return
		}

		if !s.validateTelegramAuth(initData) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"success":false,"error":"Invalid auth"}`))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) validateTelegramAuth(initData string) bool {
	// Parse initData query string
	values, err := url.ParseQuery(initData)
	if err != nil {
		return false
	}

	hash := values.Get("hash")
	if hash == "" {
		return false
	}

	// Remove hash from values
	values.Del("hash")

	// Build data-check-string
	var keys []string
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var dataCheckString string
	for _, k := range keys {
		dataCheckString += k + "=" + values.Get(k) + "\n"
	}
	dataCheckString = strings.TrimSuffix(dataCheckString, "\n")

	// Get bot token from env
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if botToken == "" {
		return false
	}

	// Compute secret key: HMAC-SHA256(bot_token, "WebAppData")
	secretKey := hmac.New(sha256.New, []byte("WebAppData"))
	secretKey.Write([]byte(botToken))

	// Compute hash: HMAC-SHA256(secret_key, data_check_string)
	h := hmac.New(sha256.New, secretKey.Sum(nil))
	h.Write([]byte(dataCheckString))
	computedHash := hex.EncodeToString(h.Sum(nil))

	return computedHash == hash
}

func (s *Server) Start(port string) error {
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("API server starting on port %s", port)
	return srv.ListenAndServe()
}
