package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// Configuration
type Config struct {
	Port              string
	DatabaseURL       string
	VerifyToken       string
	AccessToken       string
	IGBusinessID      string
	Keywords          []string
	DMMessage         string
	DMDelay           time.Duration
	MaxRetries        int
	RetryBackoffBase  time.Duration
}

// Webhook verification request
type WebhookVerification struct {
	Mode      string `json:"hub.mode"`
	Challenge string `json:"hub.challenge"`
	Token     string `json:"hub.verify_token"`
}

// Webhook comment event
type WebhookPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		ID      string    `json:"id"`
		Time    int64     `json:"time"`
		Changes []Change  `json:"changes"`
	} `json:"entry"`
}

type Change struct {
	Field string      `json:"field"`
	Value CommentData `json:"value"`
}

type CommentData struct {
	ID        string `json:"id"`
	MediaID   string `json:"media_id"`
	Text      string `json:"text"`
	From      User   `json:"from"`
	ParentID  string `json:"parent_id,omitempty"`
}

type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

// Database models
type DMLog struct {
	ID          int
	UserID      string
	PostID      string
	CommentID   string
	SentAt      time.Time
	Status      string
	RetryCount  int
}

// DMJob for queue
type DMJob struct {
	UserID    string
	PostID    string
	CommentID string
	Text      string
	Username  string
	Timestamp time.Time
}

// Global dependencies
var (
	db     *sql.DB
	config Config
	dmQueue chan DMJob
)

func main() {
	// Load configuration
	config = loadConfig()
	
	// Initialize database
	initDB()
	defer db.Close()
	
	// Initialize DM queue
	dmQueue = make(chan DMJob, 100)
	
	// Start DM worker
	go dmWorker()
	
	// Setup HTTP routes
	http.HandleFunc("/webhook", webhookHandler)
	http.HandleFunc("/health", healthHandler)
	
	// Start server
	port := config.Port
	log.Printf("🚀 Instagram Auto-DM Server starting on port %s", port)
	log.Printf("📋 Keywords: %v", config.Keywords)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func loadConfig() Config {
	keywords := strings.Split(os.Getenv("KEYWORDS"), ",")
	for i := range keywords {
		keywords[i] = strings.TrimSpace(strings.ToLower(keywords[i]))
	}
	
	delay, _ := time.ParseDuration(os.Getenv("DM_DELAY"))
	if delay == 0 {
		delay = 1 * time.Minute
	}
	
	maxRetries := 3
	if mr := os.Getenv("MAX_RETRIES"); mr != "" {
		fmt.Sscanf(mr, "%d", &maxRetries)
	}
	
	return Config{
		Port:              getEnv("PORT", "8080"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		VerifyToken:       os.Getenv("VERIFY_TOKEN"),
		AccessToken:       os.Getenv("ACCESS_TOKEN"),
		IGBusinessID:      os.Getenv("IG_BUSINESS_ID"),
		Keywords:          keywords,
		DMMessage:         os.Getenv("DM_MESSAGE"),
		DMDelay:           delay,
		MaxRetries:        maxRetries,
		RetryBackoffBase:  2 * time.Second,
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func initDB() {
	var err error
	db, err = sql.Open("postgres", config.DatabaseURL)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	
	// Test connection
	if err := db.Ping(); err != nil {
		log.Fatal("Failed to ping database:", err)
	}
	
	// Create tables
	createTables()
	log.Println("✅ Database connected")
}

func createTables() {
	schema := `
	CREATE TABLE IF NOT EXISTS dm_logs (
		id SERIAL PRIMARY KEY,
		user_id VARCHAR(255) NOT NULL,
		post_id VARCHAR(255) NOT NULL,
		comment_id VARCHAR(255) NOT NULL,
		sent_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		status VARCHAR(50) NOT NULL,
		retry_count INTEGER DEFAULT 0,
		error_message TEXT,
		UNIQUE(user_id, post_id)
	);
	
	CREATE INDEX IF NOT EXISTS idx_user_post ON dm_logs(user_id, post_id);
	CREATE INDEX IF NOT EXISTS idx_status ON dm_logs(status);
	`
	
	if _, err := db.Exec(schema); err != nil {
		log.Fatal("Failed to create tables:", err)
	}
}

// Webhook handler
func webhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		handleWebhookVerification(w, r)
		return
	}
	
	if r.Method == http.MethodPost {
		handleWebhookEvent(w, r)
		return
	}
	
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func handleWebhookVerification(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")
	
	if mode == "subscribe" && token == config.VerifyToken {
		log.Println("✅ Webhook verified")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(challenge))
		return
	}
	
	log.Println("❌ Webhook verification failed")
	http.Error(w, "Verification failed", http.StatusForbidden)
}

func handleWebhookEvent(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Println("Error reading webhook body:", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	
	var payload WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Println("Error parsing webhook payload:", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	
	// Process each entry
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			if change.Field == "comments" {
				processComment(change.Value)
			}
		}
	}
	
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("EVENT_RECEIVED"))
}

func processComment(comment CommentData) {
	log.Printf("📝 New comment from @%s: %s", comment.From.Username, comment.Text)
	
	// Check if comment contains keyword
	commentLower := strings.ToLower(comment.Text)
	hasKeyword := false
	for _, keyword := range config.Keywords {
		if strings.Contains(commentLower, keyword) {
			hasKeyword = true
			break
		}
	}
	
	if !hasKeyword {
		log.Printf("⏭️  Comment doesn't contain keywords, skipping")
		return
	}
	
	// Check if DM already sent
	if isDuplicate(comment.From.ID, comment.MediaID) {
		log.Printf("⚠️  DM already sent to user %s for post %s", comment.From.ID, comment.MediaID)
		return
	}
	
	// Queue the DM job
	job := DMJob{
		UserID:    comment.From.ID,
		PostID:    comment.MediaID,
		CommentID: comment.ID,
		Text:      comment.Text,
		Username:  comment.From.Username,
		Timestamp: time.Now(),
	}
	
	dmQueue <- job
	log.Printf("✅ DM job queued for @%s", comment.From.Username)
}

func isDuplicate(userID, postID string) bool {
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM dm_logs WHERE user_id = $1 AND post_id = $2",
		userID, postID,
	).Scan(&count)
	
	if err != nil {
		log.Println("Error checking duplicate:", err)
		return false
	}
	
	return count > 0
}

