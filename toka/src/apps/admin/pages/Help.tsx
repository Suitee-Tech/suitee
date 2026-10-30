import { useEffect, useMemo, useState, type ComponentProps } from "react";
import { useSearchParams, Link } from "react-router";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { BookOpen, ChevronDown, Search, Sparkles } from "lucide-react";
import { Button } from "../../../shared/ui/button";
import { Input } from "../../../shared/ui/input";
import { cn } from "../../../shared/ui/utils";
import {
  getHelpTopic,
  helpIndexBody,
  helpTopics,
  searchHelp,
  type HelpSearchHit,
} from "../../../help/helpContent";

const mdComponents: ComponentProps<typeof ReactMarkdown>["components"] = {
  h1: ({ children, ...props }) => (
    <h1 className="mb-3 mt-2 text-2xl font-bold tracking-tight" {...props}>{children}</h1>
  ),
  h2: ({ children, ...props }) => (
    <h2 className="mb-2 mt-8 border-b pb-1 text-xl font-semibold" {...props}>{children}</h2>
  ),
  h3: ({ children, ...props }) => (
    <h3 className="mb-2 mt-6 text-lg font-semibold" {...props}>{children}</h3>
  ),
  p: ({ children, ...props }) => (
    <p className="my-3 text-sm leading-relaxed text-foreground" {...props}>{children}</p>
  ),
  ul: ({ children, ...props }) => (
    <ul className="my-3 list-disc space-y-1.5 pl-5 text-sm" {...props}>{children}</ul>
  ),
  ol: ({ children, ...props }) => (
    <ol className="my-3 list-decimal space-y-1.5 pl-5 text-sm" {...props}>{children}</ol>
  ),
  li: ({ children, ...props }) => (
    <li className="leading-relaxed" {...props}>{children}</li>
  ),
  a: ({ children, href, ...props }) => {
    const isInternal = typeof href === "string" && href.startsWith("/") && !href.startsWith("//");
    if (isInternal) {
      return (
        <Link
          to={href}
          className="font-medium text-primary underline underline-offset-2 hover:text-primary/80"
          {...props}
        >
          {children}
        </Link>
      );
    }
    return (
      <a
        href={href}
        target="_blank"
        rel="noreferrer"
        className="font-medium text-primary underline underline-offset-2 hover:text-primary/80"
        {...props}
      >
        {children}
      </a>
    );
  },
  blockquote: ({ children, ...props }) => (
    <blockquote
      className="my-3 border-l-4 border-primary/40 bg-muted/40 py-1 pl-4 pr-2 text-sm italic"
      {...props}
    >
      {children}
    </blockquote>
  ),
  code: ({ children, ...props }) => (
    <code
      className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs"
      {...props}
    >
      {children}
    </code>
  ),
  pre: ({ children, ...props }) => (
    <pre className="my-3 overflow-x-auto rounded-lg bg-muted p-3 font-mono text-xs" {...props}>
      {children}
    </pre>
  ),
  table: ({ children, ...props }) => (
    // Tabelas de referência (ex.: catálogo de variáveis) são mais largas que o
    // artigo. O scroll fica contido na tabela e nunca vaza para a página.
    <div className="my-4 w-full max-w-full overflow-x-auto rounded-lg border border-border">
      <table className="w-full min-w-[560px] border-collapse text-sm" {...props}>{children}</table>
    </div>
  ),
  thead: ({ children, ...props }) => (
    <thead className="bg-muted/60" {...props}>{children}</thead>
  ),
  th: ({ children, ...props }) => (
    <th className="border px-2.5 py-1.5 text-left font-semibold" {...props}>{children}</th>
  ),
  td: ({ children, ...props }) => (
    <td className="border px-2.5 py-1.5 align-top break-words" {...props}>{children}</td>
  ),
  tr: ({ children, ...props }) => (
    <tr className="odd:bg-background" {...props}>{children}</tr>
  ),
  img: ({ alt, ...props }) => (
    <img alt={alt ?? ""} className="my-4 max-w-full rounded-lg border shadow-sm" {...props} />
  ),
  strong: ({ children, ...props }) => (
    <strong className="font-semibold" {...props}>{children}</strong>
  ),
  hr: () => <hr className="my-6" />,
};

function HelpMarkdown({ source }: { source: string }) {
  return <ReactMarkdown remarkPlugins={[remarkGfm]} components={mdComponents}>{source}</ReactMarkdown>;
}

function TopicItem({
  id,
  title,
  summary,
  active,
  onClick,
}: {
  id: string;
  title: string;
  summary?: string;
  active?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "w-full rounded-lg px-3 py-2 text-left transition-colors",
        active
          ? "bg-primary/10 text-primary"
          : "text-foreground/80 hover:bg-muted hover:text-foreground",
      )}
    >
      <div className="text-sm font-medium">{title}</div>
      {summary ? (
        <div className="mt-0.5 line-clamp-2 text-xs text-muted-foreground">{summary}</div>
      ) : null}
    </button>
  );
}

function SearchResultItem({ hit, onClick }: { hit: HelpSearchHit; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="w-full rounded-lg px-3 py-2 text-left transition-colors hover:bg-muted"
    >
      <div className="text-sm font-medium text-foreground">{hit.topic.title}</div>
      <div className="mt-0.5 line-clamp-2 text-xs text-muted-foreground">{hit.excerpt}</div>
    </button>
  );
}

