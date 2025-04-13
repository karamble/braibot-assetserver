// Copyright (c) 2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	MaxFileSize  int64    `json:"max_file_size"`
	APIKey       string   `json:"api_key"`
	UploadDir    string   `json:"upload_dir"`
	Port         string   `json:"port"`
	Domain       string   `json:"domain"`
	AllowedTypes []string `json:"allowed_types"`
}

type Response struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	URL     string `json:"url,omitempty"`
}

var config Config

func loadConfig() error {
	// Read config file
	file, err := os.ReadFile("config.json")
	if err != nil {
		return fmt.Errorf("error reading config file: %v", err)
	}

	// Parse JSON
	if err := json.Unmarshal(file, &config); err != nil {
		return fmt.Errorf("error parsing config file: %v", err)
	}

	// Validate config
	if config.MaxFileSize <= 0 {
		return fmt.Errorf("max_file_size must be greater than 0")
	}
	if config.APIKey == "" {
		return fmt.Errorf("api_key cannot be empty")
	}
	if config.UploadDir == "" {
		return fmt.Errorf("upload_dir cannot be empty")
	}
	if config.Port == "" {
		config.Port = ":8080" // Default port
	}
	if config.Domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}

	// Set default allowed types if not specified
	if len(config.AllowedTypes) == 0 {
		config.AllowedTypes = []string{
			"image/jpeg", "image/png", "image/gif", "image/webp", "image/svg+xml",
			"audio/mpeg", "audio/ogg", "audio/wav", "audio/webm", "audio/aac",
		}
	}

	return nil
}

func init() {
	// Load configuration
	if err := loadConfig(); err != nil {
		log.Fatal(err)
	}

	// Create uploads directory if it doesn't exist
	if err := os.MkdirAll(config.UploadDir, 0755); err != nil {
		log.Fatal(err)
	}
}

func generateRandomFilename(originalFilename string) (string, error) {
	// Get file extension
	ext := filepath.Ext(originalFilename)

	// Generate random bytes
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	// Create random filename with original extension
	randomName := base64.URLEncoding.EncodeToString(b)
	return randomName + ext, nil
}

func isAllowedFileType(contentType string) bool {
	for _, allowedType := range config.AllowedTypes {
		if contentType == allowedType {
			return true
		}
	}
	return false
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check API key
	apiKeyHeader := r.Header.Get("X-API-Key")
	if apiKeyHeader != config.APIKey {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, config.MaxFileSize)

	// Parse multipart form
	if err := r.ParseMultipartForm(config.MaxFileSize); err != nil {
		sendJSONResponse(w, false, "File too large", "")
		return
	}
	defer r.MultipartForm.RemoveAll()

	// Get file from form
	file, header, err := r.FormFile("file")
	if err != nil {
		sendJSONResponse(w, false, "Error retrieving file", "")
		return
	}
	defer file.Close()

	// Check file type
	contentType := header.Header.Get("Content-Type")
	if !isAllowedFileType(contentType) {
		sendJSONResponse(w, false, "File type not allowed", "")
		return
	}

	// Generate random filename
	randomFilename, err := generateRandomFilename(header.Filename)
	if err != nil {
		sendJSONResponse(w, false, "Error generating filename", "")
		return
	}

	// Create file path
	filepath := filepath.Join(config.UploadDir, randomFilename)

	// Create new file
	dst, err := os.Create(filepath)
	if err != nil {
		sendJSONResponse(w, false, "Error creating file", "")
		return
	}
	defer dst.Close()

	// Copy file contents
	if _, err := io.Copy(dst, file); err != nil {
		sendJSONResponse(w, false, "Error saving file", "")
		return
	}

	// Generate download URL with domain
	downloadURL := fmt.Sprintf("https://%s/download/%s", config.Domain, randomFilename)
	sendJSONResponse(w, true, "File uploaded successfully", downloadURL)
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract filename from URL
	filename := strings.TrimPrefix(r.URL.Path, "/download/")
	if filename == "" {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// Construct full file path
	filepath := filepath.Join(config.UploadDir, filename)

	// Open the file
	file, err := os.Open(filepath)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	// Get file info for Content-Disposition
	fileInfo, err := file.Stat()
	if err != nil {
		http.Error(w, "Error reading file info", http.StatusInternalServerError)
		return
	}

	// Set headers for file download
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))

	// Stream file to response
	io.Copy(w, file)

	// Delete file after successful download
	go func() {
		// Small delay to ensure file is fully sent
		time.Sleep(time.Second)
		os.Remove(filepath)
	}()
}

func testHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check API key
	apiKeyHeader := r.Header.Get("X-API-Key")
	if apiKeyHeader != config.APIKey {
		sendJSONResponse(w, false, "Invalid API key", "")
		return
	}

	// If we get here, the API key is valid
	sendJSONResponse(w, true, "API key is valid", "")
}

func sendJSONResponse(w http.ResponseWriter, success bool, message string, url string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Response{
		Success: success,
		Message: message,
		URL:     url,
	})
}

func main() {
	http.HandleFunc("/upload", uploadHandler)
	http.HandleFunc("/download/", downloadHandler)
	http.HandleFunc("/test", testHandler)

	fmt.Printf("Server starting on port %s...\n", config.Port)
	if err := http.ListenAndServe(config.Port, nil); err != nil {
		log.Fatal(err)
	}
}
