package main

import (
	"encoding/base64"
	"encoding/json"
	"hash/fnv"
	"net/url"
	"strconv"
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

func codexDashboard() json.RawMessage {
	return json.RawMessage(`{"type":"page","children":[{"type":"settings_summary","title":"运行概览","fields":[{"name":"scheduling","label":"账号调度方式","default":"round_robin"},{"name":"base_url","label":"Codex 服务地址","default":"https://chatgpt.com"}]},{"type":"settings_list","name":"pools","title":"账号池","description":"已启用的账号池可在上级渠道中被选择。","empty_text":"暂无账号池，请先前往插件设置创建。","columns":[{"key":"id","label":"账号池 ID"},{"key":"name","label":"名称"},{"key":"enabled","label":"状态","format":"boolean"}]},{"type":"settings_list","name":"accounts","title":"账号","description":"登录凭据不会在此页面显示。","empty_text":"暂无账号，请在插件设置中添加。","columns":[{"key":"id","label":"账号 ID"},{"key":"pool_id","label":"所属账号池"},{"key":"enabled","label":"状态","format":"boolean"}]}]}`)
}

func codexUsageGuide() json.RawMessage {
	return json.RawMessage(`{"type":"page","children":[{"type":"card","title":"1. 创建账号池","children":[{"type":"text","text":"在插件设置中添加账号池；账号池 ID 会用于上级渠道选择。"}]},{"type":"card","title":"2. 添加账号","children":[{"type":"text","text":"点击“添加账号”，选择所属账号池，然后粘贴一份 Codex OAuth 登录凭据 JSON。凭据仅保存在服务端。"},{"type":"json","value":{"access_token":"eyJ...","refresh_token":"rt_...","account_id":"acct_...","expired":"2026-01-02T15:04:05Z","type":"codex"}}]},{"type":"card","title":"3. 创建上级渠道","children":[{"type":"text","text":"在渠道管理中新增“ChatGPT 订阅（Codex）”，并在渠道插件设置中选择账号池。默认服务地址为 https://chatgpt.com。"}]},{"type":"card","title":"4. 分配设备访问","children":[{"type":"text","text":"为每台设备创建独立的 Veloce API Key，并只授予需要的用户渠道访问权限。插件会按调度方式选择账号并自动刷新 OAuth 凭据。"}]}]}`)
}

var app *plugin.Plugin

func newPlugin() *plugin.Plugin {
	return plugin.New(plugin.Manifest{
		ID:          pluginID,
		Name:        "ChatGPT 订阅（Codex）",
		Version:     "0.1.1",
		Description: "通过账号池将一个或多个 ChatGPT Codex 订阅安全提供给多台已授权设备使用。",
		Author:      "WindyPear Team",
		Permissions: []string{"plugin.settings.global", "plugin.channel.http"},
		Settings: plugin.SettingsSchema{Type: "form", Fields: []plugin.SettingsField{
			{Type: "input", Name: "base_url", Label: "Codex 服务地址", Default: defaultBaseURL, Description: "通常保持为 https://chatgpt.com。"},
			{Type: "select", Name: "scheduling", Label: "账号调度方式", Default: "round_robin", Options: []plugin.SelectOption{{Label: "轮询", Value: "round_robin"}, {Label: "随机", Value: "random"}}},
			{Type: "editable_list", Name: "pools", Label: "账号池", Description: "先创建账号池，再在“账号”中添加具体登录凭据。", Options: []plugin.SelectOption{{Label: "账号池 ID", Value: "id"}, {Label: "账号池名称", Value: "name"}, {Label: "启用", Value: "enabled"}}},
			{Type: "editable_list", Name: "accounts", Label: "账号", Description: "添加账号时选择账号池并粘贴一份 Codex OAuth 登录凭据 JSON。", Options: []plugin.SelectOption{{Label: "账号 ID", Value: "id"}, {Label: "所属账号池", Value: "pool_id"}, {Label: "登录凭据 JSON", Value: "credentials_json"}, {Label: "启用", Value: "enabled"}}},
			{Type: "textarea", Name: "system_prompt", Label: "默认系统提示词", Description: "当 Responses 请求没有 instructions 时使用。"},
			{Type: "switch", Name: "system_prompt_override", Label: "在已有 instructions 前追加", Default: false},
		}},
		Upstreams: []plugin.UpstreamType{{
			ID: upstreamID, Name: "ChatGPT 订阅（Codex）", Protocol: plugin.UpstreamProtocolResponses, DefaultBaseURL: defaultBaseURL,
			Description:   "使用共享账号池并由插件统一刷新 OAuth 凭据的 ChatGPT Codex 上游。",
			PrepareAction: "upstream.prepare", RefreshAction: "upstream.refresh",
			Config: plugin.SettingsSchema{Fields: []plugin.SettingsField{{
				Type: "select", Name: "pool_id", Label: "账号池", Required: true,
				Description: "选择这个上级渠道使用的插件账号池。",
				OptionsFrom: "pools", OptionLabel: "name", OptionValue: "id",
			}}},
		}},
		Frontend: plugin.Frontend{Sidebar: []plugin.SidebarItem{
			{Label: "Codex 账号池", Path: "status", Access: plugin.FrontendAccessAdmin},
			{Label: "Codex 使用流程", Path: "guide", Access: plugin.FrontendAccessAdmin},
		}, Routes: []plugin.Route{
			{Path: "status", Title: "Codex 账号池状态", Description: "查看账号池与账号状态。", Page: codexDashboard(), Access: plugin.FrontendAccessAdmin},
			{Path: "guide", Title: "Codex 使用流程", Description: "按步骤完成账号池、上级渠道和设备访问配置。", Page: codexUsageGuide(), Access: plugin.FrontendAccessAdmin},
		}},
	})
}

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

type accountPool struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Enabled            bool   `json:"enabled"`
	LegacyAccountsJSON string `json:"accounts_json,omitempty"`
}

