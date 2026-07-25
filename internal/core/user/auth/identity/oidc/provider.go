// Copyright (c) 2026 MosaicPlane Authors
// SPDX-License-Identifier: Apache-2.0

package oidc

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/erda-project/erda-infra/base/servicehub"
	"github.com/erda-project/erda-infra/pkg/transport"
	"github.com/erda-project/erda-proto-go/core/user/identity/pb"
	"github.com/erda-project/erda/internal/core/user/auth/oidcstore"
)

type Config struct {
	SessionTTL     time.Duration `file:"session_ttl" default:"24h"`
	RedisKeyPrefix string        `file:"redis_key_prefix" default:"mosaicplane:oidc:"`
}

type provider struct {
	pb.UnimplementedUserIdentityServiceServer
	Register transport.Register `autowired:"service-register"`
	Config   *Config
	Redis    *redis.Client `autowired:"redis-client"`
	store    *oidcstore.Store
}

func (p *provider) Init(_ servicehub.Context) error {
	if p.Redis == nil {
		return fmt.Errorf("OIDC identity provider requires Redis")
	}
	p.store = &oidcstore.Store{Redis: p.Redis, Prefix: p.Config.RedisKeyPrefix, SessionTTL: p.Config.SessionTTL}
	if p.Register != nil {
		pb.RegisterUserIdentityServiceImp(p.Register, p)
	}
	return nil
}

func (p *provider) GetCurrentUser(_ context.Context, req *pb.GetCurrentUserRequest) (*pb.GetCurrentUserResponse, error) {
	if req.Source != pb.TokenSource_Grant && req.Source != pb.TokenSource_Cookie {
		return nil, fmt.Errorf("unsupported OIDC credential source: %s", req.Source)
	}
	session, err := p.store.GetSession(req.AccessToken)
	if err != nil {
		return nil, err
	}
	response := &pb.GetCurrentUserResponse{Data: session.User}
	if req.Source == pb.TokenSource_Grant {
		httpOnly := true
		response.SessionRefresh = &pb.SessionRefresh{Cookie: &pb.CookieRefresh{
			Value: req.AccessToken, Path: "/", HttpOnly: &httpOnly,
			MaxAge: int32(p.Config.SessionTTL.Seconds()), ExpireAt: timestamppb.New(time.Now().Add(p.Config.SessionTTL)),
		}}
	}
	return response, nil
}

func init() {
	servicehub.Register("erda.core.user.identity.oidc", &servicehub.Spec{
		Services: pb.ServiceNames(), Types: pb.Types(), ConfigFunc: func() interface{} { return &Config{} },
		Creator: func() servicehub.Provider { return &provider{} },
	})
}
