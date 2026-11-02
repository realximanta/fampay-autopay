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
	"time"
)

var cfAccountID = os.Getenv("CF_ACCOUNT_ID")
var cfNamespaceID = os.Getenv("CF_NAMESPACE_ID")
var cfAPIToken = os.Getenv("CF_API_TOKEN")
var kvBaseURL = fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/storage/kv/namespaces/%s/values", cfAccountID, cfNamespaceID)

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
