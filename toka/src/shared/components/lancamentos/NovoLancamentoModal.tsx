import { ArrowDownCircle, ArrowUpCircle, Briefcase, DollarSign, Info } from "lucide-react";

import { Sheet, SheetContent, SheetDescription, SheetTitle } from "../../ui/sheet";

export type NovoLancamentoKind = "charge" | "commission" | "entrada" | "saida";

interface NovoLancamentoModalProps {
  onClose: () => void;
  onSelect: (kind: NovoLancamentoKind) => void;
}

const CHOICES: { kind: NovoLancamentoKind; label: string; description: string; icon: typeof DollarSign }[] = [
  { kind: "charge", label: "Cobrança", description: "Cobrança manual vinculada a um contrato.", icon: DollarSign },
  { kind: "commission", label: "Comissão", description: "Distribuição da comissão entre os participantes de um negócio.", icon: Briefcase },
  { kind: "entrada", label: "Entrada (caixa)", description: "Recebimento manual, com vínculo ao contrato quando aplicável.", icon: ArrowDownCircle },
  { kind: "saida", label: "Saída (caixa)", description: "Pagamento manual, operacional ou administrativo.", icon: ArrowUpCircle },
];

export function NovoLancamentoModal({ onClose, onSelect }: NovoLancamentoModalProps) {
  return (
    <Sheet open onOpenChange={(next) => { if (!next) onClose(); }}>
      <SheetContent side="right" className="flex h-full w-full flex-col gap-0 overflow-hidden p-0 sm:max-w-md">
        <div className="border-b border-border bg-muted/50 px-6 py-4 pr-12">
          <SheetTitle className="text-lg font-bold text-foreground">Novo Lançamento</SheetTitle>
          <SheetDescription className="text-xs">Escolha o tipo de lançamento que deseja criar.</SheetDescription>
        </div>

        <div className="flex-1 space-y-2 overflow-y-auto p-4">
          {CHOICES.map((c) => {
            const Icon = c.icon;
            return (
              <button
                key={c.kind}
                type="button"
                onClick={() => onSelect(c.kind)}
                className="w-full flex items-center gap-3 rounded-xl border border-border bg-card px-4 py-3 text-left transition-all hover:border-primary-500 hover:bg-primary-50/40 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
              >
                <span className="p-2 bg-primary-100 rounded-xl shrink-0">
                  <Icon className="w-5 h-5 text-primary-600" />
                </span>
                <span className="min-w-0">
                  <span className="block text-sm font-bold text-foreground">{c.label}</span>
                  <span className="block text-xs text-muted-foreground">{c.description}</span>
                </span>
              </button>
            );
          })}

          <div className="mt-4 flex gap-3 rounded-xl border border-primary-200 bg-primary-50/60 px-4 py-3 text-xs text-muted-foreground dark:border-primary-800 dark:bg-primary-900/40">
            <Info className="mt-0.5 h-4 w-4 shrink-0 text-primary-600 dark:text-primary-300" />
            <p className="leading-relaxed">
              <strong className="text-foreground">Repasses não são lançados manualmente.</strong>{" "}
              A obrigação ao proprietário nasce da baixa da cobrança ou do recebimento vinculado e segue para aprovação em Repasses.
            </p>
          </div>
        </div>
      </SheetContent>
    </Sheet>
  );
}
