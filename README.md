# veloce-channel-codex

ChatGPT Codex 订阅的 WASM 上游适配器。它通过账号池让一个或多个服务端 OAuth
账号为多台已授权设备提供服务，订阅凭据不会复制到设备上。

## 构建

```bash
go mod download
tinygo build -target=wasi -o veloce-channel-codex.wasm .
```

在 Veloce 插件管理器上传生成的 WASM。插件声明的上游类型为
`plugin--codex-subscription--codex`。

## 配置

1. 打开插件设置，选择账号调度方式，先添加一个或多个账号池。
2. 在“账号”中点击“添加账号”，选择所属账号池，并粘贴一份 OAuth 登录凭据 JSON。
3. 创建 **ChatGPT 订阅（Codex）** 上级渠道，在渠道插件设置中选择账号池。Base URL
   默认是 `https://chatgpt.com`，渠道 API Key 不使用。
4. 为该上级渠道添加支持的 Codex 模型，并关联要开放订阅的用户渠道。
5. 每台设备使用独立的 Veloce API Key，并仅授予需要的用户渠道。全部设备会使用所选
   服务端账号池。

登录凭据必须是单个 JSON 对象（不是数组），至少包含 `access_token` 和 `account_id`。
增加 `refresh_token` 和 `expired` 后插件会自动刷新：

```json
{
  "access_token": "eyJ...",
  "refresh_token": "rt_...",
  "account_id": "acct_...",
  "expired": "2026-07-25T12:00:00Z",
  "type": "codex"
}
```

仅支持 OpenAI Responses 请求。普通请求转发到 `/backend-api/codex/responses`；
`POST /v1/responses/compact` 转发到 Codex compact 接口且不支持流式响应。OAuth
刷新在 WASM 插件中经权限控制的 helper HTTP 宿主调用完成，刷新后的凭据会由 Veloce
写回插件账号设置。
