package jwt

import (
	"encoding/json"
	"strings"

	"github.com/dgrijalva/jwt-go"
	"github.com/pkg/errors"
)

func GetJWTTokenClaims(jwtToken string) (map[string]interface{}, error) {
	parts := strings.Split(jwtToken, ".")
	if len(parts) != 3 {
		return nil, errors.New("token contains an invalid number of segments")
	}

	payloadBytes, err := jwt.DecodeSegment(parts[1])
	if err != nil {
		return nil, errors.Wrap(err, "Failed to decode jwtToken payload")
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, errors.Wrap(err, "Failed to unmarshal jwtToken payload")
	}

	return claims, nil
}
