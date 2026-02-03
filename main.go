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

	"github.com/chromedp/chromedp"
	"github.com/joho/godotenv"
	"github.com/rs/cors"
)

type AnalyzeRequest struct {
	URL  string `json:"url"`
	Text string `json:"text"`
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

func analyzeTextWithAI(text string, apiToken string) (*AIResponse, error) {
	apiURL := "https://router.huggingface.co/hf-inference/models/facebook/bart-large-mnli"

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

	fmt.Println("RAW AI RESPONSE:", string(body))

	type HFError struct {
		Error string `json:"error"`
	}
	var errResp HFError
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != "" {
		return nil, fmt.Errorf("API Error: %s", errResp.Error)
	}

	type HFResponseItem struct {
		Label string  `json:"label"`
		Score float64 `json:"score"`
	}
	var rawResponse []HFResponseItem

	if err := json.Unmarshal(body, &rawResponse); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %v", err)
	}

	var labels []string
	var scores []float64

	for _, item := range rawResponse {
		labels = append(labels, item.Label)
		scores = append(scores, item.Score)
	}

	finalResponse := &AIResponse{
		Sequence: text,
		Labels:   labels,
		Scores:   scores,
	}

	return finalResponse, nil
}

func analyzeHandler(w http.ResponseWriter, r *http.Request) {
	apiToken := os.Getenv("HF_API_TOKEN")

	var req AnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var articleText string

	if req.Text != "" {
		fmt.Println("Received direct text input (skipping scrape)")
		articleText = req.Text
	} else if req.URL != "" {
		fmt.Println("Received URL, starting scrape:", req.URL)
		
		allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), chromedp.DefaultExecAllocatorOptions[:]...)
		defer cancel()

		ctx, cancel := chromedp.NewContext(allocCtx)
		defer cancel()

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
		fmt.Println("Scraping finished.")
	} else {
		http.Error(w, "Please provide either a URL or Text", http.StatusBadRequest)
		return
	}

	if len(articleText) > 2000 {
		articleText = articleText[:2000]
	}

	fmt.Println("Sending text to AI for analysis...")

	analysis, err := analyzeTextWithAI(articleText, apiToken)
	if err != nil {
		log.Println("ERROR: AI analysis failed:", err)
		http.Error(w, "Failed to analyze the text", http.StatusInternalServerError)
		return
	}

	fmt.Println("AI analysis complete!")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(analysis)
}

func main() {
	godotenv.Load()

	mux := http.NewServeMux()
	
	mux.HandleFunc("/analyze", analyzeHandler) 
	mux.HandleFunc("/scrape", analyzeHandler) 

	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"POST"},
	})

	handler := c.Handler(mux)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Println("Starting server on port " + port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatal(err)
	}
}