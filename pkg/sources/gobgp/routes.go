package gobgp

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"time"

	gobgpapi "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/packet/bgp"

	"github.com/alice-lg/alice-lg/pkg/api"
	"github.com/alice-lg/alice-lg/pkg/pools"
	"github.com/alice-lg/alice-lg/pkg/sources/gobgp/apiutil"
)

// NewRoutesResponse creates a new routes response
func NewRoutesResponse() api.RoutesResponse {
	routes := api.RoutesResponse{}
	routes.Imported = make(api.Routes, 0)
	routes.Filtered = make(api.Routes, 0)
	routes.NotExported = make(api.Routes, 0)
	return routes
}

func (gobgp *GoBGP) lookupNeighbor(
	ctx context.Context,
	neighborID string,
) (*gobgpapi.Peer, error) {

	peers, err := gobgp.GetNeighbors(ctx)
	if err != nil {
		return nil, err
	}
	for _, peer := range peers {
		peerID := PeerHash(peer)
		if neighborID == "" || peerID == neighborID {
			return peer, nil
		}
	}

	return nil, fmt.Errorf("could not lookup neighbor")
}

// GetNeighbors retrieves all neighbors and returns
// a list of peers.
func (gobgp *GoBGP) GetNeighbors(
	ctx context.Context,
) ([]*gobgpapi.Peer, error) {
	ctx, cancel := context.WithTimeout(
		ctx, time.Second*time.Duration(gobgp.config.ProcessingTimeout))
	defer cancel()

	peerStream, err := gobgp.client.ListPeer(
		ctx, &gobgpapi.ListPeerRequest{EnableAdvertised: true})
	if err != nil {
		return nil, err
	}

	peers := make([]*gobgpapi.Peer, 0)

	for {
		peer, err := peerStream.Recv()
		if err == io.EOF {
			break
		}
		peers = append(peers, peer.Peer)
	}
	return peers, nil
}

// supportedFamilies is the list of address families
// alice will query from gobgp.
var supportedFamilies = []*gobgpapi.Family{
	{
		Afi:  gobgpapi.Family_AFI_IP,
		Safi: gobgpapi.Family_SAFI_UNICAST,
	},
	{
		Afi:  gobgpapi.Family_AFI_IP6,
		Safi: gobgpapi.Family_SAFI_UNICAST,
	},
	{
		Afi:  gobgpapi.Family_AFI_IP,
		Safi: gobgpapi.Family_SAFI_MPLS_VPN,
	},
	{
		Afi:  gobgpapi.Family_AFI_IP6,
		Safi: gobgpapi.Family_SAFI_MPLS_VPN,
	},
}

// familyName returns a human readable name of the family
func familyName(f *gobgpapi.Family) string {
	switch {
	case f.Afi == gobgpapi.Family_AFI_IP && f.Safi == gobgpapi.Family_SAFI_UNICAST:
		return "ipv4-unicast"
	case f.Afi == gobgpapi.Family_AFI_IP6 && f.Safi == gobgpapi.Family_SAFI_UNICAST:
		return "ipv6-unicast"
	case f.Afi == gobgpapi.Family_AFI_IP && f.Safi == gobgpapi.Family_SAFI_MPLS_VPN:
		return "l3vpn-ipv4-unicast"
	case f.Afi == gobgpapi.Family_AFI_IP6 && f.Safi == gobgpapi.Family_SAFI_MPLS_VPN:
		return "l3vpn-ipv6-unicast"
	}
	return fmt.Sprintf("afi=%d/safi=%d", f.Afi, f.Safi)
}

// peerFamilies returns the supported address families
// configured for the peer. If the peer does not provide
// any information about its families, all supported
// families are returned.
func peerFamilies(peer *gobgpapi.Peer) []*gobgpapi.Family {
	if peer == nil || len(peer.AfiSafis) == 0 {
		return supportedFamilies
	}
	families := make([]*gobgpapi.Family, 0, len(supportedFamilies))
	for _, family := range supportedFamilies {
		for _, afiSafi := range peer.AfiSafis {
			if afiSafi.Config == nil || afiSafi.Config.Family == nil {
				continue
			}
			f := afiSafi.Config.Family
			if f.Afi == family.Afi && f.Safi == family.Safi {
				families = append(families, family)
				break
			}
		}
	}
	if len(families) == 0 {
		return supportedFamilies
	}
	return families
}

// vpnInfo extracts the route distinguisher and the
// MPLS label stack from a L3VPN NLRI.
func vpnInfo(
	rd bgp.RouteDistinguisherInterface,
	labels *bgp.MPLSLabelStack,
) (string, []uint32) {
	rdStr := ""
	if rd != nil {
		rdStr = rd.String()
	}
	var mplsLabels []uint32
	if labels != nil && len(labels.Labels) > 0 {
		mplsLabels = make([]uint32, len(labels.Labels))
		copy(mplsLabels, labels.Labels)
	}
	return rdStr, mplsLabels
}

