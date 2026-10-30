import { useEffect, useMemo, useState } from "react";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "./ui/dialog";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Switch } from "./ui/switch";
import { toast } from "sonner";
import { brokerService, type BrokerDTO, type BusinessHoursDTO } from "../services/broker.service";
import { WeeklyHoursGrid, type BusinessHoursValue } from "./WeeklyHoursGrid";

function TagsInput(props: { value: string[]; onChange: (v: string[]) => void; placeholder?: string }) {
  const [text, setText] = useState("");
  const normalized = useMemo(() => (props.value ?? []).map((t) => String(t).trim()).filter(Boolean), [props.value]);

  const add = (raw: string) => {
    const v = raw.trim();
    if (!v) return;
    if (normalized.includes(v)) return;
    props.onChange([...normalized, v]);
  };

  return (
    <div className="rounded-xl border border-border bg-card p-3">
      <div className="flex flex-wrap gap-2">
        {normalized.map((t) => (
          <button
            key={t}
            type="button"
            className="inline-flex items-center gap-2 rounded-lg bg-muted px-2 py-1 text-xs font-bold text-foreground"
            onClick={() => props.onChange(normalized.filter((x) => x !== t))}
            aria-label={`Remover ${t}`}
          >
            {t}
            <span className="text-muted-foreground">×</span>
          </button>
        ))}
      </div>
      <div className="mt-2 flex gap-2">
        <Input
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder={props.placeholder ?? "Adicionar..."}
          onKeyDown={(e) => {
            if (e.key === "Enter" || e.key === ",") {
              e.preventDefault();
              add(text);
              setText("");
            }
          }}
          aria-label={props.placeholder ?? "Tags"}
        />
        <Button
          type="button"
          variant="secondary"
          onClick={() => {
            add(text);
            setText("");
          }}
        >
          Adicionar
        </Button>
      </div>
    </div>
  );
}

export function BrokerCapabilitiesDialog(props: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  broker: BrokerDTO | null;
  onSaved?: () => void;
  tenantTimezone?: string;
}) {
  const b = props.broker;
  const [specialties, setSpecialties] = useState<string[]>([]);
  const [regions, setRegions] = useState<string[]>([]);
  const [weight, setWeight] = useState<number>(1);
  const [isPriority, setIsPriority] = useState<boolean>(false);
  const [maxActiveLeads, setMaxActiveLeads] = useState<number>(0);
  const [paused, setPaused] = useState<boolean>(false);
  const [businessHours, setBusinessHours] = useState<BusinessHoursValue>(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!b) return;
    setSpecialties(b.specialties ?? []);
    setRegions(b.regions ?? []);
    setWeight(typeof b.weight === "number" ? b.weight : 1);
    setIsPriority(!!b.is_priority);
    setMaxActiveLeads(typeof b.max_active_leads === "number" ? b.max_active_leads : 0);
    setPaused(!!b.paused);
    setBusinessHours((b.business_hours as BusinessHoursDTO) ?? null);
  }, [b?.id]);

  const save = async () => {
    if (!b) return;
    setSaving(true);
    try {
      await brokerService.update(b.id, {
        specialties,
        regions,
        weight,
        is_priority: isPriority,
        max_active_leads: maxActiveLeads,
        paused,
        business_hours: businessHours ?? undefined,
      });
      toast.success("Corretor atualizado");
      props.onSaved?.();
      props.onOpenChange(false);
    } catch {
      toast.error("Erro ao atualizar corretor");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-[720px]">
        <DialogHeader>
          <DialogTitle>Capacidade do corretor</DialogTitle>
        </DialogHeader>

        {!b ? (
          <div className="text-sm text-muted-foreground">Selecione um corretor</div>
        ) : (
          <div className="grid gap-4 py-2">
            <div className="grid gap-2">
              <div className="text-xs font-bold text-muted-foreground">Especialidades</div>
              <TagsInput value={specialties} onChange={setSpecialties} placeholder="Adicionar especialidade" />
            </div>

            <div className="grid gap-2">
              <div className="text-xs font-bold text-muted-foreground">Regiões</div>
              <TagsInput value={regions} onChange={setRegions} placeholder="Adicionar região" />
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              <div className="grid gap-2">
                <div className="text-xs font-bold text-muted-foreground">Peso</div>
                <Input
                  type="number"
                  min={0}
                  step={0.1}
                  value={Number.isFinite(weight) ? String(weight) : "1"}
                  onChange={(e) => setWeight(Number(e.target.value))}
                  aria-label="Peso"
                />
              </div>
              <div className="grid gap-2">
                <div className="text-xs font-bold text-muted-foreground">Máx. leads ativos (0 = ilimitado)</div>
                <Input
                  type="number"
                  min={0}
                  step={1}
                  value={String(maxActiveLeads)}
                  onChange={(e) => setMaxActiveLeads(Number(e.target.value))}
                  aria-label="Máx. leads ativos"
                />
              </div>
            </div>

            <div className="flex items-center justify-between rounded-xl border border-border bg-muted/30 p-3">
              <div>
                <div className="text-sm font-bold text-foreground">Prioridade</div>
                <div className="text-xs text-muted-foreground">Recebe leads primeiro quando o método é prioridade</div>
              </div>
              <Switch checked={isPriority} onCheckedChange={setIsPriority} aria-label="Prioridade" />
            </div>

            <div className="flex items-center justify-between rounded-xl border border-border bg-muted/30 p-3">
              <div>
                <div className="text-sm font-bold text-foreground">Pausado</div>
                <div className="text-xs text-muted-foreground">Pausa recebimento até retomar manualmente</div>
              </div>
              <Switch checked={paused} onCheckedChange={setPaused} aria-label="Pausado" />
            </div>

            <div className="rounded-xl border border-border bg-card p-3">
              <div className="text-sm font-bold text-foreground">Horário comercial</div>
              <div className="mt-3">
                <WeeklyHoursGrid value={businessHours} onChange={setBusinessHours} timezone={props.tenantTimezone} />
              </div>
            </div>
          </div>
        )}

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => props.onOpenChange(false)} disabled={saving}>
            Cancelar
          </Button>
          <Button type="button" onClick={save} disabled={saving || !b}>
            {saving ? "Salvando..." : "Salvar"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

