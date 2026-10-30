import { useEffect, useMemo, useState } from "react";
import type { PartyDTO, PartyRepresentationDTO, RepresentationType, PartyRole } from "../services/deal.service";
import { ExpandableSidePanel } from "./ExpandableSidePanel";
import { Button } from "../ui/button";
import { Checkbox } from "../ui/checkbox";
import { Label } from "../ui/label";

const typeLabel: Record<RepresentationType, string> = {
  attorney: "Procurador",
  legal_representative: "Representante legal",
  guardian: "Tutor",
  curator: "Curador",
};

const roleFromType: Record<RepresentationType, PartyRole> = {
  attorney: "attorney",
  legal_representative: "legal_representative",
  guardian: "guardian",
  curator: "curator",
};

export function PartyRepresentationPanel({
  open,
  onOpenChange,
  party,
  parties,
  onSave,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  party: PartyDTO | null;
  parties: PartyDTO[];
  onSave: (representation: PartyRepresentationDTO | null) => Promise<void> | void;
}) {
  const [type, setType] = useState<RepresentationType | "">("");
  const [representativeId, setRepresentativeId] = useState("");
  const [requiresPrincipalSignature, setRequiresPrincipalSignature] = useState(false);
  const [allowRepresentativeProposal, setAllowRepresentativeProposal] = useState(true);

  useEffect(() => {
    if (!party) return;
    if (!open) return;
    const rep = party.representation;
    if (!rep) {
      setType("");
      setRepresentativeId("");
      setRequiresPrincipalSignature(false);
      setAllowRepresentativeProposal(true);
      return;
    }
    setType(rep.type);
    setRepresentativeId(rep.representative_party_id);
    setRequiresPrincipalSignature(!!rep.requires_principal_signature);
    setAllowRepresentativeProposal(rep.allow_representative_proposal !== false);
  }, [open, party]);

  const availableRepresentatives = useMemo(() => {
    if (!type) return [];
    const role = roleFromType[type];
    return parties.filter((p) => p.role === role && p.id !== party?.id);
  }, [parties, party?.id, type]);

  const canSave =
    !!party && (type === "" || (!!representativeId && availableRepresentatives.some((p) => p.id === representativeId)));

  return (
    <ExpandableSidePanel
      open={open}
      onClose={() => onOpenChange(false)}
      defaultSize="lg"
      expandedSize="2xl"
      zIndex={70}
      renderHeader={({ expandButton, closeButton }) => (
        <div className="flex-none px-6 py-5 border-b border-border bg-card">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <h2 className="text-lg font-bold text-foreground">Representação legal</h2>
              <p className="text-sm text-muted-foreground mt-0.5">
                Configure procurador/representante legal e os requisitos de assinatura e proposta.
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
        {!party ? (
          <div className="p-4 border border-border rounded-2xl bg-card text-sm text-muted-foreground">Nenhuma parte selecionada.</div>
        ) : (
          <>
            <div className="space-y-1">
              <Label>Parte</Label>
              <div className="px-3 py-2 rounded-xl border border-border bg-muted text-sm font-semibold text-foreground">
                {party.name || party.email || party.id}
              </div>
            </div>

            <div className="space-y-1">
              <Label>Tipo de representação</Label>
              <select
                value={type}
                onChange={(e) => {
                  const next = e.target.value as any;
                  setType(next);
                  setRepresentativeId("");
                }}
                className="h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="">Sem representação</option>
                <option value="attorney">{typeLabel.attorney}</option>
                <option value="legal_representative">{typeLabel.legal_representative}</option>
                <option value="guardian">{typeLabel.guardian}</option>
                <option value="curator">{typeLabel.curator}</option>
              </select>
            </div>

            {type !== "" && (
              <>
                <div className="space-y-1">
                  <Label>Representante</Label>
                  <select
                    value={representativeId}
                    onChange={(e) => setRepresentativeId(e.target.value)}
                    className="h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                  >
                    <option value="">Selecione…</option>
                    {availableRepresentatives.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.name || p.email || p.id}
                      </option>
                    ))}
                  </select>
                  {availableRepresentatives.length === 0 && (
                    <div className="text-xs text-muted-foreground">
                      Adicione uma parte com papel {typeLabel[type]} para aparecer aqui.
                    </div>
                  )}
                </div>

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-1">
                  <div className="space-y-1">
                    <Label>Assinatura</Label>
                    <div className="h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm flex items-center gap-2">
                      <Checkbox checked={requiresPrincipalSignature} onCheckedChange={(v) => setRequiresPrincipalSignature(Boolean(v))} />
                      <span>Principal também assina</span>
                    </div>
                  </div>

                  <div className="space-y-1">
                    <Label>Proposta</Label>
                    <div className="h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm flex items-center gap-2">
                      <Checkbox checked={allowRepresentativeProposal} onCheckedChange={(v) => setAllowRepresentativeProposal(Boolean(v))} />
                      <span>Representante pode aprovar</span>
                    </div>
                  </div>
                </div>
              </>
            )}
          </>
        )}
      </div>

      <div className="flex-none px-6 py-4 border-t border-border bg-card">
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancelar
          </Button>
          <Button
            disabled={!canSave}
            onClick={async () => {
              if (!party) return;
              if (type === "") {
                await onSave(null);
                onOpenChange(false);
                return;
              }
              await onSave({
                type,
                representative_party_id: representativeId,
                requires_principal_signature: requiresPrincipalSignature,
                allow_representative_proposal: allowRepresentativeProposal,
              });
              onOpenChange(false);
            }}
          >
            Salvar
          </Button>
        </div>
      </div>
    </ExpandableSidePanel>
  );
}

