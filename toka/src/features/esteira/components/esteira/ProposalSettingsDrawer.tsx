import { useEffect, useMemo, useState } from "react";
import { AlertTriangle, Settings2, FileSignature, Loader2, UserCheck } from "lucide-react";
import { clsx } from "clsx";
import { ExpandableSidePanel } from "../../../../shared/components/ExpandableSidePanel";
import { useDealsContext } from "../../../../shared/context/DealsContext";
import { useRequestScopeGuard } from "../../../../shared/hooks/useRequestScopeGuard";
import type { DealDTO, PartyDTO, Signer } from "../../../../shared/services/deal.service";
import { notifyApiError } from "../../../../shared/services/errorToast";
import { toast } from "sonner";

interface ProposalSettingsDrawerProps {
 open: boolean;
 onClose: () => void;
 deal: DealDTO;
}

const partyRoleLabel: Record<string, string> = {
 buyer: "Comprador",
 tenant: "Inquilino",
 owner: "Proprietário",
 spouse: "Cônjuge",
 guarantor: "Fiador",
 witness: "Testemunha",
 representative: "Representante",
 attorney: "Procurador",
};

const signerRoleLabel: Record<string, string> = {
 comprador: "Comprador",
 vendedor: "Vendedor",
 inquilino: "Inquilino",
 locatario: "Locatário",
 locador: "Locador",
 proprietario: "Proprietário",
 fiador: "Fiador",
 avalista: "Avalista",
 testemunha: "Testemunha",
 conjuge: "Cônjuge",
 procurador: "Procurador",
 representante_legal: "Representante legal",
 tutor: "Tutor",
 curador: "Curador",
};

function isPartyRequired(p: PartyDTO): boolean {
 return p.signature_required ?? p.is_primary;
}

function isSignerRequired(s: Signer): boolean {
 return s.is_required ?? true;
}

type RequirementOverrides = Record<string, boolean>;
type PendingRequirements = Record<string, true>;

function withoutRequirement<T extends RequirementOverrides | PendingRequirements>(
 current: T,
 id: string,
): T {
 if (!(id in current)) return current;
 const next = { ...current };
 delete next[id];
 return next;
}

