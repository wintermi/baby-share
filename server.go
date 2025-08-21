package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	httpServer *http.Server
	staticDir  string
}

func NewServer(addr, staticDir string) (*Server, error) {
	absDir, err := filepath.Abs(staticDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	if _, err := os.Stat(absDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("static directory does not exist: %s", absDir)
	}

	r := chi.NewRouter()

	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	fileServer := http.FileServer(http.Dir(absDir))
	r.Handle("/*", fileServer)

	server := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return &Server{
		httpServer: server,
		staticDir:  absDir,
	}, nil
}

func (s *Server) Start() error {
	log.Printf("Starting server on %s, serving files from %s", s.httpServer.Addr, s.staticDir)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	log.Println("Shutting down server...")
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) GetStaticDir() string {
	return s.staticDir
}

//---------------------------------------------------------------------------------------

func main() {
	if err := mainNoExit(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

//---------------------------------------------------------------------------------------

func mainNoExit() error {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run server.go <static-directory> [port]")
	}

	staticDir := os.Args[1]
	port := "8080"
	if len(os.Args) > 2 {
		port = os.Args[2]
	}

	addr := ":" + port

	server, err := NewServer(addr, staticDir)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	go func() {
		if err := server.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
		return err
	}

	log.Println("Server exited")
	return nil
}
