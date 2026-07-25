package main

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	plugin "github.com/veloce-ailab/veloce-plugin-helper"
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

func TestDecodeInputUsesSelectedAccountPool(t *testing.T) {
	credentials, _ := json.Marshal(oauthKey{AccessToken: "access", AccountID: "acct_123"})
	ctx := &plugin.ActionContext{
		RequestID: "request-1",
		Values:    map[string]any{"channel": map[string]any{"config": map[string]any{"pool_id": "shared"}}},
		Settings:  plugin.Values{"pools": []map[string]any{{"id": "shared", "name": "Shared", "enabled": true}}, "accounts": []map[string]any{{"id": "account-1", "pool_id": "shared", "enabled": true, "credentials_json": string(credentials)}}},
	}
	_, accounts, indexes, decoded, err := decodeInput(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 || len(indexes) != 1 || indexes[0] != 0 || len(decoded) != 1 || decoded[0].AccountID != "acct_123" {
		t.Fatalf("decoded accounts = %#v, indexes = %#v, keys = %#v", accounts, indexes, decoded)
	}
	if index, err := selectAccount(ctx, len(decoded)); err != nil || index != 0 {
		t.Fatalf("account selection = %d, %v", index, err)
	}
}

func TestManifestDeclaresPoolBackedUpstream(t *testing.T) {
	if len(app.Manifest.Upstreams) != 1 {
		t.Fatalf("upstreams = %#v", app.Manifest.Upstreams)
	}
	upstream := app.Manifest.Upstreams[0]
	if upstream.Protocol != plugin.UpstreamProtocolResponses || upstream.PrepareAction != "upstream.prepare" || upstream.DefaultBaseURL != defaultBaseURL {
		t.Fatalf("upstream = %#v", upstream)
	}
	if len(upstream.Config.Fields) != 1 || upstream.Config.Fields[0].OptionsFrom != "pools" {
		t.Fatalf("config = %#v", upstream.Config)
	}
}