export default function Help() {
  const [searchParams, setSearchParams] = useSearchParams();
  const selectedId = searchParams.get("t") ?? "";
  const [query, setQuery] = useState("");
  // No mobile a navegação começa recolhida: o conteúdo fica a um toque de
  // distância, em vez de exigir rolar a lista inteira de assuntos.
  const [mobileNavOpen, setMobileNavOpen] = useState(false);

  const results = useMemo(() => (query.trim() ? searchHelp(query, 8) : []), [query]);

  const openTopic = (id: string) => {
    setQuery("");
    setMobileNavOpen(false);
    setSearchParams(id ? { t: id } : {});
  };

  const selected = selectedId ? getHelpTopic(selectedId) : undefined;
  const showIndex = !selected && query.trim().length === 0;

  // Trocar de tópico volta ao topo do conteúdo — o contêiner de scroll é o
  // <main> do shell (a própria página não rola).
  useEffect(() => {
    document.querySelector("main")?.scrollTo({ top: 0, behavior: "smooth" });
  }, [selectedId]);

  return (
    <div className="mx-auto flex w-full max-w-7xl flex-col gap-4 p-4 sm:p-6 lg:flex-row lg:items-start lg:gap-6">
      {/* Barra lateral: fixa no desktop, recolhível no mobile */}
      <aside className="w-full lg:sticky lg:top-6 lg:max-h-[calc(100dvh-3rem)] lg:w-72 lg:shrink-0 lg:self-start lg:overflow-y-auto lg:overscroll-contain lg:pr-1">
        <div className="mb-3 flex items-center gap-2">
          <BookOpen className="h-5 w-5 shrink-0 text-primary" />
          <h1 className="text-lg font-bold tracking-tight">Central de Ajuda</h1>
        </div>

        <div className="sticky top-0 z-10 -mx-4 mb-3 bg-background px-4 pb-2 sm:-mx-6 sm:px-6 lg:static lg:mx-0 lg:bg-transparent lg:px-0 lg:pb-0">
          <div className="relative">
            <Search className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Buscar na ajuda (ex.: gerar DIMOB)"
              className="pl-8"
              aria-label="Buscar na Central de Ajuda"
              type="search"
            />
          </div>
        </div>

        {/* Botão de assuntos (somente mobile) */}
        {!query.trim() && (
          <button
            type="button"
            onClick={() => setMobileNavOpen((v) => !v)}
            aria-expanded={mobileNavOpen}
            aria-controls="ajuda-topicos"
            className="mb-2 flex w-full items-center justify-between rounded-lg border bg-card px-3 py-2.5 text-left text-sm font-medium lg:hidden"
          >
            <span className="truncate">
              {selected ? selected.title : "Visão geral"}
            </span>
            <span className="flex shrink-0 items-center gap-1 text-xs text-muted-foreground">
              {mobileNavOpen ? "Fechar" : "Assuntos"}
              <ChevronDown className={cn("h-4 w-4 transition-transform", mobileNavOpen && "rotate-180")} />
            </span>
          </button>
        )}

        <nav
          id="ajuda-topicos"
          aria-label="Tópicos da Central de Ajuda"
          className={cn(
            "space-y-0.5 lg:block lg:max-h-none lg:overflow-visible",
            query.trim() || mobileNavOpen
              ? "max-h-[60dvh] overflow-y-auto overscroll-contain rounded-lg border bg-card p-1 lg:border-0 lg:bg-transparent lg:p-0"
              : "hidden",
          )}
        >
          {query.trim() ? (
            results.length > 0 ? (
              results.map((hit) => (
                <SearchResultItem key={hit.topic.id} hit={hit} onClick={() => openTopic(hit.topic.id)} />
              ))
            ) : (
              <p className="px-3 py-2 text-sm text-muted-foreground">
                Nenhum tópico encontrado para “{query}”. Tente outras palavras — ou pergunte à
                Suitee IA.
              </p>
            )
          ) : (
            <>
              <TopicItem id="index" title="Visão geral" active={showIndex} onClick={() => openTopic("")} />
              {helpTopics.map((t) => (
                <TopicItem
                  key={t.id}
                  id={t.id}
                  title={t.title}
                  active={selectedId === t.id}
                  onClick={() => openTopic(t.id)}
                />
              ))}
            </>
          )}
        </nav>

        <div className="mt-4 rounded-xl border bg-muted/40 p-3">
          <div className="flex items-start gap-2">
            <Sparkles className="mt-0.5 h-4 w-4 shrink-0 text-primary" />
            <div>
              <p className="text-sm font-medium">Resposta rápida com a Suitee IA</p>
              <p className="mt-1 text-xs text-muted-foreground">
                O chat de IA consulta esta mesma Central de Ajuda.
              </p>
              <Button
                variant="link"
                size="sm"
                className="mt-1 h-auto p-0 text-xs"
                onClick={() => window.location.assign("/inteligencia")}
              >
                Abrir a Suitee IA
              </Button>
            </div>
          </div>
        </div>
      </aside>

      {/* Conteúdo */}
      <div className="min-w-0 flex-1">
        <article
          aria-label={selected ? selected.title : "Central de Ajuda"}
          className="rounded-xl border bg-card p-4 shadow-token-sm sm:p-6 lg:max-w-3xl"
        >
          {selected ? (
            <>
              <div className="mb-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                Central de Ajuda
              </div>
              <HelpMarkdown source={selected.body} />
            </>
          ) : (
            <HelpMarkdown source={helpIndexBody} />
          )}
        </article>
      </div>
    </div>
  );
}
