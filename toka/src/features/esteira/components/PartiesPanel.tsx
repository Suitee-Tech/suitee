import { useEffect, useMemo, useRef, useState } from "react";
import type { DealDTO, PartyDTO } from "../../../shared/services/deal.service";
import { useDealsContext } from "../../../shared/context/DealsContext";
import { useLeadsContext } from "../../../shared/context/LeadsContext";
import { AddPartyDialog } from "../../../shared/components/AddPartyDialog";
import { EditPartyDialog } from "../../../shared/components/EditPartyDialog";
import { PartyRepresentationPanel } from "../../../shared/components/PartyRepresentationPanel";
import { Button } from "../../../shared/ui/button";
import { ConfirmDialog } from "../../../shared/components/ConfirmDialog";
import { Avatar, AvatarFallback } from "../../../shared/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../../../shared/ui/dropdown-menu";
import { Crown, Loader2, MoreHorizontal, Pencil, Scale, Send, Star, Trash2, Undo2, UserPlus, UserX, Users } from "lucide-react";
import { useNavigate } from "react-router";
import { StatusDot, StatusPill, type StatusTone } from "./esteira/PanelPrimitives";
import { formatCpfCnpj, formatDateBR, formatPhoneBR } from "../../../shared/utils/masks";
import { FieldProvenanceIndicator } from "../../../shared/ui/FieldWithProvenance";
import { notifyApiError } from "../../../shared/services/errorToast";
import { useRequestScopeGuard } from "../../../shared/hooks/useRequestScopeGuard";
import { useSubscriptionAccess } from "../../../shared/context/SubscriptionAccessContext";

function partyInitials(name: string, email?: string): string {
  const source = (name && name.trim()) || (email && email.trim()) || "?";
  const parts = source.split(/[\s@.]+/).filter(Boolean);
  const first = parts[0]?.[0] || "?";
  const second = parts[1]?.[0] || "";
  return (first + second).toUpperCase().slice(0, 2);
}

const roleLabel: Record<string, string> = {
  tenant: "Inquilinos",
  buyer: "Compradores",
  owner: "Proprietários",
  guarantor: "Fiadores/Avalistas",
  witness: "Testemunhas",
  spouse: "Cônjuges",
  attorney: "Procuradores",
  legal_representative: "Representantes legais",
  guardian: "Tutores",
  curator: "Curadores",
};

const roleOptionLabel: Record<string, string> = {
  tenant: "Inquilino",
  buyer: "Comprador",
  owner: "Proprietário",
  guarantor: "Fiador/Avalista",
  witness: "Testemunha",
  spouse: "Cônjuge",
  attorney: "Procurador",
  legal_representative: "Representante legal",
  guardian: "Tutor",
  curator: "Curador",
};

const ddStatusLabel: Record<string, string> = {
  APPROVED: "Aprovado",
  REJECTED: "Reprovado",
  PENDING: "Pendente",
};

const signatureStatusLabel: Record<string, string> = {
  SIGNED: "Assinado",
  REJECTED: "Rejeitado",
  PENDING: "Pendente",
};

const partyRoleOrderSale = ["buyer", "owner", "guarantor", "spouse", "attorney", "legal_representative", "guardian", "curator", "witness"] as const;
const partyRoleOrderRent = ["tenant", "owner", "guarantor", "spouse", "attorney", "legal_representative", "guardian", "curator", "witness"] as const;

const isIncompleteParty = (p: PartyDTO) => {
  const n = (p.name ?? "").trim().toLowerCase();
  const e = (p.email ?? "").trim().toLowerCase();
  if (!n && !e) return true;
  if (n === "unknown") return true;
  return false;
};

const displayPartyLabel = (p: PartyDTO) => {
  const n = (p.name ?? "").trim();
  const e = (p.email ?? "").trim();
  if (n) return n;
  if (e) return e;
  return "Cadastro pendente";
};

