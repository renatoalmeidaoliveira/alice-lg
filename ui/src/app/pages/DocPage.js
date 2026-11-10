
import { useMemo
       , useCallback
       }
  from 'react';
import { useParams
       , useNavigate
       }
  from 'react-router-dom';

import { Marked }
  from 'marked';
import DOMPurify
  from 'dompurify';

import { useDoc }
  from 'app/context/docs';

import PageHeader
  from 'app/components/page/Header';
import LoadingIndicator
  from 'app/components/spinners/LoadingIndicator';


const DOCS_ASSETS = "/docs-assets/";

// Links and images with a scheme (http:, mailto:, ...),
// protocol relative or absolute paths are not rewritten.
const isExternal = (href) =>
  /^([a-z][a-z0-9+.-]*:|\/|#)/i.test(href);

/**
 * rewriteHref maps relative links in a document:
 *  - other documents: "02-vrfs.md#rt" -> "/docs/02-vrfs#rt"
 *  - other files:     "img/topo.png"  -> "/docs-assets/img/topo.png"
 */
const rewriteHref = (href) => {
  if (!href || isExternal(href)) {
    return href;
  }
  const clean = href.replace(/^\.\//, "");
  const doc = clean.match(/^([^/?#]+)\.md(#.*)?$/i);
  if (doc) {
    return `/docs/${doc[1]}${doc[2] || ""}`;
  }
  return DOCS_ASSETS + clean;
}

const markdown = new Marked({
  gfm: true,
  walkTokens: (token) => {
    if (token.type === "link" || token.type === "image") {
      token.href = rewriteHref(token.href);
    }
  },
});

const renderMarkdown = (content) =>
  DOMPurify.sanitize(markdown.parse(content || ""));


const DocContent = ({doc}) => {
  const navigate = useNavigate();
  const html = useMemo(() => renderMarkdown(doc.content), [doc]);

  // Open links to other documents within the app
  const onClick = useCallback((e) => {
    const anchor = e.target.closest("a");
    if (!anchor || anchor.target === "_blank") {
      return;
    }
    const href = anchor.getAttribute("href") || "";
    if (href.startsWith("/docs/")) {
      e.preventDefault();
      navigate(href);
    }
  }, [navigate]);

  return (
    <div className="doc-content"
         onClick={onClick}
         dangerouslySetInnerHTML={{__html: html}} />
  );
}


const DocPage = () => {
  const { docId } = useParams();
  const { loading, doc, error } = useDoc(docId);

  let body = null;
  if (loading) {
    body = <LoadingIndicator />;
  } else if (error || !doc) {
    body = (
      <div className="doc-not-found">
        The requested document could not be found.
      </div>
    );
  } else {
    body = <DocContent doc={doc} />;
  }

  return (
    <div className="doc-page">
      <PageHeader></PageHeader>
      <div className="row details-main">
        <div className="col-main col-lg-9 col-md-12">
          <div className="card">
            {body}
          </div>
        </div>
      </div>
    </div>
  );
}

export default DocPage;
