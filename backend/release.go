package main

import (
	"encoding/json"
	"net/http"
	"strings"
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

	// Fetch latest release from GitHub API
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/realximanta/fampay-autopay/releases/latest", nil)
	req.Header.Set("User-Agent", "AutoPay-Backend")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		// If the repo has no releases yet, return a fallback
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
	// Look for the .apk file in the release assets
	for _, asset := range release.Assets {
		if strings.HasSuffix(strings.ToLower(asset.Name), ".apk") {
			apkURL = asset.BrowserDownloadURL
			break
		}
	}

	// If no APK found, fallback to the releases page
	if apkURL == "" {
		apkURL = "https://github.com/realximanta/fampay-autopay/releases"
	}

	json.NewEncoder(w).Encode(ReleaseResponse{
		Version:     release.TagName,
		DownloadURL: apkURL,
	})
}
