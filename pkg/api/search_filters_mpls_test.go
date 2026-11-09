package api

import (
	"net/url"
	"testing"
)

func makeVPNRoute(labels ...uint32) *Route {
	return &Route{
		Network:            "10.1.2.0/24",
		RouteDistinguisher: "65000:100",
		MPLSLabels:         labels,
		AddrFamily:         AddrFamilyIPv4,
		BGP:                &BGPInfo{},
	}
}

func TestFiltersFromQueryMPLSLabels(t *testing.T) {
	query, _ := url.ParseQuery("mpls_labels=16001,24002")
	filters, err := FiltersFromQuery(query)
	if err != nil {
		t.Fatal(err)
	}
	group := filters.GetGroupByKey(SearchKeyMPLSLabels)
	if len(group.Filters) != 2 {
		t.Fatal("expected two label filters, got:", group.Filters)
	}
	if group.Filters[0].Value.(int) != 16001 {
		t.Error("unexpected label:", group.Filters[0].Value)
	}

	for _, invalid := range []string{"abc", "-1", "1048576"} {
		query, _ := url.ParseQuery("mpls_labels=" + invalid)
		if _, err := FiltersFromQuery(query); err == nil {
			t.Error("expected error for label:", invalid)
		}
	}
}

func TestMatchRouteMPLSLabels(t *testing.T) {
	query, _ := url.ParseQuery("mpls_labels=16001,24002")
	filters, err := FiltersFromQuery(query)
	if err != nil {
		t.Fatal(err)
	}

	if !filters.MatchRoute(makeVPNRoute(16001)) {
		t.Error("route with label 16001 should match")
	}
	if !filters.MatchRoute(makeVPNRoute(24002)) {
		t.Error("route with label 24002 should match (any)")
	}
	if filters.MatchRoute(makeVPNRoute(33)) {
		t.Error("route with label 33 should not match")
	}
	if filters.MatchRoute(makeVPNRoute()) {
		t.Error("route without label should not match")
	}

	// Without label filter everything matches
	if !NewSearchFilters().MatchRoute(makeVPNRoute()) {
		t.Error("unfiltered route should match")
	}
}

func TestUpdateFromRouteMPLSLabels(t *testing.T) {
	filters := NewSearchFilters()
	filters.UpdateFromRoute(makeVPNRoute(16001))
	filters.UpdateFromRoute(makeVPNRoute(16001))
	filters.UpdateFromRoute(makeVPNRoute(24002))
	filters.UpdateFromRoute(makeVPNRoute())

	group := filters.GetGroupByKey(SearchKeyMPLSLabels)
	if len(group.Filters) != 2 {
		t.Fatal("expected two labels, got:", group.Filters)
	}
	f := group.GetFilterByValue(16001)
	if f == nil || f.Cardinality != 2 || f.Name != "16001" {
		t.Error("unexpected filter for label 16001:", f)
	}
}

func TestFiltersFromTokensMPLSLabel(t *testing.T) {
	filters, err := FiltersFromTokens([]string{"label:24002", "#rt:65000:100"})
	if err != nil {
		t.Fatal(err)
	}
	labels := filters.GetGroupByKey(SearchKeyMPLSLabels)
	if len(labels.Filters) != 1 || labels.Filters[0].Value.(int) != 24002 {
		t.Error("unexpected label filters:", labels.Filters)
	}
	ext := filters.GetGroupByKey(SearchKeyExtCommunities)
	if len(ext.Filters) != 1 {
		t.Error("expected ext community filter:", ext.Filters)
	}

	if _, err := FiltersFromTokens([]string{"label:foo"}); err == nil {
		t.Error("expected error for invalid label token")
	}
}

func TestWithoutGroupMPLSLabels(t *testing.T) {
	query, _ := url.ParseQuery("mpls_labels=16001&asns=65000")
	filters, err := FiltersFromQuery(query)
	if err != nil {
		t.Fatal(err)
	}
	without := filters.WithoutGroup(SearchKeyMPLSLabels)
	if without.HasGroup(SearchKeyMPLSLabels) {
		t.Error("label group should be empty")
	}
	if !without.HasGroup(SearchKeyASNS) {
		t.Error("asn group should be kept")
	}
	if !filters.HasGroup(SearchKeyMPLSLabels) {
		t.Error("original filters must not be modified")
	}
	if !without.MatchRoute(makeVPNRoute(33)) {
		t.Error("route should match without label filter")
	}
}
