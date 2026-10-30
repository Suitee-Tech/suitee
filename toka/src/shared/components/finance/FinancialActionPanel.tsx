import type { ReactNode } from "react";
import { AlertCircle, Info } from "lucide-react";

import { ExpandableSidePanel } from "../ExpandableSidePanel";
import { Button } from "../../ui/button";
import { cn } from "../../ui/utils";

type PanelSize = "sm" | "md" | "lg" | "xl" | "2xl" | "3xl" | "4xl" | "full";

/**
 * FinancialActionPanel é o shell ÚNICO das ações financeiras do painel da
 * imobiliária: baixa de cobrança, baixa de ordem de pagamento, repasse,
 * comissão e lançamentos.
 *
 * Por que existe: cada fluxo tinha um shell próprio (Dialog com `sm:max-w-md`,
 * Sheet, às vezes AlertDialog no meio), então o mesmo gesto — anexar
 * comprovante e confirmar um valor — mudava de forma, de tamanho e de
 * comportamento entre telas. Aqui o padrão é um só:
 *
 *   - painel lateral direito (`ExpandableSidePanel`), com expandir/recolher e
 *     tela cheia no celular;
 *   - cabeçalho com ícone do contexto, título e subtítulo (o "o quê" e o
 *     "de quem"), que quebra em duas linhas no mobile em vez de espremer;
 *   - corpo rolável com seções nomeadas (`PanelSection`);
 *   - rodapé FIXO com Cancelar + ação primária, sempre alcançável no celular
 *     (safe-area do iPhone incluída) e protegido contra envio duplo;
 *   - saída única governada pelo envio: enquanto `submitting`, Cancelar, X,
 *     Escape e clique fora ficam inertes (FIN-06), sem cada fluxo repetir a
 *     regra;
 *   - erro da operação no mesmo lugar, com `role="alert"`, em todos os fluxos.
 *
 * AlertDialog/ConfirmDialog ficam reservados a CONFIRMAÇÃO DE AÇÃO CRÍTICA
 * (cancelar ordem, estornar, excluir) — nunca para preencher dados, que é o
 * papel deste painel.
 */
export interface FinancialActionPanelProps {
  onClose: () => void;
  title: string;
  subtitle?: ReactNode;
  /** Ícone do contexto (ex.: CheckCircle2 para baixa, Wallet para repasse). */
  icon?: ReactNode;
  /** Classes do "chip" do ícone — use os tokens de status, não cores cruas. */
  iconClassName?: string;
  /** Largura padrão do painel; `2xl` comporta formulário + resumo de efeitos. */
  size?: PanelSize;
  /**
   * Camada do painel. O padrão (`60`) fica acima das superfícies que abrem a
   * ação — `ExpandableSidePanel` e `Sheet` usam `50` —, porque o fluxo é sempre
   * disparado de dentro de uma lista ou drawer que continua aberto atrás.
   */
  zIndex?: number;
  /** id do `<form>` do corpo: permite o submit viver no rodapé fixo. */
  formId?: string;
  submitLabel: string;
  submitIcon?: ReactNode;
  submitVariant?: "default" | "success";
  submitting?: boolean;
  submittingLabel?: string;
  disableSubmit?: boolean;
  cancelLabel?: string;
  /** Erro da última tentativa (upload ou gravação). */
  error?: string | null;
  /** Ação crítica opcional exposta no rodapé (ex.: cancelar ordem). */
  secondaryAction?: ReactNode;
  /**
   * Quando a ação primária já é "Fechar", o botão genérico de cancelar vira
   * uma segunda saída idêntica. O fluxo pode escondê-lo para manter uma única
   * ação de fechamento.
   */
  showCancel?: boolean;
  ariaLabel?: string;
  children: ReactNode;
}