export function ProposalSettingsDrawer({ open, onClose, deal }: ProposalSettingsDrawerProps) {
 const { updateParty, updateSigner } = useDealsContext();
 const beginScopedRequest = useRequestScopeGuard(deal.id);
 const [savingPartyIds, setSavingPartyIds] = useState<PendingRequirements>({});
 const [savingSignerIds, setSavingSignerIds] = useState<PendingRequirements>({});

 const parties = deal.parties ?? [];
 const signers = deal.signers ?? [];

 useEffect(() => {
  setSavingPartyIds({});
  setSavingSignerIds({});
 }, [deal.id]);

 const partyIsRequired = (party: PartyDTO) => isPartyRequired(party);
 const signerIsRequired = (signer: Signer) => isSignerRequired(signer);

 const requiredPartiesCount = useMemo(
  () => parties.filter(partyIsRequired).length,
  [parties],
 );
 const requiredSignersCount = useMemo(
  () => signers.filter(signerIsRequired).length,
  [signers],
 );

 const handleTogglePartyRequired = async (party: PartyDTO, next: boolean) => {
 const isCurrent = beginScopedRequest();
 // Commit-on-confirm: never change the checked state optimistically. Show the
 // loading flag and only expose the value once the mutation response (already
 // written to the deal cache via upsertDeal) confirms persistence.
 setSavingPartyIds((current) => ({ ...current, [party.id]: true }));
 try {
 await updateParty(deal.id, party.id, { signature_required: next });
 if (!isCurrent()) return;
 toast.success(next ? "Aceite marcado como obrigatório." : "Aceite marcado como opcional.");
 } catch (err) {
 if (!isCurrent()) return;
 notifyApiError(err, { context: "Atualizar parte" });
 } finally {
 if (isCurrent()) {
  setSavingPartyIds((current) => withoutRequirement(current, party.id));
 }
 }
 };

 const handleToggleSignerRequired = async (signer: Signer, next: boolean) => {
 const isCurrent = beginScopedRequest();
 // Commit-on-confirm: never change the checked state optimistically.
 setSavingSignerIds((current) => ({ ...current, [signer.id]: true }));
 try {
 await updateSigner(deal.id, signer.id, { is_required: next });
 if (!isCurrent()) return;
 toast.success(next ? "Assinatura marcada como obrigatória." : "Assinatura marcada como opcional.");
 } catch (err) {
 if (!isCurrent()) return;
 notifyApiError(err, { context: "Atualizar assinatura" });
 } finally {
 if (isCurrent()) {
  setSavingSignerIds((current) => withoutRequirement(current, signer.id));
 }
 }
 };

 return (
 <ExpandableSidePanel
 open={open}
 onClose={onClose}
 defaultSize="md"
 expandedSize="lg"
 zIndex={60}
 renderHeader={({ closeButton }) => (
 <div className="flex items-center justify-between gap-3 px-6 py-4 border-b border-border">
 <div className="flex items-center gap-2">
 <Settings2 className="w-5 h-5 text-primary-600" />
 <div>
 <h2 className="text-base font-bold text-foreground">Parâmetros da Proposta Digital</h2>
 <p className="text-xs text-muted-foreground">Configure quais partes e assinantes são obrigatórios neste deal.</p>
 </div>
 </div>
 <div className="flex items-center gap-2">{closeButton}</div>
 </div>
 )}
 >
 <div className="flex flex-col gap-6 p-6 overflow-y-auto">
 {/* Aceite da Proposta */}
 <section className="flex flex-col gap-3">
 <header className="flex items-center gap-2">
 <UserCheck className="w-4 h-4 text-primary-600" />
 <h3 className="text-sm font-semibold text-foreground">Aceite da Proposta</h3>
 </header>
 <p className="text-xs text-muted-foreground">
 Marque quais partes precisam aceitar a proposta para destravar Análise/Contrato.
 Default: partes principais (primárias).
 </p>

 {parties.length === 0 && (
 <div className="rounded-lg border border-dashed border-border bg-muted/30 px-4 py-6 text-center text-sm text-muted-foreground">
 Nenhuma parte cadastrada ainda.
 </div>
 )}

 {parties.length > 0 && requiredPartiesCount === 0 && (
 <div className="flex items-start gap-2 rounded-lg border border-status-warning/30 bg-status-warning-soft px-3 py-2 text-xs text-status-warning">
 <AlertTriangle className="w-4 h-4 mt-0.5 shrink-0" />
 <span>Sem partes obrigatórias, o deal avança sem aceite formal da proposta.</span>
 </div>
 )}

 <ul className="flex flex-col gap-2">
 {parties.map((party) => {
 const required = partyIsRequired(party);
 const saving = Boolean(savingPartyIds[party.id]);
 const label = partyRoleLabel[party.role] ?? party.role;
 return (
 <li
 key={party.id}
 className="flex items-center justify-between gap-3 rounded-lg border border-border bg-card px-3 py-2"
 >
 <div className="min-w-0">
 <div className="flex items-center gap-2">
 <span className="text-sm font-medium text-foreground truncate">
 {party.name || "(sem nome)"}
 </span>
 {party.is_primary && (
 <span className="text-caption uppercase font-semibold text-primary-700 bg-primary-50 border border-primary-200 px-1.5 py-0.5 rounded">
 Principal
 </span>
 )}
 </div>
 <div className="text-xs text-muted-foreground">
 {label}
 {party.email ? ` • ${party.email}` : ""}
 </div>
 </div>
 <RequirementToggle
 checked={required}
 loading={saving}
 accessibleLabel={`Aceite obrigatório para ${party.name || "parte sem nome"}`}
 onChange={(next) => handleTogglePartyRequired(party, next)}
 />
 </li>
 );
 })}
 </ul>
 </section>

 <div className="border-t border-border" />

 {/* Assinatura do Contrato */}
 <section className="flex flex-col gap-3">
 <header className="flex items-center gap-2">
 <FileSignature className="w-4 h-4 text-primary-600" />
 <h3 className="text-sm font-semibold text-foreground">Assinatura do Contrato</h3>
 </header>
 <p className="text-xs text-muted-foreground">
 Marque quais signatários precisam assinar o contrato para concluir o deal.
 Default: todos obrigatórios.
 </p>

 {signers.length === 0 && (
 <div className="rounded-lg border border-dashed border-border bg-muted/30 px-4 py-6 text-center text-sm text-muted-foreground">
 Nenhum assinante cadastrado ainda.
 </div>
 )}

 {signers.length > 0 && requiredSignersCount === 0 && (
 <div className="flex items-start gap-2 rounded-lg border border-status-warning/30 bg-status-warning-soft px-3 py-2 text-xs text-status-warning">
 <AlertTriangle className="w-4 h-4 mt-0.5 shrink-0" />
 <span>Sem assinantes obrigatórios, o contrato pode ser concluído sem nenhuma assinatura.</span>
 </div>
 )}

 <ul className="flex flex-col gap-2">
 {signers.map((signer) => {
 const required = signerIsRequired(signer);
 const saving = Boolean(savingSignerIds[signer.id]);
 const label = signerRoleLabel[(signer.role ?? "").toLowerCase()] ?? signer.role;
 return (
 <li
 key={signer.id}
 className="flex items-center justify-between gap-3 rounded-lg border border-border bg-card px-3 py-2"
 >
 <div className="min-w-0">
 <div className="flex items-center gap-2">
 <span className="text-sm font-medium text-foreground truncate">
 {signer.name || "(sem nome)"}
 </span>
 <span
 className={clsx(
 "text-caption uppercase font-semibold px-1.5 py-0.5 rounded border",
 signer.status === "SIGNED"
 ? "text-status-ok bg-status-ok-soft border-status-ok/30"
 : signer.status === "REJECTED"
 ? "text-status-critical bg-status-critical-soft border-status-critical/30"
 : "text-muted-foreground bg-muted border-border",
 )}
 >
 {signer.status}
 </span>
 </div>
 <div className="text-xs text-muted-foreground">
 {label}
 {signer.email ? ` • ${signer.email}` : ""}
 </div>
 </div>
 <RequirementToggle
 checked={required}
 loading={saving}
 accessibleLabel={`Assinatura obrigatória para ${signer.name || "assinante sem nome"}`}
 onChange={(next) => handleToggleSignerRequired(signer, next)}
 />
 </li>
 );
 })}
 </ul>
 </section>
 </div>
 </ExpandableSidePanel>
 );
}

