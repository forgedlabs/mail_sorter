package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/redis/go-redis/v9"
	"gopkg.in/yaml.v3"
)

// Import the verifier package (assumes smtp-verifier.go is in same directory)
// In production, this would be a proper package import

type Server struct {
	verifier *SMTPVerifier
	router   *mux.Router
	config   *Config
}

type ValidateRequest struct {
	Email     string `json:"email"`
	SkipCache bool   `json:"skip_cache,omitempty"`
}

type ValidateResponse struct {
	*ValidationResult
}

type BatchValidateRequest struct {
	Emails   []string `json:"emails"`
	Priority string   `json:"priority,omitempty"`
}

type BatchValidateResponse struct {
	Results []*ValidationResult `json:"results"`
}

func main() {
	// Load configuration
	config := loadConfig()

	// Initialize Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", getEnv("REDIS_HOST", "localhost"), 6379),
		Password: getEnv("REDIS_PASSWORD", ""),
		DB:       0,
	})

	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Println("✓ Connected to Redis")

	// Initialize SMTP Verifier
	verifier := NewSMTPVerifier(config, redisClient)

	// Create server
	server := &Server{
		verifier: verifier,
		router:   mux.NewRouter(),
		config:   config,
	}

	// Setup routes
	server.setupRoutes()

	// Start HTTP server
	addr := fmt.Sprintf(":%s", getEnv("SERVER_PORT", "8080"))
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      server.router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		log.Printf("🚀 Email Validator API starting on %s", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("✓ Server exited")
}

func (s *Server) setupRoutes() {
	// API routes
	api := s.router.PathPrefix("/v1").Subrouter()
	api.HandleFunc("/validate", s.handleValidate).Methods("POST", "OPTIONS")
	api.HandleFunc("/validate/batch", s.handleBatchValidate).Methods("POST", "OPTIONS")

	// Health check
	s.router.HandleFunc("/health", s.handleHealth).Methods("GET")

	// Metrics (Prometheus-compatible)
	s.router.HandleFunc("/metrics", s.handleMetrics).Methods("GET")

	// CORS middleware - must be first
	s.router.Use(corsMiddleware)
	s.router.Use(loggingMiddleware)
}

func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	var req ValidateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Email == "" {
		http.Error(w, "Email is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	result, err := s.verifier.Verify(ctx, req.Email)
	if err != nil {
		http.Error(w, fmt.Sprintf("Validation failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleBatchValidate(w http.ResponseWriter, r *http.Request) {
	var req BatchValidateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if len(req.Emails) == 0 {
		http.Error(w, "Emails array is required", http.StatusBadRequest)
		return
	}

	if len(req.Emails) > 1000 {
		http.Error(w, "Maximum 1000 emails per batch", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	results := make([]*ValidationResult, len(req.Emails))

	// Process each email
	for i, email := range req.Emails {
		result, err := s.verifier.Verify(ctx, email)
		if err != nil {
			// Create error result
			results[i] = &ValidationResult{
				Email:      email,
				Status:     StatusUnknown,
				Reason:     fmt.Sprintf("Verification error: %v", err),
				Confidence: 0.0,
				CheckedAt:  time.Now(),
			}
		} else {
			results[i] = result
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(BatchValidateResponse{Results: results})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":    "healthy",
		"version":   "1.0.0",
		"timestamp": time.Now().Format(time.RFC3339),
		"checks": map[string]bool{
			"redis": s.verifier.redis.Ping(r.Context()).Err() == nil,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	// Basic Prometheus metrics
	// In production, use github.com/prometheus/client_golang
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "# HELP email_validator_validations_total Total validations\n")
	fmt.Fprintf(w, "# TYPE email_validator_validations_total counter\n")
	fmt.Fprintf(w, "email_validator_validations_total 0\n")
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(start))
	})
}

func loadConfig() *Config {
	configPath := getEnv("CONFIG_PATH", "config/config.yaml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		log.Printf("Warning: Could not load config file, using defaults: %v", err)
		return DefaultConfig()
	}

	var fileConfig struct {
		SMTP struct {
			ConnectTimeout time.Duration `yaml:"connect_timeout"`
			ReadTimeout    time.Duration `yaml:"read_timeout"`
			EHLOHostname   string        `yaml:"ehlo_hostname"`
			MailFrom       string        `yaml:"mail_from"`
		} `yaml:"smtp"`
	}

	if err := yaml.Unmarshal(data, &fileConfig); err != nil {
		log.Printf("Warning: Could not parse config file, using defaults: %v", err)
		return DefaultConfig()
	}

	config := DefaultConfig()
	if fileConfig.SMTP.ConnectTimeout > 0 {
		config.SMTPConnectTimeout = fileConfig.SMTP.ConnectTimeout
	}
	if fileConfig.SMTP.ReadTimeout > 0 {
		config.SMTPReadTimeout = fileConfig.SMTP.ReadTimeout
	}
	if fileConfig.SMTP.EHLOHostname != "" {
		config.EHLOHostname = fileConfig.SMTP.EHLOHostname
	}
	if fileConfig.SMTP.MailFrom != "" {
		config.MailFrom = fileConfig.SMTP.MailFrom
	}

	return config
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
