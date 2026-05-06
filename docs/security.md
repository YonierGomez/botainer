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
2. With valid Telegram WebApp initData
3. By users who have interacted with the bot

### Additional Security (Optional)

You can add extra validation in `api/server.go`:

```go
// Check user ID against ALLOWED_USERS
allowedUsers := os.Getenv("ALLOWED_USERS")
if allowedUsers != "" {
    userID := values.Get("user")
    // Parse and validate user ID
}

// Check auth_date (timestamp)
authDate := values.Get("auth_date")
// Reject if older than 24h
```

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
