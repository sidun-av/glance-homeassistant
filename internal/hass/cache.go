package hass

import (
	"context"
	"sync"
	"time"
)

// AreaCache wraps Client.FetchAreas with a TTL: room→entity assignment
// changes rarely (only when the user edits areas in HA), so both /widget and
// /live.json share one cached copy instead of each hitting /api/template on
// every request. On a refresh failure, it serves the last-known mapping
// rather than propagating the error, as long as it has one to serve.
type AreaCache struct {
	mu        sync.Mutex
	client    *Client
	ttl       time.Duration
	rooms     []Room
	fetchedAt time.Time
}

func NewAreaCache(client *Client, ttl time.Duration) *AreaCache {
	return &AreaCache{client: client, ttl: ttl}
}

func (c *AreaCache) Get(ctx context.Context) ([]Room, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.rooms != nil && time.Since(c.fetchedAt) < c.ttl {
		return c.rooms, nil
	}

	rooms, err := c.client.FetchAreas(ctx)
	if err != nil {
		if c.rooms != nil {
			return c.rooms, nil
		}
		return nil, err
	}

	c.rooms = rooms
	c.fetchedAt = time.Now()
	return c.rooms, nil
}
