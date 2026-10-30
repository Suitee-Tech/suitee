import { useMemo } from "react";
import { AlertCircle, Check, CheckCircle2, ChevronsUpDown, Clock, Crown, Download, FileSignature, FileText, Loader2, Plus, Send, User, UserPlus, X } from "lucide-react";
import { clsx } from "clsx";
import { useNavigate } from "react-router";

import type { Contract, SignatureEnvelopeSigner } from "../../../../shared/contracts/types";
import { SignerFormModal } from "../../../../shared/components/signature/SignerFormModal";
import { SignatureStatusBadge } from "../../../../shared/components/signature/SignatureStatusBadge";
import { SigningGroupBadge } from "../../../../shared/components/signature/SigningGroupBadge";
import type { ApiLead } from "../../../../shared/services/lead.service";
import type { DealDTO, Signer } from "../../../../shared/services/deal.service";
import { formatPhoneBR } from "../../../../shared/utils/masks";
import { Button } from "../../../../shared/ui/button";
import { useSubscriptionAccess } from "../../../../shared/context/SubscriptionAccessContext";
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList, CommandSeparator } from "../../../../shared/ui/command";
import { Popover, PopoverContent, PopoverTrigger } from "../../../../shared/ui/popover";
import type { ContractSignatureEnvelopeController } from "./useContractSignatureEnvelope";

