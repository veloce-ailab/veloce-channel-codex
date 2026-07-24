# veloce-channel-codex

WASM upstream adapter for a ChatGPT Codex subscription. It lets one server-side
OAuth credential serve Veloce users on multiple authorized devices without
copying the subscription token onto those devices.

## Build

```bash
go mod download
tinygo build -target=wasi -o veloce-channel-codex.wasm .
```

Upload the resulting WASM file in the Veloce plugin manager. The plugin declares
the upstream type `plugin--codex-subscription--codex`.

## Configure

1. Open the plugin settings and configure the optional base URL and default
   system prompt.
2. Create an upstream channel of type **ChatGPT Subscription (Codex)**.
3. Use `https://chatgpt.com` as Base URL and paste the OAuth JSON into API Key.
4. Add the supported Codex models to that upstream channel and attach it to the
   user channels that should expose the subscription.
5. Give every device a distinct Veloce API key limited to the appropriate user
   channel. All devices route through the same upstream channel and share the
   server-side subscription credential.

The API Key must be a JSON object with `access_token` and `account_id`. Add a
`refresh_token` and `expired` timestamp to enable automatic refresh:

```json
{
  "access_token": "eyJ...",
  "refresh_token": "rt_...",
  "account_id": "acct_...",
  "expired": "2026-07-25T12:00:00Z",
  "type": "codex"
}
```

Only OpenAI Responses requests are supported. Normal requests forward to
`/backend-api/codex/responses`; `POST /v1/responses/compact` forwards to the
Codex compact endpoint and is non-streaming. OAuth refreshes happen in the
WASM plugin through the permission-controlled helper HTTP host call, and the
renewed key is written back to the upstream channel by Veloce.
