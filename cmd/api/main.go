package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"auth/internal/config"
	"auth/internal/database"
	"auth/internal/handlers"
	"auth/internal/logger"
	"auth/internal/middleware"
	"auth/internal/repository"
	"auth/internal/repository/postgres"
	"auth/internal/services"
	_ "auth/docs"
	"github.com/swaggo/http-swagger"
)

// @title User Auth API
// @version 1.0
// @description This is a professional user authentication API with PostgreSQL, structured logging, and proper error handling.
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url http://www.swagger.io/support
// @contact.email support@swagger.io

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @host localhost:8081
// @BasePath /
// @schemes http

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization
func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Application failed to start: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load configuration
	cfg := config.Load()
	
	// Initialize logger
	log := logger.New(os.Getenv("LOG_LEVEL"))
	log.Info("starting application", "version", "1.0")

	// Initialize database
	db, err := database.New(cfg, log)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer db.Close()

	// Run migrations
	if err := db.RunMigrations(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	log.Info("database migrations completed successfully")

	// Initialize repositories
	userRepo := postgres.NewUserRepository(db.DB)
	repo := repository.New(userRepo)

	// Initialize services
	authService := services.NewAuthService(repo, cfg, log)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(authService, log)

	// Initialize middleware
	mw := middleware.New(cfg, log)

	// Setup HTTP server
	server := setupServer(cfg, mw, authHandler, log)

	// Channel to listen for interrupt signal to terminate server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine
	go func() {
		log.Info("server starting", "address", server.Addr)
		log.Info("swagger UI available", "url", fmt.Sprintf("http://%s:%s/swagger/index.html", cfg.Server.Host, cfg.Server.Port))
		
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server failed to start", "error", err)
		}
	}()

	// Wait for interrupt signal
	<-quit
	log.Info("shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Error("server forced to shutdown", "error", err)
		return err
	}

	log.Info("server shutdown complete")
	return nil
}

func setupServer(cfg *config.Config, mw *middleware.Middleware, authHandler *handlers.AuthHandler, log *logger.Logger) *http.Server {
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy","timestamp":"` + time.Now().UTC().Format(time.RFC3339) + `"}`))
	})

	// API routes
	mux.HandleFunc("/signup", authHandler.SignUp)
	mux.HandleFunc("/login", authHandler.Login)
	
	// Protected routes
	protectedMux := http.NewServeMux()
	protectedMux.HandleFunc("/profile", authHandler.GetProfile)
	mux.Handle("/profile", mw.JWT(protectedMux))

	// Swagger documentation
	mux.Handle("/swagger/", httpSwagger.WrapHandler)

	// Root endpoint
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message":"User Auth API is running","version":"1.0"}`))
	})

	// Apply middleware chain
	handler := mw.Recovery(
		mw.Logging(
			mw.RequestID(
				mw.CORS(mux),
			),
		),
	)

	return &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      handler,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}
}
