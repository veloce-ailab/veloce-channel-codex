package main

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestApplyInstructionsAndNormalRequestRules(t *testing.T) {
	payload := map[string]any{"model": "gpt-5-codex", "max_output_tokens": 100, "temperature": 0.5}
	applyInstructions(payload, "system", false)
	if payload["instructions"] != "system" {
		t.Fatalf("instructions = %#v", payload["instructions"])
	}
	payload["instructions"] = "existing"
	applyInstructions(payload, "system", true)
	if payload["instructions"] != "system\nexisting" {
		t.Fatalf("overridden instructions = %#v", payload["instructions"])
	}
}

func TestOAuthKeyClaimsAndRefreshWindow(t *testing.T) {
	claims, _ := json.Marshal(map[string]any{"email": "user@example.com", "https://api.openai.com/auth.chatgpt_account_id": "acct_123"})
	token := "header." + base64.RawURLEncoding.EncodeToString(claims) + ".signature"
	key := oauthKey{AccessToken: token, Expired: time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}
	fillTokenClaims(&key)
	if key.AccountID != "acct_123" || key.Email != "user@example.com" {
		t.Fatalf("claims = %#v", key)
	}
	if !key.needsRefresh(time.Now()) {
		t.Fatal("expected key expiring within 24 hours to refresh")
	}
	key.Expired = time.Now().Add(25 * time.Hour).UTC().Format(time.RFC3339)
	if key.needsRefresh(time.Now()) {
		t.Fatal("did not expect key beyond refresh window to refresh")
	}
}
