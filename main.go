// Copyright (c) 2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
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
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	URL         string `json:"url,omitempty"`
	MaxFileSize int64  `json:"max_file_size,omitempty"`
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
			// Images
			"image/jpeg", "image/jpg", "image/pjpeg",
			"image/png",
			"image/gif",
			"image/webp",
			"image/svg+xml",
			// Generic image type pattern
			"image/*",
			// Audio
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
	fmt.Printf("Checking if content type is allowed: %s\n", contentType)
	fmt.Printf("Allowed types: %v\n", config.AllowedTypes)

	// Convert to lowercase for case-insensitive comparison
	contentTypeLower := strings.ToLower(contentType)

	for _, allowedType := range config.AllowedTypes {
		// Convert allowed type to lowercase as well
		allowedTypeLower := strings.ToLower(allowedType)

		if contentTypeLower == allowedTypeLower {
			fmt.Printf("Content type %s is allowed\n", contentType)
			return true
		}
	}

	// Also check if it's a more generic match (e.g., image/*)
	for _, allowedType := range config.AllowedTypes {
		allowedTypeLower := strings.ToLower(allowedType)

		// Check if it's a wildcard type (e.g., image/*)
		if strings.HasSuffix(allowedTypeLower, "/*") {
			prefix := strings.TrimSuffix(allowedTypeLower, "/*")
			if strings.HasPrefix(contentTypeLower, prefix) {
				fmt.Printf("Content type %s is allowed via wildcard %s\n", contentType, allowedType)
				return true
			}
		}
	}

	fmt.Printf("Content type %s is NOT allowed\n", contentType)
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

	// Check content type
	contentType := r.Header.Get("Content-Type")
	isMultipart := strings.HasPrefix(contentType, "multipart/form-data")
	isFormUrlEncoded := contentType == "application/x-www-form-urlencoded"

	// Print debug info
	fmt.Printf("Upload request received: Content-Type=%s, Content-Length=%d\n",
		contentType, r.ContentLength)

	// Handle based on content type
	if isMultipart {
		handleMultipartUpload(w, r)
	} else if isFormUrlEncoded {
		handleFormUrlEncodedUpload(w, r)
	} else {
		sendJSONResponse(w, false, "Unsupported content type", "")
	}
}

func handleMultipartUpload(w http.ResponseWriter, r *http.Request) {
	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, config.MaxFileSize)

	// Parse multipart form
	if err := r.ParseMultipartForm(config.MaxFileSize); err != nil {
		fmt.Printf("Error parsing multipart form: %v\n", err)
		sendJSONResponse(w, false, "File too large", "")
		return
	}
	defer r.MultipartForm.RemoveAll()

	// Get file from form
	file, header, err := r.FormFile("file")
	if err != nil {
		fmt.Printf("Error retrieving file from form: %v\n", err)
		sendJSONResponse(w, false, "Error retrieving file", "")
		return
	}
	defer file.Close()

	// Check file size
	// This is a more direct check of actual file size
	fileData, err := io.ReadAll(file)
	if err != nil {
		fmt.Printf("Error reading file data: %v\n", err)
		sendJSONResponse(w, false, "Error reading file", "")
		return
	}

	if int64(len(fileData)) > config.MaxFileSize {
		fmt.Printf("File too large: %d bytes (max: %d)\n", len(fileData), config.MaxFileSize)
		sendJSONResponse(w, false, "File too large", "")
		return
	}

	// We'll reuse file with fileData
	fileReader := bytes.NewReader(fileData)

	// Get content type from header or from X-File-Type header
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = r.Header.Get("X-File-Type")
	}

	// If still empty, check if a filetype field was provided in the form
	if contentType == "" {
		contentType = r.FormValue("filetype")
		fmt.Printf("Using filetype from form field: %s\n", contentType)
	}

	// If still empty, try to detect from the file data
	if contentType == "" {
		contentType = http.DetectContentType(fileData)
		fmt.Printf("Detected content type from file data: %s\n", contentType)
	}

	// Check file type
	if !isAllowedFileType(contentType) {
		fmt.Printf("File type not allowed: %s\n", contentType)
		sendJSONResponse(w, false, "File type not allowed", "")
		return
	}

	// Generate random filename
	randomFilename, err := generateRandomFilename(header.Filename)
	if err != nil {
		sendJSONResponse(w, false, "Error generating filename", "")
		return
	}

	// Save file and generate URL
	downloadURL, err := saveFileAndGenerateURL(randomFilename, fileReader)
	if err != nil {
		sendJSONResponse(w, false, fmt.Sprintf("Error saving file: %v", err), "")
		return
	}

	sendJSONResponse(w, true, "File uploaded successfully", downloadURL)
}

func handleFormUrlEncodedUpload(w http.ResponseWriter, r *http.Request) {
	// Parse form
	if err := r.ParseForm(); err != nil {
		fmt.Printf("Error parsing form: %v\n", err)
		sendJSONResponse(w, false, "Error parsing form", "")
		return
	}

	// Get form data
	filename := r.FormValue("filename")
	if filename == "" {
		filename = "file.dat"
	}

	fileType := r.FormValue("type")
	if fileType == "" {
		fileType = r.Header.Get("X-File-Type")
	}

	base64Data := r.FormValue("data")
	if base64Data == "" {
		sendJSONResponse(w, false, "No file data provided", "")
		return
	}

	// Print debug info
	fmt.Printf("Form data received: filename=%s, type=%s, data length=%d\n",
		filename, fileType, len(base64Data))

	// Decode base64 data
	fileData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		fmt.Printf("Error decoding base64 data: %v\n", err)
		sendJSONResponse(w, false, "Error decoding base64 data", "")
		return
	}

	// Check file size
	if int64(len(fileData)) > config.MaxFileSize {
		fmt.Printf("File too large: %d bytes (max: %d)\n", len(fileData), config.MaxFileSize)
		sendJSONResponse(w, false, "File too large", "")
		return
	}

	// If file type is not specified, detect it
	if fileType == "" {
		fileType = http.DetectContentType(fileData)
	}

	// Check file type
	if !isAllowedFileType(fileType) {
		fmt.Printf("File type not allowed: %s\n", fileType)
		sendJSONResponse(w, false, "File type not allowed", "")
		return
	}

	// Generate random filename
	randomFilename, err := generateRandomFilename(filename)
	if err != nil {
		sendJSONResponse(w, false, "Error generating filename", "")
		return
	}

	// Save file and generate URL
	downloadURL, err := saveFileAndGenerateURL(randomFilename, bytes.NewReader(fileData))
	if err != nil {
		sendJSONResponse(w, false, fmt.Sprintf("Error saving file: %v", err), "")
		return
	}

	sendJSONResponse(w, true, "File uploaded successfully", downloadURL)
}

func saveFileAndGenerateURL(filename string, data io.Reader) (string, error) {
	// Create file path
	filepath := filepath.Join(config.UploadDir, filename)

	// Create new file
	dst, err := os.Create(filepath)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	// Copy file contents
	if _, err := io.Copy(dst, data); err != nil {
		return "", err
	}

	// Generate download URL with domain
	downloadURL := fmt.Sprintf("https://%s/download/%s", config.Domain, filename)
	return downloadURL, nil
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
	resp := Response{
		Success:     true,
		Message:     "API key is valid",
		MaxFileSize: config.MaxFileSize,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
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