func extCommunitySubTypeName(subType bgp.ExtendedCommunityAttrSubType) string {
	switch subType {
	case bgp.EC_SUBTYPE_ROUTE_TARGET:
		return "rt"
	case bgp.EC_SUBTYPE_ROUTE_ORIGIN:
		return "ro"
	default:
		return "generic"
	}
}

// extCommunityToAPI converts an extended community into
// the alice representation [name, int, int].
//
// Route targets and route origins with an AS as global
// administrator are rendered as e.g. rt:52890:20.
// All other types (e.g. opaque or vendor specific
// communities like [64821] in the gobgp cli) are rendered
// as 0xTT:SUBTYPE:VALUE, where TT is the type, SUBTYPE the
// sub type and VALUE the remaining 6 bytes as an integer.
//
// IPv4 address specific communities can not be expressed
// with integers only and are ignored.
func extCommunityToAPI(
	community bgp.ExtendedCommunityInterface,
) (api.ExtCommunity, bool) {
	switch c := community.(type) {
	case *bgp.TwoOctetAsSpecificExtended:
		return api.ExtCommunity{
			extCommunitySubTypeName(c.SubType),
			int(c.AS),
			int(c.LocalAdmin),
		}, true
	case *bgp.FourOctetAsSpecificExtended:
		return api.ExtCommunity{
			extCommunitySubTypeName(c.SubType),
			int(c.AS),
			int(c.LocalAdmin),
		}, true
	case *bgp.IPv4AddressSpecificExtended:
		return nil, false
	}

	buf, err := community.Serialize()
	if err != nil || len(buf) != 8 {
		return nil, false
	}
	value := make([]byte, 8)
	copy(value[2:], buf[2:])
	return api.ExtCommunity{
		fmt.Sprintf("0x%02x", buf[0]),
		int(buf[1]),
		int(binary.BigEndian.Uint64(value)),
	}, true
}

