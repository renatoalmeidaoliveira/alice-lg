package api

import (
	"errors"
	"strconv"
	"strings"
)

// Errors
var (
	ErrExtCommunityIncomplete = errors.New("incomplete extended community")
	ErrInvalidMPLSLabel       = errors.New("invalid MPLS label")
)

// MPLSLabelTokenPrefix marks an MPLS label in a search
// query, e.g. "label:24002".
const MPLSLabelTokenPrefix = "label:"

// MaxMPLSLabel is the largest valid MPLS label (20 bit)
const MaxMPLSLabel = 1<<20 - 1

// IsMPLSLabelToken checks if a search query token
// is an MPLS label filter.
func IsMPLSLabelToken(token string) bool {
	return strings.HasPrefix(strings.ToLower(token), MPLSLabelTokenPrefix)
}

// parseMPLSLabelValue parses an MPLS label filter value
func parseMPLSLabelValue(value string) (*SearchFilter, error) {
	label, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || label < 0 || label > MaxMPLSLabel {
		return nil, ErrInvalidMPLSLabel
	}
	return &SearchFilter{
		Name:  strconv.Itoa(label),
		Value: label,
	}, nil
}

// FilterQueryParser parses a filter value into a search filter
type FilterQueryParser func(value string) (*SearchFilter, error)

func parseQueryValueList(parser FilterQueryParser, value string) ([]*SearchFilter, error) {
	components := strings.Split(value, ",")
	result := make([]*SearchFilter, 0, len(components))

	for _, component := range components {
		filter, err := parser(strings.TrimSpace(component))
		if err != nil {
			return result, err
		}
		result = append(result, filter)
	}

	return result, nil
}

func parseIntValue(value string) (*SearchFilter, error) {
	v, err := strconv.Atoi(value)
	if err != nil {
		return nil, err
	}

	return &SearchFilter{
		Value: v,
	}, nil
}

func parseStringValue(value string) (*SearchFilter, error) {
	return &SearchFilter{
		Value: value,
	}, nil
}

func parseCommunityValue(value string) (*SearchFilter, error) {
	components := strings.Split(value, ":")
	community := make(Community, len(components))

	for i, c := range components {
		v, err := strconv.Atoi(c)
		if err != nil {
			return nil, err
		}
		community[i] = v
	}

	return &SearchFilter{
		Name:  community.String(),
		Value: community,
	}, nil
}

func parseExtCommunityValue(value string) (*SearchFilter, error) {
	components := strings.Split(value, ":")
	community := make(ExtCommunity, len(components))

	if len(community) != 3 {
		return nil, ErrExtCommunityIncomplete
	}

	// Check if the community is incomplete
	if components[0] == "" || components[1] == "" || components[2] == "" {
		return nil, ErrExtCommunityIncomplete
	}
	// TODO: Mixing strings and integers is not a good idea
	community[0] = components[0]
	community[1], _ = strconv.Atoi(components[1])
	community[2], _ = strconv.Atoi(components[2])

	return &SearchFilter{
		Name:  community.String(),
		Value: community,
	}, nil
}
