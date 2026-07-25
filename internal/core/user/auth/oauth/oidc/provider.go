// Copyright (c) 2026 MosaicPlane Authors
// SPDX-License-Identifier: Apache-2.0

package oidc

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"strings"
	"time"

	coreoidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-redis/redis"
	"golang.org/x/oauth2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/erda-project/erda-infra/base/logs"
	"github.com/erda-project/erda-infra/base/servicehub"
	"github.com/erda-project/erda-infra/pkg/transport"
	"github.com/erda-project/erda-proto-go/core/user/oauth/pb"
	"github.com/erda-project/erda/internal/core/user/auth/oidcstore"
	directory "github.com/erda-project/erda/internal/core/user/impl/oidc"
)

type Config struct {
	IssuerURL      string        `file:"issuer_url"`
	ClientID       string        `file:"client_id"`
	ClientSecret   string        `file:"client_secret"`
	RedirectURI    string        `file:"redirect_uri"`
	Scopes         string        `file:"scopes" default:"openid profile email"`
	StateSecret    string        `file:"state_secret"`
	StateTTL       time.Duration `file:"state_ttl" default:"10m"`
	SessionTTL     time.Duration `file:"session_ttl" default:"24h"`
	RedisKeyPrefix string        `file:"redis_key_prefix" default:"mosaicplane:oidc:"`
}

type provider struct {
	pb.UnimplementedUserOAuthServiceServer
	pb.UnimplementedUserOAuthSessionServiceServer

	Register  transport.Register `autowired:"service-register" required:"true"`
	Log       logs.Logger
	Config    *Config
	Redis     *redis.Client       `autowired:"redis-client"`
	Directory directory.Interface `autowired:"erda.core.user.oidc"`

	oauth2Config       *oauth2.Config
	oidcProvider       *coreoidc.Provider
	verifier           *coreoidc.IDTokenVerifier
	store              *oidcstore.Store
	endSessionEndpoint string
}

type claims struct {
	Issuer            string `json:"iss"`
	Subject           string `json:"sub"`
	Nonce             string `json:"nonce"`
	PreferredUsername string `json:"preferred_username"`
	Name              string `json:"name"`
	Email             string `json:"email"`
	Phone             string `json:"phone_number"`
	Picture           string `json:"picture"`
}

func (p *provider) Init(_ servicehub.Context) error {
	if err := p.validateConfig(); err != nil {
		return err
	}
	discovery, err := coreoidc.NewProvider(context.Background(), strings.TrimRight(p.Config.IssuerURL, "/"))
	if err != nil {
		return fmt.Errorf("discover OIDC provider: %w", err)
	}
	p.oauth2Config = &oauth2.Config{
		ClientID: p.Config.ClientID, ClientSecret: p.Config.ClientSecret,
		Endpoint: discovery.Endpoint(), RedirectURL: p.Config.RedirectURI,
		Scopes: strings.Fields(p.Config.Scopes),
	}
	p.oidcProvider = discovery
	p.verifier = discovery.Verifier(&coreoidc.Config{ClientID: p.Config.ClientID})
	var metadata struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	if err := discovery.Claims(&metadata); err != nil {
		return fmt.Errorf("decode OIDC discovery: %w", err)
	}
	p.endSessionEndpoint = metadata.EndSessionEndpoint
	secret := p.Config.StateSecret
	if secret == "" {
		secret = p.Config.ClientSecret
	}
	key := sha256.Sum256([]byte(secret))
	p.store = &oidcstore.Store{Redis: p.Redis, Prefix: p.Config.RedisKeyPrefix, StateSecret: key[:], StateTTL: p.Config.StateTTL, SessionTTL: p.Config.SessionTTL}
	if p.Register != nil {
		pb.RegisterUserOAuthServiceImp(p.Register, p)
		pb.RegisterUserOAuthSessionServiceImp(p.Register, p)
	}
	return nil
}

func (p *provider) AuthURL(_ context.Context, req *pb.AuthURLRequest) (*pb.AuthURLResponse, error) {
	state, nonce, err := p.store.NewState(req.Referer)
	if err != nil {
		return nil, fmt.Errorf("create OIDC state: %w", err)
	}
	return &pb.AuthURLResponse{Data: p.oauth2Config.AuthCodeURL(state, coreoidc.Nonce(nonce), oauth2.AccessTypeOffline)}, nil
}

func (p *provider) DecodeState(state string) (string, error) { return p.store.DecodeState(state) }

func (p *provider) Revoke(_ context.Context, sessionID string) error {
	return p.store.DeleteSession(sessionID)
}

