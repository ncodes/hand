package browser

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

const localAuthorizationUsername = "morph"
const localRelayTokenParameter = "_morph_browser_token"

type localAuthorization struct {
	password string
}

func newLocalAuthorization() (localAuthorization, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return localAuthorization{}, errors.New("browser local authorization could not be created")
	}

	return localAuthorization{password: base64.RawURLEncoding.EncodeToString(secret)}, nil
}

func (a localAuthorization) userinfo() *url.Userinfo {
	return url.UserPassword(localAuthorizationUsername, a.password)
}

func (a localAuthorization) credentials() (string, string) {
	return localAuthorizationUsername, a.password
}

func (a localAuthorization) header() string {
	credentials := localAuthorizationUsername + ":" + a.password
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(credentials))
}

func (a localAuthorization) matches(header string) bool {
	scheme, encoded, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "Basic") {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return false
	}

	return subtle.ConstantTimeCompare(decoded, []byte(localAuthorizationUsername+":"+a.password)) == 1
}

func (a localAuthorization) matchesSecret(secret string) bool {
	return subtle.ConstantTimeCompare([]byte(secret), []byte(a.password)) == 1
}

func (a localAuthorization) requireProxy(writer http.ResponseWriter, request *http.Request) bool {
	if a.matches(request.Header.Get("Proxy-Authorization")) {
		return true
	}
	writer.Header().Set("Proxy-Authenticate", `Basic realm="morph-browser"`)
	http.Error(writer, "proxy authentication required", http.StatusProxyAuthRequired)
	return false
}

func (a localAuthorization) requireRelay(writer http.ResponseWriter, request *http.Request) bool {
	query := request.URL.Query()
	if !a.matchesSecret(query.Get(localRelayTokenParameter)) {
		http.Error(writer, "authentication required", http.StatusUnauthorized)
		return false
	}
	query.Del(localRelayTokenParameter)
	request.URL.RawQuery = query.Encode()
	return true
}