type poolAccount struct {
	ID              string `json:"id"`
	PoolID          string `json:"pool_id"`
	CredentialsJSON string `json:"credentials_json"`
	Enabled         bool   `json:"enabled"`
}

func init() {
	app = newPlugin()
	app.Action("upstream.prepare", prepare)
	app.Action("upstream.refresh", refresh)
}

func prepare(ctx *plugin.ActionContext) (any, error) {
	input, accounts, accountIndexes, keys, err := decodeInput(ctx)
	if err != nil {
		return nil, err
	}
	keyIndex, err := selectAccount(ctx, len(keys))
	if err != nil {
		return nil, err
	}
	key := keys[keyIndex]
	settingsPatch := map[string]any(nil)
	if key.needsRefresh(time.Now()) {
		if strings.TrimSpace(key.RefreshToken) == "" {
			return nil, plugin.ErrorWithCode("codex_token_expired", "Codex 登录凭据已过期，且未配置 refresh_token")
		}
		if err := refreshOAuth(ctx, &key); err != nil {
			return nil, err
		}
		accounts[accountIndexes[keyIndex]].CredentialsJSON = encodeOAuthKey(key)
		settingsPatch = map[string]any{"accounts": accounts}
	}
	result, err := upstreamRequest(ctx, input, key)
	if err != nil {
		return nil, err
	}
	if settingsPatch != nil {
		result = result.WithSettingsPatch(settingsPatch)
	}
	return result, nil
}

func refresh(ctx *plugin.ActionContext) (any, error) {
	_, accounts, accountIndexes, keys, err := decodeInput(ctx)
	if err != nil {
		return nil, err
	}
	changed := false
	for index := range keys {
		if !keys[index].needsRefresh(time.Now()) {
			continue
		}
		if strings.TrimSpace(keys[index].RefreshToken) == "" {
			continue
		}
		if err := refreshOAuth(ctx, &keys[index]); err != nil {
			return nil, err
		}
		accounts[accountIndexes[index]].CredentialsJSON = encodeOAuthKey(keys[index])
		changed = true
	}
	result := plugin.UpstreamRefreshResult()
	if changed {
		result = result.WithSettingsPatch(map[string]any{"accounts": accounts})
	}
	return result, nil
}

