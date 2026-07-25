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

package oidcstore

import (
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis"

	commonpb "github.com/erda-project/erda-proto-go/common/pb"
)

func testStore(t *testing.T) (*Store, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return &Store{Redis: client, StateSecret: []byte(strings.Repeat("s", 32)), StateTTL: time.Minute, SessionTTL: time.Hour}, server
}

func TestStateIsSignedAndSingleUse(t *testing.T) {
	store, _ := testStore(t)
	state, nonce, err := store.NewState("https://mosaicplane.local/projects")
	if err != nil {
		t.Fatal(err)
	}
	if nonce == "" {
		t.Fatal("expected nonce")
	}
	referer, err := store.DecodeState(state)
	if err != nil || referer != "https://mosaicplane.local/projects" {
		t.Fatalf("decode state: %q, %v", referer, err)
	}
	referer, consumedNonce, err := store.ConsumeState(state)
	if err != nil || referer == "" || consumedNonce != nonce {
		t.Fatalf("consume state: %q %q %v", referer, consumedNonce, err)
	}
	if _, _, err := store.ConsumeState(state); err == nil {
		t.Fatal("expected replayed state to fail")
	}
}

func TestStateRejectsTampering(t *testing.T) {
	store, _ := testStore(t)
	state, _, err := store.NewState("https://mosaicplane.local")
	if err != nil {
		t.Fatal(err)
	}
	state = "x" + state[1:]
	if _, err := store.DecodeState(state); err == nil {
		t.Fatal("expected tampered state to fail")
	}
}

func TestSessionRoundTripAndExpiry(t *testing.T) {
	store, server := testStore(t)
	store.SessionTTL = time.Minute
	id, err := store.NewSession(&commonpb.UserInfo{Id: "user-1", Name: "admin"}, "https://issuer", "subject")
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.GetSession(id)
	if err != nil || session.User.Id != "user-1" || session.Subject != "subject" {
		t.Fatalf("session: %+v, %v", session, err)
	}
	server.FastForward(2 * time.Minute)
	if _, err := store.GetSession(id); err == nil {
		t.Fatal("expected expired session to fail")
	}
}
