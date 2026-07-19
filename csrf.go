package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	csrfCookieName = "quester_csrf"
	csrfFieldName  = "csrf"
	csrfSecretHex  = 64 // 32 random bytes
	csrfNonceHex   = 32 // 16 random bytes
	csrfMACHex     = sha256.Size * 2
)

// ensureCSRFToken returns a fresh form token for this response, bound to the
// browser's CSRF cookie. The cookie holds a random secret and is set when
// missing; a token is the hex nonce plus an HMAC of it under that secret, so
// every rendered page carries a distinct token that only the cookie holder
// can mint.
func (a *App) ensureCSRFToken(c *gin.Context) string {
	secret, err := c.Cookie(csrfCookieName)
	if err != nil || !isHexToken(secret, csrfSecretHex) {
		secret = randomHex(csrfSecretHex / 2)
		c.SetSameSite(http.SameSiteStrictMode)
		c.SetCookie(csrfCookieName, secret, 0, a.cookiePath(), "", false, true)
	}
	nonce := randomHex(csrfNonceHex / 2)
	return nonce + csrfMAC(secret, nonce)
}

// validCSRFToken accepts a form token that was minted from this request's
// cookie secret. Requests without the cookie or token fail closed.
func (a *App) validCSRFToken(c *gin.Context) bool {
	secret, err := c.Cookie(csrfCookieName)
	if err != nil || !isHexToken(secret, csrfSecretHex) {
		return false
	}
	token := c.PostForm(csrfFieldName)
	if len(token) != csrfNonceHex+csrfMACHex {
		return false
	}
	nonce, mac := token[:csrfNonceHex], token[csrfNonceHex:]
	return hmac.Equal([]byte(mac), []byte(csrfMAC(secret, nonce)))
}

func csrfMAC(secret, nonce string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(nonce))
	return hex.EncodeToString(mac.Sum(nil))
}

func (a *App) cookiePath() string {
	if a.base == "" {
		return "/"
	}
	return a.base
}

func randomHex(bytes int) string {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		panic(err) // crypto/rand.Read is documented never to fail
	}
	return hex.EncodeToString(buf)
}
