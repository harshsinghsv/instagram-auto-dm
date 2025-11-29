package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/julienschmidt/httprouter"
	_ "github.com/lib/pq"
)

// CONFIG STRUCT
type Config struct {
	Port             string
	DatabaseURL      string
	VerifyToken      string
	AccessToken      string
	IGBusinessID     string
	Keywords         []string
	DMMessage        string
	DMDelay          time.Duration
	MaxRetries       int
	RetryBackoffBase time.Duration
}

// WEBHOOK STRUCTS
type WebhookPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		ID      string   `json:"id"`
		Time    int64    `json:"time"`
		Changes []Change `json:"changes"`
	} `json:"entry"`
}

type Change struct {
	Field string      `json:"field"`
	Value CommentData `json:"value"`
}

type CommentData struct {
	ID       string `json:"id"`
	MediaID  string `json:"media_id"`
	Text     string `json:"text"`
	From     User   `json:"from"`
	ParentID string `json:"parent_id,omitempty"`
}

type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

// DATABASE MODEL
type DMLog struct {
	ID         int
	UserID     string
	PostID     string
	CommentID  string
	SentAt     time.Time
	Status     string
	RetryCount int
}

// JOB QUEUE STRUCT
type DMJob struct {
	UserID    string
	PostID    string
	CommentID string
	Text      string
	Username  string
	Timestamp time.Time
}

// GLOBALS
var (
	db      *sql.DB
	config  Config
	dmQueue chan DMJob
)

// CORS Middleware
func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Handle preflight requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ENTRY POINT
func main() {
	// Load .env before anything else
	err := godotenv.Load()
	if err != nil {
		log.Println("⚠️  .env file not found, using system env variables")
	}

	// Load configuration
	config = loadConfig()

	// Init DB
	initDB()
	defer db.Close()

	// DM queue
	dmQueue = make(chan DMJob, 100)

	// Start worker
	go dmWorker()

	// Routes
	router := httprouter.New()
	router.GET("/webhook", webhookGETHandler)
	router.POST("/webhook", webhookPOSTHandler)
	router.GET("/health", healthHandler)

	// Add test endpoint
	router.GET("/test", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		log.Println("✅ Test route called!")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "test OK"})
	})

	// Add production API routes
	log.Println("📌 Setting up production API routes...")
	setupRoutes(router)
	log.Println("✅ Production API routes registered")
	// Add CORS middleware wrapper
	corsRouter := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		router.ServeHTTP(w, r)
	})

	// Start server
	log.Printf("🚀 Instagram Auto-DM Server running on port %s", config.Port)
	log.Printf("📌 Keywords: %v", config.Keywords)
	log.Fatal(http.ListenAndServe(":"+config.Port, corsRouter))
}

