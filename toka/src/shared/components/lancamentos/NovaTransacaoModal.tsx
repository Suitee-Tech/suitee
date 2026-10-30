import { useMemo, useState, useEffect } from "react";
import { ArrowDownCircle, ArrowUpCircle } from "lucide-react";
import { toast } from "sonner";

import type { Contract } from "../../contracts/types";
import type { Lead, Property } from "../../data";
import type { Agent } from "../../types/agent";
import { Sheet, SheetContent, SheetDescription, SheetTitle } from "../../ui/sheet";
import { Button } from "../../ui/button";
import { Input } from "../../ui/input";
import { MoneyInput } from "../../ui/money-input";
import { SearchableSelect } from "../../ui/searchable-select";
import { transactionService, type PayerKind } from "../../services/transaction.service";

export type NovaTransacaoDirection = "entrada" | "saida";

interface NovaTransacaoModalProps {
  direction: NovaTransacaoDirection;
  onClose: () => void;
  onCreated: () => void;
  contracts?: Contract[];
  properties?: Property[];
  leads?: Lead[];
  agents?: Agent[];
}

const PAYER_KINDS: { value: PayerKind; label: string }[] = [
  { value: "tenant", label: "Inquilino" },
  { value: "owner", label: "Proprietário" },
  { value: "broker", label: "Corretor" },
  { value: "partner", label: "Parceiro" },
  { value: "external", label: "Externo" },
];

const MANUAL_OPTION = "__manual__";
const NO_LINK_OPTION = "__no_link__";

const CATEGORY_OPTIONS: Record<NovaTransacaoDirection, { value: string; label: string }[]> = {
  entrada: [
    { value: "Receita de aluguel", label: "Receita de aluguel" },
    { value: "Receita de venda", label: "Receita de venda" },
    { value: "Comissão recebida", label: "Comissão recebida" },
    { value: "Reembolso recebido", label: "Reembolso recebido" },
  ],
  saida: [
    { value: "Comissão", label: "Comissão" },
    { value: "Condomínio", label: "Condomínio" },
    { value: "IPTU", label: "IPTU" },
    { value: "Manutenção", label: "Manutenção / reparo" },
    { value: "Taxa bancária", label: "Taxa bancária" },
  ],
};

const selectClassName =
  "h-10 w-full rounded-xl border border-border bg-card px-3 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500/20";

function todayISO(): string {
  return new Date().toISOString().slice(0, 10);
}

