package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/julienschmidt/httprouter"
	"golang.org/x/crypto/bcrypt"
)

// ============================================
// MULTI-USER PRODUCTION API STRUCTURE
// ============================================

// Models for API requests/responses

type CreatorLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type CreatorLoginResponse struct {
	Token  string `json:"token"`
	UserID int64  `json:"user_id"`
	Name   string `json:"name"`
}

type ConnectIGAccountRequest struct {
	AccessToken    string `json:"access_token"`
	IGBusinessID   string `json:"ig_business_id"`
	IGUsername     string `json:"ig_username"`
	IGBusinessName string `json:"ig_business_name"`
}

type CreateProductRequest struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	ImageURL    string  `json:"image_url"`
	ProductLink string  `json:"product_link"`
}

type CreateDMTemplateRequest struct {
	ProductID           int    `json:"product_id"`
	TemplateName        string `json:"template_name"`
	MessageText         string `json:"message_text"`
	IncludeDownloadLink bool   `json:"include_download_link"`
	DownloadLink        string `json:"download_link"`
	IncludeProductInfo  bool   `json:"include_product_info"`
	IsDefault           bool   `json:"is_default"`
}

type PublishPostRequest struct {
	Caption      string `json:"caption"`
	ImageURL     string `json:"image_url"`
	ProductID    int    `json:"product_id"`
	DMTemplateID int    `json:"dm_template_id"`
}

type PublishReelRequest struct {
	Caption      string `json:"caption"`
	VideoURL     string `json:"video_url"`
	ThumbnailURL string `json:"thumbnail_url"`
	ProductID    int    `json:"product_id"`
	DMTemplateID int    `json:"dm_template_id"`
}

type CommentWebhookPayload struct {
	Entry []struct {
		ID      string `json:"id"`
		Changes []struct {
			Field string `json:"field"`
			Value struct {
				ID      string `json:"id"`
				MediaID string `json:"media_id"`
				Text    string `json:"text"`
				From    struct {
					ID       string `json:"id"`
					Username string `json:"username"`
				} `json:"from"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

type LiveChatWebhookPayload struct {
	Entry []struct {
		ID        string `json:"id"`
		Messaging []struct {
			Sender struct {
				ID string `json:"id"`
			} `json:"sender"`
			Message struct {
				Text string `json:"text"`
				Mid  string `json:"mid"`
			} `json:"message"`
			Timestamp int64 `json:"timestamp"`
		} `json:"messaging"`
	} `json:"entry"`
}

// JWT Claims
type UserClaims struct {
	UserID int64  `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

var jwtSecret = []byte("your-secret-key-change-this-in-prod") // TODO: Move to env var

// ============================================
// API ENDPOINTS
// ============================================

// Auth Endpoints
func creatorLoginHandler(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CreatorLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Authenticate user
	userID, email, name, err := authenticateUser(req.Email, req.Password)
	if err != nil {
		log.Printf("Auth failed for %s: %v", req.Email, err)
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Generate JWT token
	token, err := generateJWT(userID, email)
	if err != nil {
		http.Error(w, "Token generation failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(CreatorLoginResponse{
		Token:  token,
		UserID: userID,
		Name:   name,
	})
}

// Signup Handler
func creatorSignupHandler(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Validate inputs
	if req.Email == "" || req.Password == "" || req.Name == "" {
		http.Error(w, "Email, password, and name are required", http.StatusBadRequest)
		return
	}

	// Create user
	userID, err := createUser(req.Email, req.Password, req.Name)
	if err != nil {
		log.Printf("Failed to create user: %v", err)
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "already exists") {
			http.Error(w, "Email already registered", http.StatusConflict)
		} else {
			http.Error(w, "Failed to create account", http.StatusInternalServerError)
		}
		return
	}

	// Generate JWT token
	token, err := generateJWT(userID, req.Email)
	if err != nil {
		http.Error(w, "Token generation failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(CreatorLoginResponse{
		Token:  token,
		UserID: userID,
		Name:   req.Name,
	})
}

// Connect Instagram Account
func connectIGAccountHandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Verify user token
	claims, err := verifyJWT(r.Header.Get("Authorization"))
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req ConnectIGAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	userID := fmt.Sprintf("%d", claims.UserID)

	// Connect IG account to user
	accountID, err := connectInstagramAccount(
		userID,
		req.AccessToken,
		req.IGBusinessID,
		req.IGUsername,
		req.IGBusinessName,
	)
	if err != nil {
		log.Printf("Failed to connect IG account: %v", err)
		http.Error(w, "Failed to connect account", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"account_id": accountID,
		"message":    "Instagram account connected successfully",
	})
}

// Create Product
func createProductHandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	accountID := p.ByName("account_id")
	if _, err := verifyJWT(r.Header.Get("Authorization")); err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreateProductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	productID, err := createProduct(accountID, req.Name, req.Description, req.Price, req.ImageURL, req.ProductLink)
	if err != nil {
		http.Error(w, "Failed to create product", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"product_id": productID,
		"message":    "Product created successfully",
	})
}

// Create DM Template
func createDMTemplateHandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	accountID := p.ByName("account_id")
	if _, err := verifyJWT(r.Header.Get("Authorization")); err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreateDMTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	templateID, err := createDMTemplate(
		accountID,
		req.ProductID,
		req.TemplateName,
		req.MessageText,
		req.IncludeDownloadLink,
		req.DownloadLink,
		req.IncludeProductInfo,
	)
	if err != nil {
		http.Error(w, "Failed to create template", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"template_id": templateID,
		"message":     "DM template created successfully",
	})
}

// Publish Post
func publishPostHandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	accountID := p.ByName("account_id")

	// Verify JWT token - allow for testing if no auth header
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		if _, err := verifyJWT(authHeader); err != nil {
			log.Printf("JWT verification failed: %v", err)
			http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
			return
		}
	}

	var req PublishPostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("Failed to decode request: %v", err)
		http.Error(w, "Invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("Publishing post for account %s with caption: %s and image URL: %s", accountID, req.Caption, req.ImageURL)

	postID, err := publishPost(accountID, req.Caption, req.ImageURL, req.ProductID, req.DMTemplateID)
	if err != nil {
		log.Printf("Failed to publish post: %v", err)
		http.Error(w, "Failed to publish post: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"post_id": postID,
		"message": "Post published successfully",
	})
}

