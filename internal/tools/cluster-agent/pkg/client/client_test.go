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

package client

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	clientgotesting "k8s.io/client-go/testing"

	"github.com/erda-project/erda/internal/tools/cluster-agent/config"
	"github.com/erda-project/erda/pkg/k8sclient"
)

func TestLoadClusterInfo_UsesTokenRequest(t *testing.T) {
	now := time.Date(2026, time.July, 25, 8, 0, 0, 0, time.UTC)
	expiresAt := now.Add(time.Hour)
	clientSet := fakeclientset.NewSimpleClientset()
	clientSet.PrependReactor("create", "serviceaccounts", func(action clientgotesting.Action) (bool, runtime.Object, error) {
		assert.Equal(t, "token", action.GetSubresource())
		create, ok := action.(clientgotesting.CreateAction)
		require.True(t, ok)
		request, ok := create.GetObject().(*authenticationv1.TokenRequest)
		require.True(t, ok)
		require.NotNil(t, request.Spec.ExpirationSeconds)
		assert.Equal(t, int64(3600), *request.Spec.ExpirationSeconds)
		return true, newTokenRequest(" short-lived-token\n", expiresAt), nil
	})

	c := newLoadClusterInfoTestClient(clientSet, now)
	clusterInfo, err := c.loadClusterInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "https://kubernetes.default.svc", clusterInfo.Address)
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("fake ca data")), clusterInfo.CACert)
	assert.Equal(t, "short-lived-token", clusterInfo.Token)
	assert.Equal(t, expiresAt, clusterInfo.tokenExpiresAt)

	for _, action := range clientSet.Actions() {
		assert.NotEqual(t, "secrets", action.GetResource().Resource, "cluster info loading must not use legacy token Secrets")
	}
}

func TestLoadClusterInfo_UsesConfiguredTokenLifetime(t *testing.T) {
	now := time.Date(2026, time.July, 25, 8, 0, 0, 0, time.UTC)
	clientSet := fakeclientset.NewSimpleClientset()
	clientSet.PrependReactor("create", "serviceaccounts", func(action clientgotesting.Action) (bool, runtime.Object, error) {
		create := action.(clientgotesting.CreateAction)
		request := create.GetObject().(*authenticationv1.TokenRequest)
		require.NotNil(t, request.Spec.ExpirationSeconds)
		assert.Equal(t, int64(7200), *request.Spec.ExpirationSeconds)
		return true, newTokenRequest("token", now.Add(2*time.Hour)), nil
	})

	c := newLoadClusterInfoTestClient(clientSet, now)
	c.cfg.TokenExpirationSeconds = 7200
	_, err := c.loadClusterInfo(context.Background())
	require.NoError(t, err)
}

func TestLoadClusterInfo_ReturnsCAReadErrorBeforeRequestingToken(t *testing.T) {
	clientSet := fakeclientset.NewSimpleClientset()
	c := newLoadClusterInfoTestClient(clientSet, time.Now())
	c.readFile = func(string) ([]byte, error) {
		return nil, errors.New("CA volume unavailable")
	}

	_, err := c.loadClusterInfo(context.Background())
	require.EqualError(t, err, "read in-cluster service account CA: CA volume unavailable")
	assert.Empty(t, clientSet.Actions())
}

func TestLoadClusterInfo_ReturnsInClusterClientError(t *testing.T) {
	c := New(WithConfig(newLoadClusterInfoTestConfig()))
	c.readFile = func(string) ([]byte, error) { return []byte("ca"), nil }
	c.newInClusterClient = func(...k8sclient.Option) (*k8sclient.K8sClient, error) {
		return nil, errors.New("new in-cluster client failed")
	}

	_, err := c.loadClusterInfo(context.Background())
	require.EqualError(t, err, "new in-cluster client failed")
}

func TestLoadClusterInfo_ReturnsTokenRequestError(t *testing.T) {
	clientSet := fakeclientset.NewSimpleClientset()
	clientSet.PrependReactor("create", "serviceaccounts", func(clientgotesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("token API unavailable")
	})
	c := newLoadClusterInfoTestClient(clientSet, time.Now())

	_, err := c.loadClusterInfo(context.Background())
	require.EqualError(t, err, "request service account token: token API unavailable")
}

func TestLoadClusterInfo_RejectsInvalidTokenResponses(t *testing.T) {
	now := time.Date(2026, time.July, 25, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		response *authenticationv1.TokenRequest
		wantErr  string
	}{
		{
			name:     "empty token",
			response: newTokenRequest("", now.Add(time.Hour)),
			wantErr:  "service account token response is empty",
		},
		{
			name:     "missing expiration",
			response: newTokenRequest("token", time.Time{}),
			wantErr:  "service account token response has no expiration timestamp",
		},
		{
			name:     "already expired",
			response: newTokenRequest("token", now.Add(-time.Second)),
			wantErr:  "service account token is already expired",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientSet := fakeclientset.NewSimpleClientset()
			clientSet.PrependReactor("create", "serviceaccounts", func(clientgotesting.Action) (bool, runtime.Object, error) {
				return true, tt.response, nil
			})
			c := newLoadClusterInfoTestClient(clientSet, now)

			_, err := c.loadClusterInfo(context.Background())
			require.EqualError(t, err, tt.wantErr)
		})
	}
}

func TestTokenRefreshDeadline_UsesEightyPercentOfLifetime(t *testing.T) {
	now := time.Date(2026, time.July, 25, 8, 0, 0, 0, time.UTC)
	c := New()
	c.now = func() time.Time { return now }

	assert.Equal(t, now.Add(48*time.Minute), c.tokenRefreshDeadline(now.Add(time.Hour)))
	assert.Equal(t, now, c.tokenRefreshDeadline(now.Add(-time.Second)))
}

func TestDisConnect_DoesNotBlockWithoutReceiver(t *testing.T) {
	c := New()

	done := make(chan struct{})
	go func() {
		c.DisConnect()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("DisConnect blocked without an active connect receiver")
	}
}

func TestOnConnect_ReturnsWhenDisConnectRequested(t *testing.T) {
	c := New()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.setActiveConnectCancel(cancel)

	done := make(chan error, 1)
	go func() {
		done <- c.onConnect(ctx, nil)
	}()

	c.DisConnect()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("onConnect did not return after disconnect request")
	}
}

func newLoadClusterInfoTestClient(clientSet *fakeclientset.Clientset, now time.Time) *Client {
	c := New(WithConfig(newLoadClusterInfoTestConfig()))
	c.newInClusterClient = func(...k8sclient.Option) (*k8sclient.K8sClient, error) {
		return &k8sclient.K8sClient{ClientSet: clientSet}, nil
	}
	c.readFile = func(string) ([]byte, error) {
		return []byte("fake ca data"), nil
	}
	c.now = func() time.Time { return now }
	return c
}

func newLoadClusterInfoTestConfig() *config.Config {
	return &config.Config{
		CollectClusterInfo:     true,
		ErdaNamespace:          metav1.NamespaceDefault,
		K8SApiServerAddr:       "https://kubernetes.default.svc",
		ServiceAccount:         "cluster-agent",
		TokenExpirationSeconds: 3600,
	}
}

func newTokenRequest(token string, expiresAt time.Time) *authenticationv1.TokenRequest {
	return &authenticationv1.TokenRequest{
		Status: authenticationv1.TokenRequestStatus{
			Token:               token,
			ExpirationTimestamp: metav1.NewTime(expiresAt),
		},
	}
}
