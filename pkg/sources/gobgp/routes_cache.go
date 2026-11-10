package gobgp

import (
	"context"
	"sync"
	"time"

	gobgpapi "github.com/osrg/gobgp/v3/api"

	"github.com/alice-lg/alice-lg/pkg/api"
	"github.com/alice-lg/alice-lg/pkg/caches"
)

// routesFetch is a ListPath request of a neighbor table
// in flight. Concurrent requests for the same table wait
// for its result instead of querying GoBGP again.
type routesFetch struct {
	done chan struct{}
	res  *api.RoutesResponse
	err  error
}

// routesFetcher loads the routes of a neighbor table
// once and keeps the result in a cache for the
// configured TTL. Paginating and filtering the routes
// of a neighbor in the UI is then done in memory.
type routesFetcher struct {
	sync.Mutex
	inflight map[string]*routesFetch
}

func newRoutesFetcher() *routesFetcher {
	return &routesFetcher{
		inflight: make(map[string]*routesFetch),
	}
}

// cachedRoutes returns the routes of the neighbor table
// from the cache or fetches them from GoBGP.
// The returned response is shared and must not be modified.
func (gobgp *GoBGP) cachedRoutes(
	ctx context.Context,
	cache *caches.RoutesCache,
	tableType gobgpapi.TableType,
	neighborID string,
) (*api.RoutesResponse, error) {
	if res := cache.Get(neighborID); res != nil {
		return fromCache(res), nil
	}

	key := tableType.String() + "/" + neighborID
	f := gobgp.fetcher

	f.Lock()
	fetch, ok := f.inflight[key]
	if !ok {
		// Check the cache again: a fetch could have
		// completed in the meantime.
		if res := cache.Get(neighborID); res != nil {
			f.Unlock()
			return fromCache(res), nil
		}
		fetch = &routesFetch{done: make(chan struct{})}
		f.inflight[key] = fetch
		go gobgp.fetchRoutes(ctx, cache, tableType, neighborID, key, fetch)
	}
	f.Unlock()

	select {
	case <-fetch.done:
		return fetch.res, fetch.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// fetchRoutes queries GoBGP and stores the result in the
// cache. The fetch is not canceled with the request that
// started it, as other requests may wait for the result.
func (gobgp *GoBGP) fetchRoutes(
	ctx context.Context,
	cache *caches.RoutesCache,
	tableType gobgpapi.TableType,
	neighborID string,
	key string,
	fetch *routesFetch,
) {
	defer func() {
		gobgp.fetcher.Lock()
		delete(gobgp.fetcher.inflight, key)
		gobgp.fetcher.Unlock()
		close(fetch.done)
	}()

	ctx = context.WithoutCancel(ctx)
	neigh, err := gobgp.lookupNeighbor(ctx, neighborID)
	if err != nil {
		fetch.err = err
		return
	}

	routes := NewRoutesResponse()
	if err := gobgp.GetRoutes(ctx, neigh, tableType, &routes); err != nil {
		fetch.err = err
		return
	}

	now := time.Now().UTC()
	ttl := gobgp.config.CacheTTL
	routes.Response.Meta = &api.Meta{
		CacheStatus: api.CacheStatus{
			CachedAt: now,
			OrigTTL:  int(ttl.Seconds()),
		},
		TTL: now.Add(ttl),
	}

	cache.Set(neighborID, &routes)
	fetch.res = &routes
}

// fromCache marks a shallow copy of a cached
// response as result from cache.
func fromCache(res *api.RoutesResponse) *api.RoutesResponse {
	meta := *res.Response.Meta
	meta.ResultFromCache = true

	cached := *res
	cached.Response.Meta = &meta
	return &cached
}
