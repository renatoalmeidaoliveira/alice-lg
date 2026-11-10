package gobgp

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gobgpapi "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/packet/bgp"
	"google.golang.org/grpc"
)

// fakeClient is a GoBGP API client serving a single
// VPNv4 peer with one accepted and one filtered route.
type fakeClient struct {
	gobgpapi.GobgpApiClient

	peer      *gobgpapi.Peer
	paths     []*gobgpapi.Path
	prefix    string
	listPaths atomic.Int32
}

type fakePeerStream struct {
	grpc.ClientStream
	peers []*gobgpapi.Peer
}

func (s *fakePeerStream) Recv() (*gobgpapi.ListPeerResponse, error) {
	if len(s.peers) == 0 {
		return nil, io.EOF
	}
	peer := s.peers[0]
	s.peers = s.peers[1:]
	return &gobgpapi.ListPeerResponse{Peer: peer}, nil
}

type fakePathStream struct {
	grpc.ClientStream
	destinations []*gobgpapi.Destination
}

func (s *fakePathStream) Recv() (*gobgpapi.ListPathResponse, error) {
	if len(s.destinations) == 0 {
		return nil, io.EOF
	}
	dst := s.destinations[0]
	s.destinations = s.destinations[1:]
	return &gobgpapi.ListPathResponse{Destination: dst}, nil
}

func (c *fakeClient) ListPeer(
	_ context.Context,
	_ *gobgpapi.ListPeerRequest,
	_ ...grpc.CallOption,
) (gobgpapi.GobgpApi_ListPeerClient, error) {
	return &fakePeerStream{peers: []*gobgpapi.Peer{c.peer}}, nil
}

func (c *fakeClient) ListPath(
	_ context.Context,
	_ *gobgpapi.ListPathRequest,
	_ ...grpc.CallOption,
) (gobgpapi.GobgpApi_ListPathClient, error) {
	c.listPaths.Add(1)
	// Give concurrent requests time to pile up
	time.Sleep(50 * time.Millisecond)
	return &fakePathStream{destinations: []*gobgpapi.Destination{
		{Prefix: c.prefix, Paths: c.paths},
	}}, nil
}

func newFakeGoBGP(t *testing.T, ttl time.Duration) (*GoBGP, *fakeClient) {
	rd, err := bgp.ParseRouteDistinguisher("65000:100")
	if err != nil {
		t.Fatal(err)
	}
	nlri := bgp.NewLabeledVPNIPAddrPrefix(
		24, "10.1.2.0", *bgp.NewMPLSLabelStack(16001), rd)
	accepted := makeVPNPath(t, nlri, "192.0.2.1")
	filtered := makeVPNPath(t, nlri, "192.0.2.2")
	filtered.Filtered = true

	client := &fakeClient{
		peer: &gobgpapi.Peer{
			State: &gobgpapi.PeerState{
				NeighborAddress: "192.0.2.1",
				PeerAsn:         64512,
			},
			AfiSafis: []*gobgpapi.AfiSafi{
				{Config: &gobgpapi.AfiSafiConfig{Family: &gobgpapi.Family{
					Afi:  gobgpapi.Family_AFI_IP,
					Safi: gobgpapi.Family_SAFI_MPLS_VPN,
				}}},
			},
		},
		paths:  []*gobgpapi.Path{accepted, filtered},
		prefix: nlri.String(),
	}

	src := NewGoBGP(Config{
		Host:              "127.0.0.1:0",
		Insecure:          true,
		ProcessingTimeout: 10,
		CacheTTL:          ttl,
		RoutesCacheSize:   8,
	})
	src.client = client
	return src, client
}

func TestRoutesCacheSharedFetch(t *testing.T) {
	src, client := newFakeGoBGP(t, time.Minute)
	neighborID := PeerHash(client.peer)
	ctx := context.Background()

	// The UI requests received and filtered routes at once
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 4; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			res, err := src.RoutesReceived(ctx, neighborID)
			if err == nil && len(res.Imported) != 1 {
				t.Error("expected 1 received route, got:", len(res.Imported))
			}
			errs <- err
		}()
		go func() {
			defer wg.Done()
			res, err := src.RoutesFiltered(ctx, neighborID)
			if err == nil && len(res.Filtered) != 1 {
				t.Error("expected 1 filtered route, got:", len(res.Filtered))
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	if n := client.listPaths.Load(); n != 1 {
		t.Error("expected a single ListPath call, got:", n)
	}

	// Paginating afterwards is served from the cache
	res, err := src.RoutesReceived(ctx, neighborID)
	if err != nil {
		t.Fatal(err)
	}
	if n := client.listPaths.Load(); n != 1 {
		t.Error("expected the cache to be hit, got calls:", n)
	}
	if !res.Response.Meta.ResultFromCache {
		t.Error("expected result from cache")
	}
	if res.Filtered != nil {
		t.Error("received routes response should not include filtered")
	}

	// Not exported routes are a separate table
	if _, err := src.RoutesNotExported(ctx, neighborID); err != nil {
		t.Fatal(err)
	}
	if n := client.listPaths.Load(); n != 2 {
		t.Error("expected an Adj-RIB-Out ListPath call, got:", n)
	}
}

func TestRoutesCacheDisabled(t *testing.T) {
	src, client := newFakeGoBGP(t, 0)
	neighborID := PeerHash(client.peer)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		if _, err := src.RoutesReceived(ctx, neighborID); err != nil {
			t.Fatal(err)
		}
	}
	if n := client.listPaths.Load(); n != 2 {
		t.Error("expected no caching with cache_ttl = 0, got calls:", n)
	}
}

func TestRoutesCacheCanceledRequest(t *testing.T) {
	src, client := newFakeGoBGP(t, time.Minute)
	neighborID := PeerHash(client.peer)

	// The request starting the fetch is canceled ...
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := src.RoutesReceived(ctx, neighborID); err == nil {
		t.Error("expected the canceled request to fail")
	}

	// ... but the fetch completes and fills the cache.
	res, err := src.RoutesReceived(context.Background(), neighborID)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Imported) != 1 {
		t.Error("unexpected routes:", res.Imported)
	}
	if n := client.listPaths.Load(); n != 1 {
		t.Error("expected a single ListPath call, got:", n)
	}
}
