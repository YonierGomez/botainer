package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

// buildInitData constructs a valid, correctly-signed Telegram WebApp
// initData string for the given botToken, user JSON and auth date, mirroring
// what the real Telegram client would send.
func buildInitData(botToken string, userJSON string, authDate time.Time) string {
	values := url.Values{}
	values.Set("query_id", "AAFtest")
	values.Set("user", userJSON)
	values.Set("auth_date", fmt.Sprintf("%d", authDate.Unix()))

	var keys []string
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var dataCheckString string
	for _, k := range keys {
		dataCheckString += k + "=" + values.Get(k) + "\n"
	}
	dataCheckString = strings.TrimSuffix(dataCheckString, "\n")

	secretKey := hmac.New(sha256.New, []byte("WebAppData"))
	secretKey.Write([]byte(botToken))

	h := hmac.New(sha256.New, secretKey.Sum(nil))
	h.Write([]byte(dataCheckString))
	hash := hex.EncodeToString(h.Sum(nil))

	values.Set("hash", hash)
	return values.Encode()
}

func withEnv(t *testing.T, key, value string) {
	t.Helper()
	old, hadOld := os.LookupEnv(key)
	if value == "" {
		os.Unsetenv(key)
	} else {
		os.Setenv(key, value)
	}
	t.Cleanup(func() {
		if hadOld {
			os.Setenv(key, old)
		} else {
			os.Unsetenv(key)
		}
	})
}

func TestValidateTelegramAuth_ValidRecentAuthDate_NoAllowList(t *testing.T) {
	const botToken = "test-token-123"
	withEnv(t, "TELEGRAM_BOT_TOKEN", botToken)
	withEnv(t, "ALLOWED_USERS", "")

	s := &Server{}
	initData := buildInitData(botToken, `{"id":193935030,"first_name":"Owner"}`, time.Now())

	if !s.validateTelegramAuth(initData) {
		t.Errorf("expected valid initData with recent auth_date and no ALLOWED_USERS to pass")
	}
}

func TestValidateTelegramAuth_ExpiredAuthDate_Rejected(t *testing.T) {
	const botToken = "test-token-123"
	withEnv(t, "TELEGRAM_BOT_TOKEN", botToken)
	withEnv(t, "ALLOWED_USERS", "")

	s := &Server{}
	// auth_date far in the past (well beyond the 24h window)
	initData := buildInitData(botToken, `{"id":193935030,"first_name":"Owner"}`, time.Now().Add(-48*time.Hour))

	if s.validateTelegramAuth(initData) {
		t.Errorf("expected expired initData (auth_date 48h old) to be rejected")
	}
}

func TestValidateTelegramAuth_FutureAuthDate_Rejected(t *testing.T) {
	const botToken = "test-token-123"
	withEnv(t, "TELEGRAM_BOT_TOKEN", botToken)
	withEnv(t, "ALLOWED_USERS", "")

	s := &Server{}
	initData := buildInitData(botToken, `{"id":193935030,"first_name":"Owner"}`, time.Now().Add(1*time.Hour))

	if s.validateTelegramAuth(initData) {
		t.Errorf("expected initData with auth_date far in the future to be rejected")
	}
}

func TestValidateTelegramAuth_UserNotInAllowList_Rejected(t *testing.T) {
	const botToken = "test-token-123"
	withEnv(t, "TELEGRAM_BOT_TOKEN", botToken)
	withEnv(t, "ALLOWED_USERS", "193935030")

	s := &Server{}
	// Different, unauthorized Telegram user ID (e.g. the attacker's account)
	initData := buildInitData(botToken, `{"id":999999999,"first_name":"Attacker"}`, time.Now())

	if s.validateTelegramAuth(initData) {
		t.Errorf("expected initData for a user not in ALLOWED_USERS to be rejected")
	}
}

func TestValidateTelegramAuth_UserInAllowList_Accepted(t *testing.T) {
	const botToken = "test-token-123"
	withEnv(t, "TELEGRAM_BOT_TOKEN", botToken)
	withEnv(t, "ALLOWED_USERS", "193935030,111111111")

	s := &Server{}
	initData := buildInitData(botToken, `{"id":193935030,"first_name":"Owner"}`, time.Now())

	if !s.validateTelegramAuth(initData) {
		t.Errorf("expected initData for a user in ALLOWED_USERS to be accepted")
	}
}

func TestValidateTelegramAuth_InvalidSignature_Rejected(t *testing.T) {
	const botToken = "test-token-123"
	withEnv(t, "TELEGRAM_BOT_TOKEN", botToken)
	withEnv(t, "ALLOWED_USERS", "")

	s := &Server{}
	// Signed with a different token than the one the server expects.
	initData := buildInitData("wrong-token", `{"id":193935030,"first_name":"Owner"}`, time.Now())

	if s.validateTelegramAuth(initData) {
		t.Errorf("expected initData signed with wrong bot token to be rejected")
	}
}

func TestValidateTelegramAuth_MissingUserField_RejectedWhenAllowListSet(t *testing.T) {
	const botToken = "test-token-123"
	withEnv(t, "TELEGRAM_BOT_TOKEN", botToken)
	withEnv(t, "ALLOWED_USERS", "193935030")

	values := url.Values{}
	values.Set("query_id", "AAFtest")
	values.Set("auth_date", fmt.Sprintf("%d", time.Now().Unix()))

	var keys []string
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var dataCheckString string
	for _, k := range keys {
		dataCheckString += k + "=" + values.Get(k) + "\n"
	}
	dataCheckString = strings.TrimSuffix(dataCheckString, "\n")

	secretKey := hmac.New(sha256.New, []byte("WebAppData"))
	secretKey.Write([]byte(botToken))
	h := hmac.New(sha256.New, secretKey.Sum(nil))
	h.Write([]byte(dataCheckString))
	hash := hex.EncodeToString(h.Sum(nil))
	values.Set("hash", hash)

	s := &Server{}
	if s.validateTelegramAuth(values.Encode()) {
		t.Errorf("expected initData without a user field to be rejected when ALLOWED_USERS is set")
	}
}
