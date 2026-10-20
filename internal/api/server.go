package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jovil96/balanza-agent/internal/config"
	"github.com/jovil96/balanza-agent/internal/scale"
)

type Server struct {
	cfg   config.APIConfig
	scale *scale.Service
}

func New(cfg config.APIConfig, service *scale.Service) *Server {
	return &Server{cfg: cfg, scale: service}
}

func (s *Server) Start(ctx context.Context) {
	if !s.cfg.Enabled {
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", s.health)
	mux.HandleFunc("/api/v1/reading", s.reading)
	mux.HandleFunc("/api/v1/events", s.events)
	httpServer := &http.Server{Addr: s.cfg.Address, Handler: s.cors(s.requireAPIKey(mux)), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdown)
	}()
	go func() {
		log.Printf("API local disponible en http://%s", s.cfg.Address)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("API error: %v", err)
		}
	}()
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) reading(w http.ResponseWriter, _ *http.Request) {
	reading, ok := s.scale.Latest()
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "waiting_for_scale"})
		return
	}
	writeJSON(w, http.StatusOK, reading)
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	readings, unsubscribe := s.scale.Subscribe()
	defer unsubscribe()
	for {
		select {
		case <-r.Context().Done():
			return
		case reading, open := <-readings:
			if !open {
				return
			}
			data, _ := json.Marshal(reading)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (s *Server) requireAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.APIKey == "" {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("X-Api-Key") != s.cfg.APIKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) cors(next http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(s.cfg.AllowedOrigins))
	for _, origin := range s.cfg.AllowedOrigins {
		allowed[strings.TrimRight(origin, "/")] = struct{}{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimRight(r.Header.Get("Origin"), "/")
		if origin != "" {
			if _, ok := allowed[origin]; !ok {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Api-Key")
		if r.Header.Get("Access-Control-Request-Private-Network") == "true" {
			w.Header().Set("Access-Control-Allow-Private-Network", "true")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
