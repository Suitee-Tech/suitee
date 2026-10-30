import type { ReactNode } from "react";
import { clsx } from "clsx";

type PortalPageProps = {
  children: ReactNode;
  /**
   * `scroll` (padrão): a página rola com a janela, como Painel e Financeiro.
   * `full`: a página ocupa a altura útil e rola por dentro (chat, detalhe do
   * negócio, extrato com cabeçalho fixo).
   */
  variant?: "scroll" | "full";
  /**
   * Largura máxima. `lg` (padrão) é a coluna do celular; `xl`/`wide` servem às
   * telas que aproveitam o desktop (busca, painel, mensagens).
   */
  width?: "lg" | "xl" | "wide";
  className?: string;
};

/**
 * Contêiner único das páginas internas do portal.
 *
 * Regras que ele centraliza (e que estavam divergentes tela a tela):
 *
 *  1. **A barra inferior continua visível.** A altura útil desconta as 4rem da
 *     navegação e a área segura do aparelho; antes havia páginas com `h-dvh`
 *     puro, cujo rodapé ficava atrás da barra — o usuário via a página "sem
 *     menu inferior".
 *  2. **Respiro no fim da rolagem.** O padding inferior é maior que a barra,
 *     para o último item não ficar escondido atrás dela.
 *  3. **Mesmo fundo e mesma largura** em todas as telas (`bg-background`,
 *     `max-w-lg`), em vez do `#fafafa` fixo que só existia aqui.
 */
const WIDTH_CLASS: Record<NonNullable<PortalPageProps["width"]>, string> = {
  lg: "max-w-lg",
  xl: "max-w-5xl",
  wide: "max-w-6xl",
};

export function PortalPage({ children, variant = "scroll", width = "lg", className }: PortalPageProps) {
  return (
    <div
      className={clsx(
        "mx-auto w-full bg-background",
        WIDTH_CLASS[width],
        variant === "full"
          ? "flex h-[calc(100dvh-4rem-env(safe-area-inset-bottom))] flex-col overflow-hidden"
          : "min-h-[calc(100dvh-4rem-env(safe-area-inset-bottom))] pb-[calc(5rem+env(safe-area-inset-bottom))]",
        className,
      )}
    >
      {children}
    </div>
  );
}
