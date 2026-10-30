import { useCallback, useEffect, useState, useRef, type ReactNode } from "react";
import * as Dialog from "@radix-ui/react-dialog";
import { motion } from "motion/react";
import { Maximize2, Minimize2, X } from "lucide-react";
import { clsx } from "clsx";

type PanelSize = "sm" | "md" | "lg" | "xl" | "2xl" | "3xl" | "4xl" | "full";

const SIZE_CLASSES: Record<PanelSize, string> = {
  sm: "max-w-sm",
  md: "max-w-md",
  lg: "max-w-lg",
  xl: "max-w-xl",
  "2xl": "max-w-2xl",
  "3xl": "max-w-3xl",
  "4xl": "max-w-4xl",
  full: "max-w-full",
};

interface ExpandableSidePanelProps {
  open: boolean;
  onClose: () => void;
  defaultSize?: PanelSize;
  expandedSize?: PanelSize;
  bgClass?: string;
  className?: string;
  children: ReactNode;
  header?: ReactNode;
  /**
   * Cabeçalho do consumidor. No CELULAR o painel ocupa a tela inteira, então o
   * `closeButton` entregue aqui é a única forma de sair: o cabeçalho precisa
   * caber em 320–390px. Use `flex-wrap` na linha, `min-w-0` no bloco de texto
   * (nome/UUID/descrição longos não encolhem sozinhos) e deixe o cluster de
   * controles ocupar a própria linha no mobile (`w-full justify-end sm:w-auto`).
   * O painel de detalhes da esteira é a referência.
   */
  renderHeader?: (props: {
    isExpanded: boolean;
    onToggleExpand: () => void;
    onClose: () => void;
    expandButton: ReactNode;
    closeButton: ReactNode;
  }) => ReactNode;
  zIndex?: number;
  ariaLabel?: string;
}

