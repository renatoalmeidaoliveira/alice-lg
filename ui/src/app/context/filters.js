
import { useMemo
       , useContext
       , createContext
       , useCallback
       }
  from 'react';

import { useRoutesReceived
       , useRoutesFiltered
       , useRoutesNotExported
       }
  from 'app/context/routes';
import { useQuery
       , cleanParams
       , PARAM_QUERY
       , PARAM_LOAD_NOT_EXPORTED
       }
  from 'app/context/query';


/**
 * Filter Groups
 */
const FILTER_KEY_SOURCES = "sources";
const FILTER_KEY_ASNS = "asns";
const FILTER_KEY_COMMUNITIES = "communities";
const FILTER_KEY_EXT_COMMUNITIES = "ext_communities";
const FILTER_KEY_LARGE_COMMUNITIES = "large_communities";
const FILTER_KEY_ADDR_FAMILY = "addr_family";
const FILTER_KEY_MPLS_LABELS = "mpls_labels";

export const FILTER_GROUP_SOURCES = 0;
export const FILTER_GROUP_ASNS = 1;
export const FILTER_GROUP_COMMUNITIES = 2;
export const FILTER_GROUP_EXT_COMMUNITIES = 3;
export const FILTER_GROUP_LARGE_COMMUNITIES = 4;
export const FILTER_GROUP_ADDR_FAMILY = 5;
export const FILTER_GROUP_MPLS_LABELS = 6;

const FILTER_GROUP_KEYS = {
  [FILTER_GROUP_SOURCES]: FILTER_KEY_SOURCES,
  [FILTER_GROUP_ASNS]: FILTER_KEY_ASNS,
  [FILTER_GROUP_COMMUNITIES]: FILTER_KEY_COMMUNITIES,
  [FILTER_GROUP_EXT_COMMUNITIES]: FILTER_KEY_EXT_COMMUNITIES,
  [FILTER_GROUP_LARGE_COMMUNITIES]: FILTER_KEY_LARGE_COMMUNITIES,
  [FILTER_GROUP_ADDR_FAMILY]: FILTER_KEY_ADDR_FAMILY,
  [FILTER_GROUP_MPLS_LABELS]: FILTER_KEY_MPLS_LABELS,
};


/**
 * Make initialState
 */
export const initializeFilterState = () => ([
  {"key": "sources", "filters": []},
  {"key": "asns", "filters": []},
  {"key": "communities", "filters": []},
  {"key": "ext_communities", "filters": []},
  {"key": "large_communities", "filters": []},
  {"key": "addr_family", "filters": []},
  {"key": "mpls_labels", "filters": []},
]);

// Compare values
const cmpValue = (a, b) => a === b;

// Compare values, stringly typed
const cmpValueString = (a, b) => String(a) === String(b)

// Compare list values
const cmpList = (a, b) => 
  a.map((v, i) => v === b[i]).reduce(
    (part, match) => (match && part), true);


const FILTER_VALUE_CMP = {
  [FILTER_GROUP_SOURCES]: cmpValue,
  [FILTER_GROUP_ASNS]: cmpValue,
  [FILTER_GROUP_COMMUNITIES]: cmpList,
  [FILTER_GROUP_EXT_COMMUNITIES]: cmpList,
  [FILTER_GROUP_LARGE_COMMUNITIES]: cmpList,
  [FILTER_GROUP_ADDR_FAMILY]: cmpValueString,
  [FILTER_GROUP_MPLS_LABELS]: cmpValue,
}

/*
 * Merge list of filters: Filters with the same value
 * are combined and their cardinalities are added up.
 * The filters of the responses are not modified.
 */
const filterKey = (filter) =>
  Array.isArray(filter.value)
    ? filter.value.join(":")
    : String(filter.value);

const mergeFilterSet = (a, b) => {
  const merged = new Map();
  for (const set of [a, b]) {
    for (const f of set) {
      const key = filterKey(f);
      const present = merged.get(key);
      if (present) {
        present.cardinality += f.cardinality;
        continue;
      }
      merged.set(key, {...f});
    }
  }
  return Array.from(merged.values());
}


/*
 * Merge filters
 */
const _mergeFilters = (a, b) => {
  let groups = initializeFilterState();
  if (!a || !b) {
    return groups;
  }
  for (const i in groups) {
    if (a[i]?.filters && b[i]?.filters) {
      groups[i].filters = mergeFilterSet(a[i].filters, b[i].filters);
    }
    else if(a[i]?.filters) {
      groups[i].filters = a[i].filters;
    }
    else if(b[i]?.filters) {
      groups[i].filters = b[i].filters;
    }
    // Filters omitted by the server (e.g. rare MPLS labels)
    groups[i].omitted = Math.max(a[i]?.omitted || 0, b[i]?.omitted || 0);
  }
  return groups;
}