// Publish Reel
func publishReelHandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	accountID := p.ByName("account_id")
	if _, err := verifyJWT(r.Header.Get("Authorization")); err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req PublishReelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	reelID, err := publishReel(accountID, req.Caption, req.VideoURL, req.ThumbnailURL, req.ProductID, req.DMTemplateID)
	if err != nil {
		log.Printf("Failed to publish reel: %v", err)
		http.Error(w, "Failed to publish reel: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"reel_id": reelID,
		"message": "Reel published successfully",
	})
}

// Comment Webhook
func commentWebhookHandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	accountID := p.ByName("account_id")

	if r.Method == http.MethodGet {
		handleWebhookVerification(w, r)
		return
	}

	if r.Method == http.MethodPost {
		var payload CommentWebhookPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			log.Printf("Failed to decode webhook: %v", err)
			w.WriteHeader(http.StatusOK)
			return
		}

		// Process comments asynchronously
		go processCommentWebhook(accountID, payload)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "received"})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// Live Chat Webhook (for live streams)
func liveChatWebhookHandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	accountID := p.ByName("account_id")

	if r.Method == http.MethodPost {
		var payload LiveChatWebhookPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			log.Printf("Failed to decode live chat webhook: %v", err)
			w.WriteHeader(http.StatusOK)
			return
		}

		// Process live chat messages asynchronously
		go processLiveChatWebhook(accountID, payload)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "received"})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// ============================================
// HELPER FUNCTIONS (DATABASE OPERATIONS)
// ============================================