func refreshOAuth(ctx *plugin.ActionContext, key *oauthKey) error {
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {key.RefreshToken}, "client_id": {oauthClientID}}
	response, err := ctx.Host.HTTP(ctx, plugin.HTTPRequest{Method: "POST", URL: oauthTokenURL, Headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Accept": "application/json"}, Body: form.Encode()})
	if err != nil {
		return err
	}
	if response.Status < 200 || response.Status >= 300 {
		return plugin.ErrorWithCode("codex_refresh_failed", "Codex OAuth 刷新请求被拒绝")
	}
	var refreshed map[string]any
	if err := json.Unmarshal([]byte(response.Body), &refreshed); err != nil {
		return plugin.ErrorWithCode("codex_refresh_failed", "Codex OAuth 刷新响应格式不正确")
	}
	accessToken, _ := refreshed["access_token"].(string)
	refreshToken, _ := refreshed["refresh_token"].(string)
	expiresIn := integerValue(refreshed["expires_in"])
	if strings.TrimSpace(accessToken) == "" || strings.TrimSpace(refreshToken) == "" || expiresIn <= 0 {
		return plugin.ErrorWithCode("codex_refresh_failed", "Codex OAuth 刷新响应缺少必要字段")
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

func decodeInput(ctx *plugin.ActionContext) (plugin.UpstreamInvocation, []poolAccount, []int, []oauthKey, error) {
	input, err := ctx.Upstream()
	if err != nil {
		return plugin.UpstreamInvocation{}, nil, nil, nil, err
	}
	poolID := strings.TrimSpace(input.Channel.Config.String("pool_id", ""))
	if poolID == "" {
		return plugin.UpstreamInvocation{}, nil, nil, nil, plugin.ErrorWithCode("invalid_pool", "请选择上级渠道使用的账号池")
	}
	var pools []accountPool
	if err := ctx.Settings.Decode("pools", &pools); err != nil {
		return plugin.UpstreamInvocation{}, nil, nil, nil, plugin.ErrorWithCode("invalid_pool", "账号池配置格式不正确")
	}
	if !poolEnabled(pools, poolID) {
		return plugin.UpstreamInvocation{}, nil, nil, nil, plugin.ErrorWithCode("invalid_pool", "所选账号池不存在或已停用")
	}
	accounts, err := configuredAccounts(ctx, pools)
	if err != nil {
		return plugin.UpstreamInvocation{}, nil, nil, nil, err
	}
	keys := make([]oauthKey, 0, len(accounts))
	accountIndexes := make([]int, 0, len(accounts))
	for index := range accounts {
		if !accounts[index].Enabled || strings.TrimSpace(accounts[index].PoolID) != poolID {
			continue
		}
		key, err := decodeOAuthKey(accounts[index].CredentialsJSON)
		if err != nil {
			return plugin.UpstreamInvocation{}, nil, nil, nil, err
		}
		keys = append(keys, key)
		accountIndexes = append(accountIndexes, index)
	}
	if len(keys) == 0 {
		return plugin.UpstreamInvocation{}, nil, nil, nil, plugin.ErrorWithCode("invalid_pool", "所选账号池中没有可用账号")
	}
	return input, accounts, accountIndexes, keys, nil
}

func selectAccount(ctx *plugin.ActionContext, count int) (int, error) {
	if count == 0 {
		return 0, plugin.ErrorWithCode("invalid_pool", "所选账号池中没有可用账号")
	}
	if ctx.Settings.String("scheduling", "round_robin") == "random" {
		return plugin.SecureIndex(count)
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(ctx.RequestID))
	return int(hash.Sum32() % uint32(count)), nil
}

func configuredAccounts(ctx *plugin.ActionContext, pools []accountPool) ([]poolAccount, error) {
	var accounts []poolAccount
	if err := ctx.Settings.Decode("accounts", &accounts); err != nil {
		return nil, plugin.ErrorWithCode("invalid_account", "账号配置格式不正确")
	}
	if len(accounts) > 0 {
		return accounts, nil
	}
	for _, pool := range pools {
		if strings.TrimSpace(pool.LegacyAccountsJSON) == "" {
			continue
		}
		var keys []oauthKey
		if err := json.Unmarshal([]byte(pool.LegacyAccountsJSON), &keys); err != nil {
			return nil, plugin.ErrorWithCode("invalid_account", "旧账号池中的凭据 JSON 格式不正确")
		}
		for index, key := range keys {
			accounts = append(accounts, poolAccount{ID: pool.ID + "-" + strconv.Itoa(index+1), PoolID: pool.ID, CredentialsJSON: encodeOAuthKey(key), Enabled: true})
		}
	}
	return accounts, nil
}

func poolEnabled(pools []accountPool, poolID string) bool {
	for _, pool := range pools {
		if strings.TrimSpace(pool.ID) == poolID {
			return pool.Enabled
		}
	}
	return false
}

func decodeOAuthKey(raw string) (oauthKey, error) {
	var key oauthKey
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &key); err != nil {
		return oauthKey{}, plugin.ErrorWithCode("invalid_codex_key", "账号登录凭据必须是 Codex OAuth JSON 对象")
	}
	key.AccessToken = strings.TrimSpace(key.AccessToken)
	key.AccountID = strings.TrimSpace(key.AccountID)
	if key.AccessToken == "" || key.AccountID == "" {
		fillTokenClaims(&key)
	}
	if key.AccessToken == "" || key.AccountID == "" {
		return oauthKey{}, plugin.ErrorWithCode("invalid_codex_key", "登录凭据需要包含 access_token 和 account_id")
	}
	return key, nil
}

func encodeOAuthKey(key oauthKey) string {
	raw, _ := json.Marshal(key)
	return string(raw)
}

func upstreamRequest(ctx *plugin.ActionContext, input plugin.UpstreamInvocation, key oauthKey) (plugin.UpstreamResult, error) {
	payload := cloneMap(input.Request.Payload)
	applyInstructions(payload, ctx.Settings.String("system_prompt", ""), ctx.Settings.Bool("system_prompt_override", false))
	path := "/backend-api/codex/responses"
	if input.Request.Compact {
		if input.Request.Stream {
			return plugin.UpstreamResult{}, plugin.ErrorWithCode("invalid_request", "Codex compact 请求不支持流式响应")
		}
		path += "/compact"
	} else {
		payload["store"] = false
		delete(payload, "max_output_tokens")
		delete(payload, "temperature")
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
	request, err := plugin.JSONPostRequest(baseURL+path, payload, map[string]string{"Authorization": "Bearer " + key.AccessToken, "chatgpt-account-id": key.AccountID, "OpenAI-Beta": "responses=experimental", "originator": "codex_cli_rs", "Accept": accept})
	if err != nil {
		return plugin.UpstreamResult{}, err
	}
	return plugin.UpstreamRequestResult(request), nil
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
func manifest() { app.ExportManifest() }

//export plugin_init
func initPlugin() { app.ExportInit() }

//export plugin_handle_action
func action() { app.ExportAction() }

//export plugin_handle_hook
func hook() { app.ExportHook() }
