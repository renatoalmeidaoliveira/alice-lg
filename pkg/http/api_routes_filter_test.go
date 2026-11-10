package http

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alice-lg/alice-lg/pkg/api"
)

// makeVPNRoutes creates n VPNv4 routes with a label per
// prefix, as allocated by most PEs.
func makeVPNRoutes(n int) api.Routes {
	neighborID := "neighbor"
	routes := make(api.Routes, 0, n)
	for i := 0; i < n; i++ {
		gw := fmt.Sprintf("192.0.2.%d", i%8)
		routes = append(routes, &api.Route{
			NeighborID:         &neighborID,
			Network:            fmt.Sprintf("10.%d.%d.0/24", i/256%256, i%256),
			Gateway:            &gw,
			AddrFamily:         api.AddrFamilyIPv4,
			RouteDistinguisher: fmt.Sprintf("64512:%d", i%20),
			MPLSLabels:         []uint32{uint32(16 + i)},
			BGP: &api.BGPInfo{
				NextHop: &gw,
				AsPath:  []int{64512, 64513 + i%10},
				Communities: api.Communities{
					{64512, i % 5}, {64512, 100},
				},
				ExtCommunities: api.ExtCommunities{
					{"rt", 64512, i % 20}, {"rt", 64512, 1000},
				},
			},
		})
	}
	return routes
}

func cachedMeta() *api.Meta {
	now := time.Now().UTC()
	return &api.Meta{
		CacheStatus: api.CacheStatus{CachedAt: now},
		TTL:         now.Add(time.Minute),
	}
}

func BenchmarkFilterRoutes(b *testing.B) {
	routes := makeVPNRoutes(108000)
	req := httptest.NewRequest("GET", "/routes/received?page=0", nil)
	meta := cachedMeta()

	b.Run("uncached", func(b *testing.B) {
		s := &Server{filteredRoutes: newFilteredRoutesCache()}
		for i := 0; i < b.N; i++ {
			s.filteredRoutes = newFilteredRoutesCache()
			_, available, _, err := s.filterRoutes(
				req, "rs", "neighbor", routesReceived, meta, routes)
			if err != nil {
				b.Fatal(err)
			}
			if i == 0 {
				payload, _ := json.Marshal(available)
				b.Logf("filters available: %d bytes", len(payload))
			}
		}
	})

	b.Run("cached", func(b *testing.B) {
		s := &Server{filteredRoutes: newFilteredRoutesCache()}
		for i := 0; i < b.N; i++ {
			if _, _, _, err := s.filterRoutes(
				req, "rs", "neighbor", routesReceived, meta, routes); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestFilterRoutesCache(t *testing.T) {
	routes := makeVPNRoutes(100)
	meta := cachedMeta()
	s := &Server{filteredRoutes: newFilteredRoutesCache()}

	req := httptest.NewRequest("GET", "/routes/received?page=0", nil)
	res, _, _, err := s.filterRoutes(
		req, "rs", "neighbor", routesReceived, meta, routes)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 100 {
		t.Fatal("expected all routes, got:", len(res))
	}

	// Another page of the same result set is cached
	req = httptest.NewRequest("GET", "/routes/received?page=3", nil)
	res2, _, _, _ := s.filterRoutes(
		req, "rs", "neighbor", routesReceived, meta, routes[:10])
	if len(res2) != 100 {
		t.Error("expected the cached result set, got:", len(res2))
	}

	// Filters are part of the key
	req = httptest.NewRequest(
		"GET", "/routes/received?communities=64512:1", nil)
	res3, _, applied, err := s.filterRoutes(
		req, "rs", "neighbor", routesReceived, meta, routes)
	if err != nil {
		t.Fatal(err)
	}
	if len(res3) != 20 {
		t.Error("expected 20 routes with community 64512:1, got:", len(res3))
	}
	if g := applied.GetGroupByKey(api.SearchKeyCommunities); len(g.Filters) != 1 {
		t.Error("expected applied community filter")
	}

	// A refreshed source response is not served from the cache
	refreshed := cachedMeta()
	refreshed.CacheStatus.CachedAt = meta.CacheStatus.CachedAt.Add(time.Second)
	req = httptest.NewRequest("GET", "/routes/received?page=0", nil)
	res4, _, _, _ := s.filterRoutes(
		req, "rs", "neighbor", routesReceived, refreshed, routes[:10])
	if len(res4) != 10 {
		t.Error("expected a new result set, got:", len(res4))
	}

	// Responses without cache information are not cached
	res5, _, _, _ := s.filterRoutes(
		req, "rs", "neighbor", routesReceived, nil, routes[:5])
	res6, _, _, _ := s.filterRoutes(
		req, "rs", "neighbor", routesReceived, nil, routes[:7])
	if len(res5) != 5 || len(res6) != 7 {
		t.Error("expected uncached results:", len(res5), len(res6))
	}
}

func TestFilteredRoutesCacheEviction(t *testing.T) {
	c := newFilteredRoutesCache()
	now := time.Now()
	for i := 0; i < filteredRoutesCacheSize+10; i++ {
		c.Set(fmt.Sprint(i), &filteredRoutes{
			expires: now.Add(time.Duration(i+1) * time.Minute),
		})
	}
	if len(c.entries) != filteredRoutesCacheSize {
		t.Error("unexpected cache size:", len(c.entries))
	}
	if c.Get("0") != nil {
		t.Error("expected the entry expiring first to be evicted")
	}

	c.Set("expired", &filteredRoutes{expires: now.Add(-time.Second)})
	if c.Get("expired") != nil {
		t.Error("expected expired entry to be ignored")
	}
}
