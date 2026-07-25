// Copyright (c) 2026 MosaicPlane Authors
// SPDX-License-Identifier: Apache-2.0

package oidcstore

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/go-redis/redis"

	commonpb "github.com/erda-project/erda-proto-go/common/pb"
)

const defaultPrefix = "mosaicplane:oidc:"

type Store struct {
	Redis       *redis.Client
	Prefix      string
	StateSecret []byte
	StateTTL    time.Duration
	SessionTTL  time.Duration
}

type statePayload struct {
	ID      string `json:"id"`
	Referer string `json:"referer"`
}

type stateValue struct {
	Nonce string `json:"nonce"`
}

type Session struct {
	User      *commonpb.UserInfo `json:"user"`
	Issuer    string             `json:"issuer"`
	Subject   string             `json:"subject"`
	CreatedAt time.Time          `json:"createdAt"`
}

func (s *Store) NewState(referer string) (state, nonce string, err error) {
	if err = s.validate(); err != nil {
		return "", "", err
	}
	id, err := randomToken(32)
	if err != nil {
		return "", "", err
	}
	nonce, err = randomToken(32)
	if err != nil {
		return "", "", err
	}
	body, err := json.Marshal(statePayload{ID: id, Referer: referer})
	if err != nil {
		return "", "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(body)
	state = encoded + "." + s.sign(encoded)
	value, _ := json.Marshal(stateValue{Nonce: nonce})
	if err := s.Redis.Set(s.key("state:"+id), value, s.stateTTL()).Err(); err != nil {
		return "", "", err
	}
	return state, nonce, nil
}

func (s *Store) DecodeState(state string) (string, error) {
	payload, err := s.parseState(state)
	if err != nil {
		return "", err
	}
	return payload.Referer, nil
}

func (s *Store) ConsumeState(state string) (referer, nonce string, err error) {
	payload, err := s.parseState(state)
	if err != nil {
		return "", "", err
	}
	key := s.key("state:" + payload.ID)
	value, err := s.Redis.Get(key).Bytes()
	if err != nil {
		return "", "", errors.New("OIDC state is invalid or expired")
	}
	if deleted, err := s.Redis.Del(key).Result(); err != nil || deleted != 1 {
		return "", "", errors.New("OIDC state has already been consumed")
	}
	var stored stateValue
	if err := json.Unmarshal(value, &stored); err != nil || stored.Nonce == "" {
		return "", "", errors.New("invalid OIDC state payload")
	}
	return payload.Referer, stored.Nonce, nil
}

func (s *Store) NewSession(user *commonpb.UserInfo, issuer, subject string) (string, error) {
	if err := s.validate(); err != nil {
		return "", err
	}
	id, err := randomToken(32)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(Session{User: user, Issuer: issuer, Subject: subject, CreatedAt: time.Now().UTC()})
	if err != nil {
		return "", err
	}
	if err := s.Redis.Set(s.key("session:"+id), body, s.sessionTTL()).Err(); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Store) GetSession(id string) (*Session, error) {
	if id == "" {
		return nil, errors.New("missing OIDC session")
	}
	body, err := s.Redis.Get(s.key("session:" + id)).Bytes()
	if err != nil {
		return nil, errors.New("OIDC session is invalid or expired")
	}
	var session Session
	if err := json.Unmarshal(body, &session); err != nil {
		return nil, err
	}
	if session.User == nil || session.User.Id == "" {
		return nil, errors.New("invalid OIDC session payload")
	}
	return &session, nil
}

func (s *Store) DeleteSession(id string) error {
	if id == "" {
		return nil
	}
	return s.Redis.Del(s.key("session:" + id)).Err()
}

func (s *Store) parseState(state string) (*statePayload, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	parts := strings.Split(state, ".")
	if len(parts) != 2 || !hmac.Equal([]byte(parts[1]), []byte(s.sign(parts[0]))) {
		return nil, errors.New("invalid OIDC state signature")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("invalid OIDC state encoding")
	}
	var payload statePayload
	if err := json.Unmarshal(body, &payload); err != nil || payload.ID == "" || payload.Referer == "" {
		return nil, errors.New("invalid OIDC state payload")
	}
	return &payload, nil
}

func (s *Store) sign(value string) string {
	mac := hmac.New(sha256.New, s.StateSecret)
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Store) validate() error {
	if s.Redis == nil {
		return errors.New("OIDC store requires Redis")
	}
	if len(s.StateSecret) < 32 {
		return errors.New("OIDC state secret must contain at least 32 bytes")
	}
	return nil
}

func (s *Store) key(suffix string) string {
	prefix := s.Prefix
	if prefix == "" {
		prefix = defaultPrefix
	}
	return prefix + suffix
}

func (s *Store) stateTTL() time.Duration {
	if s.StateTTL <= 0 {
		return 10 * time.Minute
	}
	return s.StateTTL
}

func (s *Store) sessionTTL() time.Duration {
	if s.SessionTTL <= 0 {
		return 24 * time.Hour
	}
	return s.SessionTTL
}

func randomToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