export function NovaTransacaoModal({
  direction,
  onClose,
  onCreated,
  contracts = [],
  properties = [],
  leads = [],
  agents = [],
}: NovaTransacaoModalProps) {
  const isEntrada = direction === "entrada";
  const descriptionId = `nova-transacao-${direction}-descrição`;
  const valueId = `nova-transacao-${direction}-valor`;
  const dateId = `nova-transacao-${direction}-competência`;
  const dueDateId = `nova-transacao-${direction}-vencimento`;
  const payerKindId = `nova-transacao-${direction}-papel`;
  const payerNameId = `nova-transacao-${direction}-contraparte`;
  const [description, setDescription] = useState("");
  const [value, setValue] = useState<number | undefined>(undefined);
  const [date, setDate] = useState<string>(todayISO());
  const [dueDate, setDueDate] = useState<string>("");
  const [category, setCategory] = useState("");
  const [categoryChoice, setCategoryChoice] = useState("");
  const [contractId, setContractId] = useState("");
  const [entityRef, setEntityRef] = useState("");
  const [payerName, setPayerName] = useState("");
  const [payerKind, setPayerKind] = useState<PayerKind | "">("");
  const [submitting, setSubmitting] = useState(false);

  const selectedContract = useMemo(
    () => contracts.find((contract) => contract.id === contractId),
    [contractId, contracts],
  );
  const selectedProperty = useMemo(
    () => (selectedContract?.property_id ? properties.find((property) => property.id === selectedContract.property_id) : undefined),
    [properties, selectedContract?.property_id],
  );
  const contractOptions = useMemo(
    () =>
      contracts.map((contract) => ({
        value: contract.id,
        label: `${contract.property_title || "Imóvel sem título"} — ${contract.client || "Sem cliente"}`,
        searchText: `${contract.property_title || ""} ${contract.client || ""} ${contract.id}`,
      })),
    [contracts],
  );
  const entityOptions = useMemo(() => {
    const options: { value: string; label: string; kind: PayerKind; name: string }[] = [];
    const customerRef = selectedContract?.customer_id || selectedContract?.client;
    if (customerRef) {
      const customerName = selectedContract?.client || `Cliente · ${customerRef.slice(0, 8)}`;
      options.push({
        value: `tenant:${customerRef}`,
        label: `Cliente / inquilino · ${customerName}`,
        kind: "tenant",
        name: customerName,
      });
    }

    const ownerIds = Array.from(
      new Set([...(selectedProperty?.owners?.map((owner) => owner.id) ?? []), selectedProperty?.ownerId].filter(Boolean)),
    ) as string[];
    ownerIds.forEach((ownerId) => {
      const owner = leads.find((lead) => lead.id === ownerId);
      const ownerName = owner?.name || `Proprietário · ${ownerId.slice(0, 8)}`;
      options.push({ value: `owner:${ownerId}`, label: ownerName, kind: "owner", name: ownerName });
    });

    agents
      .filter((agent) => selectedContract && agent.id === (selectedContract as Contract & { broker_id?: string }).broker_id)
      .forEach((agent) => {
        options.push({ value: `broker:${agent.id}`, label: `Corretor · ${agent.name}`, kind: "broker", name: agent.name });
      });

    options.push({ value: MANUAL_OPTION, label: "Outra entidade / sem vínculo (exceção)", kind: "external", name: "" });
    return options;
  }, [agents, leads, selectedContract, selectedProperty]);

  useEffect(() => {
    if (!contractId) {
      setEntityRef(MANUAL_OPTION);
      setPayerName("");
      setPayerKind("");
      return;
    }
    const preferred = entityOptions.find((option) => option.kind === "tenant") ?? entityOptions.find((option) => option.kind === "owner");
    if (!preferred) {
      setEntityRef(MANUAL_OPTION);
      setPayerName("");
      setPayerKind("external");
      return;
    }
    setEntityRef(preferred.value);
    setPayerName(preferred.name);
    setPayerKind(preferred.kind);
  }, [contractId, entityOptions, selectedContract?.client]);

  const handleEntityChange = (nextRef: string) => {
    setEntityRef(nextRef);
    const selected = entityOptions.find((option) => option.value === nextRef);
    if (!selected || nextRef === MANUAL_OPTION) {
      setPayerName("");
      setPayerKind("external");
      return;
    }
    setPayerName(selected.name);
    setPayerKind(selected.kind);
  };

  const canSubmit = description.trim().length > 0 && (value ?? 0) > 0 && category.trim().length > 0 && !submitting;

  const handleSubmit = async () => {
    if (!canSubmit) return;
    setSubmitting(true);
    try {
      await transactionService.create({
        contract_id: contractId,
        type: isEntrada ? "receita" : "despesa",
        description: description.trim(),
        value: value ?? 0,
        date: new Date(`${date}T00:00:00`).toISOString(),
        status: "Pendente",
        due_date: dueDate ? new Date(`${dueDate}T00:00:00`).toISOString() : undefined,
        category: category.trim() || undefined,
        payer_name: payerName.trim() || undefined,
        payer_kind: payerKind || undefined,
      });
      toast.success(isEntrada ? "Entrada registrada." : "Saída registrada.");
      onCreated();
      onClose();
    } catch (err) {
      console.error(err);
      toast.error("Não foi possível registrar o lançamento.");
    } finally {
      setSubmitting(false);
    }
  };

  const Icon = isEntrada ? ArrowDownCircle : ArrowUpCircle;
  const accent = isEntrada ? "text-status-ok" : "text-status-critical";
  const contractSelectValue = contractId || NO_LINK_OPTION;
  const categorySelectValue = categoryChoice || NO_LINK_OPTION;

  return (
    <Sheet open onOpenChange={(next) => { if (!next) onClose(); }}>
      <SheetContent
        side="right"
        expandable
        expandedWidth="sm:max-w-3xl lg:max-w-5xl"
        className="flex h-full w-full flex-col gap-0 overflow-hidden p-0 sm:max-w-md"
      >
        <div className="border-b border-border bg-muted/50 px-6 py-4 pr-12">
          <SheetTitle className="flex items-center gap-2 text-lg font-bold text-foreground">
            <Icon className={`h-5 w-5 ${accent}`} />
            {isEntrada ? "Nova Entrada" : "Nova Saída"}
          </SheetTitle>
          <SheetDescription className="text-xs">
            Registre a movimentação e relacione-a ao contrato quando houver uma origem conhecida.
          </SheetDescription>
        </div>

        <div className="flex-1 space-y-3 overflow-y-auto p-4">
          <div>
            <label htmlFor={descriptionId} className="mb-1 block text-xs font-bold text-foreground">Descrição *</label>
            <Input
              id={descriptionId}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder={isEntrada ? "Ex.: Recebimento do aluguel de agosto" : "Ex.: Pagamento do IPTU do imóvel"}
            />
          </div>

          <div className="grid grid-cols-1 items-start gap-3 sm:grid-cols-2">
            <div className="min-h-[76px]">
              <label htmlFor={valueId} className="mb-1 block text-xs font-bold text-foreground">Valor *</label>
              <MoneyInput id={valueId} value={value} onChange={setValue} withSymbol className={selectClassName} />
            </div>
            <div className="min-h-[76px]">
              <label htmlFor={dateId} className="mb-1 block text-xs font-bold text-foreground">Data do fato gerador *</label>
              <Input id={dateId} type="date" value={date} onChange={(e) => setDate(e.target.value)} />
              <p className="mt-1 text-[11px] text-muted-foreground">Quando a receita ou despesa passou a existir.</p>
            </div>
          </div>

          <div className="rounded-xl border border-border bg-muted/30 p-3 space-y-3">
            <div>
              <p className="text-sm font-bold text-foreground">Relacionamento financeiro</p>
              <p className="text-xs text-muted-foreground">
                O vínculo organiza o lançamento por contrato e evita atribuir a outra entidade por engano.
              </p>
            </div>
            <div className="min-h-[82px]">
              <label className="mb-1 block text-xs font-bold text-foreground">Contrato</label>
              <SearchableSelect
                value={contractSelectValue}
                onValueChange={(next) => setContractId(next === NO_LINK_OPTION ? "" : next)}
                options={[
                  { value: NO_LINK_OPTION, label: "Sem vínculo — lançamento avulso (exceção)" },
                  ...contractOptions,
                ]}
                placeholder="Selecione um contrato..."
                searchPlaceholder="Buscar contrato, cliente ou imóvel..."
                emptyMessage="Nenhum contrato encontrado."
                aria-label="Contrato"
                className={selectClassName}
              />
              <p className="mt-1 text-xs text-muted-foreground">
                {selectedContract ? "Cliente, proprietário e corretor serão sugeridos a partir deste contrato." : "Use sem contrato apenas para receitas ou despesas administrativas."}
              </p>
            </div>

            <div className="grid grid-cols-1 items-start gap-3 sm:grid-cols-2">
              <div className="min-h-[76px]">
                <label className="mb-1 block text-xs font-bold text-foreground">Entidade relacionada</label>
                <SearchableSelect
                  value={entityRef || MANUAL_OPTION}
                  onValueChange={handleEntityChange}
                  options={entityOptions.map((option) => ({
                    value: option.value,
                    label: option.label,
                    searchText: `${option.label} ${option.name}`,
                  }))}
                  placeholder="Selecione uma entidade..."
                  searchPlaceholder="Buscar entidade..."
                  emptyMessage="Nenhuma entidade encontrada."
                  aria-label="Entidade relacionada"
                  className={selectClassName}
                />
              </div>
              <div className="min-h-[76px]">
                <label htmlFor={payerKindId} className="mb-1 block text-xs font-bold text-foreground">Papel da contraparte</label>
                <select
                  id={payerKindId}
                  value={payerKind}
                  onChange={(event) => setPayerKind(event.target.value as PayerKind | "")}
                  disabled={!!contractId && entityRef !== MANUAL_OPTION}
                  className={`${selectClassName} disabled:cursor-not-allowed disabled:bg-muted`}
                >
                  <option value="">—</option>
                  {PAYER_KINDS.map((payer) => (
                    <option key={payer.value} value={payer.value}>{payer.label}</option>
                  ))}
                </select>
              </div>
            </div>

            {(!contractId || entityRef === MANUAL_OPTION) && (
              <div>
                <label htmlFor={payerNameId} className="mb-1 block text-xs font-bold text-foreground">{isEntrada ? "Pagador" : "Recebedor / fornecedor"}</label>
                <Input id={payerNameId} value={payerName} onChange={(event) => setPayerName(event.target.value)} placeholder="Nome da entidade externa" />
              </div>
            )}
          </div>

          <div className="grid grid-cols-1 items-start gap-3 sm:grid-cols-2">
            <div className="min-h-[76px]">
              <label htmlFor={dueDateId} className="mb-1 block text-xs font-bold text-foreground">Vencimento</label>
              <Input id={dueDateId} type="date" value={dueDate} onChange={(e) => setDueDate(e.target.value)} />
              <p className="mt-1 text-[11px] text-muted-foreground">Prazo esperado para receber ou pagar.</p>
            </div>
            <div className="min-h-[76px]">
              <label className="mb-1 block text-xs font-bold text-foreground">Tipo de conta / categoria *</label>
              <SearchableSelect
                value={categorySelectValue}
                onValueChange={(next) => {
                  const choice = next === NO_LINK_OPTION ? "" : next;
                  setCategoryChoice(choice);
                  setCategory(choice === MANUAL_OPTION ? "" : choice);
                }}
                options={[
                  { value: NO_LINK_OPTION, label: "Selecione…" },
                  ...CATEGORY_OPTIONS[direction],
                  { value: MANUAL_OPTION, label: "Outra categoria (exceção)" },
                ]}
                placeholder="Selecione uma categoria..."
                searchPlaceholder="Buscar tipo de conta..."
                emptyMessage="Nenhuma categoria encontrada."
                aria-label="Tipo de conta / categoria"
                className={selectClassName}
              />
              {!category && <p className="mt-1 text-[11px] text-muted-foreground">Obrigatória para classificação financeira e relatórios.</p>}
            </div>
          </div>

          {categoryChoice === MANUAL_OPTION && (
            <div>
              <label className="mb-1 block text-xs font-bold text-foreground">Categoria personalizada</label>
              <Input value={category} onChange={(event) => setCategory(event.target.value)} placeholder="Ex.: Despesa fixa" />
            </div>
          )}
        </div>

        <div className="mt-auto flex items-center justify-end gap-2 border-t border-border bg-muted/30 px-6 py-4">
          <Button variant="ghost" onClick={onClose}>Cancelar</Button>
          <Button onClick={handleSubmit} disabled={!canSubmit}>
            {submitting ? "Salvando…" : isEntrada ? "Registrar entrada" : "Registrar saída"}
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  );
}
