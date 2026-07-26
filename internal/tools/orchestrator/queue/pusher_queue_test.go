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
