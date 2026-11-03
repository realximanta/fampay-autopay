package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

var oauthConfig = &oauth2.Config{
	ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
	ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
	RedirectURL:  os.Getenv("GOOGLE_REDIRECT_URI"),
	Scopes: []string{
		"https://www.googleapis.com/auth/gmail.readonly",
		"https://www.googleapis.com/auth/gmail.settings.sharing",
		"https://www.googleapis.com/auth/gmail.settings.basic", // NEW: Required to create filters
		"https://www.googleapis.com/auth/userinfo.email",
	},
	Endpoint: google.Endpoint,
}

func GeneratePKCE() (string, string) {
	verifier := generateSecureRandomString(64)
	h := sha256.New()
	h.Write([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h.Sum(nil))
	return verifier, challenge
}

func GetLoginURL(state string, challenge string) string {
	return oauthConfig.AuthCodeURL(state, 
		oauth2.AccessTypeOffline, 
		oauth2.ApprovalForce,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
}

func ExchangeCode(ctx context.Context, code string, verifier string) (*oauth2.Token, error) {
	return oauthConfig.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", verifier))
}

func GetUserEmail(ctx context.Context, token *oauth2.Token) (string, error) {
	client := oauthConfig.Client(ctx, token)
	srv, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return "", err
	}
	profile, err := srv.Users.GetProfile("me").Do()
	if err != nil {
		return "", err
	}
	return profile.EmailAddress, nil
}

// SetupAutoForwarding creates a filter to forward ONLY FamPay emails
func SetupAutoForwarding(ctx context.Context, token *oauth2.Token) error {
	client := oauthConfig.Client(ctx, token)
	srv, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return err
	}

	targetEmail := "payments@auto-payment.ximanta.xyz"
	
	// 1. Request forwarding address (Requires settings.sharing)
	req := &gmail.ForwardingAddress{ForwardingEmail: targetEmail}
	_, err = srv.Users.Settings.ForwardingAddresses.Create("me", req).Do()
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		fmt.Println("Forwarding Create Error:", err)
	}

	// 2. Poll until Cloudflare Worker clicks the verification link
	verified := false
	for i := 0; i < 15; i++ {
		addr, err := srv.Users.Settings.ForwardingAddresses.Get("me", targetEmail).Do()
		if err == nil && addr.VerificationStatus == "accepted" {
			verified = true
			break
		}
		time.Sleep(2 * time.Second)
	}

	if !verified {
		return fmt.Errorf("forwarding address %s was not verified in time", targetEmail)
	}

	// 3. Check if we already created the filter to prevent duplicates
	existingFilters, err := srv.Users.Settings.Filters.List("me").Do()
	if err == nil {
		for _, f := range existingFilters.Filter {
			if f.Action != nil && f.Action.Forward == targetEmail {
				fmt.Println("✅ Auto-forwarding filter already exists.")
				return nil // Already set up
			}
		}
	}

	// 4. Create the Filter (Requires settings.basic)
	// This ensures ONLY FamPay emails are forwarded, protecting the rest of their inbox
	filter := &gmail.Filter{
		Criteria: &gmail.FilterCriteria{
			From: "famapp.in OR fampay.in OR famx.fampay.in",
		},
		Action: &gmail.FilterAction{
			Forward: targetEmail,
		},
	}

	_, err = srv.Users.Settings.Filters.Create("me", filter).Do()
	if err != nil {
		fmt.Println("Filter Create Error:", err)
		return err
	}
	
	fmt.Println("✅ Auto-forwarding filter created successfully for FamPay emails.")
	return nil
}
