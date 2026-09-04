# Botainer Security

## Telegram Mini App Authentication

### How it works

1. **Telegram WebApp SDK** sends `initData` with every request
2. **Backend validates** the signature using HMAC-SHA256
3. **Only authenticated users** from Telegram can access the API

### Implementation

#### Backend (Go)

```go
func (s *Server) validateTelegramAuth(initData string) bool {
    // Parse query string
    values, _ := url.ParseQuery(initData)
    hash := values.Get("hash")
    values.Del("hash")
    
    // Build data-check-string (sorted keys)
    var keys []string
    for k := range values {
        keys = append(keys, k)
    }
    sort.Strings(keys)
    
    var dataCheckString string
    for _, k := range keys {
        dataCheckString += k + "=" + values.Get(k) + "\n"
    }
    
    // Compute secret key: HMAC-SHA256(bot_token, "WebAppData")
    secretKey := hmac.New(sha256.New, []byte("WebAppData"))
    secretKey.Write([]byte(botToken))
    
    // Compute hash: HMAC-SHA256(secret_key, data_check_string)
    h := hmac.New(sha256.New, secretKey.Sum(nil))
    h.Write([]byte(dataCheckString))
    computedHash := hex.EncodeToString(h.Sum(nil))
    
    return computedHash == hash
}
```

#### Frontend (React)

```typescript
const getAuthHeaders = () => {
  const initData = window.Telegram?.WebApp?.initData || ''
  return {
    'Content-Type': 'application/json',
    'X-Telegram-Init-Data': initData
  }
}

// Use in all API calls
fetch('/api/containers', {
  headers: getAuthHeaders()
})
```

### Security Features

✅ **Cryptographic signature validation** - Uses HMAC-SHA256  
✅ **No password storage** - Telegram handles authentication  
✅ **Automatic expiration** - initData expires after 24h  
✅ **User verification** - Only Telegram users can access  
✅ **No direct URL access** - Must open from Telegram bot  

### Access Control

The Mini App can only be accessed:
1. From the Telegram bot menu button
2. With valid, non-expired Telegram WebApp initData (rejected if `auth_date` is older than 24h)
3. By users who have interacted with the bot **and** are present in `ALLOWED_USERS` (if set)

### User Whitelist & Expiration (enforced since v2.4.3)

`api/server.go` enforces both of the following in `validateTelegramAuth()`,
in addition to the HMAC-SHA256 signature check:

```go
// Reject expired initData (auth_date older than 24h, or set in the future)
authDate := time.Unix(authDateUnix, 0)
if time.Since(authDate) > maxInitDataAge { return false }

// Enforce ALLOWED_USERS whitelist against initData's user.id
if !s.isUserAllowed(values.Get("user")) { return false }
```

Prior to v2.4.3, `ALLOWED_USERS` was only enforced in the Telegram bot's
command handlers (`main.go`), not in the Mini App REST API — meaning any
user who had opened the bot could bypass the whitelist by calling the API
directly with a validly signed `initData`. This is now fixed; the same
`ALLOWED_USERS` env var is enforced consistently across both the bot and
the API.

### Testing

**Without auth (should fail):**
```bash
curl http://localhost:8080/api/containers
# {"success":false,"error":"Unauthorized"}
```

**With valid Telegram initData (should work):**
Only possible from Telegram WebApp - cannot be tested with curl.

### References

- [Telegram WebApp Authentication](https://core.telegram.org/bots/webapps#validating-data-received-via-the-mini-app)
- [HMAC-SHA256 Validation](https://en.wikipedia.org/wiki/HMAC)
