import type { ReactNode } from "react";
import { ArrowLeft } from "lucide-react";
import { clsx } from "clsx";
import { Button } from "../../../shared/ui/button";
import { usePortalBack } from "./usePortalBack";

type PortalPageHeaderProps = {
  title: string;
  subtitle?: string;
  /**
   * Destino quando não há histórico do portal para voltar (link direto, aba
   * nova, recarga). O botão sempre tenta `navigate(-1)` primeiro — voltar para
   * onde a pessoa estava é o comportamento esperado, e mandar para a home
   * incondicionalmente era o que fazia a navegação parecer quebrada.
   */
  backTo?: string;
  /** `false` na raiz (Painel): não há para onde voltar. */
  showBack?: boolean;
  /** Ações à direita (ex.: "Marcar todas lidas"). */
  actions?: ReactNode;
  className?: string;
};

/**
 * Cabeçalho único das páginas internas do portal.
 *
 * Antes cada tela montava o seu — algumas com `sticky`, outras sem; umas com
 * botão de voltar apontando para `/portal`, outras sem botão nenhum — e a
 * pessoa ficava sem caminho de volta em Notificações, Suporte, Perfil,
 * Atendimentos, Buscar e Menu. Aqui o cabeçalho, o botão acessível
 * (`aria-label="Voltar"`) e o fundo por token são os mesmos em todas elas.
 */
export function PortalPageHeader({
  title,
  subtitle,
  backTo = "/portal",
  showBack = true,
  actions,
  className,
}: PortalPageHeaderProps) {
  const goBack = usePortalBack(backTo);

  return (
    <header
      className={clsx(
        "sticky top-0 z-20 flex items-center gap-3 border-b border-border/60 bg-background/95 px-5 pb-4 pt-6 backdrop-blur",
        className,
      )}
    >
      {showBack && (
        <Button
          type="button"
          onClick={goBack}
          variant="ghost"
          size="icon"
          aria-label="Voltar"
          title="Voltar"
          className="-ml-2 size-10 shrink-0 rounded-full bg-muted text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="size-5" aria-hidden="true" />
        </Button>
      )}
      <div className="min-w-0 flex-1">
        <h1 className="truncate text-section font-bold tracking-tight text-foreground">{title}</h1>
        {subtitle && <p className="mt-0.5 truncate text-xs text-muted-foreground">{subtitle}</p>}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </header>
  );
}
