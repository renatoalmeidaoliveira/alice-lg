
import { useMemo
       , useCallback
       }
  from 'react';

import { useMPLSLabelFilters }
  from 'app/context/filters';

import ButtonRemoveFilter
  from 'app/components/filters/ButtonRemoveFilter';


/**
 * AppliedLabel renders an active MPLS label filter
 * with a button for removal.
 */
const AppliedLabel = ({filter, onRemove}) => {
  const remove = useCallback(() => {
    onRemove(filter.value);
  }, [filter, onRemove]);

  return (
    <tr>
      <td className="select-container">
        <span className="label label-mpls">
          Label {filter.value}
        </span>
      </td>
      <td>
        <ButtonRemoveFilter onClick={remove} />
      </td>
    </tr>
  );
}


/**
 * MPLSLabelsSelect filters (L3VPN) routes by their
 * MPLS label. Multiple labels can be selected, a route
 * matches if it carries any of the selected labels.
 */
const MPLSLabelsSelect = () => {
  const {filters, applyFilter, removeFilter} = useMPLSLabelFilters();
  const {applied, available} = filters;

  const sortedAvailable = useMemo(() =>
    [...available].sort((a, b) => a.value - b.value),
    [available]);

  const apply = useCallback((e) => {
    const label = parseInt(e.target.value, 10);
    if (isNaN(label)) {
      return;
    }
    applyFilter(label);
  }, [applyFilter]);

  // Nothing to do if no route carries a label
  if (available.length === 0 && applied.length === 0) {
    return null;
  }

  return (
    <table className="select-ctrl">
      <tbody>
        {applied.map((filter) =>
          <AppliedLabel
            key={filter.value}
            filter={filter}
            onRemove={removeFilter} />)}
        {sortedAvailable.length > 0 &&
          <tr>
            <td className="select-container" colSpan="2">
              <select value="none"
                      onChange={apply}
                      className="form-control">
                <option value="none" className="options-title">
                  {applied.length > 0
                    ? "Add another MPLS label..."
                    : "Show only routes with MPLS label..."}
                </option>
                {sortedAvailable.map((filter) =>
                  <option key={filter.value} value={filter.value}>
                    {filter.value} ({filter.cardinality})
                  </option>)}
              </select>
            </td>
          </tr>}
      </tbody>
    </table>
  );
}

export default MPLSLabelsSelect;
