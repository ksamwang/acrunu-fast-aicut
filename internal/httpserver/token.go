package httpserver

import (
	"encoding/base64"
	"encoding/json"

	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
)

func makeDevToken(user auth.User) string {
	payload, _ := json.Marshal(user)
	return base64.RawURLEncoding.EncodeToString(payload)
}

func parseDevToken(token string) (auth.User, bool) {
	payload, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return auth.User{}, false
	}

	var user auth.User
	if err := json.Unmarshal(payload, &user); err != nil {
		return auth.User{}, false
	}
	if user.ID == "" || user.Username == "" || user.Role == "" {
		return auth.User{}, false
	}

	return user, true
}