export function PartiesPanel({
  deal,
  onRefresh,
}: {
  deal: DealDTO;
  onRefresh: () => Promise<void> | void;
}) {
  const { leads } = useLeadsContext();
  const { addParty, removeParty, setPartyPrimary, updatePartyRole, updateParty, inviteSigner, addChecklistItem } = useDealsContext();
  const navigate = useNavigate();
  const { hasFeature } = useSubscriptionAccess();
  const hasClientPortal = hasFeature("client_portal");
  const [openAdd, setOpenAdd] = useState(false);
  const [spouseFor, setSpouseFor] = useState<{ partyId: string; label: string } | null>(null);
  const [removing, setRemoving] = useState<{ id: string; label: string } | null>(null);
  const [repFor, setRepFor] = useState<PartyDTO | null>(null);
  const [openRep, setOpenRep] = useState(false);
  const [addingRole, setAddingRole] = useState<{ role: any; title: string; description: string; isPrimary: boolean } | null>(null);
  const [showIncomplete, setShowIncomplete] = useState<Record<string, boolean>>({});
  const [editingParty, setEditingParty] = useState<PartyDTO | null>(null);
  const [updatingSpousePartyIds, setUpdatingSpousePartyIds] = useState<Set<string>>(() => new Set());
  const spouseDeclarationRequests = useRef<Set<string>>(new Set());
  const beginSpouseDeclarationRequest = useRequestScopeGuard(deal.id);

  const parties = deal.parties ?? [];

  useEffect(() => {
    spouseDeclarationRequests.current.clear();
    setUpdatingSpousePartyIds(new Set());
  }, [deal.id]);

  const toggleSpouseDeclaration = async (party: PartyDTO) => {
    if (spouseDeclarationRequests.current.has(party.id)) return;

    const isCurrentRequest = beginSpouseDeclarationRequest();
    const nextValue = !Boolean(party.no_spouse_declared);

    spouseDeclarationRequests.current.add(party.id);
    setUpdatingSpousePartyIds((current) => {
      const next = new Set(current);
      next.add(party.id);
      return next;
    });

    try {
      // Commit-on-confirm: never change the chip value optimistically. The
      // mutation response is already written to the deal cache (upsertDeal),
      // so the chip re-derives its value from the authoritative deal only
      // after the value is persisted. While in flight, the card shows
      // "Atualizando cônjuge...".
      await updateParty(deal.id, party.id, {
        no_spouse_declared: nextValue,
      });
    } catch (error) {
      if (!isCurrentRequest()) return;
      notifyApiError(error, { context: "Atualizar declaração de cônjuge" });
    } finally {
      if (isCurrentRequest()) {
        spouseDeclarationRequests.current.delete(party.id);
        setUpdatingSpousePartyIds((current) => {
          const next = new Set(current);
          next.delete(party.id);
          return next;
        });
      }
    }
  };

  const rolesToRender = useMemo(() => {
    const base = deal.type === "SALE" ? partyRoleOrderSale : partyRoleOrderRent;
    return base as unknown as string[];
  }, [deal.type]);

  const partiesByRole = useMemo(() => {
    const out: Record<string, PartyDTO[]> = {};
    for (const role of rolesToRender) out[role] = [];
    for (const p of parties) {
      const r = p.role ?? "other";
      out[r] = [...(out[r] ?? []), p];
    }
    return out;
  }, [parties, rolesToRender]);

  const partyNameById = useMemo(() => {
    const out: Record<string, string> = {};
    for (const p of parties) out[p.id] = displayPartyLabel(p);
    return out;
  }, [parties]);

  const spouseByPrincipalId = useMemo(() => {
    const out: Record<string, PartyDTO[]> = {};
    for (const p of parties) {
      if (p.role !== "spouse" || !p.spouse_of_party_id) continue;
      out[p.spouse_of_party_id] = [...(out[p.spouse_of_party_id] ?? []), p];
    }
    return out;
  }, [parties]);

  const principalsByRepresentativeId = useMemo(() => {
    const out: Record<string, PartyDTO[]> = {};
    for (const p of parties) {
      const rep = p.representation;
      if (!rep?.representative_party_id) continue;
      out[rep.representative_party_id] = [...(out[rep.representative_party_id] ?? []), p];
    }
    return out;
  }, [parties]);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <div className="text-lg font-bold text-foreground">Partes</div>
          <div className="text-sm text-muted-foreground">Gerencie roles, primários e convites.</div>
        </div>
        <Button onClick={() => setOpenAdd(true)} className="gap-2">
          <UserPlus className="w-4 h-4" />
          Adicionar
        </Button>
      </div>

      <AddPartyDialog
        open={openAdd}
        onOpenChange={setOpenAdd}
        dealType={deal.type}
        leads={leads}
        onAdd={async (party) => {
          await addParty(deal.id, party);
          await onRefresh();
        }}
      />

      <AddPartyDialog
        open={!!addingRole}
        onOpenChange={(v) => !v && setAddingRole(null)}
        dealType={deal.type}
        leads={leads}
        title={addingRole?.title}
        description={addingRole?.description}
        defaultRole={addingRole?.role}
        defaultIsPrimary={addingRole?.isPrimary}
        lockPrimary={addingRole?.isPrimary}
        onAdd={async (party) => {
          await addParty(deal.id, party);
          await onRefresh();
          setAddingRole(null);
        }}
      />

      <AddPartyDialog
        open={!!spouseFor}
        onOpenChange={(v) => !v && setSpouseFor(null)}
        dealType={deal.type}
        leads={leads}
        title="Adicionar cônjuge"
        description={spouseFor ? `Cônjuge vinculado a ${spouseFor.label}.` : undefined}
        defaultRole="spouse"
        lockRole
        spouseOfPartyId={spouseFor?.partyId}
        onAdd={async (party) => {
          if (!spouseFor) return;
          await addParty(deal.id, party);
          await updateParty(deal.id, spouseFor.partyId, { no_spouse_declared: false });
          await onRefresh();
          setSpouseFor(null);
        }}
      />

      <PartyRepresentationPanel
        open={openRep}
        onOpenChange={(v) => {
          setOpenRep(v);
          if (!v) setRepFor(null);
        }}
        party={repFor}
        parties={parties}
        onSave={async (representation) => {
          if (!repFor) return;
          await updateParty(deal.id, repFor.id, { representation });
          if (representation) {
            const exists = (deal.checklist ?? []).some((i) => i.party_id === repFor.id && i.name.toLowerCase().includes("procuração"));
            if (!exists) {
              await addChecklistItem(deal.id, {
                name: "Procuração",
                category: "juridico",
                required: true,
                description: "Documento de procuração para representação legal.",
                partyId: repFor.id,
              } as any);
            }
          }
          await onRefresh();
        }}
      />

      <EditPartyDialog
        open={!!editingParty}
        onOpenChange={(v) => !v && setEditingParty(null)}
        party={editingParty}
        onSave={async (patch) => {
          if (!editingParty) return;
          await updateParty(deal.id, editingParty.id, patch);
          await onRefresh();
          setEditingParty(null);
        }}
      />

      <ConfirmDialog
        open={!!removing}
        title="Remover parte"
        description={removing ? `Remover "${removing.label}" deste negócio?` : ""}
        confirmLabel="Remover"
        cancelLabel="Cancelar"
        variant="danger"
        onCancel={() => setRemoving(null)}
        onConfirm={async () => {
          if (!removing) return;
          const id = removing.id;
          setRemoving(null);
          await removeParty(deal.id, id);
          await onRefresh();
        }}
      />

      {(() => {
        // Split roles into "filled" (≥1 party) and "empty" so we can give the
        // panel a calm grid: filled roles get full cards, empty ones collapse
        // into a single "Adicionar" chip strip at the bottom. This avoids the
        // 7-empty-cards visual noise that polluted the previous layout.
        const filledRoles: string[] = [];
        const emptyRoles: string[] = [];
        for (const role of rolesToRender) {
          const items = partiesByRole[role] ?? [];
          const completeItems = items.filter((p) => !isIncompleteParty(p));
          if (completeItems.length > 0 || items.some(isIncompleteParty)) {
            filledRoles.push(role);
          } else {
            emptyRoles.push(role);
          }
        }

        return (
          <div className="space-y-3">
            {parties.length === 0 && (
              <div className="p-6 border border-dashed border-border rounded-2xl bg-card text-sm text-muted-foreground text-center">
                Nenhuma parte cadastrada ainda.
              </div>
            )}

            {filledRoles.map((role) => {
              const items = partiesByRole[role] ?? [];
              const incompleteCount = items.filter(isIncompleteParty).length;
              const visibleItems = showIncomplete[role] ? items : items.filter((p) => !isIncompleteParty(p));
              const canAddPrimaryRole =
                (deal.type === "SALE" && (role === "buyer" || role === "owner")) ||
                (deal.type !== "SALE" && (role === "tenant" || role === "owner"));

              // Aggregate role-level status so users can scan without expanding.
              const completeItems = items.filter((p) => !isIncompleteParty(p));
              const signersInRole = (deal.signers ?? []).filter((s) =>
                completeItems.some((p) => p.id === s.id || p.email === s.email)
              );
              const signedInRole = signersInRole.filter((s) => (s.status ?? "").toUpperCase() === "SIGNED").length;
              const ddPending = completeItems.filter((p) => (p.due_diligence?.status ?? "").toUpperCase() === "PENDING").length;
              const sectionTone: StatusTone =
                completeItems.length === 0
                  ? "neutral"
                  : signersInRole.length > 0 && signedInRole === signersInRole.length
                    ? "ok"
                    : ddPending > 0 || (signersInRole.length > 0 && signedInRole < signersInRole.length)
                      ? "pending"
                      : "neutral";
              const sectionSummary =
                signersInRole.length > 0
                  ? `${signedInRole}/${signersInRole.length} assinatura${signersInRole.length > 1 ? "s" : ""}`
                  : `${completeItems.length} cadastr${completeItems.length === 1 ? "ado" : "ados"}`;

              return (
                <section key={role} className="border border-border rounded-2xl bg-card overflow-hidden">
                  {/* Role section header — status + counter + add button */}
                  <header className="px-4 py-3 border-b border-border flex items-center justify-between gap-3">
                    <div className="flex items-center gap-2.5 min-w-0">
                      <StatusDot tone={sectionTone} />
                      <h3 className="text-label font-bold text-foreground uppercase tracking-wide truncate">
                        {roleLabel[role] ?? role}
                      </h3>
                      <span className="text-caption text-muted-foreground/80 tabular-nums">{sectionSummary}</span>
                    </div>
                    <Button
                      size="sm"
                      variant="ghost"
                      className="gap-1.5 -mr-2 text-muted-foreground hover:text-foreground"
                      onClick={() =>
                        setAddingRole({
                          role,
                          title: `Adicionar ${roleOptionLabel[role] ?? "parte"}`,
                          description: "Cadastre a pessoa e defina o papel dela na transação.",
                          isPrimary: canAddPrimaryRole,
                        })
                      }
                    >
                      <UserPlus className="w-3.5 h-3.5" />
                      Adicionar
                    </Button>
                  </header>

                  {incompleteCount > 0 && !showIncomplete[role] && (
                    <div className="px-4 py-2.5 border-b border-border bg-muted/40 flex items-center justify-between gap-3">
                      <div className="text-caption text-muted-foreground">
                        {incompleteCount} registro{incompleteCount > 1 ? "s" : ""} com dados incompletos oculto{incompleteCount > 1 ? "s" : ""}.
                      </div>
                      <Button
                        size="sm"
                        variant="ghost"
                        className="text-caption -my-1"
                        onClick={() => setShowIncomplete((prev) => ({ ...prev, [role]: true }))}
                      >
                        Ver
                      </Button>
                    </div>
                  )}

                  {/* Party cards */}
                  <div className="divide-y divide-border">
                    {visibleItems.map((p) => {
                      const signer = (deal.signers ?? []).find((s) => s.id === p.id);
                      const label = partyNameById[p.id];
                      const ddStatusRaw = (p.due_diligence?.status ?? "").toUpperCase();
                      const ddStatus = ddStatusLabel[ddStatusRaw] ?? (p.due_diligence?.status ?? "—");
                      const signStatusRaw = ((p.sign_status ?? signer?.status ?? "") as string).toUpperCase();
                      const signStatus = signatureStatusLabel[signStatusRaw] ?? (p.sign_status ?? signer?.status ?? "—");
                      const spouseList = spouseByPrincipalId[p.id] ?? [];
                      const spouse = spouseList[0];
                      const isCustomerPrimary =
                        (deal.type === "SALE" && p.role === "buyer" && p.is_primary) ||
                        (deal.type !== "SALE" && p.role === "tenant" && p.is_primary);
                      const isOwnerPrimary = p.role === "owner" && p.is_primary;
                      const needsSpouse = isCustomerPrimary || isOwnerPrimary;
                      const isRepresented = !!p.representation?.representative_party_id;
                      const representedBy = isRepresented ? partyNameById[p.representation!.representative_party_id] : null;
                      const represents = principalsByRepresentativeId[p.id] ?? [];
                      const canInvite = !!p.email && signStatusRaw !== "SIGNED" && !!signer;
                      const inviteDisabledByRep =
                        (p.role === "buyer" || p.role === "tenant" || p.role === "owner") &&
                        p.representation &&
                        p.representation.representative_party_id &&
                        p.representation.requires_principal_signature === false;
                      // Commit-on-confirm: the chip always reflects the
                      // authoritative deal value (no optimistic change).
                      // While the request is in flight the card shows
                      // "Atualizando cônjuge..." and updates once persisted.
                      const noSpouseDeclared = Boolean(p.no_spouse_declared);
                      const isUpdatingSpouse = updatingSpousePartyIds.has(p.id);

                      const ddTone: StatusTone =
                        ddStatusRaw === "APPROVED" ? "ok"
                        : ddStatusRaw === "REJECTED" ? "critical"
                        : ddStatusRaw === "PENDING" ? "pending"
                        : "neutral";
                      const signTone: StatusTone =
                        signStatusRaw === "SIGNED" ? "ok"
                        : signStatusRaw === "REJECTED" ? "critical"
                        : signStatusRaw === "PENDING" ? "pending"
                        : "neutral";

                      return (
                        <article
                          key={p.id}
                          className="px-4 py-4 flex flex-col gap-3.5"
                          aria-busy={isUpdatingSpouse || undefined}
                        >
                          {/* Identity row: avatar + name + email + status pills + actions menu */}
                          <div className="flex items-start gap-3">
                            <Avatar className="w-10 h-10 shrink-0">
                              <AvatarFallback className="bg-muted text-foreground/80 text-caption font-bold">
                                {partyInitials(p.name || "", p.email)}
                              </AvatarFallback>
                            </Avatar>

                            <div className="flex-1 min-w-0 space-y-1">
                              <div className="flex items-center gap-2 flex-wrap">
                                <h4 className="text-title font-bold text-foreground truncate">{label}</h4>
                                <FieldProvenanceIndicator provenance={p.field_provenance?.name} className="shrink-0" />
                                {p.is_primary && (
                                  <StatusPill tone="ok" icon={<Star className="w-3 h-3" />}>
                                    Primário
                                  </StatusPill>
                                )}
                              </div>
                              {p.email && (
                                <div className="flex items-center gap-1.5">
                                  <p className="text-caption text-muted-foreground truncate">{p.email}</p>
                                  <FieldProvenanceIndicator provenance={p.field_provenance?.email} className="shrink-0" />
                                </div>
                              )}
                              {p.document ? (
                                <div className="flex items-center gap-1.5">
                                  <p className="text-caption text-muted-foreground truncate">
                                    CPF/CNPJ: {formatCpfCnpj(p.document)}
                                  </p>
                                  <FieldProvenanceIndicator provenance={p.field_provenance?.document} className="shrink-0" />
                                </div>
                              ) : p.role !== "spouse" && p.role !== "witness" ? (
                                <p className="text-caption text-muted-foreground/80 italic truncate">
                                  Documento não informado
                                </p>
                              ) : null}

                              {/* Consulta dos campos que antes só existiam no
                                  formulário: telefone e data de nascimento ficam
                                  visíveis no cartão da parte, com proveniência. */}
                              <div className="flex flex-wrap items-center gap-x-3 gap-y-0.5 pt-0.5">
                                <div className="flex items-center gap-1.5">
                                  <p className="text-caption text-muted-foreground">
                                    Telefone: {p.phone ? formatPhoneBR(p.phone) : "Não informado"}
                                  </p>
                                  <FieldProvenanceIndicator
                                    provenance={p.field_provenance?.phone}
                                    className="shrink-0"
                                  />
                                </div>
                                <div className="flex items-center gap-1.5">
                                  <p className="text-caption text-muted-foreground">
                                    Nascimento:{" "}
                                    {p.birth_date ? formatDateBR(p.birth_date) : "Não informado"}
                                  </p>
                                  <FieldProvenanceIndicator
                                    provenance={p.field_provenance?.birth_date}
                                    className="shrink-0"
                                  />
                                </div>
                              </div>

                              {/* Relational pills (spouse / representation) — only when relevant */}
                              {(needsSpouse || isRepresented || represents.length > 0) && (
                                <div className="flex flex-wrap gap-1.5 pt-1">
                                  {needsSpouse && (isUpdatingSpouse ? (
                                    <StatusPill tone="neutral" icon={<Loader2 className="w-3 h-3 animate-spin" />}>
                                      Atualizando cônjuge...
                                    </StatusPill>
                                  ) : spouse ? (
                                    <StatusPill tone="neutral" icon={<Users className="w-3 h-3" />}>
                                      Cônjuge: {partyNameById[spouse.id]}
                                    </StatusPill>
                                  ) : noSpouseDeclared ? (
                                    <StatusPill tone="warning" icon={<UserX className="w-3 h-3" />}>
                                      Sem cônjuge declarado
                                    </StatusPill>
                                  ) : (
                                    <StatusPill tone="critical" icon={<Users className="w-3 h-3" />}>
                                      Cônjuge pendente
                                    </StatusPill>
                                  ))}
                                  {isRepresented && representedBy && (
                                    <StatusPill tone="neutral" icon={<Scale className="w-3 h-3" />}>
                                      Representado por {representedBy}
                                    </StatusPill>
                                  )}
                                  {represents.length > 0 && (
                                    <StatusPill tone="neutral" icon={<Scale className="w-3 h-3" />}>
                                      Representa {represents.map((x) => partyNameById[x.id]).join(", ")}
                                    </StatusPill>
                                  )}
                                </div>
                              )}
                            </div>

                            {/* All actions consolidated into the overflow menu.
                                The status pills above already convey current state (Cônjuge: X,
                                Sem cônjuge declarado, Representado por X, Primário) — the menu
                                is where the user goes to *change* that state.
                                Native <button> on the trigger to avoid Radix Slot nesting issues
                                that previously kept the menu from opening. `onSelect` fires the
                                handler AND closes the menu (Radix idiomatic). */}
                            <div className="shrink-0">
                              <DropdownMenu>
                                <DropdownMenuTrigger
                                  type="button"
                                  disabled={isUpdatingSpouse}
                                  className="inline-flex items-center justify-center h-8 w-8 rounded-md text-muted-foreground hover:bg-muted hover:text-foreground transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-60"
                                  aria-label="Mais ações"
                                >
                                  {isUpdatingSpouse ? (
                                    <Loader2 className="w-4 h-4 animate-spin" />
                                  ) : (
                                    <MoreHorizontal className="w-4 h-4" />
                                  )}
                                </DropdownMenuTrigger>
                                <DropdownMenuContent align="end" className="w-56">
                                  <DropdownMenuItem onSelect={() => setEditingParty(p)}>
                                    <Pencil className="w-4 h-4 mr-2" />
                                    Editar / ver dados
                                  </DropdownMenuItem>
                                  {canInvite && !inviteDisabledByRep && (
                                    <DropdownMenuItem
                                      onSelect={async () => {
                                        if (!hasClientPortal) {
                                          navigate("/assinatura");
                                          return;
                                        }
                                        if (signer) {
                                          await inviteSigner(deal.id, signer.id);
                                          await onRefresh();
                                        }
                                      }}
                                    >
                                      {hasClientPortal ? <Send className="w-4 h-4 mr-2" /> : <Crown className="w-4 h-4 mr-2" />}
                                      {hasClientPortal ? "Convidar portal" : "Conheça o Portal Pro"}
                                    </DropdownMenuItem>
                                  )}
                                  {!p.is_primary && (
                                    <DropdownMenuItem
                                      onSelect={async () => {
                                        await setPartyPrimary(deal.id, p.id);
                                        await onRefresh();
                                      }}
                                    >
                                      <Star className="w-4 h-4 mr-2" />
                                      Definir como primário
                                    </DropdownMenuItem>
                                  )}
                                  {needsSpouse && !spouse && (
                                    <DropdownMenuItem
                                      disabled={isUpdatingSpouse}
                                      onSelect={() => setSpouseFor({ partyId: p.id, label })}
                                    >
                                      <Users className="w-4 h-4 mr-2" />
                                      Adicionar cônjuge
                                    </DropdownMenuItem>
                                  )}
                                  {needsSpouse && (
                                    <DropdownMenuItem
                                      disabled={isUpdatingSpouse}
                                      onSelect={() => void toggleSpouseDeclaration(p)}
                                    >
                                      {noSpouseDeclared ? (
                                        <>
                                          <Undo2 className="w-4 h-4 mr-2" />
                                          Desfazer "sem cônjuge"
                                        </>
                                      ) : (
                                        <>
                                          <UserX className="w-4 h-4 mr-2" />
                                          Marcar sem cônjuge
                                        </>
                                      )}
                                    </DropdownMenuItem>
                                  )}
                                  {p.role !== "spouse" && (
                                    <DropdownMenuItem
                                      onSelect={() => {
                                        setRepFor(p);
                                        setOpenRep(true);
                                      }}
                                    >
                                      <Scale className="w-4 h-4 mr-2" />
                                      Representação
                                    </DropdownMenuItem>
                                  )}
                                  <DropdownMenuSeparator />
                                  <DropdownMenuItem
                                    onSelect={() => setRemoving({ id: p.id, label })}
                                    className="text-destructive focus:text-destructive focus:bg-destructive/10"
                                  >
                                    <Trash2 className="w-4 h-4 mr-2" />
                                    Remover do negócio
                                  </DropdownMenuItem>
                                </DropdownMenuContent>
                              </DropdownMenu>
                            </div>
                          </div>

                          {/* Compact meta grid: 3 status fields in a single row, denser & more scannable */}
                          <dl className="grid grid-cols-1 sm:grid-cols-3 gap-px rounded-xl border border-border bg-border overflow-hidden text-caption">
                            <div className="px-3 py-2 bg-card flex flex-col gap-0.5">
                              <dt className="text-micro font-bold text-muted-foreground uppercase tracking-wide">Due Diligence</dt>
                              <dd className="inline-flex items-center gap-1.5 font-semibold text-foreground">
                                <StatusDot tone={ddTone} />
                                <span className="truncate">{ddStatus}</span>
                              </dd>
                            </div>
                            <div className="px-3 py-2 bg-card flex flex-col gap-0.5">
                              <dt className="text-micro font-bold text-muted-foreground uppercase tracking-wide">Assinatura</dt>
                              <dd className="inline-flex items-center gap-1.5 font-semibold text-foreground">
                                <StatusDot tone={signTone} />
                                <span className="truncate">{signStatus}</span>
                              </dd>
                            </div>
                            <div className="px-3 py-2 bg-card flex flex-col gap-0.5">
                              <dt className="text-micro font-bold text-muted-foreground uppercase tracking-wide">Papel</dt>
                              <dd className="font-semibold text-foreground">
                                <select
                                  value={p.role}
                                  onChange={async (e) => {
                                    await updatePartyRole(deal.id, p.id, e.target.value as any);
                                    await onRefresh();
                                  }}
                                  className="w-full bg-transparent text-foreground font-semibold outline-none cursor-pointer hover:text-primary transition-colors"
                                >
                                  {(deal.type === "SALE"
                                    ? (["buyer", "spouse", "owner", "guarantor", "attorney", "legal_representative", "guardian", "curator", "witness"] as const)
                                    : (["tenant", "spouse", "owner", "guarantor", "attorney", "legal_representative", "guardian", "curator", "witness"] as const)
                                  ).map((r) => (
                                    <option key={r} value={r}>
                                      {roleOptionLabel[r] ?? r}
                                    </option>
                                  ))}
                                </select>
                              </dd>
                            </div>
                          </dl>
                        </article>
                      );
                    })}
                  </div>
                </section>
              );
            })}

            {/* Empty roles condensed into a single chip strip — major UX win:
                7 redundant "Adicionar" cards become a single calm row. */}
            {emptyRoles.length > 0 && (
              <section className="border border-dashed border-border rounded-2xl bg-card p-4">
                <header className="flex items-center gap-2 mb-3">
                  <UserPlus className="w-3.5 h-3.5 text-muted-foreground" />
                  <h3 className="text-label font-bold text-foreground uppercase tracking-wide">
                    Adicionar mais partes
                  </h3>
                </header>
                <div className="flex flex-wrap gap-2">
                  {emptyRoles.map((role) => {
                    const canAddPrimaryRole =
                      (deal.type === "SALE" && (role === "buyer" || role === "owner")) ||
                      (deal.type !== "SALE" && (role === "tenant" || role === "owner"));
                    return (
                      <button
                        key={role}
                        type="button"
                        onClick={() =>
                          setAddingRole({
                            role,
                            title: `Adicionar ${roleOptionLabel[role] ?? "parte"}`,
                            description: "Cadastre a pessoa e defina o papel dela na transação.",
                            isPrimary: canAddPrimaryRole,
                          })
                        }
                        className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full border border-border bg-card text-caption font-semibold text-foreground/80 hover:border-primary/40 hover:text-primary hover:bg-primary/5 transition-colors"
                      >
                        <UserPlus className="w-3 h-3" />
                        {roleLabel[role] ?? role}
                      </button>
                    );
                  })}
                </div>
              </section>
            )}
          </div>
        );
      })()}
    </div>
  );
}