export function FinancialActionPanel({
  onClose,
  title,
  subtitle,
  icon,
  iconClassName,
  size = "2xl",
  zIndex = 60,
  formId,
  submitLabel,
  submitIcon,
  submitVariant = "success",
  submitting = false,
  submittingLabel,
  disableSubmit = false,
  cancelLabel = "Cancelar",
  error,
  secondaryAction,
  showCancel = true,
  ariaLabel,
  children,
}: FinancialActionPanelProps) {
  // FIN-06 em um só lugar: fechar durante o envio descartaria o comprovante já
  // enviado e deixaria a gravação órfã. Enquanto `submitting`, X, Escape e
  // clique fora são inertes em TODOS os fluxos — nenhum deles precisa repetir
  // essa regra.
  const requestClose = () => {
    if (!submitting) onClose();
  };

  return (
    <ExpandableSidePanel
      open
      onClose={requestClose}
      defaultSize={size}
      expandedSize="full"
      zIndex={zIndex}
      bgClass="bg-card"
      ariaLabel={ariaLabel ?? title}
      renderHeader={({ expandButton, closeButton }) => (
        // Cabeçalho padrão do painel financeiro: ícone + título/subtítulo e os
        // controles em linha própria no mobile (o painel ocupa a tela toda, e o
        // botão de fechar é a única saída — ver contrato em ExpandableSidePanel).
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-border bg-card px-4 py-4 sm:px-6">
          {icon && (
            <div className={cn("rounded-xl p-2", iconClassName ?? "bg-primary-50 text-primary-700")}>{icon}</div>
          )}
          <div className="min-w-0 flex-1">
            <h2 className="truncate text-base font-bold text-foreground sm:text-lg">{title}</h2>
            {subtitle && <div className="mt-0.5 truncate text-xs text-muted-foreground">{subtitle}</div>}
          </div>
          <div className="ml-auto flex w-full items-center justify-end gap-2 sm:w-auto">
            {expandButton}
            {closeButton}
          </div>
        </div>
      )}
    >
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
        <div className="min-h-0 flex-1 space-y-5 overflow-y-auto p-4 sm:p-6">
          {children}
          {error && <FinancialActionError message={error} />}
        </div>

        <div className="flex shrink-0 flex-wrap items-center justify-end gap-2 border-t border-border bg-card px-4 py-4 pb-[max(1rem,env(safe-area-inset-bottom))] sm:gap-3 sm:px-6">
          {secondaryAction}
          {showCancel && (
            <Button type="button" variant="outline" onClick={onClose} disabled={submitting}>
              {cancelLabel}
            </Button>
          )}
          <Button
            type={formId ? "submit" : "button"}
            form={formId}
            variant={submitVariant}
            loading={submitting}
            loadingText={submittingLabel ?? "Salvando…"}
            disabled={disableSubmit || submitting}
          >
            {submitIcon}
            {submitLabel}
          </Button>
        </div>
      </div>
    </ExpandableSidePanel>
  );
}

/**
 * PanelSection agrupa campos com o mesmo título/descrição em TODOS os fluxos, no
 * lugar de cada tela inventar seu espaçamento e hierarquia de texto.
 */
export function PanelSection({
  title,
  description,
  children,
  className,
}: {
  title: string;
  description?: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={cn("space-y-3", className)}>
      <div>
        <h3 className="text-sm font-bold text-foreground">{title}</h3>
        {description && <p className="mt-0.5 text-xs text-muted-foreground">{description}</p>}
      </div>
      {children}
    </section>
  );
}

/**
 * Bloco de erro padrão das ações financeiras: sempre `role="alert"`, sempre com
 * o mesmo par título + orientação, para o operador reconhecer o estado em
 * qualquer fluxo.
 */
export function FinancialActionError({
  message,
  title = "Não foi possível concluir",
}: {
  message: string;
  title?: string;
}) {
  return (
    <div
      role="alert"
      className="flex items-start gap-2 rounded-xl border border-status-critical/30 bg-status-critical-soft p-3 text-xs text-status-critical"
    >
      <AlertCircle className="mt-0.5 size-4 shrink-0" />
      <div className="min-w-0">
        <p className="font-bold">{title}</p>
        <p className="mt-1 leading-relaxed">{message}</p>
      </div>
    </div>
  );
}

/**
 * Aviso neutro para informação que não é erro (política do tenant, efeito da
 * baixa). Centraliza o tom para não haver três amarelos diferentes.
 */
export function FinancialActionNotice({ children }: { children: ReactNode }) {
  return (
    <div className="flex items-start gap-2 rounded-xl border border-status-warning/30 bg-status-warning-soft p-3 text-xs text-status-warning">
      <Info className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
      <div className="min-w-0 leading-relaxed">{children}</div>
    </div>
  );
}
