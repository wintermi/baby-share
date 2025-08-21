package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewServer(t *testing.T) {
	t.Run("valid directory", func(t *testing.T) {
		tempDir := t.TempDir()

		server, err := NewServer(":8080", tempDir)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if server == nil {
			t.Fatal("Expected server to be created")
		}

		if server.GetStaticDir() != tempDir {
			t.Errorf("Expected static dir %s, got %s", tempDir, server.GetStaticDir())
		}
	})

	t.Run("non-existent directory", func(t *testing.T) {
		_, err := NewServer(":8080", "/non/existent/directory")
		if err == nil {
			t.Fatal("Expected error for non-existent directory")
		}

		if !strings.Contains(err.Error(), "does not exist") {
			t.Errorf("Expected 'does not exist' in error, got %v", err)
		}
	})

	t.Run("relative path conversion", func(t *testing.T) {
		tempDir := t.TempDir()
		relativeDir := filepath.Base(tempDir)

		oldWd, _ := os.Getwd()
		defer func() {
			_ = os.Chdir(oldWd)
		}()

		parentDir := filepath.Dir(tempDir)
		_ = os.Chdir(parentDir)

		server, err := NewServer(":8080", relativeDir)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if !filepath.IsAbs(server.GetStaticDir()) {
			t.Error("Expected absolute path for static directory")
		}
	})
}

func TestServerFileServing(t *testing.T) {
	tempDir := t.TempDir()

	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "Hello, World!"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	subDir := filepath.Join(tempDir, "subdir")
	err = os.Mkdir(subDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	subFile := filepath.Join(subDir, "nested.txt")
	subContent := "Nested content"
	err = os.WriteFile(subFile, []byte(subContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create nested test file: %v", err)
	}

	server, err := NewServer(":8080", tempDir)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	testServer := httptest.NewServer(server.httpServer.Handler)
	defer testServer.Close()

	t.Run("serve root file", func(t *testing.T) {
		resp, err := http.Get(testServer.URL + "/test.txt")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		if string(body) != testContent {
			t.Errorf("Expected content %q, got %q", testContent, string(body))
		}
	})

	t.Run("serve nested file", func(t *testing.T) {
		resp, err := http.Get(testServer.URL + "/subdir/nested.txt")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		if string(body) != subContent {
			t.Errorf("Expected content %q, got %q", subContent, string(body))
		}
	})

	t.Run("directory listing", func(t *testing.T) {
		resp, err := http.Get(testServer.URL + "/")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		bodyStr := string(body)
		if !strings.Contains(bodyStr, "test.txt") {
			t.Error("Expected directory listing to contain test.txt")
		}

		if !strings.Contains(bodyStr, "subdir/") {
			t.Error("Expected directory listing to contain subdir/")
		}
	})

	t.Run("non-existent file", func(t *testing.T) {
		resp, err := http.Get(testServer.URL + "/nonexistent.txt")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", resp.StatusCode)
		}
	})
}

func TestServerShutdown(t *testing.T) {
	tempDir := t.TempDir()

	server, err := NewServer(":0", tempDir)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	testServer := httptest.NewServer(server.httpServer.Handler)
	defer testServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = server.Shutdown(ctx)
	if err != nil {
		t.Errorf("Expected no error during shutdown, got %v", err)
	}
}

func TestServerTimeouts(t *testing.T) {
	tempDir := t.TempDir()

	server, err := NewServer(":8080", tempDir)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	if server.httpServer.ReadTimeout != 3600*time.Second {
		t.Errorf("Expected ReadTimeout 3600s, got %v", server.httpServer.ReadTimeout)
	}

	if server.httpServer.WriteTimeout != 3600*time.Second {
		t.Errorf("Expected WriteTimeout 3600s, got %v", server.httpServer.WriteTimeout)
	}

	if server.httpServer.IdleTimeout != 60*time.Second {
		t.Errorf("Expected IdleTimeout 60s, got %v", server.httpServer.IdleTimeout)
	}
}

func TestMiddleware(t *testing.T) {
	tempDir := t.TempDir()

	testFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	server, err := NewServer(":8080", tempDir)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	testServer := httptest.NewServer(server.httpServer.Handler)
	defer testServer.Close()

	t.Run("middleware handles requests", func(t *testing.T) {
		req, err := http.NewRequest("GET", testServer.URL+"/test.txt", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		req.Header.Set("X-Real-IP", "192.168.1.100")
		req.Header.Set("X-Forwarded-For", "203.0.113.1, 192.168.1.100")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		if string(body) != "test content" {
			t.Errorf("Expected 'test content', got %q", string(body))
		}
	})
}
