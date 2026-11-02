package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

func SendExpiryEmail(toEmail string, daysLeft int) error {
	apiKey := os.Getenv("RESEND_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("RESEND_API_KEY not set")
	}

	subject := "Your Auto-Pay Subscription is Expiring"
	htmlBody := fmt.Sprintf(`
		<div style="font-family: sans-serif; padding: 20px; background: #f9f9f9;">
			<div style="max-width: 600px; margin: 0 auto; background: white; padding: 30px; border-radius: 8px;">
				<h2 style="color: #333;">Subscription Expiring Soon</h2>
				<p style="color: #555;">Hey there,</p>
				<p style="color: #555;">Your Auto-Pay API key will expire in <strong>%d days</strong>.</p>
				<p style="color: #555;">To avoid service interruption, please recharge your account.</p>
				<a href="https://auto-payment.ximanta.xyz/dashboard" style="display: inline-block; padding: 10px 20px; background: #000; color: white; text-decoration: none; border-radius: 5px; margin-top: 20px;">Go to Dashboard</a>
			</div>
		</div>
	`, daysLeft)

	payload := map[string]interface{}{
		"from":    "noreply@auto-payment.ximanta.xyz",
		"to":      []string{toEmail},
		"subject": subject,
		"html":    htmlBody,
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("resend failed with status %d", resp.StatusCode)
	}
	return nil
}
