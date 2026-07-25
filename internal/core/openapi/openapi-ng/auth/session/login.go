// Copyright (c) 2021 Terminus, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package oauth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/erda-project/erda-proto-go/core/user/oauth/pb"
	"github.com/erda-project/erda/internal/core/openapi/openapi-ng/common"
	"github.com/erda-project/erda/internal/core/user/auth/domain"
)

func (p *provider) LoginURL(rw http.ResponseWriter, r *http.Request) {
	referer := r.Header.Get("Referer")
	if len(referer) <= 0 {
		referer = p.Cfg.RedirectAfterLogin
	}

	authURL, err := p.UserOauthSessionSvc.AuthURL(r.Context(), &pb.AuthURLRequest{
		Referer: referer,
	})
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	common.ResponseJSON(rw, &struct {
		URL string `json:"url"`
	}{
		URL: authURL.Data,
	})
}

func (p *provider) LoginCallback(rw http.ResponseWriter, r *http.Request) {
	queryValues := r.URL.Query()
	code := queryValues.Get("code")
	referer := queryValues.Get("referer")
	state := queryValues.Get("state")

	redirectAfterLogin := state
	if decoder, ok := p.UserOauthSessionSvc.(domain.OAuthStateDecoder); ok && state != "" {
		decoded, err := decoder.DecodeState(state)
		if err != nil {
			p.Log.Errorf("failed to decode oauth state: %v", err)
			http.Error(rw, "invalid oauth state", http.StatusBadRequest)
			return
		}
		redirectAfterLogin = decoded
	}
	if redirectAfterLogin == "" {
		redirectAfterLogin = referer
	}

	if redirectAfterLogin == "" {
		err := errors.New("missing redirect url after login")
		p.Log.Error(err)
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	if code == "" {
		err := errors.New("missing oauth code")
		p.Log.Error(err)
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	user := p.UserAuth.NewState()
	if err := user.Login(code, queryValues); err != nil {
		p.Log.Errorf("failed to login: %v", err)
		http.Error(rw, err.Error(), http.StatusUnauthorized)
		return
	}
	if info, result := user.GetInfo(r); result.Code == domain.AuthSuccess && info.SessionRefresh != nil {
		if err := p.WriteRefresh(rw, r, info.SessionRefresh); err != nil {
			p.Log.Errorf("failed to persist login session: %v", err)
			http.Error(rw, "failed to persist login session", http.StatusInternalServerError)
			return
		}
	}

	if !p.referMatcher.Match(redirectAfterLogin) {
		http.Error(rw, "invalid referer", http.StatusBadRequest)
		return
	}

	http.Redirect(rw, r, redirectAfterLogin, http.StatusFound)
}

func (p *provider) Logout(rw http.ResponseWriter, r *http.Request) {
	referer := r.Header.Get("Referer")
	if len(referer) <= 0 {
		referer = p.Cfg.RedirectAfterLogin
	}
	if !p.referMatcher.Match(referer) {
		http.Error(rw, "invalid referer", http.StatusBadRequest)
		return
	}
	if cookie, err := r.Cookie(p.Cfg.SessionCookieName); err == nil {
		if revoker, ok := p.UserOauthSessionSvc.(domain.SessionRevoker); ok {
			if err := revoker.Revoke(r.Context(), cookie.Value); err != nil {
				p.Log.Errorf("failed to revoke local session: %v", err)
				http.Error(rw, "failed to revoke local session", http.StatusInternalServerError)
				return
			}
		}
	}
	p.clearSessionCookies(rw, r)

	logoutURL, err := p.UserOauthSessionSvc.LogoutURL(r.Context(), &pb.LogoutURLRequest{
		Referer: referer,
	})
	if err != nil {
		p.Log.Errorf("failed to get logout url, %v", err)
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	common.ResponseJSON(rw, &struct {
		URL string `json:"url"`
	}{
		URL: logoutURL.Data,
	})
}

func (p *provider) clearSessionCookies(rw http.ResponseWriter, r *http.Request) {
	domains := p.Cfg.SessionCookieDomains
	if len(domains) == 0 {
		domains = []string{""}
	}
	for _, domain := range domains {
		http.SetCookie(rw, &http.Cookie{
			Name: p.Cfg.SessionCookieName, Value: "", Path: "/", Domain: strings.TrimSpace(domain),
			HttpOnly: true, Secure: strings.EqualFold(p.Cfg.PlatformProtocol, "https") || r.TLS != nil,
			SameSite: http.SameSite(p.Cfg.CookieSameSite), MaxAge: -1, Expires: time.Unix(1, 0),
		})
	}
}
