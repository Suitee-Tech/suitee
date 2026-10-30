import { Crown, Sparkles } from "lucide-react";
import { clsx } from "clsx";

/**
 * Marca do Suitee Intelligence.
 *
 * O produto tinha quatro desenhos diferentes do MESMO símbolo (cabeçalho do
 * chat, avatar do assistente, indicador "digitando" e o ícone da navbar, que era
 * só um `Sparkles` solto) e cada tela nova reinventava o brilho. Aqui existe uma
 * única marca: núcleo circular com gradiente da paleta do tema, anel que pulsa
 * quando o assistente está trabalhando, respiro no núcleo quando está vivo e um
 * brilho que atravessa no hover. Tamanho e comportamento são props — a
 * identidade visual não muda entre navbar e chat.
 *
 * Puramente decorativa: `aria-hidden` sempre, porque quem carrega o nome
 * acessível é o botão/título que a contém.
 *
 * `tone` separa as duas leituras da marca sem criar um segundo desenho:
 * "soft" é o tom ambiente (cabeçalho do chat, avatares) e "prominent" é o da
 * navbar — núcleo sólido na cor primária, com halo permanente, porque ali a
 * marca disputa atenção com botões de ícone iguais aos outros e precisa ser o
 * elemento de destaque da barra.
 */
type MarkSize = "sm" | "md" | "lg";

const SIZE_CLASSES: Record<MarkSize, { frame: string; glyph: string; ring: string }> = {
  // 28px — navbar (mantém o rodapé de 40px do botão com `p-1.5`).
  sm: { frame: "h-7 w-7", glyph: "h-3.5 w-3.5", ring: "inset-[-2px]" },
  // 32px — avatar do assistente e indicador de digitação.
  md: { frame: "h-8 w-8", glyph: "h-4 w-4", ring: "inset-[-2px]" },
  // 36px — cabeçalho do chat.
  lg: { frame: "h-9 w-9", glyph: "h-4 w-4", ring: "inset-[-2px]" },
};

export function SuiteeIntelligenceMark({
  size = "md",
  /** Núcleo respirando: o assistente está vivo (aberto ou trabalhando). */
  live = false,
  /** Anel externo: "sempre" enquanto trabalha, "hover" onde o clique é a ação. */
  pulse = "none",
  /** Realce suave no hover do container (`group`). */
  overlay = false,
  /** Feixe de luz que atravessa no hover. */
  shimmer = false,
  /** Perfil sem acesso à IA: a coroa substitui o brilho, no mesmo quadro. */
  locked = false,
  /** "soft" ambiente (chat) · "prominent" sólido com halo (navbar). */
  tone = "soft",
  className,
}: {
  size?: MarkSize;
  live?: boolean;
  pulse?: "none" | "hover" | "always";
  overlay?: boolean;
  shimmer?: boolean;
  locked?: boolean;
  tone?: "soft" | "prominent";
  className?: string;
}) {
  const dims = SIZE_CLASSES[size];
  const Glyph = locked ? Crown : Sparkles;
  const prominent = tone === "prominent";

  return (
    <span
      aria-hidden="true"
      data-testid="suitee-intelligence-mark"
      className={clsx(
        "relative flex shrink-0 items-center justify-center rounded-full overflow-visible",
        "transition-transform duration-300 group-hover:scale-105",
        prominent
          ? "bg-primary text-primary-foreground border border-primary/40 shadow-md shadow-primary/30"
          : "bg-gradient-to-br from-primary/10 via-accent/20 to-secondary/10 text-primary border border-primary/20 shadow-sm",
        dims.frame,
        className,
      )}
    >
      {/* Halo permanente no tom de destaque: é o que separa a marca dos outros
          botões de ícone da barra mesmo sem hover. */}
      {prominent && (
        <span className="absolute -inset-1 rounded-full bg-primary/20 blur-md transition-colors group-hover:bg-primary/30" />
      )}
      <span
        className={clsx(
          "absolute rounded-full border",
          prominent ? "border-primary-foreground/30" : "border-primary/30",
          dims.ring,
          pulse === "always" && "opacity-100 motion-safe:animate-ai-pulse",
          pulse === "hover" && "opacity-0 transition-opacity group-hover:opacity-100 motion-safe:group-hover:animate-ai-pulse",
        )}
      />
      {prominent ? (
        <>
          {/* Brilho de topo: dá volume ao núcleo sólido sem gradiente colorido. */}
          <span className="absolute inset-0 rounded-full bg-gradient-to-b from-white/25 to-transparent" />
          {live && <span className="absolute inset-0 rounded-full bg-accent/30 motion-safe:animate-ai-core" />}
        </>
      ) : (
        <span
          className={clsx(
            "absolute inset-0 rounded-full bg-gradient-to-br from-primary/20 via-accent/30 to-secondary/20",
            live && "motion-safe:animate-ai-core",
          )}
        />
      )}
      {overlay && (
        <span className="absolute inset-0 z-10 rounded-full bg-white/40 opacity-0 transition-opacity group-hover:opacity-100 dark:bg-white/10" />
      )}
      {shimmer && (
        <span className="absolute inset-0 z-10 overflow-hidden rounded-full">
          <span className="absolute inset-y-0 -left-full w-1/2 bg-gradient-to-tr from-transparent via-white/50 to-transparent transition-transform duration-1000 group-hover:translate-x-[300%]" />
        </span>
      )}
      <Glyph
        className={clsx(
          "relative z-20",
          prominent ? "drop-shadow-[0_1px_2px_rgba(0,0,0,0.35)]" : "drop-shadow-[0_0_6px_rgba(var(--primary),0.5)]",
          dims.glyph,
        )}
      />
    </span>
  );
}
