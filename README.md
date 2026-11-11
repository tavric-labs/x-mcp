# x-mcp

An MCP server that lets a signed-in person read their X profile and publish
posts as themselves. It is built to run on [MCP Runtime](https://github.com/tavric-labs/mcp-runtime)
behind [mcp-auth](https://github.com/tavric-labs/mcp-auth), the same way
[linkedin-mcp](https://github.com/tavric-labs/linkedin-mcp) does.

## How it works

1. The MCP client signs the person in through mcp-auth (an X login method) and
   sends the resulting access token to this server.
2. The server verifies the token (`mcpauth.RequireToken`, audience = this server's URL).
3. For each tool call it exchanges that token for **that person's own X token**
   (RFC 8693, authenticating as a resource client with `private_key_jwt`) and calls
   X as them. It stores no X token and caches nothing, so ending the person's
   mcp-auth session ends their X access at once.

Audit (who called which tool, when, allowed or denied) comes from the MCP Runtime
gateway in front of the server; the server also logs the subject, tool and outcome,
never a token or post text.

## Tools

| Tool | Scope | Notes |
|---|---|---|
| `x_get_profile` | `tools:read` | Display name, `@username`, and user id. |
| `x_create_post` | `tools:write` to publish | A **dry run** unless `confirm: true`. Up to 280 characters. |

X access tokens last about two hours; request `offline.access` so mcp-auth can
refresh them. When the session is gone the tools answer "sign in again".

## Cost

X API is **pay-per-use**. Reads and posts deduct credits in the Developer Console.
A small test budget (on the order of ₹1000) is enough for light smoke tests.
With no credits, tools fail with a clear "credits are required" message.

## Configuration

| Variable | Purpose |
|---|---|
| `X_MCP_RESOURCE` | This server's public MCP URL (the token audience). |
| `X_MCP_ISSUER` | The mcp-auth issuer URL. `X_JWKS_URL` overrides discovery. |
| `X_MCP_CLIENT_ID`, `X_MCP_CLIENT_KEY_ID` | The resource client registered in mcp-auth. |
| `X_MCP_CLIENT_KEY` or `X_MCP_CLIENT_KEY_FILE` | The client's RSA private key (PEM). |
| `X_API_BASE` | Defaults to `https://api.x.com`. |
| `X_MCP_ALLOW_INSECURE=true` | Plain http issuer, local development only. |

## Sign-in wiring (administrator)

Signing in with X is configured in MCP Auth, not in this server. `.mcp/auth-resource.yaml`
holds everything X-specific (endpoints, scopes, PKCE on, `downstream_token: upstream_session`);
fill its `<...>` values and apply it with
`mcp-runtime cluster mcp-auth resource apply --file .mcp/auth-resource.yaml`.
It is applied by an administrator and kept out of `servers.yaml`, which a tenant deploys.

In the X Developer Console, create a **Web** or **Native** app (not Automated), enable
OAuth 2.0, allow scopes `tweet.read tweet.write users.read offline.access`, and register
`<MCP Auth issuer URL>/identity/callback` as the callback. Put the app client secret in
the cluster as `X_CLIENT_SECRET` on `mcp-auth-connector-secrets` (never in git).

**Known gap:** X's `GET /2/users/me` returns `{"data":{"id":...}}`, while mcp-auth plain
OAuth userinfo expects a top-level identity claim. End-to-end login may need a small
mcp-auth change to unwrap nested `data` before identity resolution. The MCP server and
X client themselves are ready.

## Develop

```bash
gofmt -l . && go vet ./... && go test -race ./...
```

Deploy with the MCP Runtime workflow: `server validate`, `server build image`,
`server push`, `server deploy` using `.mcp/servers.yaml`.
