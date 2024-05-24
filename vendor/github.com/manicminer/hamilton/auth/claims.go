package auth

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"golang.org/x/oauth2"
)

// Claims is used to unmarshall the claims from a JWT issued by the Microsoft Identity Platform.
type Claims struct {
	Audience          string   `json:"aud"`
	Issuer            string   `json:"iss"`
	IdentityProvider  string   `json:"idp"`
	ObjectId          string   `json:"oid"`
	Roles             []string `json:"roles"`
	Scopes            string   `json:"scp"`
	Subject           string   `json:"sub"`
	TenantRegionScope string   `json:"tenant_region_scope"`
	TenantId          string   `json:"tid"`
	Version           string   `json:"ver"`

	AppDisplayName string `json:"app_displayname,omitempty"`
	AppId          string `json:"appid,omitempty"`
	IdType         string `json:"idtyp,omitempty"`
}

// ParseClaims retrieves and parses the claims from a JWT issued by the Microsoft Identity Platform.
func ParseClaims(token *oauth2.Token) (claims Claims, err error) {
	if token == nil {
		return
	}
	jwt := strings.Split(token.AccessToken, ".")
	payload, err := base64.RawURLEncoding.DecodeString(jwt[1])
	if err != nil {
		return
	}
	err = json.Unmarshal(payload, &claims)
	return
}
