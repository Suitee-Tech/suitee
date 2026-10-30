import { Dialog, DialogContent, DialogTitle } from "../ui/dialog";
import { useCallback, useRef } from "react";
import { Printer, X, Download } from "lucide-react";
import { Charge, chargeService } from "../services/charge.service";
import { formatMoneyBR, formatCompetencia, formatISODateBR, formatDateTimeBR as _formatDateTimeBR } from "../utils/format";
import { Button } from "../ui/button";
import { getChargeComposition } from "../utils/chargeComposition";
import { normalizeChargeStatus } from "../utils/charge";
import { getActiveCompany, getCompanyDisplayName } from "../utils/tenant";
import { useOptionalAuth } from "../context/AuthContext";
import { printElement } from "../utils/printDocument";
import { toast } from "sonner";

interface ChargeReceiptModalProps {
  charge: Charge;
  tenantName?: string; // razão social da imobiliária
  payerName?: string;
  payerDocument?: string;
  contractDisplayID?: string;
  onClose: () => void;
}

// Status label helpers — keeps the heavy mapping out of the JSX so the
// component stays readable. Each status gets a stamp-style label that
// looks like a fiscal-coupon footer ("PAGO", "EM ABERTO", "VENCIDO").
const STATUS_LABEL: Record<string, string> = {
  PENDING: "EM ABERTO",
  PAID: "PAGO",
  PARTIALLY_PAID: "PARCIALMENTE PAGO",
  OVERDUE: "VENCIDO",
  CANCELLED: "CANCELADO",
  REVERSED: "REVERSADO",
};

const STATUS_TONE: Record<string, string> = {
  PAID: "text-status-ok border-status-ok/40 bg-status-ok-soft",
  PENDING: "text-status-warning border-status-warning/40 bg-status-warning-soft",
  PARTIALLY_PAID: "text-status-warning border-status-warning/40 bg-status-warning-soft",
  OVERDUE: "text-status-critical border-status-critical/40 bg-status-critical-soft",
  CANCELLED: "text-muted-foreground border-border bg-muted/40",
  REVERSED: "text-muted-foreground border-border bg-muted/40",
};

const PAYMENT_METHOD_LABEL: Record<string, string> = {
  PIX: "PIX",
  CARD: "Cartão de Crédito",
  CREDIT_CARD: "Cartão de Crédito",
  BOLETO: "Boleto bancário",
};

const TYPE_LABEL: Record<string, string> = {
  RENT: "Aluguel",
  SALE: "Compra e Venda",
  CONDOMINIO: "Condomínio",
  IPTU: "IPTU",
  UTILITY: "Consumo",
  MAINTENANCE: "Manutenção",
  OTHER: "Outro",
};

function formatDateBR(iso?: string): string {
  if (!iso) return "—";
  return formatISODateBR(iso) || "—";
}

function formatDateTimeBR(iso?: string): string {
  if (!iso) return "—";
  return _formatDateTimeBR(iso) || "—";
}

