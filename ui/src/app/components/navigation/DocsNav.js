
import { Link
       , useLocation
       }
  from 'react-router-dom';

import { useDocs }
  from 'app/context/docs';


/**
 * DocsNav lists the markdown documents in the sidebar
 */
const DocsNav = () => {
  const { title, docs } = useDocs();
  const { pathname } = useLocation();

  if (!docs || docs.length === 0) {
    return null;
  }

  return (
    <div className="docs-nav">
      <h2>{title}</h2>
      <ul>
        {docs.map((doc) => {
          const path = `/docs/${doc.id}`;
          const active = pathname === path;
          return (
            <li key={doc.id} className={active ? "active" : ""}>
              <Link to={path} className="doc-link">{doc.title}</Link>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

export default DocsNav;
