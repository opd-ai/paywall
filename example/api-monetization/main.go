package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/opd-ai/paywall"
)

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func main() {
	store := paywall.NewMemoryStore()

	pw, err := paywall.NewPaywall(paywall.Config{
		PriceInBTC:       0.0001,
		TestNet:          true,
		Store:            store,
		PaymentTimeout:   time.Hour * 1,
		MinConfirmations: 1,
	})
	if err != nil {
		log.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	http.HandleFunc("/", apiDocsHandler)
	http.HandleFunc("/api/free/status", freeStatusHandler)
	http.Handle("/api/premium/analytics", pw.Middleware(http.HandlerFunc(premiumAnalyticsHandler)))
	http.Handle("/api/premium/forecast", pw.Middleware(http.HandlerFunc(premiumForecastHandler)))
	http.Handle("/api/premium/insights", pw.Middleware(http.HandlerFunc(premiumInsightsHandler)))

	log.Println("API server running on :8080")
	log.Println("Free endpoint: http://localhost:8080/api/free/status")
	log.Println("Premium endpoints require payment (0.0001 BTC)")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func apiDocsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head><title>API Documentation</title></head>
<body>
	<h1>Data Analytics API</h1>
	<h2>Free Endpoints</h2>
	<ul>
		<li><code>GET /api/free/status</code> - API health status</li>
	</ul>
	<h2>Premium Endpoints (0.0001 BTC per request)</h2>
	<ul>
		<li><code>GET /api/premium/analytics</code> - Advanced analytics data</li>
		<li><code>GET /api/premium/forecast</code> - Predictive forecasting</li>
		<li><code>GET /api/premium/insights</code> - AI-powered insights</li>
	</ul>
	<p>Premium endpoints return JSON data after payment confirmation.</p>
</body>
</html>
`)
}

func freeStatusHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"status":    "operational",
			"timestamp": time.Now().Unix(),
			"version":   "1.0.0",
		},
	})
}

func premiumAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"users":    12543,
			"revenue":  87234.56,
			"growth":   "+23.4%",
			"requests": 156789,
			"period":   "last_30_days",
		},
	})
}

func premiumForecastHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"next_month_users":   15200,
			"revenue_projection": 102450.00,
			"confidence":         0.87,
			"trend":              "upward",
			"model":              "LSTM",
		},
	})
}

func premiumInsightsHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"key_insights": []string{
				"Peak usage hours: 14:00-16:00 UTC",
				"Top user segment: Enterprise (45%)",
				"Churn risk: Low (3.2%)",
				"Recommended action: Expand EU presence",
			},
			"score":      92,
			"updated_at": time.Now().Unix(),
		},
	})
}

func respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
