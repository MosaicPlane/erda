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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/rancher/remotedialer"
	"github.com/sirupsen/logrus"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/erda-project/erda/apistructs"
	"github.com/erda-project/erda/internal/tools/cluster-agent/config"
	"github.com/erda-project/erda/pkg/discover"
	"github.com/erda-project/erda/pkg/k8sclient"
)

const (
	serviceAccountCAPath          = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	defaultTokenExpirationSeconds = int64(3600)
)

type KubernetesClusterInfo struct {
	Address string `json:"address"`
	Token   string `json:"token"`
	CACert  string `json:"caCert"`

	tokenExpiresAt time.Time
}

type Option func(*Client)

type Client struct {
	sync.Mutex
	cfg                  *config.Config
	accessKey            string
	activeConnectCancel  context.CancelFunc
	newInClusterClient   func(...k8sclient.Option) (*k8sclient.K8sClient, error)
	newCredentialWatcher func(context.Context, kubernetes.Interface, string) (credentialWatcher, error)
	watchRetryInterval   time.Duration
	readFile             func(string) ([]byte, error)
	now                  func() time.Time
}

func New(ops ...Option) *Client {
	c := Client{
		newInClusterClient: k8sclient.NewForInCluster,
		watchRetryInterval: time.Second,
		readFile:           os.ReadFile,
		now:                time.Now,
	}
	c.newCredentialWatcher = func(ctx context.Context, cs kubernetes.Interface, ns string) (credentialWatcher, error) {
		return c.getRetryWatcher(ctx, cs, ns)
	}
	for _, op := range ops {
		op(&c)
	}
	return &c
}

func WithConfig(cfg *config.Config) Option {
	return func(c *Client) {
		c.cfg = cfg
	}
}

func (c *Client) DisConnect() {
	c.requestReconnect()
}

func (c *Client) Start(ctx context.Context) error {
	headers := http.Header{
		"X-Erda-Cluster-Key": {c.cfg.ClusterKey},
	}

	ep, err := parseDialerEndpoint(c.cfg.ClusterManagerEndpoint)
	if err != nil {
		logrus.Errorf("failed to parse dial endpoint: %v", err)
		return err
	}

	// If specified cluster access key, preferred to use it.
	if c.cfg.ClusterAccessKey == "" {
		go func() {
			if err := c.watchClusterCredential(ctx); err != nil {
				logrus.Errorf("watch cluster info error: %v", err)
				return
			}
		}()
	} else {
		// Set access key values default
		c.setAccessKey(c.cfg.ClusterAccessKey)
		logrus.Info("use specified cluster access key")
	}

	for {
		accessKey := c.getAccessKey()
		if accessKey == "" {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(c.credentialWatchRetryDelay(0)):
			}
			continue
		}

		headers.Set("Authorization", accessKey)

		var tokenExpiresAt time.Time
		if c.cfg.CollectClusterInfo {
			clusterInfo, err := c.loadClusterInfo(ctx)
			if err != nil {
				logrus.Errorf("failed to refresh cluster API credential: %v", err)
				if !c.waitForConnectionRetry(ctx) {
					return nil
				}
				continue
			}
			bytes, err := json.Marshal(clusterInfo)
			if err != nil {
				return err
			}
			headers.Set("X-Erda-Cluster-Info", base64.StdEncoding.EncodeToString(bytes))
			tokenExpiresAt = clusterInfo.tokenExpiresAt
		}

		connectCtx, cancel := c.newConnectContext(ctx, tokenExpiresAt)
		c.setActiveConnectCancel(cancel)
		_ = remotedialer.ClientConnect(connectCtx, ep, headers, nil, func(proto, address string) bool {
			switch proto {
			case "tcp":
				return true
			case "unix":
				return address == "/var/run/docker.sock"
			case "npipe":
				return address == "//./pipe/docker_engine"
			}
			return false
		}, c.onConnect)
		c.setActiveConnectCancel(nil)
		cancel()

		if ctx.Err() != nil {
			return nil
		}
		if errors.Is(connectCtx.Err(), context.DeadlineExceeded) {
			logrus.Info("cluster API token refresh deadline reached, reconnecting")
			continue
		}
		if !c.waitForConnectionRetry(ctx) {
			return nil
		}
	}
}

