import { useEffect } from "react";
import { useLocation } from "react-router";

import { formatDocumentTitle } from "../lib/documentTitle";

/** Mantém `document.title` sincronizado com a rota atual. */
export function RouteTitle() {
  const { pathname } = useLocation();

  useEffect(() => {
    document.title = formatDocumentTitle(pathname);
  }, [pathname]);

  return null;
}