interface RequirementToggleProps {
 checked: boolean;
 loading?: boolean;
 accessibleLabel: string;
 onChange: (next: boolean) => void;
}

function RequirementToggle({
 checked,
 loading = false,
 accessibleLabel,
 onChange,
}: RequirementToggleProps) {
 return (
 <div
 className={clsx(
 "inline-flex items-center gap-2 select-none",
 loading && "cursor-wait",
 )}
 >
 <span
 role="status"
 aria-live="polite"
 className="inline-flex min-w-[5.25rem] items-center justify-end gap-1 text-xs text-muted-foreground"
 >
 {loading ? (
 <>
 <Loader2 className="h-3.5 w-3.5 animate-spin" aria-hidden="true" />
 Salvando
 </>
 ) : (
 "Obrigatório"
 )}
 </span>
 <button
 type="button"
 role="switch"
 aria-checked={checked}
 aria-busy={loading}
 aria-label={accessibleLabel}
 disabled={loading}
 onClick={() => onChange(!checked)}
 className={clsx(
 "relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2 disabled:cursor-wait disabled:opacity-70",
 checked ? "bg-primary-500" : "bg-muted border border-border",
 )}
 >
 <span
 className={clsx(
 "inline-block h-4 w-4 rounded-full bg-card shadow transform transition-transform",
 checked ? "translate-x-4" : "translate-x-0.5",
 )}
 />
 </button>
 </div>
 );
}
