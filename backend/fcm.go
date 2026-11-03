package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"golang.org/x/oauth2/google"
)

// Structs for parsing the Service Account JSON and FCM Payload
type ServiceAccount struct {
	ProjectID string `json:"project_id"`
}

type FCMMessage struct {
	Message struct {
		Token string `json:"token"`
		Notification struct {
			Title string `json:"title"`
			Body  string `json:"body"`
		} `json:"notification"`
	} `json:"message"`
}

// SendPushNotification sends a real-time notification to the user's device
func SendPushNotification(email, title, body string) {
	// 1. Get the FCM token saved during login/registration
	var fcmToken string
	if err := KVGet("fcm:"+email, &fcmToken); err != nil || fcmToken == "" {
		return // User hasn't opened the app recently, skip push
	}

	// 2. Load Firebase Credentials
	credsJSON := os.Getenv("FIREBASE_SERVICE_ACCOUNT_JSON")
	if credsJSON == "" {
		log.Println("⚠️ FIREBASE_SERVICE_ACCOUNT_JSON not set, skipping push")
		return
	}

	var sa ServiceAccount
	if err := json.Unmarshal([]byte(credsJSON), &sa); err != nil {
		log.Println("❌ Failed to parse Firebase credentials:", err)
		return
	}

	// 3. Get OAuth2 Token
	ctx := context.Background()
	creds, err := google.CredentialsFromJSON(ctx, []byte(credsJSON), "https://www.googleapis.com/auth/firebase.messaging")
	if err != nil {
		log.Println("❌ Failed to create Firebase credentials:", err)
		return
	}
	token, err := creds.TokenSource.Token()
	if err != nil {
		log.Println("❌ Failed to get Firebase access token:", err)
		return
	}

	// 4. Build FCM Payload
	var msg FCMMessage
	msg.Message.Token = fcmToken
	msg.Message.Notification.Title = title
	msg.Message.Notification.Body = body

	payload, _ := json.Marshal(msg)
	url := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", sa.ProjectID)

	// 5. Send Request
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Println("❌ FCM Request Failed:", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("❌ FCM Error (status %d)\n", resp.StatusCode)
	} else {
		log.Printf("🔔 Push Notification sent to %s\n", email)
	}
}
