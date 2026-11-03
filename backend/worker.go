package main

import (
	"fmt"
	"log"
	"time"
)

// StartEmailWorker launches a background goroutine that checks for expiries
func StartEmailWorker() {
	log.Println("⏰ Background Email & Notification Worker started.")
	
	// Run immediately once on boot
	go processExpiryEmails()

	// Then run every hour
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		for range ticker.C {
			processExpiryEmails()
		}
	}()
}

func processExpiryEmails() {
	log.Println("🔍 Checking for expiring/expired subscriptions...")
	
	users, err := ListAllUsers()
	if err != nil {
		log.Println("❌ Worker error: Failed to list users:", err)
		return
	}

	now := time.Now().Unix()
	
	for _, u := range users {
		// Skip users without an expiry (infinite)
		if u.ExpiresAt == 0 {
			continue
		}

		daysLeft := int((u.ExpiresAt - now) / 86400)
		
		// 1. Handle Expired (0 days or less)
		if daysLeft <= 0 {
			key := fmt.Sprintf("email_sent:expired:%s", u.Email)
			var dummy string
			if KVGet(key, &dummy) != nil {
				// Key doesn't exist, so we haven't sent it recently
				SendExpiryEmail(u.Email, 0)
				LogNotification(u.Email, "Subscription Expired", "Your API key has expired. Please recharge to resume service.")
				KVPut(key, "sent", 86400) // Mark as sent for 24 hours
				log.Printf("📧 Sent EXPIRED email & notification to %s\n", u.Email)
			}
		}

		// 2. Handle Expiring Tomorrow (1 day left)
		if daysLeft == 1 {
			key := fmt.Sprintf("email_sent:expiring:%s", u.Email)
			var dummy string
			if KVGet(key, &dummy) != nil {
				// Key doesn't exist
				SendExpiryEmail(u.Email, 1)
				LogNotification(u.Email, "Subscription Expiring Soon", "Your API key will expire in 1 day. Recharge now to avoid interruption.")
				KVPut(key, "sent", 86400) // Mark as sent for 24 hours
				log.Printf("📧 Sent EXPIRING email & notification to %s\n", u.Email)
			}
		}
	}
	log.Println("✅ Expiry check complete.")
}