func createUser(email, password, name string) (int64, error) {
	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, fmt.Errorf("failed to hash password: %v", err)
	}

	var userID int64
	err = db.QueryRow(
		"INSERT INTO tbl_app_users (email, password_hash, name) VALUES ($1, $2, $3) RETURNING id",
		email, string(hashedPassword), name,
	).Scan(&userID)

	if err != nil {
		return 0, err
	}

	log.Printf("User created successfully: %s (ID: %d)", email, userID)
	return userID, nil
}

func authenticateUser(email, password string) (int64, string, string, error) {
	var userID int64
	var name string
	var passwordHash string

	err := db.QueryRow("SELECT id, name, password_hash FROM tbl_app_users WHERE email = $1", email).Scan(&userID, &name, &passwordHash)
	if err != nil {
		return 0, "", "", err
	}

	err = bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password))
	if err != nil {
		return 0, "", "", fmt.Errorf("invalid password")
	}

	return userID, email, name, nil
}

func generateJWT(userID int64, email string) (string, error) {
	claims := UserClaims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func verifyJWT(authHeader string) (*UserClaims, error) {
	if authHeader == "" {
		return nil, fmt.Errorf("missing authorization header")
	}

	tokenString := ""
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		tokenString = authHeader[7:]
	} else {
		return nil, fmt.Errorf("invalid authorization header format")
	}

	token, err := jwt.ParseWithClaims(tokenString, &UserClaims{}, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*UserClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

func connectInstagramAccount(userID, accessToken, igID, username, businessName string) (string, error) {
	// Verify token is valid with Instagram API first
	verifyURL := fmt.Sprintf("https://graph.instagram.com/v18.0/me?access_token=%s", accessToken)
	resp, err := http.Get(verifyURL)
	if err != nil {
		log.Printf("Failed to verify Instagram token: %v", err)
		return "", fmt.Errorf("invalid access token or network error")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Instagram API verification failed: %s", string(body))
		return "", fmt.Errorf("access token verification failed: %d", resp.StatusCode)
	}

	var verifyResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&verifyResp)

	// Store IG account in database
	var accountID string
	insertQuery := `
		INSERT INTO tbl_ig_accounts (
			app_user_id, platform_ig_account_id, platform_user_account_id,
			username, name, access_token, platform
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (platform_ig_account_id) 
		DO UPDATE SET 
			access_token = $6,
			username = $4,
			updated_at = CURRENT_TIMESTAMP
		RETURNING id
	`

	err = db.QueryRow(
		insertQuery,
		userID,
		igID,
		igID,
		username,
		businessName,
		accessToken,
		"instagram",
	).Scan(&accountID)

	if err != nil {
		log.Printf("Failed to store IG account in database: %v", err)
		return "", fmt.Errorf("database error: %v", err)
	}

	log.Printf("IG account %s connected successfully with ID: %s", igID, accountID)
	return accountID, nil
}

func createProduct(accountID string, name, description string, price float64, imageURL, productLink string) (int, error) {
	// Insert product into tbl_products
	var productID int
	query := `
		INSERT INTO tbl_products (ig_account_id, name, description, price, image_url, product_link)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`

	err := db.QueryRow(query, accountID, name, description, price, imageURL, productLink).Scan(&productID)
	if err != nil {
		log.Printf("Failed to create product: %v", err)
		return 0, fmt.Errorf("database error: %v", err)
	}

	log.Printf("Product created successfully with ID: %d", productID)
	return productID, nil
}

func createDMTemplate(accountID string, productID int, templateName, messageText string, includeLink bool, downloadLink string, includeProductInfo bool) (int, error) {
	// Insert DM template into tbl_dm_templates
	var templateID int
	query := `
		INSERT INTO tbl_dm_templates (
			ig_account_id, product_id, template_name, message_text,
			include_download_link, download_link, include_product_info, is_default
		) VALUES ($1, $2, $3, $4, $5, $6, $7, true)
		RETURNING id
	`

	err := db.QueryRow(
		query,
		accountID,
		productID,
		templateName,
		messageText,
		includeLink,
		downloadLink,
		includeProductInfo,
	).Scan(&templateID)

	if err != nil {
		log.Printf("Failed to create DM template: %v", err)
		return 0, fmt.Errorf("database error: %v", err)
	}

	log.Printf("DM template created successfully with ID: %d", templateID)
	return templateID, nil
}

func publishPost(accountID, caption, imageURL string, productID, dmTemplateID int) (string, error) {
	// Validate inputs
	if caption == "" || imageURL == "" {
		return "", fmt.Errorf("caption and image URL are required")
	}

	// Get account details from database
	var igUserID, accessToken string
	err := db.QueryRow(
		`SELECT platform_ig_account_id, access_token FROM tbl_ig_accounts WHERE id = $1`,
		accountID,
	).Scan(&igUserID, &accessToken)
	if err != nil {
		log.Printf("Failed to get account details: %v", err)
		return "", fmt.Errorf("account not found: %v", err)
	}

	if igUserID == "" || accessToken == "" {
		return "", fmt.Errorf("missing account credentials")
	}

	// Step 1: Validate image URL is accessible
	if !isValidImageURL(imageURL) {
		log.Printf("Invalid or unreachable image URL: %s", imageURL)
		return "", fmt.Errorf("image URL is invalid or unreachable - please check the URL and try again")
	}

	// Step 2: Create media container for image
	containerID, err := createImageContainer(igUserID, accessToken, caption, imageURL)
	if err != nil {
		log.Printf("Failed to create image container: %v", err)
		return "", fmt.Errorf("failed to create image container: %v", err)
	}

	if containerID == "" {
		return "", fmt.Errorf("no container ID returned from Instagram API")
	}

	// Step 3: Publish the media
	publishURL := fmt.Sprintf(
		"https://graph.instagram.com/v18.0/%s/media_publish?access_token=%s",
		igUserID,
		accessToken,
	)

	payload := map[string]string{
		"creation_id": containerID,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal payload: %v", err)
		return "", fmt.Errorf("payload error: %v", err)
	}

	req, err := http.NewRequest("POST", publishURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		log.Printf("Failed to create HTTP request: %v", err)
		return "", fmt.Errorf("request error: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Failed to publish post (network error): %v", err)
		return "", fmt.Errorf("network error while publishing: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		log.Printf("Instagram API error response: %s (status: %d)", string(body), resp.StatusCode)
		return "", fmt.Errorf("instagram API error: %d - %s", resp.StatusCode, string(body))
	}

	var publishResp map[string]interface{}
	if err := json.Unmarshal(body, &publishResp); err != nil {
		log.Printf("Failed to decode response: %v", err)
		return "", fmt.Errorf("failed to decode response: %v", err)
	}

	postID, ok := publishResp["id"].(string)
	if !ok {
		if val, exists := publishResp["id"]; exists {
			postID = fmt.Sprintf("%v", val)
		} else {
			return "", fmt.Errorf("no post ID in response")
		}
	}
	log.Printf("Post published successfully with ID: %s", postID)
	return postID, nil
}

func publishReel(accountID, caption, videoURL, thumbnailURL string, productID, dmTemplateID int) (string, error) {
	// Get account details from database
	var igUserID, accessToken string
	err := db.QueryRow(
		`SELECT platform_ig_account_id, access_token FROM tbl_ig_accounts WHERE id = $1`,
		accountID,
	).Scan(&igUserID, &accessToken)
	if err != nil {
		log.Printf("Failed to get account details: %v", err)
		return "", fmt.Errorf("account not found: %v", err)
	}

	if igUserID == "" || accessToken == "" {
		return "", fmt.Errorf("missing account credentials")
	}

	// Step 1: Create media container for video
	containerID, err := createVideoContainer(igUserID, accessToken, caption, videoURL, thumbnailURL)
	if err != nil {
		log.Printf("Failed to create video container: %v", err)
		return "", err
	}

	// Step 2: Publish the media
	publishURL := fmt.Sprintf(
		"https://graph.instagram.com/v18.0/%s/media_publish?access_token=%s",
		igUserID,
		accessToken,
	)

	payload := map[string]string{
		"creation_id": containerID,
	}

	payloadBytes, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", publishURL, bytes.NewBuffer(payloadBytes))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Failed to publish reel: %v", err)
		return "", fmt.Errorf("network error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Instagram API error response: %s (status: %d)", string(body), resp.StatusCode)
		return "", fmt.Errorf("instagram API error: %d - %s", resp.StatusCode, string(body))
	}

	var publishResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&publishResp); err != nil {
		log.Printf("Failed to decode response: %v", err)
		return "", err
	}

	reelID, ok := publishResp["id"].(string)
	if !ok {
		reelID = fmt.Sprintf("%v", publishResp["id"])
	}
	log.Printf("Reel published successfully with ID: %s", reelID)
	return reelID, nil
}

