package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"time"
)

var cfAccountID = os.Getenv("CF_ACCOUNT_ID")
var cfNamespaceID = os.Getenv("CF_NAMESPACE_ID")
var cfHistoryNamespaceID = os.Getenv("CF_HISTORY_NAMESPACE_ID")
var cfNotificationsNamespaceID = os.Getenv("CF_NOTIFICATIONS_NAMESPACE_ID")
var cfAPIToken = os.Getenv("CF_API_TOKEN")

var kvBaseURL = fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/storage/kv/namespaces/%s/values", cfAccountID, cfNamespaceID)
var historyBaseURL = fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/storage/kv/namespaces/%s/values", cfAccountID, cfHistoryNamespaceID)
var notificationsBaseURL = fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/storage/kv/namespaces/%s/values", cfAccountID, cfNotificationsNamespaceID)

// --- ENCRYPTION ---
func getEncryptionKey() []byte {
	secret := os.Getenv("TOKEN_ENCRYPTION_KEY")
	if secret == "" {
		secret = "fallback-key-change-me-in-render"
	}
	hash := sha256.Sum256([]byte(secret))
	return hash[:]
}

func EncryptToken(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	key := getEncryptionKey()
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func DecryptToken(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	key := getEncryptionKey()
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// --- HMAC SIGNATURE ---
func VerifyKVSignature(payload string, signature string) bool {
	secret := os.Getenv("KV_HMAC_SECRET")
	if secret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// --- STRUCTS ---
type KeyData struct {
	Email          string `json:"email"`
	ExpiresAt      int64  `json:"expires_at"`
	EncryptedToken string `json:"encrypted_token"`
}

type PendingData struct {
	Email  string `json:"email"`
	Amount int    `json:"amount"`
	Days   int    `json:"days"`
}

type PaymentPayload struct {
	Amount    string `json:"amount"`
	Timestamp int64  `json:"timestamp"`
	UTR       string `json:"utr"`
	Signature string `json:"signature"`
}

type HistoryEntry struct {
	Event     string `json:"event"`
	Amount    string `json:"amount"`
	Status    string `json:"status"`
	Timestamp int64  `json:"timestamp"`
}

type NotificationEntry struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	Timestamp int64  `json:"timestamp"`
	Read      bool   `json:"read"`
}

// --- CORE KV FUNCTIONS ---
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

// --- UTR TRACKING ---
func IsUTRUsed(utr string) bool {
	var data PaymentPayload
	if err := KVGet("utr:"+utr, &data); err == nil {
		return true
	}
	return false
}

func MarkUTRUsed(utr string, amount string, email string) {
	payload := map[string]interface{}{
		"amount":  amount,
		"email":   email,
		"used_at": fmt.Sprintf("%d", time.Now().Unix()),
	}
	KVPut("utr:"+utr, payload, 0)
}

// --- HISTORY KV FUNCTIONS ---
func LogHistory(email, event, amount, status string) {
	if cfHistoryNamespaceID == "" {
		return
	}
	ts := time.Now().Unix()
	key := fmt.Sprintf("history:%s:%d:%s", email, ts, generateSecureRandomString(4))
	entry := HistoryEntry{Event: event, Amount: amount, Status: status, Timestamp: ts}
	body, _ := json.Marshal(entry)

	req, _ := http.NewRequest("PUT", historyBaseURL+"/"+key, bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+cfAPIToken)
	req.Header.Set("Content-Type", "application/json")
	req.URL.RawQuery = "expiration_ttl=7776000"
	http.DefaultClient.Do(req)
}

func GetUserHistory(email string) ([]HistoryEntry, error) {
	if cfHistoryNamespaceID == "" {
		return []HistoryEntry{}, nil
	}
	listURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/storage/kv/namespaces/%s/keys?prefix=history:%s:", cfAccountID, cfHistoryNamespaceID, email)
	req, _ := http.NewRequest("GET", listURL, nil)
	req.Header.Set("Authorization", "Bearer "+cfAPIToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var res struct {
		Result []struct {
			Name string `json:"name"`
		} `json:"result"`
	}
	json.NewDecoder(resp.Body).Decode(&res)

	var entries []HistoryEntry
	start := 0
	if len(res.Result) > 30 {
		start = len(res.Result) - 30
	}

	for _, k := range res.Result[start:] {
		var entry HistoryEntry
		valURL := historyBaseURL + "/" + k.Name
		vReq, _ := http.NewRequest("GET", valURL, nil)
		vReq.Header.Set("Authorization", "Bearer "+cfAPIToken)
		vResp, err := http.DefaultClient.Do(vReq)
		if err == nil && vResp.StatusCode == 200 {
			b, _ := io.ReadAll(vResp.Body)
			json.Unmarshal(b, &entry)
			entries = append(entries, entry)
		}
		if vResp != nil {
			vResp.Body.Close()
		}
	}

	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}

	return entries, nil
}

// --- NOTIFICATIONS KV FUNCTIONS ---
func LogNotification(email, title, body string) {
	if cfNotificationsNamespaceID == "" {
		return
	}
	ts := time.Now().UnixNano()
	key := fmt.Sprintf("notification:%s:%d", email, ts)
	
	entry := NotificationEntry{
		Title:     title,
		Body:      body,
		Timestamp: time.Now().Unix(),
		Read:      false,
	}

	jsonData, _ := json.Marshal(entry)
	req, _ := http.NewRequest("PUT", notificationsBaseURL+"/"+key, bytes.NewBuffer(jsonData))
	req.Header.Set("Authorization", "Bearer "+cfAPIToken)
	req.Header.Set("Content-Type", "application/json")
	req.URL.RawQuery = "expiration_ttl=7776000" 
	
	resp, err := http.DefaultClient.Do(req)
	if err == nil && resp != nil {
		resp.Body.Close()
	}

	// Trigger real-time push notification to the device
	go SendPushNotification(email, title, body)
}

func GetNotifications(email string) ([]NotificationEntry, error) {
	if cfNotificationsNamespaceID == "" {
		return []NotificationEntry{}, nil
	}

	listURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/storage/kv/namespaces/%s/keys?prefix=notification:%s:", cfAccountID, cfNotificationsNamespaceID, email)
	req, _ := http.NewRequest("GET", listURL, nil)
	req.Header.Set("Authorization", "Bearer "+cfAPIToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var res struct {
		Result []struct {
			Name string `json:"name"`
		} `json:"result"`
	}
	json.NewDecoder(resp.Body).Decode(&res)

	var entries []NotificationEntry
	start := 0
	if len(res.Result) > 50 {
		start = len(res.Result) - 50
	}

	for _, k := range res.Result[start:] {
		var entry NotificationEntry
		valURL := notificationsBaseURL + "/" + k.Name
		vReq, _ := http.NewRequest("GET", valURL, nil)
		vReq.Header.Set("Authorization", "Bearer "+cfAPIToken)
		vResp, err := http.DefaultClient.Do(vReq)
		if err == nil && vResp.StatusCode == 200 {
			b, _ := io.ReadAll(vResp.Body)
			json.Unmarshal(b, &entry)
			entries = append(entries, entry)
		}
		if vResp != nil {
			vResp.Body.Close()
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp > entries[j].Timestamp
	})

	return entries, nil
}

func MarkNotificationsRead(email string) {
	if cfNotificationsNamespaceID == "" {
		return
	}

	listURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/storage/kv/namespaces/%s/keys?prefix=notification:%s:", cfAccountID, cfNotificationsNamespaceID, email)
	req, _ := http.NewRequest("GET", listURL, nil)
	req.Header.Set("Authorization", "Bearer "+cfAPIToken)
	resp, _ := http.DefaultClient.Do(req)
	if resp == nil {
		return
	}
	defer resp.Body.Close()

	var res struct {
		Result []struct {
			Name string `json:"name"`
		} `json:"result"`
	}
	json.NewDecoder(resp.Body).Decode(&res)

	for _, k := range res.Result {
		var entry NotificationEntry
		valURL := notificationsBaseURL + "/" + k.Name
		vReq, _ := http.NewRequest("GET", valURL, nil)
		vReq.Header.Set("Authorization", "Bearer "+cfAPIToken)
		vResp, err := http.DefaultClient.Do(vReq)
		if err == nil && vResp.StatusCode == 200 {
			b, _ := io.ReadAll(vResp.Body)
			json.Unmarshal(b, &entry)
			
			if !entry.Read {
				entry.Read = true
				updatedData, _ := json.Marshal(entry)
				putReq, _ := http.NewRequest("PUT", valURL, bytes.NewBuffer(updatedData))
				putReq.Header.Set("Authorization", "Bearer "+cfAPIToken)
				putReq.Header.Set("Content-Type", "application/json")
				putResp, _ := http.DefaultClient.Do(putReq)
				if putResp != nil {
					putResp.Body.Close()
				}
			}
		}
		if vResp != nil {
			vResp.Body.Close()
		}
	}
}

func SaveFCMToken(email, token string) {
	KVPut("fcm:"+email, token, 0)
}
