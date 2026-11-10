
import { useMemo
       , useState
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


// Largest MPLS label value (20 bit)
const MAX_MPLS_LABEL = 1048575;


/**
 * MPLSLabelInput applies a filter for any label. With
 * a label per prefix, only the most common labels are
 * listed in the select.
 */
const MPLSLabelInput = ({onApply}) => {
  const [value, setValue] = useState("");

  const submit = useCallback((e) => {
    e.preventDefault();
    const label = parseInt(value, 10);
    if (isNaN(label) || label < 0 || label > MAX_MPLS_LABEL) {
      return;
    }
    onApply(label);
    setValue("");
  }, [value, onApply]);

  return (
    <form className="mpls-label-input" onSubmit={submit}>
      <div className="input-group">
        <input type="number"
               min="0"
               max={MAX_MPLS_LABEL}
               className="form-control"
               placeholder="MPLS label..."
               value={value}
               onChange={(e) => setValue(e.target.value)} />
        <span className="input-group-btn">
          <button type="submit"
                  className="btn btn-default"
                  disabled={value === ""}>
            Filter
          </button>
        </span>
      </div>
    </form>
  );
}


/**
 * MPLSLabelsSelect filters (L3VPN) routes by their
 * MPLS label. Multiple labels can be selected, a route
 * matches if it carries any of the selected labels.
 */
const MPLSLabelsSelect = () => {
  const {filters, applyFilter, removeFilter} = useMPLSLabelFilters();
  const {applied, available, omitted} = filters;

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
                  {omitted > 0
                    ? `Top ${sortedAvailable.length} labels...`
                    : applied.length > 0
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
        {omitted > 0 &&
          <tr>
            <td className="select-container" colSpan="2">
              <MPLSLabelInput onApply={applyFilter} />
            </td>
          </tr>}
      </tbody>
    </table>
  );
}

export default MPLSLabelsSelect;
