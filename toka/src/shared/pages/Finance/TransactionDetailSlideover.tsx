import { clsx } from "clsx";
import { AlertTriangle, Calendar, DollarSign, FileText, Maximize2, Minimize2 } from "lucide-react";

import { Button } from "../../ui/button";
import { Sheet, SheetContent, SheetDescription, SheetTitle } from "../../ui/sheet";
import { formatCurrency, StatusBadge } from "./presentation";
import { normalizeStatus } from "../../lib/lancamentos/status";
import { FinancialPartyTimeline } from "../../components/finance/FinancialPartyTimeline";

/**
 * Slide-over de detalhe de uma transação/cobrança selecionada no Financeiro.
 * Extraído do `Finance` (extração pura) para reuso entre Finance e as áreas
 * extraídas (Repasses/Recebimentos). O dono decide o que fazer em
 * `onRegisterPayment` (ex.: abrir MarkPaid para charge, ou confirmar + baixar).
 */
interface TransactionDetailSlideoverProps {
  transaction: any;
  isExpanded: boolean;
  onToggleExpand: () => void;
  onClose: () => void;
  onRegisterPayment: (transaction: any) => void;
  title?: string;
  actionLabel?: string;
  canRegisterPayment?: boolean;
  operationalNotice?: string;
}

export function TransactionDetailSlideover({
  transaction,
  isExpanded,
  onToggleExpand,
  onClose,
  onRegisterPayment,
  title = "Detalhes da movimentação",
  actionLabel = "Registrar pagamento",
  canRegisterPayment,
  operationalNotice,
}: TransactionDetailSlideoverProps) {
  const canonicalStatus = transaction ? normalizeStatus("transaction", transaction.status) : null;
  const inferredCanRegister = Boolean(
    transaction &&
    transaction._includedInTotals !== false &&
    (canonicalStatus === "pendente" || canonicalStatus === "vencido" || canonicalStatus === "parcial"),
  );
  const showRegisterPayment = canRegisterPayment ?? inferredCanRegister;
  const beneficiaryLabel = transaction?.beneficiaryLabel
    ?? transaction?._beneficiaryLabel
    ?? transaction?.beneficiary
    ?? transaction?.prop
    ?? transaction?.cliente
    ?? transaction?.counterparty?.label;
  const beneficiaryKind = transaction?.beneficiaryKind
    ?? transaction?._beneficiaryKind
    ?? (transaction?.type === "repasse" ? "Proprietário" : undefined);
  const expectedDueAt = transaction?.expectedDueAt
    ?? transaction?._expectedDueAt
    ?? transaction?.due_at
    ?? transaction?.due_date;
  const settledAt = transaction?.settledAt
    ?? transaction?._settledAt
    ?? transaction?.settled_at
    ?? transaction?.processed_at
    ?? transaction?.payment_date;

  return (
    // Primitiva compartilhada (Radix Dialog via Sheet): papel de diálogo,
    // título associado, portal, contenção de foco, Escape e retorno do foco ao
    // gatilho. O overlay artesanal com motion.div não tinha nenhum disso — nem
    // Escape, então o teclado não conseguia fechar o painel.
    //
    // A expansão continua controlada pelo pai (`isExpanded`/`onToggleExpand`):
    // quatro telas já mantêm esse estado, e movê-lo para dentro do painel
    // mudaria a API sem ganho.
    <Sheet open={Boolean(transaction)} onOpenChange={(next) => { if (!next) onClose(); }}>
      <SheetContent
        side="right"
        className={clsx(
          "w-full gap-0 overflow-hidden p-0 transition-[max-width] duration-300 ease-in-out",
          isExpanded ? "max-w-full" : "sm:max-w-lg",
        )}
        data-testid="transaction-detail-slideover"
      >
        {transaction && (
          <>
            <div className="px-6 py-4 border-b border-border flex items-start justify-between pr-20">
              <div>
                <SheetTitle className="text-xl font-bold text-foreground flex items-center gap-2">
                  <FileText className="w-5 h-5 text-primary-600" />
                  {title}
                </SheetTitle>
                <SheetDescription className="text-xs text-muted-foreground mt-1">
                  Código: {transaction.id}
                </SheetDescription>
              </div>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={onToggleExpand}
                  className={clsx(
                    isExpanded && "text-primary-600 bg-primary-50 border-primary-200 hover:bg-primary-100 hover:text-primary-600",
                  )}
                  aria-expanded={isExpanded}
                  aria-label={isExpanded ? "Recolher painel" : "Expandir painel"}
                >
                  {isExpanded ? (
                    <Minimize2 className="w-4 h-4" aria-hidden="true" />
                  ) : (
                    <Maximize2 className="w-4 h-4" aria-hidden="true" />
                  )}
                  <span className="hidden sm:inline">{isExpanded ? "Recolher" : "Expandir"}</span>
                </Button>
              </div>
            </div>

            <div className="flex-1 overflow-y-auto p-6 space-y-8">
              <div className="flex justify-end">
                <StatusBadge status={transaction.status} />
              </div>

              {operationalNotice && (
                <div role="status" className="flex gap-3 rounded-xl border border-status-warning/30 bg-status-warning-soft p-4 text-sm text-status-warning">
                  <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
                  <p className="leading-relaxed">{operationalNotice}</p>
                </div>
              )}

              <FinancialPartyTimeline
                beneficiaryLabel={beneficiaryLabel}
                beneficiaryKind={beneficiaryKind}
                expectedDueAt={expectedDueAt}
                settledAt={settledAt}
              />

              <div className="grid grid-cols-2 gap-6">
                <div>
                  <p className="text-xs font-bold text-muted-foreground mb-1">Tipo de Cobrança / Transação</p>
                  <div className="flex items-center gap-2 font-bold text-foreground text-sm">
                    <FileText className="w-4 h-4 text-primary-600" /> {transaction.categoria || (transaction.type === "despesa" ? "Despesa" : transaction.type === "repasse" ? "Repasse" : transaction.type === "comissao" ? "Comissão" : "Receita")}
                  </div>
                </div>
                <div>
                  <p className="text-xs font-bold text-muted-foreground mb-1">Contrato</p>
                  <div className="flex items-center gap-2 font-bold text-foreground text-sm">
                    <FileText className="w-4 h-4 text-primary-600" /> {transaction.contractId || transaction.imovel || "Avulso"}
                  </div>
                </div>
                <div>
                  <p className="text-xs font-bold text-muted-foreground mb-1">Data</p>
                  <div className="flex items-center gap-2 font-bold text-foreground text-sm">
                    <Calendar className="w-4 h-4" /> {transaction.data || transaction.date || transaction.periodo}
                  </div>
                </div>
              </div>

              <div className="bg-muted rounded-xl p-5 border border-border space-y-4">
                <div className="flex justify-between items-center text-sm">
                  <span className="text-muted-foreground">Valor Original</span>
                  <span className="font-bold text-foreground">{formatCurrency(transaction.valor ?? transaction.value ?? transaction.aluguel)}</span>
                </div>
                <div className="h-px bg-muted w-full" />
                <div className="flex justify-between items-center">
                  <span className="font-bold text-foreground text-base">Valor Total</span>
                  <span className="font-bold text-primary-600 text-xl">{formatCurrency(transaction.liquido ?? transaction.valor ?? transaction.value ?? transaction.aluguel)}</span>
                </div>
              </div>

              <div>
                <h3 className="text-sm font-bold text-foreground mb-2">Descrição</h3>
                <p className="text-sm text-muted-foreground leading-relaxed">
                  {transaction.desc || transaction.description || (transaction.prop ? `Repasse para ${transaction.prop}` : "")}
                </p>
              </div>
            </div>

            <div className="px-6 py-4 border-t border-border bg-muted flex justify-end gap-3">
              <Button variant="outline" onClick={onClose}>
                Fechar
              </Button>
              {showRegisterPayment && (
                <Button onClick={() => onRegisterPayment(transaction)}>
                  <DollarSign className="w-4 h-4" /> {actionLabel}
                </Button>
              )}
            </div>
          </>
        )}
      </SheetContent>
    </Sheet>
  );
}