// Helper function to create image media container
// Helper function to validate image URL
func isValidImageURL(imageURL string) bool {
	if imageURL == "" {
		return false
	}

	// Check if it's a valid HTTP(S) URL
	if !strings.HasPrefix(imageURL, "http://") && !strings.HasPrefix(imageURL, "https://") {
		return false
	}

	// Try to HEAD the URL to validate it's accessible
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Head(imageURL)
	if err != nil {
		log.Printf("Failed to validate image URL: %v", err)
		return false
	}
	defer resp.Body.Close()

	// Check if response is successful and content-type is image
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("Image URL returned status %d", resp.StatusCode)
		return false
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "image") {
		log.Printf("Invalid content type: %s", contentType)
		return false
	}

	return true
}

func createImageContainer(igUserID, accessToken, caption, imageURL string) (string, error) {
	containerURL := fmt.Sprintf(
		"https://graph.instagram.com/v18.0/%s/media?access_token=%s",
		igUserID,
		accessToken,
	)

	payload := map[string]string{
		"image_url": imageURL,
		"caption":   caption,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal payload: %v", err)
		return "", fmt.Errorf("payload error: %v", err)
	}

	req, err := http.NewRequest("POST", containerURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		log.Printf("Failed to create request: %v", err)
		return "", fmt.Errorf("request error: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Failed to reach Instagram API: %v", err)
		return "", fmt.Errorf("network error reaching Instagram API: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		log.Printf("Failed to create image container: %s (status: %d)", string(body), resp.StatusCode)
		return "", fmt.Errorf("failed to create container: status %d - %s", resp.StatusCode, string(body))
	}

	var respData map[string]interface{}
	err = json.Unmarshal(body, &respData)
	if err != nil {
		log.Printf("Failed to parse container response: %v", err)
		return "", fmt.Errorf("failed to parse response: %v", err)
	}

	if respData["id"] == nil {
		return "", fmt.Errorf("no ID in container response")
	}

	return fmt.Sprintf("%v", respData["id"]), nil
}

// Helper function to create video media container
func createVideoContainer(igUserID, accessToken, caption, videoURL, thumbnailURL string) (string, error) {
	containerURL := fmt.Sprintf(
		"https://graph.instagram.com/v18.0/%s/media?access_token=%s",
		igUserID,
		accessToken,
	)

	payload := map[string]string{
		"video_url":     videoURL,
		"thumbnail_url": thumbnailURL,
		"caption":       caption,
		"media_type":    "REELS",
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal payload: %v", err)
		return "", fmt.Errorf("payload error: %v", err)
	}

	req, err := http.NewRequest("POST", containerURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		log.Printf("Failed to create request: %v", err)
		return "", fmt.Errorf("request error: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Failed to reach Instagram API: %v", err)
		return "", fmt.Errorf("network error reaching Instagram API: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		log.Printf("Failed to create video container: %s (status: %d)", string(body), resp.StatusCode)
		return "", fmt.Errorf("failed to create container: status %d - %s", resp.StatusCode, string(body))
	}

	var respData map[string]interface{}
	err = json.Unmarshal(body, &respData)
	if err != nil {
		log.Printf("Failed to parse video container response: %v", err)
		return "", fmt.Errorf("failed to parse response: %v", err)
	}

	if respData["id"] == nil {
		return "", fmt.Errorf("no ID in container response")
	}

	return fmt.Sprintf("%v", respData["id"]), nil
}

func handleWebhookVerification(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	if mode == "subscribe" && token == "DDYTDTYF" {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(challenge))
		return
	}

	w.WriteHeader(http.StatusForbidden)
}

func processCommentWebhook(accountID string, payload CommentWebhookPayload) {
	// Parse comments from webhook
	// Store commenter in tbl_ig_users if not exists
	// Store comment in tbl_comments
	// Get DM template for post
	// Queue DM to be sent

	log.Printf("Processing %d comment entries for account %s", len(payload.Entry), accountID)

	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			if change.Field == "comments" {
				v := change.Value
				log.Printf("Comment from @%s: %s", v.From.Username, v.Text)

				// TODO: Store commenter and comment data
				// TODO: Match against DM template
				// TODO: Queue DM send
			}
		}
	}
}

