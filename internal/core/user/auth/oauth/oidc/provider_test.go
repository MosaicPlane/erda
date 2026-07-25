// Copyright (c) 2026 MosaicPlane Authors
// SPDX-License-Identifier: Apache-2.0

package oidc

import (
	"context"
	"crypto/sha256"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis"
	"golang.org/x/oauth2"

	"github.com/erda-project/erda-proto-go/core/user/oauth/pb"
	"github.com/erda-project/erda/internal/core/user/auth/oidcstore"
)

func TestAuthURLUsesSignedStateAndNonce(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	key := sha256.Sum256([]byte("client-secret"))
	store := &oidcstore.Store{Redis: client, StateSecret: key[:], StateTTL: time.Minute}
	p := &provider{
		Config:       &Config{ClientID: "client", RedirectURI: "https://app.example/logincb"},
		store:        store,
		oauth2Config: &oauth2.Config{ClientID: "client", RedirectURL: "https://app.example/logincb", Scopes: []string{"openid", "profile"}, Endpoint: oauth2.Endpoint{AuthURL: "https://id.example/authorize"}},
	}
	response, err := p.AuthURL(context.Background(), &pb.AuthURLRequest{Referer: "https://app.example/projects"})
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(response.Data)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("nonce") == "" || u.Query().Get("state") == "" {
		t.Fatalf("missing OIDC protections in %s", response.Data)
	}
	if got, err := p.DecodeState(u.Query().Get("state")); err != nil || got != "https://app.example/projects" {
		t.Fatalf("decode state: %q, %v", got, err)
	}
}

func TestPasswordAndClientCredentialsAreDisabled(t *testing.T) {
	p := &provider{}
	if _, err := p.ExchangePassword(context.Background(), &pb.ExchangePasswordRequest{}); err == nil {
		t.Fatal("password login must be disabled")
	}
	if _, err := p.ExchangeClientCredentials(context.Background(), &pb.ExchangeClientCredentialsRequest{}); err == nil {
		t.Fatal("client credentials must be disabled for user sessions")
	}
}

func TestMergeClaimsPrefersUserInfo(t *testing.T) {
	dst := &claims{Issuer: "issuer", Subject: "subject", Email: "old@example.com"}
	mergeClaims(dst, &claims{Name: "User", Email: "new@example.com"})
	if dst.Issuer != "issuer" || dst.Subject != "subject" || dst.Name != "User" || dst.Email != "new@example.com" {
		t.Fatalf("unexpected claims: %+v", dst)
	}
}

func TestValidateConfig(t *testing.T) {
	p := &provider{Config: &Config{IssuerURL: "not-a-url"}}
	if err := p.validateConfig(); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected required config error, got %v", err)
	}
}
