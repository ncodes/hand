package cache

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCache_NewReturnsNilWhenDisabled(t *testing.T) {
	require.Nil(t, New(Options[string, string]{}))
	require.Nil(t, New(Options[string, string]{TTL: -time.Second}))
}

func TestCache_GetSetDeleteAndClone(t *testing.T) {
	c := New(Options[string, []string]{
		TTL: time.Minute,
		Clone: func(value []string) []string {
			if value == nil {
				return nil
			}

			cloned := make([]string, len(value))
			copy(cloned, value)
			return cloned
		},
	})
	require.NotNil(t, c)

	c.Set("key", []string{"one", "two"})

	value, ok := c.Get("key")
	require.True(t, ok)
	require.Equal(t, []string{"one", "two"}, value)

	value[0] = "mutated"

	value, ok = c.Get("key")
	require.True(t, ok)
	require.Equal(t, []string{"one", "two"}, value)
	require.Equal(t, 1, c.Len())

	c.Delete("key")

	value, ok = c.Get("key")
	require.False(t, ok)
	require.Nil(t, value)
	require.Zero(t, c.Len())
}

func TestCache_GetRemovesExpiredEntries(t *testing.T) {
	now := time.Unix(100, 0)
	c := New(Options[string, string]{
		TTL: time.Minute,
		Now: func() time.Time { return now },
	})
	require.NotNil(t, c)

	c.Set("key", "value")
	now = now.Add(time.Minute)

	value, ok := c.Get("key")
	require.False(t, ok)
	require.Empty(t, value)
	require.Zero(t, c.Len())
}

func TestCache_CleanupRemovesExpiredEntriesAutomatically(t *testing.T) {
	c := New(Options[string, string]{
		TTL: 20 * time.Millisecond,
	})
	require.NotNil(t, c)

	c.Set("key", "value")
	require.Eventually(t, func() bool {
		return c.Len() == 0
	}, time.Second, 10*time.Millisecond)
}

func TestCache_IsSafeForConcurrentUse(t *testing.T) {
	c := New(Options[int, int]{TTL: time.Minute})
	require.NotNil(t, c)

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(value int) {
			defer wg.Done()
			c.Set(value, value)
			got, ok := c.Get(value)
			require.True(t, ok)
			require.Equal(t, value, got)
		}(i)
	}

	wg.Wait()
	require.Equal(t, 50, c.Len())
}

func TestCache_NilReceiverMethods(t *testing.T) {
	var c *Cache[string, string]

	value, ok := c.Get("key")
	require.False(t, ok)
	require.Empty(t, value)

	c.Set("key", "value")
	c.Delete("key")
	require.Zero(t, c.Len())
	c.cleanupExpired()
}
