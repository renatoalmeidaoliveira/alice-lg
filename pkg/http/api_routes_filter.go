package http

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alice-lg/alice-lg/pkg/api"
)

// Kinds of neighbor routes
const (
	routesReceived    = "received"
	routesFiltered    = "filtered"
	routesNotExported = "not-exported"
)

// filteredRoutesCacheSize is the max number of
// filtered result sets kept in memory.
const filteredRoutesCacheSize = 64

// maxMPLSLabelFilters is the max number of MPLS labels
// in the available filters. With a label per prefix there
// are as many labels as routes; listing all of them makes
// the response huge and the select unusable. Other labels
// can still be filtered by entering them.
const maxMPLSLabelFilters = 100

// limitFiltersAvailable keeps only the most common
// MPLS labels in the available filters.
func limitFiltersAvailable(filters *api.SearchFilters) {
	filters.GetGroupByKey(api.SearchKeyMPLSLabels).Limit(
		maxMPLSLabelFilters)
}

// filteredRoutes is the result of filtering the routes
// of a neighbor. It must not be modified once cached.
type filteredRoutes struct {
	routes    api.Routes
	available *api.SearchFilters
	applied   *api.SearchFilters

	expires time.Time
}

// filteredRoutesCache keeps filtered result sets of
// cached source responses. Paginating a result set
// does not filter all routes of the neighbor again.
type filteredRoutesCache struct {
	sync.Mutex
	entries map[string]*filteredRoutes
}

func newFilteredRoutesCache() *filteredRoutesCache {
	return &filteredRoutesCache{
		entries: make(map[string]*filteredRoutes),
	}
}

// Get a filtered result set if present and not expired
func (c *filteredRoutesCache) Get(key string) *filteredRoutes {
	c.Lock()
	defer c.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return nil
	}
	if time.Now().After(entry.expires) {
		delete(c.entries, key)
		return nil
	}
	return entry
}

// Set a filtered result set. When the cache is full,
// expired entries and the entry expiring first are
// removed.
func (c *filteredRoutesCache) Set(key string, entry *filteredRoutes) {
	c.Lock()
	defer c.Unlock()

	if len(c.entries) >= filteredRoutesCacheSize {
		now := time.Now()
		var (
			firstKey     string
			firstExpires time.Time
		)
		for k, e := range c.entries {
			if now.After(e.expires) {
				delete(c.entries, k)
				continue
			}
			if firstKey == "" || e.expires.Before(firstExpires) {
				firstKey = k
				firstExpires = e.expires
			}
		}
		if len(c.entries) >= filteredRoutesCacheSize {
			delete(c.entries, firstKey)
		}
	}
	c.entries[key] = entry
}

// filteredRoutesKey identifies a filtered result set.
// Only responses with cache information can be
// cached: the key includes the time the source
// response was created, so a refreshed response
// will not match.
func filteredRoutesKey(
	req *http.Request,
	rsID string,
	neighborID string,
	kind string,
	meta *api.Meta,
) (string, bool) {
	if meta == nil || meta.CacheStatus.CachedAt.IsZero() {
		return "", false
	}
	if !time.Now().Before(meta.TTL) {
		return "", false
	}

	// The page is not part of the result set
	query := req.URL.Query()
	query.Del("page")

	return strings.Join([]string{
		rsID,
		neighborID,
		kind,
		strconv.FormatInt(meta.CacheStatus.CachedAt.UnixNano(), 10),
		query.Encode(),
	}, "|"), true
}

// filterRoutes applies the query and the filters of
// the request to the routes of a neighbor and collects
// the filters available in the result set.
func (s *Server) filterRoutes(
	req *http.Request,
	rsID string,
	neighborID string,
	kind string,
	meta *api.Meta,
	routes api.Routes,
) (api.Routes, *api.SearchFilters, *api.SearchFilters, error) {
	key, cacheable := filteredRoutesKey(req, rsID, neighborID, kind, meta)
	if cacheable {
		if res := s.filteredRoutes.Get(key); res != nil {
			return res.routes, res.available, res.applied, nil
		}
	}

	// Filter routes based on criteria if present
	allRoutes := apiQueryFilterNextHopGateway(req, "q", routes)
	results := api.Routes{}

	// Apply other (community) filters
	filtersApplied, err := api.FiltersFromQuery(req.URL.Query())
	if err != nil {
		return nil, nil, nil, err
	}

	var hasIP4, hasIP6 bool
	filtersAvailable := api.NewSearchFilters()
	filtersNoLabels := filtersApplied.WithoutGroup(api.SearchKeyMPLSLabels)
	for _, r := range allRoutes {
		if !filtersApplied.MatchRoute(r) {
			// Keep the labels of routes, which are only excluded
			// by the MPLS label filter, selectable.
			if filtersNoLabels.MatchRoute(r) {
				filtersAvailable.UpdateMPLSLabelsFromRoute(r)
			}
			continue // Exclude route from results set
		}
		results = append(results, r)
		filtersAvailable.UpdateFromRoute(r)
		hasIP4 = hasIP4 || r.AddrFamily == api.AddrFamilyIPv4
		hasIP6 = hasIP6 || r.AddrFamily == api.AddrFamilyIPv6
	}
	filtersAvailable.SetFilterAddrFamilies(hasIP4, hasIP6)

	// Remove applied filters from available
	filtersApplied.MergeProperties(filtersAvailable)
	filtersAvailable = filtersAvailable.Sub(filtersApplied)
	limitFiltersAvailable(filtersAvailable)

	if cacheable {
		s.filteredRoutes.Set(key, &filteredRoutes{
			routes:    results,
			available: filtersAvailable,
			applied:   filtersApplied,
			expires:   meta.TTL,
		})
	}

	return results, filtersAvailable, filtersApplied, nil
}
