package main

import (
	"encoding/json"
	"net/http"
	"time"
)

// Analytics endpoint - Shows DM statistics
// Add this to your main.go routes:
// http.HandleFunc("/analytics", analyticsHandler)

type Analytics struct {
	TotalSent    int     `json:"total_sent"`
	TotalFailed  int     `json:"total_failed"`
	SuccessRate  float64 `json:"success_rate"`
	Last24Hours  int     `json:"last_24_hours"`
	TopPosts     []PostStat `json:"top_posts"`
}

type PostStat struct {
	PostID  string `json:"post_id"`
	DMCount int    `json:"dm_count"`
}

func analyticsHandler(w http.ResponseWriter, r *http.Request) {
	stats := Analytics{}
	
	// Total sent
	db.QueryRow("SELECT COUNT(*) FROM dm_logs WHERE status = 'sent'").Scan(&stats.TotalSent)
	
	// Total failed
	db.QueryRow("SELECT COUNT(*) FROM dm_logs WHERE status = 'failed'").Scan(&stats.TotalFailed)
	
	// Success rate
	total := stats.TotalSent + stats.TotalFailed
	if total > 0 {
		stats.SuccessRate = float64(stats.TotalSent) / float64(total) * 100
	}
	
	// Last 24 hours
	db.QueryRow(`
		SELECT COUNT(*) FROM dm_logs 
		WHERE sent_at > NOW() - INTERVAL '24 hours'
	`).Scan(&stats.Last24Hours)
	
	// Top posts
	rows, _ := db.Query(`
		SELECT post_id, COUNT(*) as count
		FROM dm_logs
		GROUP BY post_id
		ORDER BY count DESC
		LIMIT 5
	`)
	defer rows.Close()
	
	for rows.Next() {
		var ps PostStat
		rows.Scan(&ps.PostID, &ps.DMCount)
		stats.TopPosts = append(stats.TopPosts, ps)
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// Simple in-memory rate limiter
type RateLimiter struct {
	requests map[string][]time.Time
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
	}
}

// Allow checks if request is allowed (max 5 per minute)
func (rl *RateLimiter) Allow(key string) bool {
	now := time.Now()
	cutoff := now.Add(-1 * time.Minute)
	
	// Get existing requests
	requests := rl.requests[key]
	
	// Filter out old requests
	var valid []time.Time
	for _, t := range requests {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	
	// Check limit (5 per minute)
	if len(valid) >= 5 {
		return false
	}
	
	// Add new request
	valid = append(valid, now)
	rl.requests[key] = valid
	
	return true
}

// Usage in main.go:
// var limiter = NewRateLimiter()
// 
// Before sending DM:
// if !limiter.Allow(userID) {
//     log.Printf("Rate limited for user: %s", userID)
//     return
// }