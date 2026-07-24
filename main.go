package main

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"time"

	plugin "github.com/WindyPear-Team/veloce-plugin-helper"
)

const (
	pluginID       = "codex-subscription"
	upstreamID     = "codex"
	defaultBaseURL = "https://chatgpt.com"
	oauthTokenURL  = "https://auth.openai.com/oauth/token"
	oauthClientID  = "app_EMoamEEZ73f0CkXaXp7hrann"
)

var app = plugin.New(plugin.Manifest{
	ID:          pluginID,
	Name:        "ChatGPT Subscription (Codex)",
	Version:     "0.1.0",
	Description: "Use one ChatGPT Codex subscription through Veloce on multiple authorized devices.",
	Author:      "WindyPear Team",
	Permissions: []string{"plugin.settings.global", "plugin.channel.http"},
	Settings: plugin.SettingsSchema{Type: "form", Fields: []plugin.SettingsField{
		{Type: "input", Name: "base_url", Label: "Codex Base URL", Default: defaultBaseURL, Description: "Normally https://chatgpt.com."},
		{Type: "textarea", Name: "system_prompt", Label: "Default system prompt", Description: "Used when a Responses request has no instructions."},
		{Type: "switch", Name: "system_prompt_override", Label: "Prefix existing instructions", Default: false},
	}},
	Frontend: plugin.Frontend{Sidebar: []plugin.SidebarItem{{Label: "Codex Subscription", Path: "status"}}, Routes: []plugin.Route{{
		Path: "status", Title: "ChatGPT Subscription (Codex)", Description: "Configure the shared Codex upstream channel, then route every authorized device to it.",
		Page: plugin.Page(plugin.Card("Shared subscription", plugin.Text("OAuth credentials are stored in the upstream channel API Key, not in device clients. Each device should use its own Veloce API key with access to the intended user channel."))),
	}}},
})

type upstreamManifest struct {
	plugin.Manifest
	Upstreams []upstreamType `json:"upstreams"`
}

type upstreamType struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Protocol      string `json:"protocol"`
	PrepareAction string `json:"prepare_action"`
	RefreshAction string `json:"refresh_action"`
}

var manifestDocument = upstreamManifest{Manifest: app.Manifest, Upstreams: []upstreamType{{
	ID: upstreamID, Name: "ChatGPT Subscription (Codex)", Protocol: "responses",
	Description:   "ChatGPT Codex backend with centrally refreshed OAuth credentials.",
	PrepareAction: "upstream.prepare", RefreshAction: "upstream.refresh",
}}}

