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

package queue

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewRedisClient(t *testing.T) {
	t.Run("standalone when sentinels are empty", func(t *testing.T) {
		client := newRedisClient("redis.example:6379", "secret", "", "")
		defer client.Close()
		require.Equal(t, "redis.example:6379", client.Options().Addr)
		require.Equal(t, "secret", client.Options().Password)
	})

	t.Run("sentinel when addresses are configured", func(t *testing.T) {
		client := newRedisClient("unused:6379", "secret", "my-master", "sentinel-a:26379,sentinel-b:26379")
		defer client.Close()
		require.Equal(t, "FailoverClient", client.Options().Addr)
	})
}
