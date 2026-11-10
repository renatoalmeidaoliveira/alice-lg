package gobgp

import (
	"testing"
	"time"

	gobgpapi "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/packet/bgp"

	"github.com/alice-lg/alice-lg/pkg/api"
	"github.com/alice-lg/alice-lg/pkg/sources/gobgp/apiutil"
)

func makeVPNPath(
	t *testing.T,
	nlri bgp.AddrPrefixInterface,
	nextHop string,
) *gobgpapi.Path {
	attrs := []bgp.PathAttributeInterface{
		bgp.NewPathAttributeOrigin(bgp.BGP_ORIGIN_ATTR_TYPE_IGP),
		bgp.NewPathAttributeMpReachNLRI(
			nextHop, []bgp.AddrPrefixInterface{nlri}),
		bgp.NewPathAttributeLocalPref(100),
		bgp.NewPathAttributeExtendedCommunities(
			[]bgp.ExtendedCommunityInterface{
				bgp.NewTwoOctetAsSpecificExtended(
					bgp.EC_SUBTYPE_ROUTE_TARGET, 65000, 100, true),
				// Shown as [4242] by the gobgp cli
				bgp.NewOpaqueExtended(
					true, []byte{0, 0, 0, 0, 0, 0x10, 0x92}),
			}),
		bgp.NewPathAttributeOriginatorId("192.0.2.11"),
		bgp.NewPathAttributeClusterList([]string{"192.0.2.77"}),
	}
	path, err := apiutil.NewPath(nlri, false, attrs, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseVPNv4PathIntoRoute(t *testing.T) {
	rd, err := bgp.ParseRouteDistinguisher("65000:100")
	if err != nil {
		t.Fatal(err)
	}
	nlri := bgp.NewLabeledVPNIPAddrPrefix(
		24, "10.1.2.0", *bgp.NewMPLSLabelStack(16001), rd)
	path := makeVPNPath(t, nlri, "192.0.2.1")

	src := &GoBGP{}
	route, err := src.parsePathIntoRoute(path, nlri.String())
	if err != nil {
		t.Fatal(err)
	}

	if route.Network != "10.1.2.0/24" {
		t.Error("unexpected network:", route.Network)
	}
	if route.AddrFamily != api.AddrFamilyIPv4 {
		t.Error("unexpected address family:", route.AddrFamily)
	}
	if route.RouteDistinguisher != "65000:100" {
		t.Error("unexpected rd:", route.RouteDistinguisher)
	}
	if len(route.MPLSLabels) != 1 || route.MPLSLabels[0] != 16001 {
		t.Error("unexpected labels:", route.MPLSLabels)
	}
	if route.BGP.NextHop == nil || *route.BGP.NextHop != "192.0.2.1" {
		t.Error("unexpected next hop:", route.BGP.NextHop)
	}
	if len(route.BGP.ExtCommunities) != 2 {
		t.Fatal("expected two ext communities:", route.BGP.ExtCommunities)
	}
	if route.BGP.ExtCommunities[0].String() != "rt:65000:100" {
		t.Error("unexpected route target:", route.BGP.ExtCommunities[0])
	}
	if route.BGP.ExtCommunities[1].String() != "0x03:0:4242" {
		t.Error("unexpected opaque community:", route.BGP.ExtCommunities[1])
	}
	if route.BGP.OriginatorID != "192.0.2.11" {
		t.Error("unexpected originator id:", route.BGP.OriginatorID)
	}
	if len(route.BGP.ClusterList) != 1 ||
		route.BGP.ClusterList[0] != "192.0.2.77" {
		t.Error("unexpected cluster list:", route.BGP.ClusterList)
	}
	if route.Key() != "65000:100:10.1.2.0/24" {
		t.Error("unexpected key:", route.Key())
	}
}

func TestParseVPNv6PathIntoRoute(t *testing.T) {
	rd, err := bgp.ParseRouteDistinguisher("192.0.2.1:42")
	if err != nil {
		t.Fatal(err)
	}
	nlri := bgp.NewLabeledVPNIPv6AddrPrefix(
		48, "2001:db8:1::", *bgp.NewMPLSLabelStack(24002), rd)
	// 6VPE: IPv4-mapped IPv6 next hop of the PE
	path := makeVPNPath(t, nlri, "::ffff:192.0.2.1")

	src := &GoBGP{}
	route, err := src.parsePathIntoRoute(path, nlri.String())
	if err != nil {
		t.Fatal(err)
	}

	if route.Network != "2001:db8:1::/48" {
		t.Error("unexpected network:", route.Network)
	}
	if route.AddrFamily != api.AddrFamilyIPv6 {
		t.Error("unexpected address family:", route.AddrFamily)
	}
	if route.RouteDistinguisher != "192.0.2.1:42" {
		t.Error("unexpected rd:", route.RouteDistinguisher)
	}
	if len(route.MPLSLabels) != 1 || route.MPLSLabels[0] != 24002 {
		t.Error("unexpected labels:", route.MPLSLabels)
	}
	if route.BGP.NextHop == nil || *route.BGP.NextHop != "192.0.2.1" {
		t.Error("unexpected next hop:", route.BGP.NextHop)
	}
}

func TestPeerFamilies(t *testing.T) {
	peer := &gobgpapi.Peer{
		AfiSafis: []*gobgpapi.AfiSafi{
			{Config: &gobgpapi.AfiSafiConfig{Family: &gobgpapi.Family{
				Afi:  gobgpapi.Family_AFI_IP,
				Safi: gobgpapi.Family_SAFI_MPLS_VPN,
			}}},
			{Config: &gobgpapi.AfiSafiConfig{Family: &gobgpapi.Family{
				Afi:  gobgpapi.Family_AFI_IP6,
				Safi: gobgpapi.Family_SAFI_MPLS_VPN,
			}}},
		},
	}
	families := peerFamilies(peer)
	if len(families) != 2 {
		t.Fatal("expected 2 families, got:", len(families))
	}
	for _, f := range families {
		if f.Safi != gobgpapi.Family_SAFI_MPLS_VPN {
			t.Error("unexpected family:", familyName(f))
		}
	}

	if len(peerFamilies(&gobgpapi.Peer{})) != len(supportedFamilies) {
		t.Error("expected fallback to all supported families")
	}
}