type oauthKey struct {
	IDToken      string `json:"id_token,omitempty"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	AccountID    string `json:"account_id"`
	LastRefresh  string `json:"last_refresh,omitempty"`
	Email        string `json:"email,omitempty"`
	Type         string `json:"type,omitempty"`
	Expired      string `json:"expired,omitempty"`
}

type upstreamInput struct {
	Channel struct {
		BaseURL string `json:"base_url"`
		APIKey  string `json:"api_key"`
	} `json:"channel"`
	Request struct {
		Payload map[string]any `json:"payload"`
		Stream  bool           `json:"stream"`
		Compact bool           `json:"compact"`
	} `json:"request"`
}

func init() {
	app.Action("upstream.prepare", prepare)
	app.Action("upstream.refresh", refresh)
}

func prepare(ctx *plugin.ActionContext) (any, error) {
	input, key, err := decodeInput(ctx)
	if err != nil {
		return nil, err
	}
	if key.needsRefresh(time.Now()) {
		if strings.TrimSpace(key.RefreshToken) == "" {
			return nil, plugin.ErrorWithCode("codex_token_expired", "Codex access token expired and no refresh token is configured")
		}
		if err := refreshOAuth(ctx, &key); err != nil {
			return nil, err
		}
	}
	return upstreamRequest(ctx, input, key)
}

func refresh(ctx *plugin.ActionContext) (any, error) {
	_, key, err := decodeInput(ctx)
	if err != nil {
		return nil, err
	}
	if key.needsRefresh(time.Now()) {
		if strings.TrimSpace(key.RefreshToken) == "" {
			return nil, plugin.ErrorWithCode("codex_token_expired", "Codex access token expired and no refresh token is configured")
		}
		if err := refreshOAuth(ctx, &key); err != nil {
			return nil, err
		}
	}
	encodedKey, _ := json.Marshal(key)
	return map[string]any{"ok": true, "api_key": string(encodedKey)}, nil
}

func refreshOAuth(ctx *plugin.ActionContext, key *oauthKey) error {
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {key.RefreshToken}, "client_id": {oauthClientID}}
	response, err := ctx.Host.HTTP(ctx, plugin.HTTPRequest{Method: "POST", URL: oauthTokenURL, Headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Accept": "application/json"}, Body: form.Encode()})
	if err != nil {
		return err
	}
	if response.Status < 200 || response.Status >= 300 {
		return plugin.ErrorWithCode("codex_refresh_failed", "Codex OAuth refresh request was rejected")
	}
	var refreshed map[string]any
	if err := json.Unmarshal([]byte(response.Body), &refreshed); err != nil {
		return plugin.ErrorWithCode("codex_refresh_failed", "Codex OAuth refresh response is invalid")
	}
	accessToken, _ := refreshed["access_token"].(string)
	refreshToken, _ := refreshed["refresh_token"].(string)
	expiresIn := integerValue(refreshed["expires_in"])
	if strings.TrimSpace(accessToken) == "" || strings.TrimSpace(refreshToken) == "" || expiresIn <= 0 {
		return plugin.ErrorWithCode("codex_refresh_failed", "Codex OAuth refresh response is incomplete")
	}
	now := time.Now().UTC()
	key.AccessToken = strings.TrimSpace(accessToken)
	key.RefreshToken = strings.TrimSpace(refreshToken)
	key.LastRefresh = now.Format(time.RFC3339)
	key.Expired = now.Add(time.Duration(expiresIn) * time.Second).Format(time.RFC3339)
	key.Type = "codex"
	fillTokenClaims(key)
	return nil
}

func decodeInput(ctx *plugin.ActionContext) (upstreamInput, oauthKey, error) {
	var input upstreamInput
	raw, err := json.Marshal(ctx.Values)
	if err != nil || json.Unmarshal(raw, &input) != nil {
		return upstreamInput{}, oauthKey{}, plugin.ErrorWithCode("invalid_request", "invalid Codex upstream invocation")
	}
	var key oauthKey
	if err := json.Unmarshal([]byte(strings.TrimSpace(input.Channel.APIKey)), &key); err != nil {
		return upstreamInput{}, oauthKey{}, plugin.ErrorWithCode("invalid_codex_key", "channel API Key must be a Codex OAuth JSON object")
	}
	key.AccessToken = strings.TrimSpace(key.AccessToken)
	key.AccountID = strings.TrimSpace(key.AccountID)
	if key.AccessToken == "" || key.AccountID == "" {
		fillTokenClaims(&key)
	}
	if key.AccessToken == "" || key.AccountID == "" {
		return upstreamInput{}, oauthKey{}, plugin.ErrorWithCode("invalid_codex_key", "Codex key requires access_token and account_id")
	}
	return input, key, nil
}

func upstreamRequest(ctx *plugin.ActionContext, input upstreamInput, key oauthKey) (map[string]any, error) {
	payload := cloneMap(input.Request.Payload)
	applyInstructions(payload, ctx.Settings.String("system_prompt", ""), ctx.Settings.Bool("system_prompt_override", false))
	path := "/backend-api/codex/responses"
	if input.Request.Compact {
		if input.Request.Stream {
			return nil, plugin.ErrorWithCode("invalid_request", "Codex compact requests do not support streaming")
		}
		path += "/compact"
	} else {
		payload["store"] = false
		delete(payload, "max_output_tokens")
		delete(payload, "temperature")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	baseURL := strings.TrimRight(strings.TrimSpace(input.Channel.BaseURL), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(ctx.Settings.String("base_url", defaultBaseURL), "/")
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	accept := "application/json"
	if input.Request.Stream {
		accept = "text/event-stream"
	}
	encodedKey, _ := json.Marshal(key)
	return map[string]any{
		"ok": true, "api_key": string(encodedKey),
		"request": map[string]any{
			"method": "POST", "url": baseURL + path, "body": string(body),
			"headers": map[string]any{"Authorization": "Bearer " + key.AccessToken, "chatgpt-account-id": key.AccountID, "OpenAI-Beta": "responses=experimental", "originator": "codex_cli_rs", "Content-Type": "application/json", "Accept": accept},
		},
	}, nil
}

func applyInstructions(payload map[string]any, systemPrompt string, override bool) {
	instructions, exists := payload["instructions"]
	if !exists || instructions == nil {
		payload["instructions"] = systemPrompt
		return
	}
	if text, ok := instructions.(string); ok && override && strings.TrimSpace(systemPrompt) != "" {
		payload["instructions"] = systemPrompt + "\n" + text
	}
	if _, ok := payload["instructions"].(string); !ok {
		payload["instructions"] = ""
	}
}

func (key oauthKey) needsRefresh(now time.Time) bool {
	if strings.TrimSpace(key.Expired) == "" {
		return false
	}
	expiresAt, err := time.Parse(time.RFC3339, key.Expired)
	return err == nil && !expiresAt.After(now.Add(24*time.Hour))
}

func fillTokenClaims(key *oauthKey) {
	claims := jwtClaims(key.AccessToken)
	if key.AccountID == "" {
		if accountID, ok := claims["https://api.openai.com/auth.chatgpt_account_id"].(string); ok {
			key.AccountID = strings.TrimSpace(accountID)
		}
	}
	if key.Email == "" {
		if email, ok := claims["email"].(string); ok {
			key.Email = strings.TrimSpace(email)
		}
	}
}

func jwtClaims(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return map[string]any{}
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return map[string]any{}
	}
	claims := map[string]any{}
	_ = json.Unmarshal(raw, &claims)
	return claims
}

func cloneMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func integerValue(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case json.Number:
		result, _ := typed.Int64()
		return int(result)
	default:
		return 0
	}
}

func main() {}

//export plugin_manifest
func manifest() {
	_ = json.NewEncoder(os.Stdout).Encode(manifestDocument)
}

//export plugin_init
func initPlugin() { app.ExportInit() }

//export plugin_handle_action
func action() { app.ExportAction() }

//export plugin_handle_hook
func hook() { app.ExportHook() }
