package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

type ReleaseResponse struct {
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
}

func HandleLatestRelease(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Try to get from cache first (saves GitHub API limits)
	var cachedData ReleaseResponse
	if err := KVGet("cache:latest_release", &cachedData); err == nil {
		json.NewEncoder(w).Encode(cachedData)
		return
	}

	req, _ := http.NewRequest("GET", "https://api.github.com/repos/realximanta/fampay-autopay/releases/latest", nil)
	req.Header.Set("User-Agent", "AutoPay-Backend")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	
	// Fallback if API fails or rate-limits
	if err != nil || resp.StatusCode != 200 {
		json.NewEncoder(w).Encode(ReleaseResponse{
			Version:     "v1.0.0",
			DownloadURL: "https://github.com/realximanta/fampay-autopay/releases",
		})
		return
	}
	defer resp.Body.Close()

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to parse release data"})
		return
	}

	var apkURL string
	for _, asset := range release.Assets {
		if strings.HasSuffix(strings.ToLower(asset.Name), ".apk") {
			apkURL = asset.BrowserDownloadURL
			break
		}
	}

	if apkURL == "" {
		apkURL = "https://github.com/realximanta/fampay-autopay/releases"
	}

	result := ReleaseResponse{
		Version:     release.TagName,
		DownloadURL: apkURL,
	}

	// Cache the result in KV for 10 minutes (600 seconds)
	KVPut("cache:latest_release", result, 600)

	json.NewEncoder(w).Encode(result)
}