const SIGNER_ROLE_LABEL: Record<string, string> = {
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

function translateLeadRole(role?: string): string {
  switch (role) {
    case "owner":
      return "Proprietário";
    case "buyer":
      return "Comprador";
    case "tenant":
      return "Inquilino";
    case "guarantor":
      return "Fiador";
    default:
      return "Lead";
  }
}

function leadRoleToBadgeColor(role?: string): string {
  switch (role) {
    case "owner":
      return "bg-status-info-soft text-status-info";
    case "buyer":
      return "bg-stage-contract/15 text-stage-contract";
    case "tenant":
      return "bg-status-warning-soft text-status-warning";
    case "guarantor":
      return "bg-status-critical-soft text-status-critical";
    default:
      return "bg-muted text-muted-foreground";
  }
}

export interface NewSignerController {
  isAdding: boolean;
  setIsAdding: (value: boolean) => void;
  name: string;
  setName: (value: string) => void;
  email: string;
  setEmail: (value: string) => void;
  role: string;
  setRole: (value: string) => void;
  manualEntryMode: boolean;
  setManualEntryMode: (value: boolean) => void;
  selectedLead: ApiLead | null;
  leadComboOpen: boolean;
  setLeadComboOpen: (value: boolean) => void;
  searchLeadTerm: string;
  setSearchLeadTerm: (value: string) => void;
  suggestedLeads: ApiLead[];
  setSuggestedLeads: (value: ApiLead[]) => void;
  searchingLeads: boolean;
  onSelectLead: (lead: ApiLead) => void;
  onClearSelection: () => void;
  onEnterManualMode: () => void;
  onCancel: () => void;
  onAddSigner: () => Promise<void>;
}

interface ContractSignatureSectionProps {
  dealDetails: DealDTO | null;
  contract: Contract | null;
  signers: Signer[];
  signature: ContractSignatureEnvelopeController;
  newSigner: NewSignerController;
  inviteSigner: (dealId: string, signerId: string) => void | Promise<void>;
}

export function ContractSignatureSection({
  dealDetails,
  contract,
  signers,
  signature,
  newSigner,
  inviteSigner,
}: ContractSignatureSectionProps) {
  const navigate = useNavigate();
  const { hasFeature } = useSubscriptionAccess();
  const hasClientPortal = hasFeature("client_portal");
  const {
    envelope,
    reconcileBusy,
    cancelBusy,
    cancelConfirmOpen,
    resendAllBusy,
    resendingSignerId,
    signerEditing,
    envelopeAddOpen,
    handleReconcile,
    handleCancel,
    handleResendAll,
    handleResendSigner,
    handleEditSignerSubmit,
    handleAddSignerToEnvelope,
    setCancelConfirmOpen,
    setSignerEditing,
    setEnvelopeAddOpen,
  } = signature;
  const {
    isAdding,
    setIsAdding,
    name,
    setName,
    email,
    setEmail,
    role,
    setRole,
    manualEntryMode,
    setManualEntryMode,
    selectedLead,
    leadComboOpen,
    setLeadComboOpen,
    searchLeadTerm,
    setSearchLeadTerm,
    suggestedLeads,
    setSuggestedLeads,
    searchingLeads,
    onSelectLead: handleSelectLead,
    onClearSelection: handleClearSelection,
    onEnterManualMode: handleEnterManualMode,
    onCancel,
    onAddSigner: handleAddSigner,
  } = newSigner;

  const roleOrder = dealDetails?.type === "SALE"
    ? ["comprador", "vendedor", "avalista", "procurador", "representante_legal", "conjuge", "testemunha"]
    : ["inquilino", "locatario", "proprietario", "locador", "fiador", "procurador", "representante_legal", "conjuge", "testemunha"];
  const signersByRole = signers.reduce<Record<string, Signer[]>>((groups, signer) => {
    const roleKey = (signer.role ?? "").toLowerCase();
    groups[roleKey] = [...(groups[roleKey] ?? []), signer];
    return groups;
  }, {});

  const envelopeSignerById = useMemo(() => {
    const byId = new Map<string, SignatureEnvelopeSigner>();
    for (const signer of envelope?.signers ?? []) {
      if (signer.signer_id) byId.set(signer.signer_id, signer);
    }
    return byId;
  }, [envelope?.signers]);
  const envelopeSignerByEmail = useMemo(() => {
    const byEmail = new Map<string, SignatureEnvelopeSigner>();
    for (const signer of envelope?.signers ?? []) {
      if (signer.email) byEmail.set(signer.email.toLowerCase(), signer);
    }
    return byEmail;
  }, [envelope?.signers]);

  const findEnvelopeSignerFor = (signer: { id?: string; email?: string }) =>
    (signer.id ? envelopeSignerById.get(signer.id) : undefined) ??
    (signer.email ? envelopeSignerByEmail.get(signer.email.toLowerCase()) : undefined);
  const effectiveSignerStatus = (signer: Signer): string => {
    const envelopeSigner = findEnvelopeSignerFor(signer);
    const envelopeStatus = (envelopeSigner?.status ?? "").toUpperCase();
    if (envelopeStatus === "SIGNED" || envelopeStatus === "REFUSED") return envelopeStatus;
    return (signer.status ?? "PENDING").toUpperCase();
  };
  const groupForSigner = (signer: Signer): number => {
    const envelopeSigner = findEnvelopeSignerFor(signer);
    if (envelopeSigner?.group != null && envelopeSigner.group > 0) return envelopeSigner.group;
    const contractSigner = contract?.signatories.find(
      (candidate) =>
        candidate.id === signer.id ||
        (signer.email && candidate.email && candidate.email.toLowerCase() === signer.email.toLowerCase()),
    );
    return contractSigner?.group ?? 0;
  };

  const activeGroup = useMemo(() => {
    const pendingGroups = (envelope?.signers ?? [])
      .filter((signer) => (signer.group ?? 0) > 0 && (signer.status ?? "").toUpperCase() !== "SIGNED")
      .map((signer) => signer.group ?? 0);
    return pendingGroups.length === 0 ? 0 : Math.min(...pendingGroups);
  }, [envelope?.signers]);
  const hasSigningOrder = (envelope?.signers ?? []).some((signer) => (signer.group ?? 0) > 0);
  const envelopeIsActive = envelope?.status === "RUNNING" || envelope?.status === "PARTIALLY_SIGNED";
  const envelopeHasPendingSigner =
    envelope?.signers?.some((signer) => (signer.status ?? "").toUpperCase() !== "SIGNED") ?? false;
  const signersCount = signers.length;
  const signedCount = signers.filter((signer) => effectiveSignerStatus(signer) === "SIGNED").length;
  const findEnvelopeSigner = (signerId: string) =>
    envelope?.signers?.find((signer) => signer.signer_id === signerId);

  return (
    <>
      <div className="bg-card border border-border rounded-2xl overflow-hidden shadow-sm">
        <div className="bg-muted border-b border-border p-4 flex justify-between items-center gap-2 flex-wrap">
          <h3 className="font-bold text-foreground flex items-center gap-2">
            <FileSignature className="w-5 h-5 text-primary" />
            Assinatura Eletrônica
          </h3>
          <div className="flex items-center gap-2 flex-wrap">
            {envelope?.id && envelopeIsActive && envelopeHasPendingSigner && (
              <Button
                variant="outline"
                size="sm"
                onClick={handleResendAll}
                loading={resendAllBusy}
                loadingText="Reenviando…"
                title="Reenvia o email/whatsapp/sms de assinatura para todos os pendentes."
                className="gap-1 text-xs font-bold"
              >
                <Send className="w-3 h-3" />
                Reenviar a todos
              </Button>
            )}
            {envelope?.id && envelopeIsActive && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => setCancelConfirmOpen(true)}
                loading={cancelBusy}
                loadingText="Cancelando…"
                title="Cancela a assinatura na Clicksign. Não pode ser desfeito."
                className="gap-1 border-status-critical/30 bg-status-critical-soft text-xs font-bold text-status-critical hover:bg-status-critical/15 hover:text-status-critical"
              >
                <X className="w-3 h-3" />
                Cancelar assinatura
              </Button>
            )}
            {envelope?.id && (
              <Button
                variant="outline"
                size="sm"
                onClick={handleReconcile}
                loading={reconcileBusy}
                loadingText="Sincronizando…"
                title="Resincronizar com a Clicksign — útil se o estado parou de atualizar."
                className="gap-1 text-xs font-bold"
              >
                <Download className="w-3 h-3" />
                Reconciliar
              </Button>
            )}
            {envelope?.id && envelope.status !== "FINISHED" && envelope.status !== "REFUSED" && envelope.status !== "CANCELED" && envelope.status !== "EXPIRED" && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => setEnvelopeAddOpen(true)}
                title="Adicionar um novo signatário ao envelope já criado."
                className="gap-1 text-xs font-bold"
              >
                <Plus className="w-3 h-3" /> Adicionar ao envelope
              </Button>
            )}
            <Button variant="outline" size="sm" onClick={() => setIsAdding(!isAdding)} className="gap-1 text-xs font-bold">
              <Plus className="w-3 h-3" /> Adicionar Assinante
            </Button>
          </div>
        </div>

        {cancelConfirmOpen && (
          <div className="px-4 py-3 bg-status-critical-soft border-b border-status-critical/30 flex items-start gap-3">
            <AlertCircle className="w-5 h-5 text-status-critical shrink-0 mt-0.5" />
            <div className="flex-1 min-w-0">
              <p className="text-sm font-bold text-status-critical">Cancelar a assinatura deste contrato?</p>
              <p className="text-xs text-status-critical/80 mt-0.5">
                Esta ação encerra o envelope na Clicksign. Os assinantes não poderão mais assinar. Não pode ser desfeito.
              </p>
            </div>
            <div className="flex gap-2 shrink-0">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setCancelConfirmOpen(false)}
                disabled={cancelBusy}
                className="text-xs font-bold"
              >
                Voltar
              </Button>
              <Button
                variant="destructive"
                size="sm"
                onClick={handleCancel}
                loading={cancelBusy}
                className="text-xs font-bold"
              >
                Confirmar cancelamento
              </Button>
            </div>
          </div>
        )}

        {isAdding && (
          <div className="p-4 bg-muted border-b border-border animate-in fade-in slide-in-from-top-2">
            <h4 className="font-bold text-sm text-foreground mb-3">Novo Assinante</h4>

            <div className="mb-4">
              <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-1.5 block">Contato</label>
              {!manualEntryMode ? (
                <div className="flex gap-2">
                  <Popover
                    open={leadComboOpen}
                    onOpenChange={(open) => {
                      setLeadComboOpen(open);
                      if (open) {
                        setSearchLeadTerm("");
                        setSuggestedLeads([]);
                      }
                    }}
                  >
                    <PopoverTrigger asChild>
                      <button
                        role="combobox"
                        aria-expanded={leadComboOpen}
                        className="flex-1 flex items-center justify-between px-3 py-2 bg-card border border-border rounded-lg text-sm hover:bg-muted focus:outline-none focus:ring-2 focus:ring-primary min-h-[38px]"
                      >
                        {selectedLead ? (
                          <div className="flex items-center gap-2 truncate">
                            <User className="w-4 h-4 text-muted-foreground shrink-0" />
                            <span className="font-medium text-foreground truncate">{selectedLead.name}</span>
                            <span className="text-xs text-muted-foreground truncate">{selectedLead.email}</span>
                            <span className={`text-micro px-1.5 py-0.5 rounded font-bold shrink-0 ${leadRoleToBadgeColor(selectedLead.role)}`}>
                              {translateLeadRole(selectedLead.role)}
                            </span>
                            {selectedLead.is_client && (
                              <span className="text-micro bg-status-ok-soft text-status-ok px-1.5 py-0.5 rounded font-bold shrink-0">Cliente</span>
                            )}
                          </div>
                        ) : (
                          <span className="text-muted-foreground">Buscar lead ou cliente...</span>
                        )}
                        <ChevronsUpDown className="w-4 h-4 text-muted-foreground shrink-0 ml-2" />
                      </button>
                    </PopoverTrigger>
                    <PopoverContent className="w-[var(--radix-popover-trigger-width)] p-0 bg-card border border-border shadow-xl rounded-lg z-50" align="start">
                      <Command shouldFilter={false}>
                        <CommandInput placeholder="Buscar por nome ou email..." onValueChange={setSearchLeadTerm} />
                        <CommandList>
                          {searchingLeads && (
                            <div className="flex items-center justify-center py-4">
                              <Loader2 className="w-4 h-4 text-primary animate-spin" />
                            </div>
                          )}
                          {!searchingLeads && suggestedLeads.length === 0 && searchLeadTerm.length > 1 && <CommandEmpty>Nenhum contato encontrado.</CommandEmpty>}
                          {!searchingLeads && suggestedLeads.length === 0 && searchLeadTerm.length <= 1 && <CommandEmpty>Digite para buscar...</CommandEmpty>}
                          {suggestedLeads.length > 0 && (
                            <CommandGroup heading="Leads / Clientes">
                              {suggestedLeads.map((lead) => (
                                <CommandItem key={lead.id} value={lead.id} onSelect={() => handleSelectLead(lead)} className="flex items-center justify-between cursor-pointer">
                                  <div className="flex items-center gap-2 truncate">
                                    <Check className={clsx("w-4 h-4 shrink-0", selectedLead?.id === lead.id ? "opacity-100 text-primary" : "opacity-0")} />
                                    <div className="truncate">
                                      <p className="font-medium text-foreground truncate text-sm">{lead.name}</p>
                                      <p className="text-xs text-muted-foreground truncate">
                                        {lead.email}
                                        {lead.phone ? ` · ${formatPhoneBR(lead.phone)}` : ""}
                                      </p>
                                    </div>
                                  </div>
                                  <div className="flex items-center gap-1.5 shrink-0 ml-2">
                                    <span className={`text-micro px-1.5 py-0.5 rounded font-bold ${leadRoleToBadgeColor(lead.role)}`}>{translateLeadRole(lead.role)}</span>
                                    {lead.is_client && <span className="text-micro bg-status-ok-soft text-status-ok px-1.5 py-0.5 rounded font-bold">Cliente</span>}
                                  </div>
                                </CommandItem>
                              ))}
                            </CommandGroup>
                          )}
                          <CommandSeparator />
                          <CommandGroup>
                            <CommandItem onSelect={handleEnterManualMode} className="cursor-pointer text-primary">
                              <UserPlus className="w-4 h-4 mr-2" />
                              <span className="font-medium">Destinatário não cadastrado</span>
                            </CommandItem>
                          </CommandGroup>
                        </CommandList>
                      </Command>
                    </PopoverContent>
                  </Popover>
                  {selectedLead && (
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      onClick={handleClearSelection}
                      className="size-9 text-muted-foreground"
                      title="Limpar seleção"
                    >
                      <X className="w-4 h-4" />
                    </Button>
                  )}
                </div>
              ) : (
                <div className="space-y-3">
                  <div className="flex items-center gap-2 px-3 py-2 bg-status-warning-soft border border-status-warning/30 rounded-lg text-xs text-status-warning">
                    <UserPlus className="w-4 h-4 shrink-0" />
                    <span>Preenchimento manual — destinatário não cadastrado no sistema.</span>
                    <Button
                      type="button"
                      variant="link"
                      size="sm"
                      onClick={() => setManualEntryMode(false)}
                      className="ml-auto h-auto whitespace-nowrap p-0 font-bold text-status-warning hover:text-status-warning"
                    >
                      Buscar existente
                    </Button>
                  </div>
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                    <input
                      type="text"
                      placeholder="Nome do destinatário"
                      value={name}
                      onChange={(e) => setName(e.target.value)}
                      className="px-3 py-2 bg-card border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                    />
                    <input
                      type="email"
                      placeholder="Email"
                      value={email}
                      onChange={(e) => setEmail(e.target.value)}
                      className="px-3 py-2 bg-card border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                    />
                  </div>
                </div>
              )}
            </div>

            <div className="mb-4">
              <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-1.5 block">Papel na assinatura</label>
              <select
                value={role}
                onChange={(e) => setRole(e.target.value)}
                className="w-full sm:w-auto px-3 py-2 bg-card border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary"
              >
                {dealDetails?.type === "SALE" ? (
                  <>
                    <option value="comprador">Comprador</option>
                    <option value="vendedor">Vendedor</option>
                    <option value="avalista">Avalista</option>
                    <option value="testemunha">Testemunha</option>
                  </>
                ) : (
                  <>
                    <option value="inquilino">Inquilino</option>
                    <option value="proprietario">Proprietário</option>
                    <option value="fiador">Fiador</option>
                    <option value="locador">Locador</option>
                    <option value="locatario">Locatário</option>
                    <option value="testemunha">Testemunha</option>
                  </>
                )}
              </select>
            </div>

            {selectedLead && !manualEntryMode && (
              <div className="mb-4 px-3 py-2.5 bg-primary/10 border border-primary-200 rounded-lg text-sm text-foreground">
                <div className="flex items-center gap-3">
                  <div className="w-8 h-8 bg-primary/15 rounded-full flex items-center justify-center shrink-0">
                    <User className="w-4 h-4 text-primary" />
                  </div>
                  <div className="truncate">
                    <p className="font-medium text-foreground">{selectedLead.name}</p>
                    <p className="text-xs text-muted-foreground">
                      {selectedLead.email}
                      {selectedLead.phone ? ` · ${selectedLead.phone}` : ""}
                    </p>
                  </div>
                </div>
              </div>
            )}

            <div className="flex gap-2 pt-1 justify-end">
              <Button
                variant="ghost"
                size="sm"
                onClick={onCancel}
                className="text-xs font-bold"
              >
                Cancelar
              </Button>
              <Button
                size="sm"
                onClick={handleAddSigner}
                disabled={!name || !email}
                className="text-xs font-bold"
              >
                Adicionar
              </Button>
            </div>
          </div>
        )}

        <div className="p-5 space-y-4">
          <div className="flex items-center justify-between gap-2 flex-wrap">
            <div className="text-xs text-muted-foreground">{signersCount > 0 ? `Progresso: ${signedCount}/${signersCount} assinados` : "Nenhum assinante ainda."}</div>
            {hasSigningOrder && (
              <div className="text-xs text-muted-foreground flex items-center gap-2">
                <span>Assinatura sequencial:</span>
                {activeGroup > 0 ? (
                  <SigningGroupBadge group={activeGroup} active suffix="em andamento" />
                ) : (
                  <span className="font-bold text-status-ok">Todos os grupos concluídos</span>
                )}
              </div>
            )}
          </div>

          {signersCount === 0 ? (
            <div className="p-4 border border-border rounded-2xl bg-muted text-sm text-muted-foreground">
              Adicione os assinantes (partes e representantes) para habilitar convites e assinatura.
            </div>
          ) : (
            <div className="space-y-4">
              {roleOrder.map((r) => {
                const items = signersByRole[r] ?? [];
                if (items.length === 0) return null;
                return (
                  <div key={r} className="border border-border rounded-2xl bg-card overflow-hidden">
                    <div className="px-4 py-3 border-b border-border flex items-center justify-between">
                      <div className="text-sm font-bold text-foreground">{SIGNER_ROLE_LABEL[r] ?? r}</div>
                      <div className="text-xs text-muted-foreground">{items.length}</div>
                    </div>
                    <div className="divide-y divide-border">
                      {items.map((signer) => {
                        const status = effectiveSignerStatus(signer);
                        const isSigned = status === "SIGNED";
                        const canInvite = !isSigned && !!signer.email && !!dealDetails?.id && !!signer.id;
                        const signerGroup = groupForSigner(signer);
                        const groupActive = signerGroup > 0 && signerGroup === activeGroup;
                        const groupWaiting = signerGroup > 0 && activeGroup > 0 && signerGroup > activeGroup && !isSigned;
                        return (
                          <div key={signer.id} className="px-4 py-3 flex items-start justify-between gap-3">
                            <div className="min-w-0">
                              <div className="flex items-center gap-2">
                                <div
                                  className={clsx(
                                    "w-9 h-9 rounded-full flex items-center justify-center border",
                                    isSigned ? "bg-status-ok-soft border-status-ok/30 text-status-ok" : "bg-status-warning-soft border-status-warning/30 text-status-warning",
                                  )}
                                >
                                  {isSigned ? <CheckCircle2 className="w-4 h-4" /> : <Clock className="w-4 h-4" />}
                                </div>
                                <div className="min-w-0">
                                  <div className="text-sm font-bold text-foreground truncate">{signer.name || signer.email || signer.id}</div>
                                  <div className="text-xs text-muted-foreground truncate">{signer.email}</div>
                                </div>
                              </div>
                            </div>

                            <div className="flex flex-col items-end gap-2 shrink-0">
                              <div className="flex items-center gap-2 flex-wrap justify-end">
                                {signerGroup > 0 && (
                                  <SigningGroupBadge
                                    group={signerGroup}
                                    active={groupActive}
                                    suffix={groupWaiting ? "aguardando" : undefined}
                                  />
                                )}
                                <SignatureStatusBadge kind="signer" status={status} size="sm" />
                              </div>
                              <div className="flex gap-2">
                                {signer.signature_url && (
                                  <Button
                                    type="button"
                                    variant="secondary"
                                    size="sm"
                                    onClick={() => window.open(signer.signature_url, "_blank")}
                                    className="text-xs font-bold"
                                  >
                                    Abrir
                                  </Button>
                                )}
                                {envelope?.id && !isSigned && (
                                  <Button
                                    type="button"
                                    variant="outline"
                                    size="sm"
                                    onClick={() => {
                                      const envSigner = findEnvelopeSigner(signer.id);
                                      if (envSigner) setSignerEditing(envSigner);
                                    }}
                                    title="Editar nome/email/telefone/CPF do signatário"
                                    className="gap-1 text-xs font-bold"
                                  >
                                    <FileText className="w-3 h-3" /> Editar
                                  </Button>
                                )}
                                {envelope?.id && envelopeIsActive && !isSigned && (
                                  <Button
                                    type="button"
                                    variant="outline"
                                    size="sm"
                                    loading={resendingSignerId === signer.id}
                                    onClick={() => handleResendSigner(signer.id)}
                                    title="Reenviar email/whatsapp/sms de assinatura"
                                    className="gap-1 border-status-warning/30 bg-status-warning-soft text-xs font-bold text-status-warning hover:bg-status-warning/15 hover:text-status-warning"
                                  >
                                    <Send className="w-3 h-3" />
                                    Reenviar
                                  </Button>
                                )}
                                <Button
                                  type="button"
                                  variant="outline"
                                  size="sm"
                                  disabled={!canInvite}
                                  onClick={() => hasClientPortal ? inviteSigner(dealDetails!.id, signer.id) : navigate("/assinatura")}
                                  className="border-status-info/30 bg-status-info-soft text-xs font-bold text-status-info hover:bg-status-info-soft hover:text-status-info"
                                >
                                  {hasClientPortal ? "Convidar portal" : <><Crown className="mr-1 size-3" />Portal Pro</>}
                                </Button>
                              </div>
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </div>

      <SignerFormModal
        mode="edit"
        open={signerEditing !== null}
        onClose={() => setSignerEditing(null)}
        onSubmit={handleEditSignerSubmit}
        initial={
          signerEditing
            ? {
                name: signerEditing.name,
                email: signerEditing.email,
                phone_number: signerEditing.phone_number,
                documentation: signerEditing.documentation,
                birthday: signerEditing.birthday,
              }
            : undefined
        }
      />
      <SignerFormModal
        mode="add"
        open={envelopeAddOpen}
        onClose={() => setEnvelopeAddOpen(false)}
        onSubmit={handleAddSignerToEnvelope}
      />
    </>
  );
}
