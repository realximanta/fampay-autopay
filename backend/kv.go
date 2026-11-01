package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

var cfAccountID = os.Getenv("CF_ACCOUNT_ID")
var cfNamespaceID = os.Getenv("CF_NAMESPACE_ID")
var cfAPIToken = os.Getenv("CF_API_TOKEN")
var kvBaseURL = fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/storage/kv/namespaces/%s/values", cfAccountID, cfNamespaceID)

type KeyData struct {
	Email     string `json:"email"`
	ExpiresAt int64  `json:"expires_at"`
}

type PendingData struct {
	Email  string `json:"email"`
	Amount int    `json:"amount"`
	Days   int    `json:"days"`
}

func KVGet(key string, v interface{}) error {
	req, _ := http.NewRequest("GET", kvBaseURL+"/"+key, nil)
	req.Header.Set("Authorization", "Bearer "+cfAPIToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return fmt.Errorf("not found")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return json.Unmarshal(body, v)
}

func KVPut(key string, v interface{}, ttl int) error {
	body, _ := json.Marshal(v)
	req, _ := http.NewRequest("PUT", kvBaseURL+"/"+key, bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+cfAPIToken)
	req.Header.Set("Content-Type", "application/json")
	if ttl > 0 {
		req.URL.RawQuery = fmt.Sprintf("expiration_ttl=%d", ttl)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return fmt.Errorf("kv put failed: %v", err)
	}
	return nil
}

func KVDelete(key string) {
	req, _ := http.NewRequest("DELETE", kvBaseURL+"/"+key, nil)
	req.Header.Set("Authorization", "Bearer "+cfAPIToken)
	http.DefaultClient.Do(req)
}
