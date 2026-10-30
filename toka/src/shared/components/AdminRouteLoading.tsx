/**
 * Fallback das rotas lazy do painel.
 *
 * A página é carregada dentro do shell (sidebar/header já montados), então um
 * spinner `h-screen` parecia um reload completo do produto. Este skeleton
 * ocupa apenas a área de conteúdo e mantém a navegação visível.
 */
export function AdminRouteLoading() {
  return (
    <div
      role="status"
      aria-live="polite"
      aria-label="Carregando página"
      className="space-y-6 p-6 lg:p-8"
    >
      <div className="space-y-2">
        <div className="h-7 w-44 animate-pulse rounded-lg bg-muted" />
        <div className="h-4 w-72 max-w-full animate-pulse rounded bg-muted" />
      </div>
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {Array.from({ length: 4 }, (_, index) => (
          <div key={index} className="h-24 animate-pulse rounded-2xl border border-border bg-card" />
        ))}
      </div>
      <div className="h-80 animate-pulse rounded-2xl border border-border bg-card" />
      <span className="sr-only">Carregando página…</span>
    </div>
  );
}