// ChargeReceiptModal renders the charge as a thermal-printer-style fiscal
// coupon. UX choices:
//   - Monospaced typography (font-mono) evokes the printed receipt feel.
//   - Width capped at 88ch / 380px so the layout reads like a 80mm
//     thermal print, even on wide screens.
//   - Dashed horizontal rules separate logical sections (header /
//     items / totals / status) following the cupom-fiscal convention.
//   - Print CSS hides chrome and forces the receipt to print at native
//     thermal width.
//   - Action buttons stay outside the receipt itself so they disappear
//     in print mode (`@media print { .no-print { display: none; } }`).
//   - The shared Dialog traps focus on open and restores it on close;
//     the receipt body has role=document
//     so screen readers announce a coherent block.
export function ChargeReceiptModal({
  charge,
  tenantName,
  payerName,
  payerDocument,
  contractDisplayID,
  onClose,
}: ChargeReceiptModalProps) {
  const receiptRef = useRef<HTMLDivElement>(null);

  const opener = useRef<HTMLElement | null>(null);

  // O nome impresso é o da imobiliária logada (empresa ativa), nunca um mock.
  // `tenantName` continua aceito para callers que já resolveram o nome.
  const auth = useOptionalAuth();
  const activeCompany = getActiveCompany(auth?.user, auth?.companies);
  const agencyName = tenantName?.trim() || getCompanyDisplayName(activeCompany) || "Imobiliária";

  const printThermal = useCallback(() => {
    printElement(receiptRef.current, { extraCss: "@page { margin: 0; size: 80mm auto; }" });
  }, []);

  const handlePrint = useCallback(() => {
    printThermal();
  }, [printThermal]);

  // "Salvar como PDF": cobrança quitada usa o recibo OFICIAL gerado no backend
  // (arquivo real, com o encoding corrigido). Pendente ainda não tem recibo —
  // cai para a impressão térmica, de onde o usuário salva como PDF.
  const handleSavePdf = useCallback(async () => {
    if (!charge) return;
    if (normalizeChargeStatus(charge.status) !== "PAID") {
      printThermal();
      return;
    }
    try {
      const blob = await chargeService.downloadReceipt(charge.id);
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = `recibo-${charge.id}.pdf`;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
    } catch {
      toast.error("Não foi possível baixar o recibo em PDF; abrindo a impressão.");
      printThermal();
    }
  }, [charge, printThermal]);



  const { items: lineItems, rentalSplit } = getChargeComposition(charge);
  const hasLineItems = lineItems.length > 0;

  const statusLabel = STATUS_LABEL[charge.status] ?? charge.status;
  const statusClass = STATUS_TONE[charge.status] ?? "text-foreground border-border bg-card";
  const typeLabel = TYPE_LABEL[charge.type] ?? charge.type;
  const methodLabel = charge.payment_method
    ? PAYMENT_METHOD_LABEL[charge.payment_method] ?? charge.payment_method
    : null;

  return (
    <Dialog open onOpenChange={open => { if (!open) onClose(); }}>
      <DialogContent
        showCloseButton={false}
        aria-describedby={undefined}
        className="gap-0 border-0 p-0 sm:max-w-md print:top-0 print:left-0 print:w-full print:max-w-none print:translate-x-0 print:translate-y-0 print:shadow-none"
        onOpenAutoFocus={() => { opener.current = document.activeElement as HTMLElement | null; }}
        onCloseAutoFocus={event => {
          if (opener.current?.isConnected) { event.preventDefault(); opener.current.focus(); }
        }}
      >

      <div className="relative flex max-h-[95vh] w-full max-w-md flex-col overflow-hidden rounded-xl bg-card shadow-2xl print:max-h-none print:max-w-none print:rounded-none print:shadow-none">
        {/* Toolbar — hidden on print */}
        <div className="no-print flex items-center justify-between border-b border-border bg-muted/40 px-4 py-2.5 print:hidden">
          <div className="flex items-center gap-2">
            <DialogTitle className="text-sm font-bold text-foreground">
              Recibo de Cobrança
            </DialogTitle>
            <span
              className={`rounded-full border px-2 py-0.5 text-caption font-bold uppercase tracking-wider ${statusClass}`}
            >
              {statusLabel}
            </span>
          </div>
          <div className="flex items-center gap-1">
            <Button size="sm" variant="ghost" onClick={handlePrint} aria-label="Imprimir recibo">
              <Printer className="h-4 w-4" />
            </Button>
            <Button size="sm" variant="ghost" onClick={() => void handleSavePdf()} aria-label="Salvar como PDF">
              <Download className="h-4 w-4" />
            </Button>
            <Button size="sm" variant="ghost" onClick={onClose} aria-label="Fechar">
              <X className="h-4 w-4" />
            </Button>
          </div>
        </div>

        {/* Printable receipt body. font-mono mimics the dot-matrix
            output of a thermal printer. Padding wraps the text in a
            comfortable reading column. */}
        <div
          ref={receiptRef}
          role="document"
          className="receipt-printable flex-1 overflow-y-auto bg-white px-6 py-5 font-mono text-[13px] leading-relaxed text-zinc-900 print:overflow-visible print:px-2 print:py-3"
        >
          {/* Header */}
          <div className="text-center">
            <div className="text-base font-bold uppercase tracking-wider">{agencyName}</div>
            <div className="mt-0.5 text-[11px] text-zinc-500">
              CUPOM NÃO FISCAL — DEMONSTRATIVO DE COBRANÇA
            </div>
          </div>

          <DashedRule />

          {/* Identifiers */}
          <div className="space-y-0.5 text-[12px]">
            <KeyValue label="N° da cobrança" value={charge.id.slice(0, 8).toUpperCase()} />
            {contractDisplayID && <KeyValue label="Contrato" value={contractDisplayID} />}
            <KeyValue label="Tipo" value={typeLabel} />
            <KeyValue label="Referência" value={formatCompetencia(charge.reference_month)} />
            <KeyValue label="Emitido em" value={formatDateTimeBR(charge.created_at)} />
            <KeyValue label="Vencimento" value={formatDateBR(charge.due_date)} bold />
          </div>

          <DashedRule />

          {/* Payer */}
          {(payerName || payerDocument) && (
            <>
              <div className="text-[11px] font-bold uppercase tracking-wider text-zinc-500">
                Pagador
              </div>
              <div className="mt-1 space-y-0.5 text-[12px]">
                {payerName && <div>{payerName}</div>}
                {payerDocument && <div className="text-zinc-600">{payerDocument}</div>}
              </div>
              <DashedRule />
            </>
          )}

          {/* Items */}
          <div className="text-[11px] font-bold uppercase tracking-wider text-zinc-500">
            Discriminação
          </div>
          <div className="mt-2 space-y-1 text-[12px]">
            {hasLineItems ? (
              lineItems.map((item) => (
                <ItemLine
                  key={item.key}
                  label={item.description}
                  value={formatMoneyBR(item.amountCents / 100)}
                />
              ))
            ) : (
              <ItemLine label={charge.description || "Cobrança"} value={formatMoneyBR(charge.amount)} />
            )}
          </div>

          {rentalSplit && (
            <div className="mt-3 border-t border-dashed border-zinc-300 pt-2 text-[11px] text-zinc-600">
              <p className="mb-1">Administração deduzida do aluguel, incluída no valor acima:</p>
              <ItemLine label="Taxa de administração" value={formatMoneyBR(rentalSplit.administrationCents / 100)} />
              <ItemLine label="Aluguel líquido do proprietário" value={formatMoneyBR(rentalSplit.ownerRentCents / 100)} />
            </div>
          )}

          <DashedRule />

          {/* Totals */}
          <div className="space-y-0.5 text-[12px]">
            <KeyValue label="Subtotal" value={formatMoneyBR(charge.amount)} />
            {charge.discount > 0 && (
              <KeyValue label="Desconto" value={`- ${formatMoneyBR(charge.discount)}`} />
            )}
            {charge.late_fee > 0 && (
              <KeyValue label="Multa" value={formatMoneyBR(charge.late_fee)} />
            )}
            {charge.interest > 0 && (
              <KeyValue label="Juros" value={formatMoneyBR(charge.interest)} />
            )}
          </div>

          {/* Grand total */}
          <div className="mt-2 flex items-baseline justify-between border-y-2 border-double border-zinc-900 py-2">
            <span className="text-sm font-bold uppercase tracking-wider">Total</span>
            <span className="text-lg font-bold tabular-nums">
              {formatMoneyBR(charge.total_amount || charge.amount)}
            </span>
          </div>

          {/* Payment block (visible when paid) */}
          {charge.status === "PAID" && (
            <>
              <DashedRule />
              <div className="text-[11px] font-bold uppercase tracking-wider text-zinc-500">
                Pagamento
              </div>
              <div className="mt-1 space-y-0.5 text-[12px]">
                <KeyValue label="Pago em" value={formatDateTimeBR(charge.paid_at)} />
                {methodLabel && <KeyValue label="Método" value={methodLabel} />}
                {charge.paid_amount !== undefined && (
                  <KeyValue label="Valor pago" value={formatMoneyBR(charge.paid_amount)} />
                )}
              </div>
            </>
          )}

          {/* Status stamp — large, centered, visually mimics the "PAGO"
              red rubber stamp that bookkeepers used to apply. */}
          <div className="mt-4 flex justify-center">
            <div
              className={`rounded-md border-2 px-6 py-1.5 text-base font-bold uppercase tracking-[0.3em] ${statusClass}`}
              style={{ transform: "rotate(-3deg)" }}
              aria-hidden
            >
              {statusLabel}
            </div>
          </div>

          {/* Footer */}
          <DashedRule />
          <div className="text-center text-[11px] text-zinc-500">
            Documento de uso interno, sem valor fiscal.
            <br />
            Emitido por {agencyName} em {formatDateTimeBR(new Date().toISOString())}.
          </div>
        </div>

        {/* Action bar — hidden on print */}
        <div className="no-print flex items-center justify-between border-t border-border bg-muted/30 px-4 py-3 print:hidden">
          <span className="text-xs text-muted-foreground">
            Use Ctrl/⌘ + P para imprimir ou salvar como PDF.
          </span>
          <div className="flex gap-2">
            <Button variant="ghost" size="sm" onClick={onClose}>
              Fechar
            </Button>
            <Button size="sm" onClick={handlePrint}>
              <Printer className="mr-1 h-4 w-4" />
              Imprimir / PDF
            </Button>
          </div>
        </div>
      </div>
      </DialogContent>
    </Dialog>
  );
}

// Small presentational helpers — kept inside the file because they're
// only meaningful in the receipt context (alignment, leader dots).

function DashedRule() {
  return (
    <div
      aria-hidden
      className="my-2 border-t border-dashed border-zinc-400"
    />
  );
}

function KeyValue({
  label,
  value,
  bold,
}: {
  label: string;
  value: string;
  bold?: boolean;
}) {
  return (
    <div className={`flex items-baseline justify-between ${bold ? "font-bold" : ""}`}>
      <span className="text-zinc-600">{label}</span>
      <span className="tabular-nums">{value}</span>
    </div>
  );
}

function ItemLine({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline justify-between gap-2">
      <span className="flex-1 truncate text-zinc-800">{label}</span>
      <span className="tabular-nums text-zinc-800">{value}</span>
    </div>
  );
}