const mergeFilters = (a, ...other) => {
  let result = a;
  for (const filters of other) {
    result = _mergeFilters(result, filters);
  }
  return result;
}

/*
 * Filter Query Decoding
 */
const decodeStringList = (value) => {
  if (!value) {
    return [];
  }
  if (value === "") {
    return [];
  }
  return value.split(",");
}

const decodeIntList = (value) =>
  decodeStringList(value).map((v) =>
    parseInt(v, 10));

const decodeCommunity = (community) =>
  community.split(":").map((p) =>
    parseInt(p, 10));

const decodeExtCommunity = (community) => {
  const comm = community.split(":");
  if (comm.length !== 3) {
    return comm; 
  }
  return [comm[0], ...comm.slice(1).map((c) => parseInt(c, 10))];
}

const decodeCommunities = (value) =>
  decodeStringList(value).map((v) =>
    decodeCommunity(v));

const decodeExtCommunities = (value) =>
  decodeStringList(value).map((v) =>
    decodeExtCommunity(v));


const decodeQuery = (query) => {
  const sources = decodeStringList(query[FILTER_KEY_SOURCES]);
  const asns = decodeIntList(query[FILTER_KEY_ASNS]);
  const communities = decodeCommunities(query[FILTER_KEY_COMMUNITIES]);
  const extCommunities = decodeExtCommunities(query[FILTER_KEY_EXT_COMMUNITIES]);
  const largeCommunities = decodeCommunities(query[FILTER_KEY_LARGE_COMMUNITIES]);
  const addrFamily = decodeStringList(query[FILTER_KEY_ADDR_FAMILY]);
  const mplsLabels = decodeIntList(query[FILTER_KEY_MPLS_LABELS]);

  let filters = {};
  filters[FILTER_KEY_SOURCES] = sources;
  filters[FILTER_KEY_ASNS] = asns;
  filters[FILTER_KEY_COMMUNITIES] = communities;
  filters[FILTER_KEY_EXT_COMMUNITIES] = extCommunities;
  filters[FILTER_KEY_LARGE_COMMUNITIES] = largeCommunities;
  filters[FILTER_KEY_ADDR_FAMILY] = addrFamily;
  filters[FILTER_KEY_MPLS_LABELS] = mplsLabels;
  return filters;
}

/*
 * Filter Query Encoding
 */
const encodeList = (value) =>
  value.join(",");

const encodeCommunity = (community) =>
  community.join(":");

const encodeCommunities = (communities) =>
  encodeList(communities.map((c) => encodeCommunity(c)));

export const encodeFilters = (filters) => {
  const sources = filters[FILTER_KEY_SOURCES];
  const asns = filters[FILTER_KEY_ASNS];
  const communities = filters[FILTER_KEY_COMMUNITIES];
  const extCommunities = filters[FILTER_KEY_EXT_COMMUNITIES];
  const largeCommunities = filters[FILTER_KEY_LARGE_COMMUNITIES];
  const addrFamily = filters[FILTER_KEY_ADDR_FAMILY];
  const mplsLabels = filters[FILTER_KEY_MPLS_LABELS] || [];
  return cleanParams({
    [FILTER_KEY_SOURCES]: encodeList(sources),
    [FILTER_KEY_ASNS]: encodeList(asns),
    [FILTER_KEY_COMMUNITIES]: encodeCommunities(communities),
    [FILTER_KEY_EXT_COMMUNITIES]: encodeCommunities(extCommunities),
    [FILTER_KEY_LARGE_COMMUNITIES]: encodeCommunities(largeCommunities),
    [FILTER_KEY_ADDR_FAMILY]: encodeList(addrFamily),
    [FILTER_KEY_MPLS_LABELS]: encodeList(mplsLabels),
  });
}


/**
 * FiltersQuery Context
 */