// CONFIG LOADER
func loadConfig() Config {
	keywords := strings.Split(os.Getenv("KEYWORDS"), ",")
	for i := range keywords {
		keywords[i] = strings.TrimSpace(strings.ToLower(keywords[i]))
	}

	delay, _ := time.ParseDuration(os.Getenv("DM_DELAY"))
	if delay == 0 {
		delay = 30 * time.Second
	}

	maxRetries := 3
	if mr := os.Getenv("MAX_RETRIES"); mr != "" {
		fmt.Sscanf(mr, "%d", &maxRetries)
	}

	return Config{
		Port:             getEnv("PORT", "8080"),
		DatabaseURL:      getEnv("DATABASE_URL", ""),
		VerifyToken:      getEnv("VERIFY_TOKEN", ""),
		AccessToken:      getEnv("ACCESS_TOKEN", ""),
		IGBusinessID:     getEnv("IG_BUSINESS_ID", ""),
		Keywords:         keywords,
		DMMessage:        getEnv("DM_MESSAGE", "Thank you! 🙏"),
		DMDelay:          delay,
		MaxRetries:       maxRetries,
		RetryBackoffBase: 2 * time.Second,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// DATABASE INIT
func initDB() {
	var err error
	db, err = sql.Open("postgres", config.DatabaseURL)
	if err != nil {
		log.Fatal("❌ Failed to connect to DB:", err)
	}

	if err := db.Ping(); err != nil {
		log.Fatal("❌ DB ping failed:", err)
	}

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

	-- New Tables for Production API
	CREATE TABLE IF NOT EXISTS tbl_app_users (
		id SERIAL PRIMARY KEY,
		email VARCHAR(255) UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		name VARCHAR(255),
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS tbl_ig_accounts (
		id SERIAL PRIMARY KEY,
		app_user_id INTEGER REFERENCES tbl_app_users(id),
		platform_ig_account_id VARCHAR(255) UNIQUE NOT NULL,
		platform_user_account_id VARCHAR(255),
		username VARCHAR(255),
		name VARCHAR(255),
		access_token TEXT,
		platform VARCHAR(50),
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS tbl_products (
		id SERIAL PRIMARY KEY,
		ig_account_id INTEGER REFERENCES tbl_ig_accounts(id),
		name VARCHAR(255) NOT NULL,
		description TEXT,
		price DECIMAL(10, 2),
		image_url TEXT,
		product_link TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS tbl_dm_templates (
		id SERIAL PRIMARY KEY,
		ig_account_id INTEGER REFERENCES tbl_ig_accounts(id),
		product_id INTEGER REFERENCES tbl_products(id),
		template_name VARCHAR(255),
		message_text TEXT,
		include_download_link BOOLEAN DEFAULT FALSE,
		download_link TEXT,
		include_product_info BOOLEAN DEFAULT FALSE,
		is_default BOOLEAN DEFAULT FALSE,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	`

	_, err := db.Exec(schema)
	if err != nil {
		log.Fatal("❌ Failed to create tables:", err)
	}
}

// WEBHOOK HANDLERS
func webhookGETHandler(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	mode := r.URL.Query().Get("hub.mode")
	challenge := r.URL.Query().Get("hub.challenge")
	token := r.URL.Query().Get("hub.verify_token")

	if mode == "subscribe" && token != "" && token == config.VerifyToken {
		log.Println("WEBHOOK VERIFIED")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(challenge))
		return
	}
	w.WriteHeader(http.StatusForbidden)
}

func webhookPOSTHandler(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	log.Printf("\n\nWebhook received %s\n", timestamp)

	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Println("Error decoding webhook body:", err)
		w.WriteHeader(http.StatusOK)
		return
	}
	out, _ := json.MarshalIndent(payload, "", "  ")
	log.Println(string(out))

	// Process the webhook payload for comments
	if entries, ok := payload["entry"].([]interface{}); ok {
		for _, e := range entries {
			entry, _ := e.(map[string]interface{})
			if changes, ok := entry["changes"].([]interface{}); ok {
				for _, c := range changes {
					change, _ := c.(map[string]interface{})
					if field, ok := change["field"].(string); ok && field == "comments" {
						if value, ok := change["value"].(map[string]interface{}); ok {
							// Convert map to CommentData struct
							processCommentFromMap(value)
						}
					}
				}
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}

// COMMENT PROCESSOR (from map)
func processCommentFromMap(commentMap map[string]interface{}) {
	// Extract fields from map
	id, _ := commentMap["id"].(string)
	mediaID, _ := commentMap["media_id"].(string)
	text, _ := commentMap["text"].(string)

	fromMap, _ := commentMap["from"].(map[string]interface{})
	userID, _ := fromMap["id"].(string)
	username, _ := fromMap["username"].(string)

	c := CommentData{
		ID:      id,
		MediaID: mediaID,
		Text:    text,
		From: User{
			ID:       userID,
			Username: username,
		},
	}

	processComment(c)
}

// COMMENT PROCESSOR
func processComment(c CommentData) {
	text := strings.ToLower(c.Text)

	// Check keywords
	match := false
	for _, kw := range config.Keywords {
		if strings.Contains(text, kw) {
			match = true
			break
		}
	}
	if !match {
		return
	}

	// Duplicate check
	if isDuplicate(c.From.ID, c.MediaID) {
		log.Println("⚠️ Duplicate DM skipped")
		return
	}

	// Queue the job
	dmQueue <- DMJob{
		UserID:    c.From.ID,
		PostID:    c.MediaID,
		CommentID: c.ID,
		Text:      c.Text,
		Username:  c.From.Username,
		Timestamp: time.Now(),
	}

	log.Printf("📩 DM job queued for @%s", c.From.Username)
}

// DUPLICATE CHECKER
func isDuplicate(userID, postID string) bool {
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM dm_logs WHERE user_id = $1 AND post_id = $2",
		userID, postID,
	).Scan(&count)

	return err == nil && count > 0
}

// DM WORKER
func dmWorker() {
	for job := range dmQueue {
		log.Printf("⏳ Waiting %v before sending DM to @%s", config.DMDelay, job.Username)
		time.Sleep(config.DMDelay)
		log.Printf("📤 Sending DM to @%s (user: %s, post: %s)", job.Username, job.UserID, job.PostID)
		err := sendDMWithRetry(job)

		if err != nil {
			log.Printf("❌ DM send failed for @%s: %v", job.Username, err)
			logDM(job, "failed", err.Error())
		} else {
			log.Printf("✅ DM sent successfully to @%s", job.Username)
			logDM(job, "sent", "")
		}
	}
}

func sendDMWithRetry(job DMJob) error {
	var last error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := config.RetryBackoffBase * time.Duration(1<<uint(attempt-1))
			time.Sleep(backoff)
		}

		err := sendDM(job.UserID, config.DMMessage)
		if err == nil {
			return nil
		}

		last = err
	}

	return last
}

// DM SENDER
// Note: Instagram's 24-hour messaging rule applies:
// You can only send DMs to users who have messaged you in the last 24 hours.
// For development/testing, use test users from your Meta app.
func sendDM(userID, message string) error {
	url := fmt.Sprintf("https://graph.instagram.com/v15.0/%s/messages", config.IGBusinessID)

	body := map[string]any{
		"recipient": map[string]string{"id": userID},
		"message":   map[string]string{"text": message},
	}

	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.AccessToken)

	log.Printf("📡 API Call: POST %s", url)
	log.Printf("📦 Payload: %s", string(jsonBody))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ Request error: %v", err)
		return err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	log.Printf("📥 Response Status: %d", resp.StatusCode)
	log.Printf("📥 Response Body: %s", string(b))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		// Parse error for better debugging
		var errResp map[string]any
		if err := json.Unmarshal(b, &errResp); err == nil {
			if errData, ok := errResp["error"].(map[string]any); ok {
				if code, ok := errData["code"].(float64); ok && code == 10 {
					if subcode, ok := errData["error_subcode"].(float64); ok && subcode == 2534022 {
						return fmt.Errorf("24_hour_messaging_window_expired: User must message you first or within 24 hours")
					}
				}
			}
		}
		return fmt.Errorf("api_error_%d: %s", resp.StatusCode, string(b))
	}

	return nil
}

// DM LOGGING
func logDM(job DMJob, status, errMsg string) {
	_, err := db.Exec(`
		INSERT INTO dm_logs (user_id, post_id, comment_id, status, error_message)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, post_id) DO UPDATE
		SET retry_count = dm_logs.retry_count + 1,
		    status = $4,
		    error_message = $5,
		    sent_at = CURRENT_TIMESTAMP
	`, job.UserID, job.PostID, job.CommentID, status, errMsg)

	if err != nil {
		log.Println("❌ DM log error:", err)
	}
}

// HEALTH CHECK
func healthHandler(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	if err := db.Ping(); err != nil {
		w.WriteHeader(503)
		json.NewEncoder(w).Encode(map[string]string{"status": "unhealthy"})
		return
	}

	json.NewEncoder(w).Encode(map[string]any{
		"status":     "healthy",
		"queue_size": len(dmQueue),
		"keywords":   config.Keywords,
	})
}