func processLiveChatWebhook(accountID string, payload LiveChatWebhookPayload) {
	// Parse live chat messages
	// Store user in tbl_ig_users
	// Store message in tbl_live_chat_messages

	log.Printf("Processing live chat messages for account %s", accountID)

	for _, entry := range payload.Entry {
		for _, msg := range entry.Messaging {
			log.Printf("Live chat from user %s", msg.Sender.ID)
			// TODO: Store live chat participant data
		}
	}
}

// ============================================
// MAIN & ROUTER SETUP
// ============================================

func testPostImageHandler(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	log.Println("🧪 Test endpoint called - attempting to post image directly")

	// Use credentials from env
	igUserID := os.Getenv("IG_BUSINESS_ID")
	accessToken := os.Getenv("ACCESS_TOKEN")

	if igUserID == "" || accessToken == "" {
		log.Println("❌ Missing credentials")
		http.Error(w, "Missing IG_BUSINESS_ID or ACCESS_TOKEN", http.StatusBadRequest)
		return
	}

	log.Printf("📌 Using IG User ID: %s", igUserID)
	log.Printf("📌 Using Access Token: %s...", accessToken[:20])

	// Step 1: Create media container
	log.Println("📝 Step 1: Creating media container...")
	caption := "Test Post from Backend - " + time.Now().Format("2006-01-02 15:04:05")
	imageURL := "https://upload.wikimedia.org/wikipedia/commons/thumb/a/a7/Camponotus_flavomarginatus_ant.jpg/320px-Camponotus_flavomarginatus_ant.jpg"

	containerURL := fmt.Sprintf(
		"https://graph.instagram.com/v18.0/%s/media?access_token=%s",
		igUserID,
		accessToken,
	)

	payload := map[string]string{
		"image_url": imageURL,
		"caption":   caption,
	}

	payloadBytes, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", containerURL, bytes.NewBuffer(payloadBytes))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ Failed to create container: %v", err)
		http.Error(w, fmt.Sprintf("Failed to create container: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("📥 Container Response Status: %d", resp.StatusCode)
	log.Printf("📥 Container Response Body: %s", string(body))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		log.Printf("❌ Failed to create container: %s", string(body))
		http.Error(w, fmt.Sprintf("Failed to create container: %s", string(body)), http.StatusInternalServerError)
		return
	}

	var respData map[string]interface{}
	json.Unmarshal(body, &respData)

	containerID := fmt.Sprintf("%v", respData["id"])
	if containerID == "" || containerID == "<nil>" {
		log.Printf("❌ No container ID in response: %v", respData)
		http.Error(w, "No container ID returned", http.StatusInternalServerError)
		return
	}

	log.Printf("✅ Container created with ID: %s", containerID)

	// Step 2: Publish the media
	log.Println("📤 Step 2: Publishing media...")
	publishURL := fmt.Sprintf(
		"https://graph.instagram.com/v18.0/%s/media_publish?access_token=%s",
		igUserID,
		accessToken,
	)

	publishPayload := map[string]string{
		"creation_id": containerID,
	}

	publishPayloadBytes, _ := json.Marshal(publishPayload)
	publishReq, _ := http.NewRequest("POST", publishURL, bytes.NewBuffer(publishPayloadBytes))
	publishReq.Header.Set("Content-Type", "application/json")

	publishResp, err := client.Do(publishReq)
	if err != nil {
		log.Printf("❌ Failed to publish: %v", err)
		http.Error(w, fmt.Sprintf("Failed to publish: %v", err), http.StatusInternalServerError)
		return
	}
	defer publishResp.Body.Close()

	publishBody, _ := io.ReadAll(publishResp.Body)
	log.Printf("📥 Publish Response Status: %d", publishResp.StatusCode)
	log.Printf("📥 Publish Response Body: %s", string(publishBody))

	if publishResp.StatusCode != http.StatusOK && publishResp.StatusCode != http.StatusCreated {
		log.Printf("❌ Failed to publish: %s", string(publishBody))
		http.Error(w, fmt.Sprintf("Failed to publish: %s", string(publishBody)), http.StatusInternalServerError)
		return
	}

	var publishData map[string]interface{}
	json.Unmarshal(publishBody, &publishData)

	postID := fmt.Sprintf("%v", publishData["id"])
	log.Printf("✅ Post published successfully with ID: %s", postID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "success",
		"post_id":      postID,
		"message":      "Image posted to Instagram successfully!",
		"container_id": containerID,
	})
}