export function ExpandableSidePanel({
  open,
  onClose,
  defaultSize = "lg",
  expandedSize = "full",
  bgClass = "bg-card",
  className,
  children,
  header,
  renderHeader,
  zIndex = 50,
  ariaLabel = "Detalhes",
}: ExpandableSidePanelProps) {
  const [isExpanded, setIsExpanded] = useState(false);
  const opener = useRef<HTMLElement | null>(null);

  // O painel é uma superfície do TEMA: `bg-card` resolve claro e escuro sozinho.
  // Existia aqui um `dark:bg-foreground/90` — 90% da cor do TEXTO (quase branco)
  // como fundo — que deixava o painel claro no modo noturno enquanto rótulos,
  // campos e botões dentro dele seguem o tema escuro: texto claro sobre painel
  // claro. Só escapavam os painéis que passavam algo diferente de "bg-card"
  // (o de Leads passava "bg-card " com espaço à direita e por isso ficou certo).
  const resolvedBg = bgClass;

  const toggleExpand = useCallback(() => {
    setIsExpanded((prev) => !prev);
  }, []);

  useEffect(() => {
    if (!open) {
      const timer = setTimeout(() => setIsExpanded(false), 300);
      return () => clearTimeout(timer);
    }
  }, [open]);

  const expandButton = (
    <button
      type="button"
      aria-label={isExpanded ? "Recolher painel" : "Expandir painel"}
      aria-pressed={isExpanded}
      onClick={toggleExpand}
      className={clsx(
        "p-2 rounded-lg border transition-all flex items-center gap-2 text-sm font-medium",
        isExpanded
          ? "text-primary-600 bg-primary-50 border-primary-200 hover:bg-primary-100 dark:text-primary-400 dark:bg-primary-900/30 dark:border-primary-700 dark:hover:bg-primary-900/50"
          : "text-muted-foreground bg-card border-border hover:bg-muted hover:text-foreground shadow-sm dark:text-background dark:bg-foreground/85 dark:border-foreground/25 dark:hover:bg-foreground/75 dark:hover:text-background",
      )}
      title={isExpanded ? "Recolher painel (Ctrl+Shift+E)" : "Expandir painel (Ctrl+Shift+E)"}
    >
      {isExpanded ? <Minimize2 className="w-4 h-4" /> : <Maximize2 className="w-4 h-4" />}
      <span className="hidden sm:inline">{isExpanded ? "Recolher" : "Expandir"}</span>
    </button>
  );

  const closeButton = (
    <button
      type="button"
      aria-label="Fechar painel"
      onClick={onClose}
      className="p-2 rounded-lg transition-colors text-muted-foreground hover:text-foreground hover:bg-muted"
      title="Fechar (Esc)"
    >
      <X className="w-5 h-5" />
    </button>
  );

  const sizeClass = isExpanded ? SIZE_CLASSES[expandedSize] : SIZE_CLASSES[defaultSize];

  return (
    <Dialog.Root open={open} onOpenChange={(nextOpen) => { if (!nextOpen) onClose(); }}>
      <Dialog.Portal>
        <Dialog.Overlay asChild>
          <motion.div
            initial={{ opacity: 0 }} animate={{ opacity: 1 }}
            className="fixed inset-0 bg-foreground/40 backdrop-blur-sm"
            style={{ zIndex }}
          />
        </Dialog.Overlay>
        <Dialog.Content
          asChild
          aria-describedby={undefined}
          onOpenAutoFocus={() => { opener.current = document.activeElement as HTMLElement | null; }}
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            if (opener.current?.isConnected) opener.current.focus();
          }}
          onEscapeKeyDown={(event) => {
            if (isExpanded) { event.preventDefault(); setIsExpanded(false); }
          }}
          onKeyDown={(event) => {
            if ((event.ctrlKey || event.metaKey) && event.shiftKey && event.key.toLowerCase() === "e") {
              event.preventDefault(); event.stopPropagation(); toggleExpand();
            }
          }}
        >
          <motion.div
            initial={{ x: "100%" }} animate={{ x: 0 }}
            transition={{ type: "spring", damping: 28, stiffness: 260 }}
            style={{ zIndex }}
            className={clsx(
              // `h-[100dvh]`, não `h-full`: num elemento `position: fixed` o
              // `height: 100%` mede a viewport GRANDE do mobile (a faixa atrás
              // da barra do navegador entra na conta), então a última linha do
              // painel — as ações do formulário — nascia abaixo da área
              // visível e, como o painel é `overflow-hidden`, não havia scroll
              // que a alcançasse. `dvh` usa a viewport visível.
              "fixed right-0 top-0 w-full h-[100dvh] shadow-2xl flex flex-col overflow-hidden transition-[max-width] duration-300 ease-in-out",
              sizeClass, resolvedBg, "dark:border-l dark:border-foreground/30", className,
            )}
          >
            <Dialog.Title className="sr-only">{ariaLabel}</Dialog.Title>
            {renderHeader
              ? renderHeader({ isExpanded, onToggleExpand: toggleExpand, onClose, expandButton, closeButton })
              : header ?? null}
            {/*
              O conteúdo vive num contêiner flexível de altura limitada. Sem
              ele, um filho com `h-full` mede 100% do PAINEL — não do espaço que
              sobra abaixo do header: header + 100% estoura a altura do painel,
              o excesso é recortado pelo `overflow-hidden` e o rodapé do filho
              (o botão principal do formulário) fica inalcançável no mobile. Era
              exatamente o caso do painel de Novo Lead, que por isso também
              passou a usar `flex-1 min-h-0` na raiz.
            */}
            <div className="flex min-h-0 flex-1 flex-col">{children}</div>
          </motion.div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

export function useSidePanelState(initialOpen = false) {
  const [isOpen, setIsOpen] = useState(initialOpen);
  return {
    isOpen,
    open: useCallback(() => setIsOpen(true), []),
    close: useCallback(() => setIsOpen(false), []),
    toggle: useCallback(() => setIsOpen((prev) => !prev), []),
  };
}
