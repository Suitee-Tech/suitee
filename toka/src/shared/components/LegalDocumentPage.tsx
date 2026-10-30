import { useEffect, useState } from "react";
import { Link, useLocation } from "react-router";
import { ArrowLeft, Loader2 } from "lucide-react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";

interface LegalDocumentPageProps {
  title: string;
  documentUrl: string;
  version: string;
}

/**
 * Os dois documentos se citam (`/privacy` nos Termos, `/terms` na Política) e o
 * MESMO markdown serve painel e portal. Sem traduzir o caminho, quem lê pelo
 * portal cai na página do painel: outra casca, outro "Voltar".
 */
const LEGAL_ROUTES: Record<string, { admin: string; portal: string }> = {
  "/privacy": { admin: "/privacy", portal: "/portal/privacy" },
  "/terms": { admin: "/terms", portal: "/portal/terms" },
};

function resolveLegalRoute(href: string | undefined, isPortal: boolean): string | null {
  if (!href) return null;
  const route = LEGAL_ROUTES[href.replace(/\/+$/, "")];
  if (!route) return null;
  return isPortal ? route.portal : route.admin;
}

export function LegalDocumentPage({ title, documentUrl, version }: LegalDocumentPageProps) {
  const [markdown, setMarkdown] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const isPortal = useLocation().pathname.startsWith("/portal");

  useEffect(() => {
    let cancelled = false;
    fetch(documentUrl)
      .then((response) => {
        if (!response.ok) throw new Error("HTTP " + response.status);
        return response.text();
      })
      .then((text) => {
        if (!cancelled) setMarkdown(text);
      })
      .catch((reason) => {
        if (!cancelled) setError(reason instanceof Error ? reason.message : "Erro ao carregar documento");
      });
    return () => {
      cancelled = true;
    };
  }, [documentUrl]);

  // Citação entre documentos navega dentro da aplicação; link externo continua
  // sendo link comum.
  const markdownComponents: Components = {
    a: ({ href, children, ...rest }) => {
      const to = resolveLegalRoute(href, isPortal);
      if (to) {
        return (
          <Link to={to} {...rest}>
            {children}
          </Link>
        );
      }
      return (
        <a href={href} target="_blank" rel="noreferrer" {...rest}>
          {children}
        </a>
      );
    },
  };

  return (
    <div className="min-h-screen bg-background">
      <header className="sticky top-0 z-20 bg-card border-b border-border px-5 py-3 flex items-center gap-3">
        <Link to="/" className="p-2 -ml-2 text-muted-foreground hover:bg-muted rounded-lg transition-colors" aria-label="Voltar">
          <ArrowLeft className="w-5 h-5" />
        </Link>
        <div>
          <h1 className="text-base font-bold text-foreground">{title}</h1>
          <p className="text-caption text-muted-foreground">Versão {version}</p>
        </div>
      </header>

      <main className="max-w-3xl mx-auto px-5 py-8">
        {error ? (
          <div className="bg-status-critical-soft border border-status-critical/30 rounded-lg p-4 text-sm">
            <p className="font-bold text-status-critical">Não foi possível carregar este documento.</p>
            <p className="text-muted-foreground mt-1">{error}</p>
            <p className="text-muted-foreground mt-2 text-xs">
              Tente recarregar ou contate <a className="underline" href="mailto:contato@suitee.com.br">contato@suitee.com.br</a>.
            </p>
          </div>
        ) : !markdown ? (
          <div className="flex items-center gap-2 text-muted-foreground">
            <Loader2 className="w-4 h-4 animate-spin" />
            Carregando...
          </div>
        ) : (
          <article className="prose prose-sm max-w-none prose-headings:text-foreground prose-p:text-foreground/90 prose-strong:text-foreground prose-li:text-foreground/90 prose-a:text-primary-600 hover:prose-a:text-primary-700 prose-table:text-sm">
            <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
              {markdown}
            </ReactMarkdown>
          </article>
        )}
      </main>
    </div>
  );
}
