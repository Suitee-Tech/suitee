import { useEffect, useMemo, useState } from "react";
import { ExpandableSidePanel } from "./ExpandableSidePanel";
import { Button } from "../ui/button";
import { Checkbox } from "../ui/checkbox";
import { Input, inputClassNames } from "../ui/input";
import { PhoneInput } from "../ui/phone-input";
import { DocumentInput } from "../ui/document-input";
import { Label } from "../ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/popover";
import { Command, CommandEmpty, CommandInput, CommandItem, CommandList } from "../ui/command";
import { Check, ChevronsUpDown, Loader2, X } from "lucide-react";
import type { Lead } from "../data";
import type { CreatePartyDTO, PartyRole, DealType } from "../services/deal.service";
import { leadService } from "../services/lead.service";
import { parsePhoneBR, isValidCpfCnpj } from "../utils/masks";

const roleOptionsByDealType: Record<DealType, PartyRole[]> = {
  RENT: ["tenant", "spouse", "owner", "guarantor", "attorney", "legal_representative", "guardian", "curator", "witness"],
  SALE: ["buyer", "spouse", "owner", "guarantor", "attorney", "legal_representative", "guardian", "curator", "witness"],
};

const roleLabel: Record<PartyRole, string> = {
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

export function AddPartyDialog({
  open,
  onOpenChange,
  dealType,
  leads,
  onAdd,
  title,
  description,
  defaultRole,
  lockRole,
  defaultIsPrimary,
  lockPrimary,
  spouseOfPartyId,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  dealType: DealType;
  leads: Lead[];
  onAdd: (party: CreatePartyDTO) => void;
  title?: string;
  description?: string;
  defaultRole?: PartyRole;
  lockRole?: boolean;
  defaultIsPrimary?: boolean;
  lockPrimary?: boolean;
  spouseOfPartyId?: string;
}) {
  const roles = roleOptionsByDealType[dealType] ?? ["tenant", "owner", "guarantor", "witness"];
  const initialLeads = useMemo(() => leads.slice(0, 10), [leads]);

  const [selectedLeadId, setSelectedLeadId] = useState<string>("");
  const [selectedLeadLabel, setSelectedLeadLabel] = useState<string>("");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [phone, setPhone] = useState("");
  const [docValue, setDocValue] = useState("");
  const [role, setRole] = useState<PartyRole>(defaultRole ?? roles[0]);
  const [isPrimary, setIsPrimary] = useState(Boolean(defaultIsPrimary));
  const [leadComboOpen, setLeadComboOpen] = useState(false);
  const [searchLeadTerm, setSearchLeadTerm] = useState("");
  const [suggestedLeads, setSuggestedLeads] = useState<Array<{ id: string; name?: string; email?: string; phone?: string }>>(initialLeads);
  const [searchingLeads, setSearchingLeads] = useState(false);
  const [initialFetched, setInitialFetched] = useState(false);

  const loadFromLead = (lead: { id: string; name?: string; email?: string; phone?: string }) => {
    setSelectedLeadId(lead.id);
    setSelectedLeadLabel(lead.name || lead.email || lead.id);
    setName(lead.name ?? "");
    setEmail(lead.email ?? "");
    setPhone(parsePhoneBR(lead.phone ?? ""));
    setLeadComboOpen(false);
  };

  const reset = () => {
    setSelectedLeadId("");
    setSelectedLeadLabel("");
    setName("");
    setEmail("");
    setPhone("");
    setDocValue("");
    setRole(defaultRole ?? roles[0]);
    setIsPrimary(Boolean(defaultIsPrimary));
    setSearchLeadTerm("");
    setSuggestedLeads(initialLeads);
    setSearchingLeads(false);
    setLeadComboOpen(false);
    setInitialFetched(false);
  };

  const documentValid = !docValue || isValidCpfCnpj(docValue);
  const canSubmit = role && (selectedLeadId || (name.trim() && email.trim())) && documentValid;

  useEffect(() => {
    if (!open) return;
    reset();
  }, [open, dealType, defaultRole, defaultIsPrimary]);

  useEffect(() => {
    if (!open || !leadComboOpen) return;

    if (!searchLeadTerm || searchLeadTerm.length < 2) {
      if (!initialFetched && initialLeads.length === 0) {
        setInitialFetched(true);
        setSearchingLeads(true);
        leadService
          .getAll({ limit: 10 })
          .then((res) => setSuggestedLeads(res.data.map((l) => ({ id: l.id, name: l.name, email: l.email, phone: l.phone }))))
          .catch(() => setSuggestedLeads([]))
          .finally(() => setSearchingLeads(false));
        return;
      }
      if (suggestedLeads.length === 0) setSuggestedLeads(initialLeads);
      return;
    }

    const timer = setTimeout(() => {
      setSearchingLeads(true);
      leadService
        .getAll({ search: searchLeadTerm, limit: 10 })
        .then((res) => {
          setSuggestedLeads(res.data.map((l) => ({ id: l.id, name: l.name, email: l.email, phone: l.phone })));
        })
        .catch(() => {
          setSuggestedLeads([]);
        })
        .finally(() => setSearchingLeads(false));
    }, 300);

    return () => clearTimeout(timer);
  }, [open, leadComboOpen, searchLeadTerm, initialLeads, suggestedLeads.length, initialFetched]);

  return (
    <ExpandableSidePanel
      open={open}
      onClose={() => onOpenChange(false)}
      defaultSize="lg"
      expandedSize="2xl"
      zIndex={60}
      renderHeader={({ expandButton, closeButton }) => (
        <div className="flex-none px-6 py-5 border-b border-border bg-card">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <h2 className="text-lg font-bold text-foreground">{title ?? "Adicionar parte"}</h2>
              <p className="text-sm text-muted-foreground mt-0.5">
                {description ?? "Inclua uma nova pessoa no negócio e defina o papel dela na transação."}
              </p>
            </div>
            <div className="flex items-center gap-2 shrink-0">
              {expandButton}
              {closeButton}
            </div>
          </div>
        </div>
      )}
    >
      <div className="flex-1 overflow-y-auto px-6 py-5 space-y-5">
        <div className="space-y-2">
          <Label>Selecionar lead (opcional)</Label>
          <div className="flex gap-2">
            <Popover
              open={leadComboOpen}
              onOpenChange={(v) => {
                setLeadComboOpen(v);
                if (v) {
                  setSearchLeadTerm("");
                  setSuggestedLeads(initialLeads);
                }
              }}
            >
              <PopoverTrigger asChild>
                <button
                  type="button"
                  role="combobox"
                  aria-expanded={leadComboOpen}
                  className="h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm flex items-center justify-between gap-2"
                >
                  <span className="truncate text-left">
                    {selectedLeadId ? selectedLeadLabel : "Buscar lead por nome ou email..."}
                  </span>
                  <ChevronsUpDown className="w-4 h-4 text-muted-foreground shrink-0" />
                </button>
              </PopoverTrigger>
              <PopoverContent className="w-[var(--radix-popover-trigger-width)] p-0 bg-card border border-border shadow-xl rounded-lg z-50" align="start">
                <Command shouldFilter={false}>
                  <CommandInput placeholder="Buscar lead por nome ou email..." onValueChange={setSearchLeadTerm} />
                  <CommandList className="max-h-64">
                    {searchingLeads && (
                      <div className="flex items-center justify-center py-4">
                        <Loader2 className="w-4 h-4 text-primary-500 animate-spin" />
                      </div>
                    )}
                    {!searchingLeads && suggestedLeads.length === 0 && searchLeadTerm.length > 1 && (
                      <CommandEmpty>Nenhum lead encontrado.</CommandEmpty>
                    )}
                    {!searchingLeads && suggestedLeads.length === 0 && searchLeadTerm.length <= 1 && (
                      <CommandEmpty>Digite para buscar…</CommandEmpty>
                    )}
                    {suggestedLeads.map((l) => (
                      <CommandItem key={l.id} value={l.id} onSelect={() => loadFromLead(l)} className="flex items-start justify-between gap-3 cursor-pointer">
                        <div className="flex items-start gap-2 min-w-0">
                          <Check className={selectedLeadId === l.id ? "w-4 h-4 text-primary-600 mt-0.5" : "w-4 h-4 opacity-0 mt-0.5"} />
                          <div className="min-w-0">
                            <div className="text-sm font-medium text-foreground truncate">{l.name || "Sem nome"}</div>
                            <div className="text-xs text-muted-foreground truncate">{l.email}</div>
                          </div>
                        </div>
                      </CommandItem>
                    ))}
                  </CommandList>
                </Command>
              </PopoverContent>
            </Popover>
            {selectedLeadId && (
              <button
                type="button"
                onClick={() => {
                  setSelectedLeadId("");
                  setSelectedLeadLabel("");
                  setName("");
                  setEmail("");
                  setPhone("");
                }}
                className="h-10 w-10 rounded-md border border-input bg-background flex items-center justify-center text-muted-foreground hover:bg-muted"
                title="Limpar seleção"
              >
                <X className="w-4 h-4" />
              </button>
            )}
          </div>
          <div className="text-xs text-muted-foreground">
            Se você selecionar um lead, os dados são preenchidos automaticamente (e você pode ajustar se necessário).
          </div>
        </div>

        <div className="space-y-4">
          <div className="space-y-1">
            <Label>Nome</Label>
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Ex: Maria Silva" />
          </div>
          <div className="space-y-1">
            <Label>Email</Label>
            <Input value={email} onChange={(e) => setEmail(e.target.value)} placeholder="maria@exemplo.com" />
          </div>
          <div className="space-y-1">
            <Label>Telefone</Label>
            <PhoneInput value={phone} onChange={setPhone} placeholder="(00) 00000-0000" className={inputClassNames} />
          </div>
          <div className="space-y-1">
            <Label>Documento (CPF/CNPJ)</Label>
            <DocumentInput
              value={docValue}
              onChange={setDocValue}
              placeholder="000.000.000-00"
              className={inputClassNames}
            />
            {docValue && !documentValid && (
              <p className="text-xs text-status-critical mt-1">CPF/CNPJ inválido.</p>
            )}
            <p className="text-xs text-muted-foreground">
              Necessário para enviar o contrato à assinatura — pode ser preenchido depois.
            </p>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div className="space-y-1">
              <Label>Papel na transação</Label>
              {lockRole ? (
                <div className="h-10 w-full rounded-md border border-input bg-muted px-3 py-2 text-sm flex items-center font-semibold text-foreground">
                  {roleLabel[role]}
                </div>
              ) : (
                <select
                  value={role}
                  onChange={(e) => setRole(e.target.value as PartyRole)}
                  className="h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                >
                  {roles.map((r) => (
                    <option key={r} value={r}>
                      {roleLabel[r]}
                    </option>
                  ))}
                </select>
              )}
            </div>

            <div className="space-y-1">
              <Label>Primário</Label>
              <div className="h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm flex items-center gap-2">
                <Checkbox
                  checked={isPrimary}
                  onCheckedChange={(v) => {
                    if (lockPrimary) return;
                    setIsPrimary(Boolean(v));
                  }}
                />
                <span>Definir como primário</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div className="flex-none px-6 py-4 border-t border-border bg-card">
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancelar
          </Button>
          <Button
            disabled={!canSubmit}
            onClick={() => {
              onAdd({
                lead_id: selectedLeadId || undefined,
                name: name.trim() || undefined,
                email: email.trim() || undefined,
                phone: phone.trim() || undefined,
                document: docValue || undefined,
                role,
                spouse_of_party_id: spouseOfPartyId,
                is_primary: isPrimary,
              });
              onOpenChange(false);
            }}
          >
            Adicionar
          </Button>
        </div>
      </div>
    </ExpandableSidePanel>
  );
}

