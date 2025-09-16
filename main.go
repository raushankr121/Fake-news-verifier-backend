package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"github.com/chromedp/chromedp"
	"github.com/rs/cors" // Import the new package
)

// --- Structs (No Change) ---
type ScrapeRequest struct {
	URL string `json:"url"`
}
type AIRequestPayload struct {
	Inputs     string   `json:"inputs"`
	Parameters AIParams `json:"parameters"`
}
type AIParams struct {
	CandidateLabels []string `json:"candidate_labels"`
}
type AIResponse struct {
	Sequence string    `json:"sequence"`
	Labels   []string  `json:"labels"`
	Scores   []float64 `json:"scores"`
}

// --- analyzeTextWithAI Function (No Change) ---
func analyzeTextWithAI(text string, apiToken string) (*AIResponse, error) {
	// ... same as before
	apiURL := "https://api-inference.huggingface.co/models/MoritzLaurer/mDeBERTa-v3-base-mnli-xnli"

	payload := AIRequestPayload{
		Inputs: text,
		Parameters: AIParams{
			CandidateLabels: []string{"Factual and Objective", "Opinion and Bias"},
		},
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var aiResponse AIResponse
	if err := json.Unmarshal(body, &aiResponse); err != nil {
		return nil, err
	}

	return &aiResponse, nil
}

// --- scrapeHandler Function (No Change) ---
func scrapeHandler(w http.ResponseWriter, r *http.Request) {
	apiToken := os.Getenv("HF_API_TOKEN")
 // Replace with your token if needed

	var req ScrapeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	fmt.Println("1. Decoded URL successfully:", req.URL)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), chromedp.DefaultExecAllocatorOptions[:]...)
	defer cancel()

	
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()
	
	var articleText string
	err := chromedp.Run(ctx,
		chromedp.Navigate(req.URL),
		chromedp.WaitVisible(`#content`),
		chromedp.Text(`#content`, &articleText, chromedp.ByQuery),
	)

	if err != nil {
		log.Println("ERROR: Chromedp failed to scrape:", err)
		http.Error(w, "Failed to scrape the page", http.StatusInternalServerError)
		return
	}
	fmt.Println("2. Scraping finished. Total characters found:", len(articleText))
	
	if len(articleText) > 2000 {
		articleText = articleText[:2000]
	}

	fmt.Println("3. Sending text to AI for analysis...")
	analysis, err := analyzeTextWithAI(articleText, apiToken)
	if err != nil {
		log.Println("ERROR: AI analysis failed:", err)
		http.Error(w, "Failed to analyze the text", http.StatusInternalServerError)
		return
	}
	fmt.Println("4. AI analysis complete!")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(analysis)
}

// --- Main Server Function (THIS IS WHERE THE CHANGES ARE) ---
func main() {
	godotenv.Load()
    mux := http.NewServeMux()
    mux.HandleFunc("/scrape", scrapeHandler)

    c := cors.New(cors.Options{
    // CHANGE THIS LINE
    AllowedOrigins: []string{"*"}, 
    AllowedMethods: []string{"POST"},
})

    handler := c.Handler(mux)

    // --- THIS IS THE NEW LOGIC ---
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080" // Use 8080 as a fallback for local development
    }

    fmt.Println("Starting server on port " + port)
    // Use the new 'port' variable here instead of a fixed number
    if err := http.ListenAndServe(":"+port, handler); err != nil {
        log.Fatal(err)
    }
}