// onConnect
func (c *Client) onConnect(ctx context.Context, _ *remotedialer.Session) error {
	<-ctx.Done()
	return nil
}

func (c *Client) requestReconnect() {
	c.Lock()
	cancel := c.activeConnectCancel
	c.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (c *Client) setActiveConnectCancel(cancel context.CancelFunc) {
	c.Lock()
	defer c.Unlock()
	c.activeConnectCancel = cancel
}

func (c *Client) loadClusterInfo(ctx context.Context) (*KubernetesClusterInfo, error) {
	clusterInfo := &KubernetesClusterInfo{
		Address: c.cfg.K8SApiServerAddr,
	}
	caData, err := c.readFile(serviceAccountCAPath)
	if err != nil {
		return nil, fmt.Errorf("read in-cluster service account CA: %w", err)
	}
	if len(caData) == 0 {
		return nil, errors.New("in-cluster service account CA is empty")
	}
	clusterInfo.CACert = base64.StdEncoding.EncodeToString(caData)

	k, err := c.newInClusterClient()
	if err != nil {
		return nil, err
	}

	expirationSeconds := c.cfg.TokenExpirationSeconds
	if expirationSeconds <= 0 {
		expirationSeconds = defaultTokenExpirationSeconds
	}
	tokenRequest, err := k.ClientSet.CoreV1().ServiceAccounts(c.cfg.ErdaNamespace).CreateToken(
		ctx,
		c.cfg.ServiceAccount,
		&authenticationv1.TokenRequest{Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: &expirationSeconds,
		}},
		metav1.CreateOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("request service account token: %w", err)
	}
	clusterInfo.Token = strings.TrimSpace(tokenRequest.Status.Token)
	if clusterInfo.Token == "" {
		return nil, errors.New("service account token response is empty")
	}
	clusterInfo.tokenExpiresAt = tokenRequest.Status.ExpirationTimestamp.Time
	if clusterInfo.tokenExpiresAt.IsZero() {
		return nil, errors.New("service account token response has no expiration timestamp")
	}
	if !clusterInfo.tokenExpiresAt.After(c.now()) {
		return nil, errors.New("service account token is already expired")
	}

	logrus.Debugf("loaded cluster API credential for %s, expires at %s", clusterInfo.Address, clusterInfo.tokenExpiresAt.Format(time.RFC3339))
	return clusterInfo, nil
}

func (c *Client) newConnectContext(ctx context.Context, tokenExpiresAt time.Time) (context.Context, context.CancelFunc) {
	if tokenExpiresAt.IsZero() {
		return context.WithCancel(ctx)
	}
	return context.WithDeadline(ctx, c.tokenRefreshDeadline(tokenExpiresAt))
}

func (c *Client) tokenRefreshDeadline(tokenExpiresAt time.Time) time.Time {
	now := c.now()
	remaining := tokenExpiresAt.Sub(now)
	if remaining <= 0 {
		return now
	}
	return now.Add(remaining * 4 / 5)
}

func (c *Client) waitForConnectionRetry(ctx context.Context) bool {
	delay := time.Duration(c.cfg.ConRetryInterval) * time.Second
	if delay <= 0 {
		delay = 10 * time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func parseDialerEndpoint(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}

	//inCluster, visit dialer inner service first.
	if os.Getenv(string(apistructs.DICE_IS_EDGE)) == "false" && discover.ClusterDialer() != "" {
		return "ws://" + discover.ClusterDialer() + u.Path, nil
	}

	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	}

	return u.String(), nil
}
