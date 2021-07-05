package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/rancher/rancher/pkg/auth/tokens"

	"github.com/harvester/harvester/pkg/util"
)

const (
	//action
	actionQuery      = "action"
	logoutActionName = "logout"
)

func NewLoginHandler() *LoginHandler {
	return &LoginHandler{}
}

type LoginHandler struct {
}

func (h *LoginHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		util.ResponseErrorMsg(rw, http.StatusMethodNotAllowed, "Only POST method is supported")
		return
	}

	action := strings.ToLower(r.URL.Query().Get(actionQuery))
	isSecure := r.URL.Scheme == "https"

	if action == logoutActionName {
		resetCookie(rw, tokens.CookieName, isSecure)
		util.ResponseOK(rw)
		return
	}

	util.ResponseErrorMsg(rw, http.StatusBadRequest, "Unsupported action")
}

func resetCookie(rw http.ResponseWriter, name string, isSecure bool) {
	cookie := &http.Cookie{
		Name:     name,
		Value:    "",
		Secure:   isSecure,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0), //January 1, 1970 UTC
	}
	http.SetCookie(rw, cookie)
}
