package request

import (
	"math/rand/v2"
	"strings"
	"time"
)

var (
	cookieNameSanitizer = strings.NewReplacer("\n", "-", "\r", "-")
)

func SanitizeCookieName(n string) string {
	return cookieNameSanitizer.Replace(n)
}

// sanitizeCookieValue produces a suitable cookie-value from v.
// It receives a quoted bool indicating whether the value was originally
// quoted.
// https://tools.ietf.org/html/rfc6265#section-4.1.1
//
//	cookie-value      = *cookie-octet / ( DQUOTE *cookie-octet DQUOTE )
//	cookie-octet      = %x21 / %x23-2B / %x2D-3A / %x3C-5B / %x5D-7E
//	          ; US-ASCII characters excluding CTLs,
//	          ; whitespace DQUOTE, comma, semicolon,
//	          ; and backslash
//
// We loosen this as spaces and commas are common in cookie values
// thus we produce a quoted cookie-value if v contains commas or spaces.
// See https://golang.org/issue/7243 for the discussion.
func SanitizeCookieValue(v string, quoted bool) string {
	v = sanitize(validCookieValueByte, v)
	if len(v) == 0 {
		return v
	}
	if strings.ContainsAny(v, " ,") || quoted {
		return `"` + v + `"`
	}
	return v
}

func sanitize(valid func(byte) bool, v string) string {
	ok := true
	for i := 0; i < len(v); i++ {
		if valid(v[i]) {
			continue
		}
		ok = false
		break
	}
	if ok {
		return v
	}
	buf := make([]byte, 0, len(v))
	for i := 0; i < len(v); i++ {
		if b := v[i]; valid(b) {
			buf = append(buf, b)
		}
	}
	return string(buf)
}

func validCookieValueByte(b byte) bool {
	return 0x20 <= b && b < 0x7f && b != '"' && b != ';' && b != '\\'
}

// RandomString returns a 13-character lowercase base36 cache-busting token.
func RandomString() string {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	var token [13]byte
	timestamp := uint64(time.Now().UnixNano())
	for i := 6; i >= 0; i-- {
		token[i] = alphabet[timestamp%36]
		timestamp /= 36
	}
	random := rand.Uint64()
	for i := len(token) - 1; i >= 7; i-- {
		token[i] = alphabet[random%36]
		random /= 36
	}
	return string(token[:])
}