export const useFiltersQuery = () => {
  const [query, setQuery] = useQuery({
    [FILTER_KEY_SOURCES]: "",
    [FILTER_KEY_ASNS]: "",
    [FILTER_KEY_COMMUNITIES]: "",
    [FILTER_KEY_EXT_COMMUNITIES]: "",
    [FILTER_KEY_LARGE_COMMUNITIES]: "",
    [FILTER_KEY_ADDR_FAMILY]: "",
    [FILTER_KEY_MPLS_LABELS]: "",
  });

  const filterQuery = useMemo(() => decodeQuery(query), [query]);
  const setFilterQuery = useCallback((key, value) => {
    const next = {...filterQuery, [key]: value};
    setQuery((q) => ({
      // Keep state
      [PARAM_QUERY]: q[PARAM_QUERY],
      [PARAM_LOAD_NOT_EXPORTED]: q[PARAM_LOAD_NOT_EXPORTED],
      // Add Filters
      ...encodeFilters(next),
    }));
  }, [filterQuery, setQuery]);

  return [filterQuery, setFilterQuery];
};


/*
 * FiltersContext
 */
const FiltersContext = createContext();

export const useFilters = () => useContext(FiltersContext);

const useRoutesFilters = (routes) => {
  return useMemo(() => {
    if (!routes.requested || routes.loading) {
      return { applied: [], available: [], notAvailable: [] };
    }
    return {
      applied: routes.filtersApplied,
      available: routes.filtersAvailable,
      notAvailable: routes.filtersNotAvailable,
    };
  }, [routes]);
}

export const useTotalFilters = () => {
  const {filters} = useFilters();
  const {applied, available} = filters;
  return useMemo(() => 
    applied.reduce(
      (total, group) => total + group.filters.length,
      0,
    ) + available.reduce(
      (total, group) => total + group.filters.length,
      0
    ), [applied, available]);
}


const createGroupFilters = (group) => () => {
  const {filters, applyFilter, removeFilter} = useFilters();
  const applyGroupFilter = useCallback((filters) => {
    applyFilter(group, filters);
  }, [applyFilter]);
  const removeGroupFilter = useCallback((filters) => {
    removeFilter(group, filters);
  }, [removeFilter]);
  return useMemo(() => ({
    filters: {
      applied:   filters.applied[group].filters,
      available: filters.available[group].filters,
      omitted:   filters.available[group].omitted || 0,
    },
    applyFilter: applyGroupFilter,
    removeFilter: removeGroupFilter,
  }), [filters, applyGroupFilter, removeGroupFilter]);
}

export const useSourceFilters = createGroupFilters(
  FILTER_GROUP_SOURCES);
export const useAsnFilters = createGroupFilters(
  FILTER_GROUP_ASNS);
export const useCommunitiesFilters = createGroupFilters(
  FILTER_GROUP_COMMUNITIES);
export const useExtCommunitiesFilters = createGroupFilters(
  FILTER_GROUP_EXT_COMMUNITIES);
export const useLargeCommunitiesFilters = createGroupFilters(
  FILTER_GROUP_LARGE_COMMUNITIES);
export const useAddrFamilyFilters = createGroupFilters(
  FILTER_GROUP_ADDR_FAMILY);
export const useMPLSLabelFilters = createGroupFilters(
  FILTER_GROUP_MPLS_LABELS);

/**
 * RoutesFiltersProvider merged the filters from the
 * received, filtered and noexport responses
 */
export const RoutesFiltersProvider = ({children}) => {
  const received = useRoutesFilters(useRoutesReceived());
  const filtered = useRoutesFilters(useRoutesFiltered());
  const noexport = useRoutesFilters(useRoutesNotExported());

  const [queryFilters, setFilterQuery] = useFiltersQuery();

  const applyFilter = useCallback((group, value) => {
    const key = FILTER_GROUP_KEYS[group];
    const values = [...queryFilters[key], value];
    setFilterQuery(key, values);
  }, [queryFilters, setFilterQuery]);

  const removeFilter = useCallback((group, value) => {
    const cmp = FILTER_VALUE_CMP[group];
    const key = FILTER_GROUP_KEYS[group];
    const values = queryFilters[key].filter((f) => !cmp(f, value));
    setFilterQuery(key, values);
  }, [queryFilters, setFilterQuery]);

  const filters = useMemo(() => {
    const applied = mergeFilters(
      received.applied,
      filtered.applied,
      noexport.applied,
    );
    const available = mergeFilters(
      received.available,
      filtered.available,
      noexport.available,
    );
    let notAvailable = [];
    if (received.notAvailable) {
      notAvailable = received.notAvailable;
    }
    return { applied, available, notAvailable };
  }, [received, filtered, noexport]);

  const context = {filters, applyFilter, removeFilter};
  return (
    <FiltersContext.Provider value={context}>
      {children}
    </FiltersContext.Provider>
  );
};