func (gobgp *GoBGP) parsePathIntoRoute(
	path *gobgpapi.Path,
	prefix string,
) (*api.Route, error) {

	route := api.Route{}
	// route.ID = fmt.Sprintf("%s_%s", path.SourceId, prefix)
	route.NeighborID = pools.Neighbors.Acquire(
		PeerHashWithASAndAddress(path.SourceAsn, path.NeighborIp))
	route.Network = prefix
	route.Interface = pools.Interfaces.Acquire("unknown")
	route.Age = time.Since(time.Unix(path.Age.GetSeconds(), int64(path.Age.GetNanos())))
	route.Primary = path.Best

	nlri, err := apiutil.GetNativeNlri(path)
	if err != nil {
		return nil, fmt.Errorf("failed to parse NLRI: %v", err)
	}

	switch nlri := nlri.(type) {
	case *bgp.IPAddrPrefix:
		route.Network = fmt.Sprintf("%s/%d", nlri.Prefix.String(), nlri.Length)
		route.AddrFamily = api.AddrFamilyIPv4
	case *bgp.IPv6AddrPrefix:
		route.Network = fmt.Sprintf("%s/%d", nlri.Prefix.String(), nlri.Length)
		route.AddrFamily = api.AddrFamilyIPv6
	case *bgp.LabeledVPNIPAddrPrefix:
		// VPNv4 (AFI 1 / SAFI 128)
		route.Network = nlri.IPPrefix()
		route.AddrFamily = api.AddrFamilyIPv4
		route.RouteDistinguisher, route.MPLSLabels = vpnInfo(
			nlri.RD, &nlri.Labels)
	case *bgp.LabeledVPNIPv6AddrPrefix:
		// VPNv6 (AFI 2 / SAFI 128)
		route.Network = nlri.IPPrefix()
		route.AddrFamily = api.AddrFamilyIPv6
		route.RouteDistinguisher, route.MPLSLabels = vpnInfo(
			nlri.RD, &nlri.Labels)
	}

	attrs, err := apiutil.GetNativePathAttributes(path)
	if err != nil {
		return nil, err
	}

	route.BGP = &api.BGPInfo{}
	route.BGP.Communities = make(api.Communities, 0)
	route.BGP.LargeCommunities = make(api.Communities, 0)
	route.BGP.ExtCommunities = make(api.ExtCommunities, 0)

	for _, attr := range attrs {
		switch attr := attr.(type) {
		case *bgp.PathAttributeMultiExitDisc:
			route.BGP.Med = int(attr.Value)
		case *bgp.PathAttributeNextHop:
			route.Gateway = pools.Gateways4.Acquire(attr.Value.String())
			route.BGP.NextHop = pools.Gateways4.Acquire(attr.Value.String())
		case *bgp.PathAttributeLocalPref:
			route.BGP.LocalPref = int(attr.Value)
		case *bgp.PathAttributeOrigin:
			switch attr.Value {
			case bgp.BGP_ORIGIN_ATTR_TYPE_IGP:
				route.BGP.Origin = pools.Origins.Acquire("IGP")
			case bgp.BGP_ORIGIN_ATTR_TYPE_EGP:
				route.BGP.Origin = pools.Origins.Acquire("EGP")
			case bgp.BGP_ORIGIN_ATTR_TYPE_INCOMPLETE:
				route.BGP.Origin = pools.Origins.Acquire("Incomplete")
			}
		case *bgp.PathAttributeAsPath:
			for _, aspth := range attr.Value {
				for _, as := range aspth.GetAS() {
					route.BGP.AsPath = append(route.BGP.AsPath, int(as))
				}
			}
		case *bgp.PathAttributeCommunities:
			for _, community := range attr.Value {
				apiComm := api.Community{
					int((0xffff0000 & community) >> 16),
					int(0xffff & community)}

				route.BGP.Communities = append(route.BGP.Communities, apiComm)
			}
		case *bgp.PathAttributeMpReachNLRI:
			// gobgp has already stripped the (zero) route
			// distinguisher from VPN next hops. In a VPNv6 (6VPE)
			// setup the next hop usually is an IPv4-mapped IPv6
			// address of the PE, so we classify it by its content
			// rather than by its length.
			if attr.Nexthop == nil || attr.Nexthop.IsUnspecified() {
				break
			}
			nextHop := attr.Nexthop.String()
			if attr.Nexthop.To4() != nil {
				route.Gateway = pools.Gateways4.Acquire(nextHop)
				route.BGP.NextHop = pools.Gateways4.Acquire(nextHop)
			} else {
				route.Gateway = pools.Gateways6.Acquire(nextHop)
				route.BGP.NextHop = pools.Gateways6.Acquire(nextHop)
			}
		case *bgp.PathAttributeExtendedCommunities:
			for _, community := range attr.Value {
				if apiComm, ok := extCommunityToAPI(community); ok {
					route.BGP.ExtCommunities = append(
						route.BGP.ExtCommunities, apiComm)
				}
			}
		case *bgp.PathAttributeOriginatorId:
			route.BGP.OriginatorID = attr.Value.String()
		case *bgp.PathAttributeClusterList:
			clusterList := make([]string, 0, len(attr.Value))
			for _, id := range attr.Value {
				clusterList = append(clusterList, id.String())
			}
			route.BGP.ClusterList = clusterList
		case *bgp.PathAttributeAggregator:
			route.BGP.Aggregator = fmt.Sprintf(
				"AS%d %s", attr.Value.AS, attr.Value.Address.String())
		case *bgp.PathAttributeAs4Aggregator:
			route.BGP.Aggregator = fmt.Sprintf(
				"AS%d %s", attr.Value.AS, attr.Value.Address.String())
		case *bgp.PathAttributeLargeCommunities:
			for _, community := range attr.Values {
				route.BGP.LargeCommunities = append(
					route.BGP.LargeCommunities,
					api.Community{
						int(community.ASN),
						int(community.LocalData1),
						int(community.LocalData2)})
			}
		}
	}

	route.BGP.AsPath = pools.ASPaths.Acquire(route.BGP.AsPath)
	route.BGP.Communities = pools.CommunitiesSets.Acquire(route.BGP.Communities)
	route.BGP.ExtCommunities = pools.ExtCommunitiesSets.Acquire(route.BGP.ExtCommunities)
	route.BGP.LargeCommunities = pools.LargeCommunitiesSets.Acquire(route.BGP.LargeCommunities)

	route.Metric = (route.BGP.LocalPref + route.BGP.Med)

	return &route, nil
}

// GetRoutes retrieves all routes from a peer
// for a table type.
func (gobgp *GoBGP) GetRoutes(
	ctx context.Context,
	peer *gobgpapi.Peer,
	tableType gobgpapi.TableType,
	response *api.RoutesResponse,
) error {
	ctx, cancel := context.WithTimeout(
		ctx, time.Second*time.Duration(gobgp.config.ProcessingTimeout))
	defer cancel()

	families := peerFamilies(peer)
	for _, family := range families {
		pathStream, err := gobgp.client.ListPath(ctx, &gobgpapi.ListPathRequest{
			Name:           peer.State.NeighborAddress,
			TableType:      tableType,
			Family:         family,
			EnableFiltered: true,
		})

		if err != nil {
			log.Print(err)
			continue
		}

		for {
			resp, err := pathStream.Recv()
			if err == io.EOF {
				break
			} else if err != nil {
				// Do not abort: other address families of this
				// peer may still provide routes.
				log.Printf(
					"error while fetching %s routes from %s: %v",
					familyName(family), peer.State.NeighborAddress, err)
				break
			}

			destination := resp.Destination

			for _, path := range destination.Paths {
				route, err := gobgp.parsePathIntoRoute(path, destination.Prefix)
				if err != nil {
					log.Println(err)
					continue
				}

				if path.Filtered {
					response.Filtered = append(response.Filtered, route)
				} else {
					response.Imported = append(response.Imported, route)
				}
			}
		}
	}

	return nil
}
