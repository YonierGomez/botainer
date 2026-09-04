package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/client"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// maxInitDataAge is the maximum age allowed for Telegram WebApp initData
// before it is considered expired and rejected.
const maxInitDataAge = 24 * time.Hour

type Server struct {
	router        *mux.Router
	docker        *client.Client
	upgrader      websocket.Upgrader
	metricsStore  *MetricsStore
	alertStore    *AlertStore
	userStore     *UserStore
	templateStore *TemplateStore
}

func NewServer(dockerClient *client.Client, metricsStore *MetricsStore, alertStore *AlertStore, userStore *UserStore, templateStore *TemplateStore) *Server {
	s := &Server{
		router:        mux.NewRouter(),
		docker:        dockerClient,
		metricsStore:  metricsStore,
		alertStore:    alertStore,
		userStore:     userStore,
		templateStore: templateStore,
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
	api.HandleFunc("/containers", s.handleCreateContainer).Methods("POST")
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
	
	// Updates endpoint
	api.HandleFunc("/updates/check", s.handleCheckUpdates).Methods("POST")
	
	// Bulk operations
	api.HandleFunc("/bulk", s.handleBulkAction).Methods("POST")
	
	// Networks
	api.HandleFunc("/networks", s.handleListNetworks).Methods("GET")
	
	// Docker Compose endpoints
	api.HandleFunc("/compose/projects", s.handleListComposeProjects).Methods("GET")
	api.HandleFunc("/compose/action", s.handleComposeAction).Methods("POST")
	
	// Alert endpoints
	api.HandleFunc("/alerts/configs", s.handleGetAlertConfigs).Methods("GET")
	api.HandleFunc("/alerts/configs", s.handleSetAlertConfig).Methods("POST")
	api.HandleFunc("/alerts/configs/{id}", s.handleDeleteAlertConfig).Methods("DELETE")
	api.HandleFunc("/alerts/history", s.handleGetAlertHistory).Methods("GET")
	
	// User management endpoints
	api.HandleFunc("/users", s.handleGetUsers).Methods("GET")
	api.HandleFunc("/users/{id}/role", s.handleUpdateUserRole).Methods("PUT")
	api.HandleFunc("/audit", s.handleGetAuditLog).Methods("GET")
	
	// Template library endpoints
	api.HandleFunc("/templates", s.handleListTemplates).Methods("GET")
	api.HandleFunc("/templates", s.handleCreateTemplate).Methods("POST")
	api.HandleFunc("/templates/{id}", s.handleGetTemplate).Methods("GET")
	api.HandleFunc("/templates/{id}", s.handleDeleteTemplate).Methods("DELETE")
	api.HandleFunc("/templates/{id}/deploy", s.handleDeployTemplate).Methods("POST")
	
	// WebSocket endpoint
	api.HandleFunc("/ws", s.handleWebSocket)
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Reflect the request Origin instead of using a wildcard, since this
		// API now carries session-like auth (X-Telegram-Init-Data) and full
		// Docker control. A wildcard would allow any website to read
		// responses if it ever obtained a valid initData token.
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Telegram-Init-Data")

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

	if computedHash != hash {
		return false
	}

	// Reject expired initData. Telegram documents that initData should be
	// treated as valid for a limited time window after auth_date.
	authDateStr := values.Get("auth_date")
	if authDateStr == "" {
		return false
	}
	authDateUnix, err := strconv.ParseInt(authDateStr, 10, 64)
	if err != nil {
		return false
	}
	authDate := time.Unix(authDateUnix, 0)
	if time.Since(authDate) > maxInitDataAge {
		log.Printf("Rejected initData: auth_date expired (%s)", authDate)
		return false
	}
	if time.Until(authDate) > 5*time.Minute {
		// auth_date is in the future beyond reasonable clock skew — reject.
		log.Printf("Rejected initData: auth_date in the future (%s)", authDate)
		return false
	}

	// Enforce ALLOWED_USERS whitelist, matching the same env var used by the
	// Telegram bot (main.go). If unset, access is allowed for any verified
	// Telegram user (kept consistent with existing bot behavior).
	if !s.isUserAllowed(values.Get("user")) {
		return false
	}

	return true
}

// telegramWebAppUser mirrors the subset of fields Telegram sends in the
// "user" JSON parameter of initData that we need for authorization checks.
type telegramWebAppUser struct {
	ID int64 `json:"id"`
}

// isUserAllowed checks the given user JSON blob (from initData's "user"
// param) against the ALLOWED_USERS environment variable. If ALLOWED_USERS is
// empty, all verified Telegram users are allowed.
func (s *Server) isUserAllowed(userJSON string) bool {
	allowedUsersStr := os.Getenv("ALLOWED_USERS")
	if allowedUsersStr == "" {
		return true
	}

	if userJSON == "" {
		return false
	}

	var user telegramWebAppUser
	if err := json.Unmarshal([]byte(userJSON), &user); err != nil {
		return false
	}

	for _, idStr := range strings.Split(allowedUsersStr, ",") {
		idStr = strings.TrimSpace(idStr)
		if idStr == "" {
			continue
		}
		allowedID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			continue
		}
		if allowedID == user.ID {
			return true
		}
	}

	return false
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