// DM Worker - processes jobs with delay
func dmWorker() {
	for job := range dmQueue {
		// Wait for configured delay
		time.Sleep(config.DMDelay)
		
		log.Printf("⏰ Sending DM to @%s (after %v delay)", job.Username, config.DMDelay)
		
		// Send DM with retry logic
		err := sendDMWithRetry(job)
		
		if err != nil {
			log.Printf("❌ Failed to send DM to @%s after retries: %v", job.Username, err)
			logDM(job, "failed", err.Error())
		} else {
			log.Printf("✅ DM sent successfully to @%s", job.Username)
			logDM(job, "sent", "")
		}
	}
}

func sendDMWithRetry(job DMJob) error {
	var lastErr error
	
	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := config.RetryBackoffBase * time.Duration(1<<uint(attempt-1))
			log.Printf("🔄 Retry %d/%d for @%s after %v", attempt, config.MaxRetries, job.Username, backoff)
			time.Sleep(backoff)
		}
		
		err := sendDM(job.UserID, config.DMMessage)
		if err == nil {
			return nil
		}
		
		lastErr = err
		log.Printf("⚠️  Attempt %d failed: %v", attempt+1, err)
	}
	
	return lastErr
}

func sendDM(recipientID, message string) error {
	url := fmt.Sprintf("https://graph.facebook.com/v21.0/%s/messages", config.IGBusinessID)
	
	payload := map[string]interface{}{
		"recipient": map[string]string{
			"id": recipientID,
		},
		"message": map[string]string{
			"text": message,
		},
	}
	
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}
	
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/json")
	req.URL.RawQuery = fmt.Sprintf("access_token=%s", config.AccessToken)
	
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}
	
	return nil
}

func logDM(job DMJob, status, errorMsg string) {
	_, err := db.Exec(`
		INSERT INTO dm_logs (user_id, post_id, comment_id, status, error_message)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, post_id) DO UPDATE 
		SET retry_count = dm_logs.retry_count + 1,
		    status = $4,
		    error_message = $5,
		    sent_at = CURRENT_TIMESTAMP
	`, job.UserID, job.PostID, job.CommentID, status, errorMsg)
	
	if err != nil {
		log.Println("Error logging DM:", err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	// Check database connection
	if err := db.Ping(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "unhealthy",
			"error":  err.Error(),
		})
		return
	}
	
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "healthy",
		"queue_size":  len(dmQueue),
		"keywords":    config.Keywords,
	})
}