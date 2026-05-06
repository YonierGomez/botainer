package api

import (
	"log"
	"net/http"
	"time"

	"github.com/docker/docker/client"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

type Server struct {
	router   *mux.Router
	docker   *client.Client
	upgrader websocket.Upgrader
}

func NewServer(dockerClient *client.Client) *Server {
	s := &Server{
		router: mux.NewRouter(),
		docker: dockerClient,
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
	
	// Stats endpoint
	api.HandleFunc("/stats", s.handleSystemStats).Methods("GET")
	
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
		// TODO: Implement Telegram auth validation
		// Temporarily disabled for testing
		next.ServeHTTP(w, r)
	})
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
