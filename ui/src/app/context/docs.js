/**
 * Docs provider
 *
 * Loads the list of markdown documents, which are
 * shown in the sidebar navigation.
 */

import axios from 'axios';

import { useState
       , useEffect
       , useContext
       , useRef
       , createContext
       }
  from 'react';

import { useErrorHandler }
  from 'app/context/errors';


const DocsContext = createContext({title: "", docs: []});

export const useDocs = () => useContext(DocsContext);

export const DocsProvider = ({children}) => {
  const init = useRef();
  const handleError = useErrorHandler();
  const [docs, setDocs] = useState({title: "", docs: []});

  useEffect(() => {
    if (init.current) {
      return;
    }
    init.current = true;
    axios.get('/api/v1/docs')
      .then(
        ({data}) => setDocs({
          title: data.title,
          docs: data.docs || [],
        }),
        (error) => handleError(error)
      );
  }, [handleError]);

  return (
    <DocsContext.Provider value={docs}>
      {children}
    </DocsContext.Provider>
  );
}

/**
 * useDoc loads a single document with its content
 */
export const useDoc = (docId) => {
  const handleError = useErrorHandler();
  const [state, setState] = useState({loading: true});

  useEffect(() => {
    let cancelled = false;
    setState({loading: true});
    axios.get(`/api/v1/docs/${encodeURIComponent(docId)}`)
      .then(
        ({data}) => {
          if (!cancelled) {
            setState({loading: false, doc: data.doc});
          }
        },
        (error) => {
          if (cancelled) {
            return;
          }
          if (error?.response?.status !== 404) {
            handleError(error);
          }
          setState({loading: false, error: error});
        });
    return () => { cancelled = true; };
  }, [docId, handleError]);

  return state;
}
