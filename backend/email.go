package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

// Generic email sender using Resend API
func sendEmail(toEmail, subject, htmlBody string) error {
	apiKey := os.Getenv("RESEND_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("RESEND_API_KEY not set")
	}

	payload := map[string]interface{}{
		"from":    "Auto-Pay <noreply@ximanta.xyz>",
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
	
	log.Printf("📧 Email sent to %s: %s\n", toEmail, subject)
	return nil
}

// 1. Welcome Email (First time sign up)
func SendWelcomeEmail(toEmail string) {
	subject := "Welcome to Auto-Pay! 🚀"
	html := `
	<div style="font-family: 'Inter', sans-serif; padding: 40px 20px; background-color: #000000; color: #ededed;">
		<div style="max-width: 500px; margin: 0 auto; background-color: #0a0a0a; padding: 40px; border-radius: 12px; border: 1px solid #222222; text-align: center;">
			<h1 style="color: #ffffff; margin-bottom: 20px;">Welcome to Auto-Pay</h1>
			<p style="color: #888888; font-size: 16px; line-height: 1.6; margin-bottom: 30px;">
				Your account has been successfully created. You can now integrate our API to automatically verify FamApp UPI payments in your bots or websites.
			</p>
			<a href="https://auto-payment.ximanta.xyz/dashboard" style="display: inline-block; padding: 12px 24px; background-color: #ffffff; color: #000000; text-decoration: none; border-radius: 8px; font-weight: 600;">Go to Dashboard</a>
			<p style="color: #444444; font-size: 12px; margin-top: 40px;">
				If you didn't create this account, please ignore this email.
			</p>
		</div>
	</div>`
	go sendEmail(toEmail, subject, html)
}

// 2. Thank You Email (On Recharge)
func SendThankYouEmail(toEmail string, amount float64, daysAdded int) {
	subject := "Recharge Successful - Auto-Pay"
	html := fmt.Sprintf(`
	<div style="font-family: 'Inter', sans-serif; padding: 40px 20px; background-color: #000000; color: #ededed;">
		<div style="max-width: 500px; margin: 0 auto; background-color: #0a0a0a; padding: 40px; border-radius: 12px; border: 1px solid #222222; text-align: center;">
			<h1 style="color: #22c55e; margin-bottom: 20px;">Payment Received!</h1>
			<p style="color: #888888; font-size: 16px; line-height: 1.6; margin-bottom: 20px;">
				Thank you for recharging your Auto-Pay subscription.
			</p>
			<div style="background-color: #000000; padding: 20px; border-radius: 8px; border: 1px solid #222222; margin-bottom: 30px;">
				<p style="color: #ffffff; font-size: 24px; font-weight: bold; margin: 0 0 10px 0;">₹%.2f</p>
				<p style="color: #888888; font-size: 14px; margin: 0;">Added %d days to your API key</p>
			</div>
			<a href="https://auto-payment.ximanta.xyz/dashboard" style="display: inline-block; padding: 12px 24px; background-color: #ffffff; color: #000000; text-decoration: none; border-radius: 8px; font-weight: 600;">View Dashboard</a>
		</div>
	</div>`, amount, daysAdded)
	go sendEmail(toEmail, subject, html)
}

// 3. Expiry Email (1 day left OR Expired)
func SendExpiryEmail(toEmail string, daysLeft int) {
	subject := "Your Auto-Pay Subscription is Expiring"
	color := "#facc15" // Yellow
	msg := fmt.Sprintf("Your API key will expire in <strong>%d days</strong>. Please recharge soon to avoid service interruption.", daysLeft)
	
	if daysLeft <= 0 {
		subject = "URGENT: Your Auto-Pay Subscription has Expired"
		color = "#ef4444" // Red
		msg = "Your API key has <strong>expired</strong>. Your API requests will be rejected until you recharge."
	}

	html := fmt.Sprintf(`
	<div style="font-family: 'Inter', sans-serif; padding: 40px 20px; background-color: #000000; color: #ededed;">
		<div style="max-width: 500px; margin: 0 auto; background-color: #0a0a0a; padding: 40px; border-radius: 12px; border: 1px solid #222222; text-align: center;">
			<h1 style="color: %s; margin-bottom: 20px;">Subscription Alert</h1>
			<p style="color: #888888; font-size: 16px; line-height: 1.6; margin-bottom: 30px;">
				%s
			</p>
			<a href="https://auto-payment.ximanta.xyz/dashboard" style="display: inline-block; padding: 12px 24px; background-color: #ffffff; color: #000000; text-decoration: none; border-radius: 8px; font-weight: 600;">Recharge Now</a>
		</div>
	</div>`, color, msg)
	go sendEmail(toEmail, subject, html)
}