func (p *provider) ExchangeCode(ctx context.Context, req *pb.ExchangeCodeRequest) (*pb.OAuthToken, error) {
	state := extraParam(req, "state")
	_, nonce, err := p.store.ConsumeState(state)
	if err != nil {
		return nil, err
	}
	token, err := p.oauth2Config.Exchange(ctx, req.Code)
	if err != nil {
		return nil, fmt.Errorf("exchange OIDC authorization code: %w", err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return nil, fmt.Errorf("OIDC token response does not contain id_token")
	}
	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("verify OIDC ID token: %w", err)
	}
	var identity claims
	if err := idToken.Claims(&identity); err != nil {
		return nil, fmt.Errorf("decode OIDC ID token: %w", err)
	}
	if identity.Nonce != nonce {
		return nil, fmt.Errorf("OIDC nonce mismatch")
	}
	userinfo, err := p.fetchUserInfo(ctx, token)
	if err != nil {
		return nil, err
	}
	if userinfo.Subject != "" && userinfo.Subject != identity.Subject {
		return nil, fmt.Errorf("OIDC userinfo subject does not match ID token subject")
	}
	mergeClaims(&identity, userinfo)
	user, err := p.Directory.UpsertIdentity(ctx, directory.Claims{
		Issuer: identity.Issuer, Subject: identity.Subject, Username: identity.PreferredUsername,
		Name: identity.Name, Email: identity.Email, Phone: identity.Phone, Avatar: identity.Picture,
	})
	if err != nil {
		return nil, fmt.Errorf("provision OIDC user: %w", err)
	}
	sessionID, err := p.store.NewSession(user, identity.Issuer, identity.Subject)
	if err != nil {
		return nil, fmt.Errorf("create OIDC session: %w", err)
	}
	return &pb.OAuthToken{AccessToken: sessionID, TokenType: "Session", ExpiresIn: int64(p.Config.SessionTTL.Seconds())}, nil
}

func (p *provider) fetchUserInfo(ctx context.Context, token *oauth2.Token) (*claims, error) {
	userinfo, err := p.oidcProvider.UserInfo(ctx, oauth2.StaticTokenSource(token))
	if err != nil {
		return nil, fmt.Errorf("request OIDC userinfo: %w", err)
	}
	var result claims
	if err := userinfo.Claims(&result); err != nil {
		return nil, fmt.Errorf("decode OIDC userinfo: %w", err)
	}
	return &result, nil
}

func (p *provider) ExchangePassword(context.Context, *pb.ExchangePasswordRequest) (*pb.OAuthToken, error) {
	return nil, status.Error(codes.Unimplemented, "local password login is disabled; use OIDC authorization code login")
}

func (p *provider) ExchangeClientCredentials(context.Context, *pb.ExchangeClientCredentialsRequest) (*pb.OAuthToken, error) {
	return nil, status.Error(codes.Unimplemented, "OIDC user sessions do not support client credentials")
}

func (p *provider) LogoutURL(_ context.Context, req *pb.LogoutURLRequest) (*pb.LogoutURLResponse, error) {
	if p.endSessionEndpoint == "" {
		return &pb.LogoutURLResponse{Data: req.Referer}, nil
	}
	u, err := url.Parse(p.endSessionEndpoint)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("client_id", p.Config.ClientID)
	q.Set("post_logout_redirect_uri", req.Referer)
	u.RawQuery = q.Encode()
	return &pb.LogoutURLResponse{Data: u.String()}, nil
}

func (p *provider) validateConfig() error {
	for name, value := range map[string]string{"issuer_url": p.Config.IssuerURL, "client_id": p.Config.ClientID, "client_secret": p.Config.ClientSecret, "redirect_uri": p.Config.RedirectURI} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("OIDC %s is required", name)
		}
	}
	issuer, err := url.Parse(p.Config.IssuerURL)
	if err != nil || issuer.Scheme == "" || issuer.Host == "" {
		return fmt.Errorf("invalid OIDC issuer URL")
	}
	if p.Redis == nil || p.Directory == nil {
		return fmt.Errorf("OIDC provider requires Redis and user directory")
	}
	return nil
}

func extraParam(req *pb.ExchangeCodeRequest, key string) string {
	if req == nil {
		return ""
	}
	value := req.ExtraParams[key]
	if value == nil || len(value.Values) == 0 {
		return ""
	}
	return value.Values[0]
}

func mergeClaims(dst, src *claims) {
	if src == nil {
		return
	}
	if src.PreferredUsername != "" {
		dst.PreferredUsername = src.PreferredUsername
	}
	if src.Name != "" {
		dst.Name = src.Name
	}
	if src.Email != "" {
		dst.Email = src.Email
	}
	if src.Phone != "" {
		dst.Phone = src.Phone
	}
	if src.Picture != "" {
		dst.Picture = src.Picture
	}
}

func init() {
	servicehub.Register("erda.core.user.oauth.oidc", &servicehub.Spec{
		Services: pb.ServiceNames(), Types: pb.Types(), ConfigFunc: func() interface{} { return &Config{} },
		Creator: func() servicehub.Provider { return &provider{} },
	})
}
