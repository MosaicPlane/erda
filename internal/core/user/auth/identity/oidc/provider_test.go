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

package oidc

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis"

	commonpb "github.com/erda-project/erda-proto-go/common/pb"
	"github.com/erda-project/erda-proto-go/core/user/identity/pb"
	"github.com/erda-project/erda/internal/core/user/auth/oidcstore"
)

func TestGetCurrentUserCreatesCookieOnlyForGrant(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	store := &oidcstore.Store{Redis: client, StateSecret: []byte(strings.Repeat("s", 32)), SessionTTL: time.Hour}
	sessionID, err := store.NewSession(&commonpb.UserInfo{Id: "user-1", Name: "admin"}, "issuer", "subject")
	if err != nil {
		t.Fatal(err)
	}
	provider := &provider{Config: &Config{SessionTTL: time.Hour}, Redis: client, store: store}

	grant, err := provider.GetCurrentUser(context.Background(), &pb.GetCurrentUserRequest{Source: pb.TokenSource_Grant, AccessToken: sessionID})
	if err != nil {
		t.Fatal(err)
	}
	if grant.Data.Id != "user-1" || grant.SessionRefresh == nil || grant.SessionRefresh.Cookie.Value != sessionID {
		t.Fatalf("unexpected grant response: %+v", grant)
	}
	cookie, err := provider.GetCurrentUser(context.Background(), &pb.GetCurrentUserRequest{Source: pb.TokenSource_Cookie, AccessToken: sessionID})
	if err != nil {
		t.Fatal(err)
	}
	if cookie.SessionRefresh != nil {
		t.Fatal("cookie lookup must not continuously rewrite the session cookie")
	}
}
