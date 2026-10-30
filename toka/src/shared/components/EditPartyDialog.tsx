import { useEffect, useState } from "react";
import { Trash2 } from "lucide-react";
import { ExpandableSidePanel } from "./ExpandableSidePanel";
import { Button } from "../ui/button";
import { Input, inputClassNames } from "../ui/input";
import { Label } from "../ui/label";
import { PhoneInput } from "../ui/phone-input";
import { DocumentInput } from "../ui/document-input";
import type { PartyDTO, UpdatePartyDTO } from "../services/deal.service";
import { parsePhoneBR, parseCpfCnpj, isValidCpfCnpj, toDateInputValue } from "../utils/masks";
import { FieldProvenanceBadge } from "../ui/FieldWithProvenance";

export function EditPartyDialog({
  open,
  onOpenChange,
  party,
  onSave,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  party: PartyDTO | null;
  onSave: (patch: UpdatePartyDTO) => Promise<void> | void;
}) {
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [phone, setPhone] = useState("");
  const [docValue, setDocValue] = useState("");
  const [birthDate, setBirthDate] = useState("");
  const [customFields, setCustomFields] = useState<Record<string, string>>({});
  const [removedCustomFields, setRemovedCustomFields] = useState<string[]>([]);
  const [newCustomKey, setNewCustomKey] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open || !party) return;
    setName(party.name ?? "");
    setEmail(party.email ?? "");
    setPhone(parsePhoneBR(party.phone ?? ""));
    setDocValue(parseCpfCnpj(party.document ?? ""));
    setBirthDate(toDateInputValue(party.birth_date ?? ""));
    setCustomFields({ ...(party.custom_fields ?? {}) });
    setRemovedCustomFields([]);
    setNewCustomKey("");
    setSaving(false);
  }, [open, party]);

  const nameChanged = (party?.name ?? "") !== name;
  const emailChanged = (party?.email ?? "") !== email;
  const phoneChanged = parsePhoneBR(party?.phone ?? "") !== phone;
  const docChanged = parseCpfCnpj(party?.document ?? "") !== docValue;
  const birthChanged = toDateInputValue(party?.birth_date ?? "") !== birthDate;

  const origin = (
    changed: boolean,
    provenance: NonNullable<PartyDTO["field_provenance"]>[string] | undefined,
  ) =>
    changed ? (
      <span className="text-micro font-bold uppercase tracking-wide text-status-warning">
        Editado agora
      </span>
    ) : (
      <FieldProvenanceBadge provenance={provenance} />
    );

  const customFieldLabel = (key: string) =>
    key
      .replace(/_/g, " ")
      .replace(/(^|\s)(\S)/g, (_, prefix: string, first: string) =>
        prefix + first.toLocaleUpperCase("pt-BR"),
      );

  const removeCustomField = (key: string) => {
    setCustomFields((prev) => {
      const next = { ...prev };
      delete next[key];
      return next;
    });
    if (party?.custom_fields && key in party.custom_fields) {
      setRemovedCustomFields((prev) => (prev.includes(key) ? prev : [...prev, key]));
    }
  };

  const documentValid = !docValue || isValidCpfCnpj(docValue);
  const canSubmit = name.trim() !== "" && email.trim() !== "" && documentValid && !saving;

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
              <h2 className="text-lg font-bold text-foreground">Editar dados da parte</h2>
              <p className="text-sm text-muted-foreground mt-0.5">
                Complete ou corrija os dados de cadastro. O documento (CPF/CNPJ) é exigido para enviar o contrato à assinatura.
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
      <div className="flex-1 overflow-y-auto px-6 py-5 space-y-4">
        <div className="space-y-1">
          <div className="flex items-center gap-1.5">
            <Label>Nome</Label>
            {origin(nameChanged, party?.field_provenance?.name)}
          </div>
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Ex: Maria Silva" />
          {!name.trim() && <p className="text-xs text-status-critical mt-1">Nome é obrigatório.</p>}
        </div>
        <div className="space-y-1">
          <div className="flex items-center gap-1.5">
            <Label>Email</Label>
            {origin(emailChanged, party?.field_provenance?.email)}
          </div>
          <Input value={email} onChange={(e) => setEmail(e.target.value)} placeholder="maria@exemplo.com" />
          {!email.trim() && <p className="text-xs text-status-critical mt-1">E-mail é obrigatório.</p>}
        </div>
        <div className="space-y-1">
          <div className="flex items-center gap-1.5">
            <Label>Telefone</Label>
            {origin(phoneChanged, party?.field_provenance?.phone)}
          </div>
          <PhoneInput value={phone} onChange={setPhone} placeholder="(00) 00000-0000" className={inputClassNames} />
        </div>
        <div className="space-y-1">
          <div className="flex items-center gap-1.5">
            <Label>Data de nascimento</Label>
            {origin(birthChanged, party?.field_provenance?.birth_date)}
          </div>
          <Input
            type="date"
            value={birthDate}
            onChange={(e) => setBirthDate(e.target.value)}
            className={inputClassNames}
          />
          <p className="text-xs text-muted-foreground">
            Opcional. Usada na qualificação das partes e no contrato.
          </p>
        </div>
        <div className="space-y-1">
          <div className="flex items-center gap-1.5">
            <Label>Documento (CPF/CNPJ)</Label>
            {origin(docChanged, party?.field_provenance?.document)}
          </div>
          <DocumentInput
            value={docValue}
            onChange={setDocValue}
            placeholder="000.000.000-00"
            className={inputClassNames}
          />
          {docValue && !documentValid && <p className="text-xs text-status-critical mt-1">CPF/CNPJ inválido.</p>}
        </div>
        <div className="space-y-2 rounded-xl border border-border bg-muted/20 p-3">
          <div className="flex items-center gap-1.5">
            <Label>Outros dados da parte</Label>
            <span className="text-micro font-semibold uppercase tracking-wide text-muted-foreground">
              editável · com origem
            </span>
          </div>
          {Object.keys(customFields).length === 0 ? (
            <p className="text-xs text-muted-foreground">
              Nenhum campo adicional. Adicione um campo abaixo (ex.: profissão, endereço).
            </p>
          ) : (
            <div className="grid gap-3 sm:grid-cols-2">
              {Object.entries(customFields).map(([key, value]) => {
                const changed = (party?.custom_fields?.[key] ?? "") !== value;
                return (
                  <div key={key} className="min-w-0 space-y-1">
                    <div className="flex items-center justify-between gap-2">
                      <div className="flex min-w-0 items-center gap-1.5">
                        <Label className="truncate">{customFieldLabel(key)}</Label>
                        {origin(changed, party?.field_provenance?.[key])}
                      </div>
                      <button
                        type="button"
                        onClick={() => removeCustomField(key)}
                        className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-status-critical"
                        aria-label={`Remover ${customFieldLabel(key)}`}
                        title={`Remover ${customFieldLabel(key)}`}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </div>
                    <Input
                      value={value}
                      onChange={(e) =>
                        setCustomFields((prev) => ({ ...prev, [key]: e.target.value }))
                      }
                      placeholder="Não informado"
                    />
                  </div>
                );
              })}
            </div>
          )}
          <div className="flex items-center gap-2 pt-1">
            <Input
              value={newCustomKey}
              onChange={(e) => setNewCustomKey(e.target.value)}
              placeholder="Novo campo (ex.: profissão)"
              className="h-8"
            />
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="h-8 shrink-0"
              disabled={!newCustomKey.trim()}
              onClick={() => {
                const key = newCustomKey.trim();
                if (!key) return;
                setCustomFields((prev) => (key in prev ? prev : { ...prev, [key]: "" }));
                setRemovedCustomFields((prev) => prev.filter((removed) => removed !== key));
                setNewCustomKey("");
              }}
            >
              Adicionar
            </Button>
          </div>
        </div>
      </div>

      <div className="flex-none px-6 py-4 border-t border-border bg-card">
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
            Cancelar
          </Button>
          <Button
            disabled={!canSubmit}
            onClick={async () => {
              if (!party) return;
              setSaving(true);
              try {
                await onSave({
                  name: name.trim(),
                  email: email.trim(),
                  phone: phone.trim(),
                  document: docValue,
                  birth_date: birthDate,
                  custom_fields: customFields,
                  removed_custom_fields: removedCustomFields,
                });
                onOpenChange(false);
              } finally {
                setSaving(false);
              }
            }}
          >
            {saving ? "Salvando..." : "Salvar"}
          </Button>
        </div>
      </div>
    </ExpandableSidePanel>
  );
}