func testPostReelHandler(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	log.Println("🧪 Test reel endpoint - checking reel infrastructure")

	// Use credentials from env
	igUserID := os.Getenv("IG_BUSINESS_ID")
	accessToken := os.Getenv("ACCESS_TOKEN")

	if igUserID == "" || accessToken == "" {
		log.Println("❌ Missing credentials")
		http.Error(w, "Missing IG_BUSINESS_ID or ACCESS_TOKEN", http.StatusBadRequest)
		return
	}

	// Step 1: Test that we can reach Instagram API and understand the flow
	log.Println("📝 Step 1: Testing reel creation flow...")

	// Note: To actually post a reel, you need to provide a video URL that Instagram can access
	// The video must be in a format Instagram accepts (MP4 with H.264 codec)
	// and must be publicly downloadable by Instagram's servers

	caption := "Test Reel - " + time.Now().Format("15:04:05")

	// Get video URL from query parameter
	videoURL := r.URL.Query().Get("video_url")
	if videoURL == "" {
		videoURL = "https://cdn.pixabay.com/media/videos/videos_download_3840x2160_25559/a0000_original_media_video_025559.mp4"
	}
	log.Printf("📌 Using Video URL: %s", videoURL)

	containerURL := fmt.Sprintf(
		"https://graph.instagram.com/v18.0/%s/media?access_token=%s",
		igUserID,
		accessToken,
	)

	// Create reel container with just required fields
	payload := map[string]interface{}{
		"video_url":  videoURL,
		"caption":    caption,
		"media_type": "REELS",
	}

	payloadBytes, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", containerURL, bytes.NewBuffer(payloadBytes))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ Failed to create reel container: %v", err)
		http.Error(w, fmt.Sprintf("Failed to create reel container: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("📥 Container Response Status: %d", resp.StatusCode)
	log.Printf("📥 Container Response Body: %s", string(body))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		log.Printf("❌ Failed to create reel container: %s", string(body))
		http.Error(w, fmt.Sprintf("Failed to create reel container: %s", string(body)), http.StatusInternalServerError)
		return
	}

	var respData map[string]interface{}
	json.Unmarshal(body, &respData)

	containerID := fmt.Sprintf("%v", respData["id"])
	if containerID == "" || containerID == "<nil>" {
		log.Printf("❌ No container ID in response: %v", respData)
		http.Error(w, "No container ID returned", http.StatusInternalServerError)
		return
	}

	log.Printf("✅ Reel container created with ID: %s", containerID)

	// Wait for Instagram to process the video
	log.Println("⏳ Waiting for Instagram to process video (checking status every 5 seconds)...")

	var finalStatus string = "UNKNOWN"
	for i := 0; i < 24; i++ { // Wait up to 2 minutes
		time.Sleep(5 * time.Second)
		log.Printf("   Checking status... (attempt %d/24)", i+1)

		// Query just the status field, not non-existent fields
		statusURL := fmt.Sprintf(
			"https://graph.instagram.com/v18.0/%s?fields=status&access_token=%s",
			containerID,
			accessToken,
		)

		statusReq, _ := http.NewRequest("GET", statusURL, nil)
		statusResp, _ := client.Do(statusReq)
		statusBody, _ := io.ReadAll(statusResp.Body)
		statusResp.Body.Close()

		var statusData map[string]interface{}
		json.Unmarshal(statusBody, &statusData)

		if status, ok := statusData["status"].(string); ok {
			finalStatus = status
			log.Printf("   📊 Status: %s", status)

			// Stop waiting if we get a terminal state
			if status == "FINISHED" || status == "READY" {
				log.Println("   ✅ Video processing complete!")
				break
			} else if status == "ERROR" {
				log.Println("❌ Video processing returned ERROR status")
				break
			}
		} else {
			// Check if there's an error in the response
			if errField, hasErr := statusData["error"]; hasErr {
				log.Printf("   API Error: %v", errField)
			} else {
				log.Printf("   Unexpected response: %s", string(statusBody))
			}
		}
	}

	log.Printf("📤 Final video status: %s - attempting to publish...", finalStatus)

	// Try to publish
	publishURL := fmt.Sprintf(
		"https://graph.instagram.com/v18.0/%s/media_publish?access_token=%s",
		igUserID,
		accessToken,
	)

	publishPayload := map[string]string{
		"creation_id": containerID,
	}

	publishPayloadBytes, _ := json.Marshal(publishPayload)
	publishReq, _ := http.NewRequest("POST", publishURL, bytes.NewBuffer(publishPayloadBytes))
	publishReq.Header.Set("Content-Type", "application/json")

	publishResp, err := client.Do(publishReq)
	if err != nil {
		log.Printf("❌ Failed to publish: %v", err)
		http.Error(w, fmt.Sprintf("Failed to publish: %v", err), http.StatusInternalServerError)
		return
	}
	defer publishResp.Body.Close()

	publishBody, _ := io.ReadAll(publishResp.Body)
	log.Printf("📥 Publish Response Status: %d", publishResp.StatusCode)
	log.Printf("📥 Publish Response Body: %s", string(publishBody))

	w.Header().Set("Content-Type", "application/json")

	if publishResp.StatusCode != http.StatusOK && publishResp.StatusCode != http.StatusCreated {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "error",
			"error":  string(publishBody),
			"hint":   "Video URL must be publicly accessible to Instagram's servers. Try with a different video URL: ?video_url=YOUR_URL",
		})
		return
	}

	var publishData map[string]interface{}
	json.Unmarshal(publishBody, &publishData)

	reelID := fmt.Sprintf("%v", publishData["id"])
	log.Printf("✅ Reel published successfully with ID: %s", reelID)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "success",
		"reel_id":      reelID,
		"message":      "Reel posted successfully!",
		"container_id": containerID,
	})
}

func setupRoutes(router *httprouter.Router) {
	// Auth routes
	router.POST("/api/auth/login", creatorLoginHandler)
	router.POST("/api/auth/signup", creatorSignupHandler)

	// Account management routes
	router.POST("/api/creators/:user_id/connect-ig", connectIGAccountHandler)

	// Product routes
	router.POST("/api/accounts/:account_id/products", createProductHandler)

	// DM Template routes
	router.POST("/api/accounts/:account_id/dm-templates", createDMTemplateHandler)

	// Content publishing routes
	router.POST("/api/accounts/:account_id/posts", publishPostHandler)
	router.POST("/api/accounts/:account_id/reels", publishReelHandler)

	// Webhook routes
	router.GET("/api/accounts/:account_id/webhook/comments", commentWebhookHandler)
	router.POST("/api/accounts/:account_id/webhook/comments", commentWebhookHandler)
	router.POST("/api/accounts/:account_id/webhook/live-chat", liveChatWebhookHandler)

	// Test endpoint - posts image directly from backend
	router.GET("/api/test-post-image", testPostImageHandler)

	// Test endpoint - posts reel directly from backend
	router.GET("/api/test-post-reel", testPostReelHandler)
}
