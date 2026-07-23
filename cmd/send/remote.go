package main

import (
	"context"
	"sync"

	"github.com/sooua/send.to/client"
)

// remoteIndex is the server's record of this owner's uploads, fetched at most
// once and only if something actually needs it — `send rm` of a link this
// machine uploaded itself must not pay for a network round trip.
type remoteIndex struct {
	ctx context.Context
	api *client.Client

	once    sync.Once
	entries []client.RemoteEntry
}

func newRemoteIndex(ctx context.Context, api *client.Client) *remoteIndex {
	return &remoteIndex{ctx: ctx, api: api}
}

func (r *remoteIndex) all() []client.RemoteEntry {
	r.once.Do(func() {
		if r.api == nil || r.api.BaseURL == "" || r.api.OwnerToken == "" {
			return
		}
		// A server that keeps no list, or a token that has uploaded nothing,
		// simply leaves the index empty.
		r.entries, _ = r.api.RemoteList(r.ctx)
	})

	return r.entries
}

// deleteURL returns the deletion link the server holds for a share URL.
func (r *remoteIndex) deleteURL(shareURL string) string {
	for _, e := range r.all() {
		if e.URL == shareURL {
			return e.DeleteURL
		}
	}
	return ""
}
