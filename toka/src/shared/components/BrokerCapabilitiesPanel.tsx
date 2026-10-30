import { useEffect, useMemo, useState } from "react";
import { Settings, User, Activity, ShieldAlert, Clock, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { clsx } from "clsx";
import { ExpandableSidePanel } from "./ExpandableSidePanel";
import { Input } from "../ui/input";
import { Switch } from "../ui/switch";
import { Button } from "../ui/button";
import { WeeklyHoursGrid, type BusinessHoursValue } from "./WeeklyHoursGrid";
import { MultiSelectTagPicker } from "./MultiSelectTagPicker";
import { brokerService, type BrokerDTO, type BusinessHoursDTO } from "../services/broker.service";
import { ConfirmDialog } from "./ConfirmDialog";

export function BrokerCapabilitiesPanel(props: {
  open: boolean;
  onClose: () => void;
  broker: BrokerDTO | null;
  brokerSummary?: {
    status: "online" | "paused";
    slaPaused?: boolean;
    load?: number;
    maxLoad?: number;
    lastAssigned?: string;
  } | null;
  stagePoolsMap?: Record<string, string[]>;
  onToggleRouletteStatus?: () => void;
  toggling?: boolean;
  onOpenPoolsConfig?: () => void;
  onSaved?: () => void;
  tenantTimezone?: string;
  specialtyOptions?: string[];
  regionOptions?: string[];
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
  const [initialSnapshot, setInitialSnapshot] = useState<string>("");
  const [confirmCloseOpen, setConfirmCloseOpen] = useState(false);

  useEffect(() => {
    if (!b) return;
    const nextSpecialties = b.specialties ?? [];
    const nextRegions = b.regions ?? [];
    const nextWeight = typeof b.weight === "number" ? b.weight : 1;
    const nextIsPriority = !!b.is_priority;
    const nextMaxActiveLeads = typeof b.max_active_leads === "number" ? b.max_active_leads : 0;
    const nextPaused = !!b.paused;
    const nextBH = (b.business_hours as BusinessHoursDTO) ?? null;

    setSpecialties(nextSpecialties);
    setRegions(nextRegions);
    setWeight(nextWeight);
    setIsPriority(nextIsPriority);
    setMaxActiveLeads(nextMaxActiveLeads);
    setPaused(nextPaused);
    setBusinessHours(nextBH);

    const snap = JSON.stringify({
      specialties: Array.from(new Set(nextSpecialties.map((s) => String(s || "").trim()).filter(Boolean))).sort((a, c) => a.localeCompare(c)),
      regions: Array.from(new Set(nextRegions.map((r) => String(r || "").trim()).filter(Boolean))).sort((a, c) => a.localeCompare(c)),
      weight: nextWeight,
      isPriority: nextIsPriority,
      maxActiveLeads: nextMaxActiveLeads,
      paused: nextPaused,
      businessHours: nextBH ?? null,
    });
    setInitialSnapshot(snap);
  }, [b?.id]);

  const stageParticipation = useMemo(() => {
    if (!b?.id) return [];
    const map = props.stagePoolsMap ?? {};
    return Object.entries(map)
      .filter(([, ids]) => Array.isArray(ids) && ids.includes(b.id))
      .map(([stage]) => stage)
      .sort((a, c) => a.localeCompare(c));
  }, [b?.id, props.stagePoolsMap]);

  const snapshot = useMemo(() => {
    const sp = Array.from(new Set((specialties ?? []).map((s) => String(s || "").trim()).filter(Boolean))).sort((a, c) => a.localeCompare(c));
    const rg = Array.from(new Set((regions ?? []).map((r) => String(r || "").trim()).filter(Boolean))).sort((a, c) => a.localeCompare(c));
    const bh = businessHours ?? null;
    return JSON.stringify({
      specialties: sp,
      regions: rg,
      weight,
      isPriority,
      maxActiveLeads,
      paused,
      businessHours: bh,
    });
  }, [businessHours, isPriority, maxActiveLeads, paused, regions, specialties, weight]);

  const isDirty = !!b && initialSnapshot !== "" && snapshot !== initialSnapshot;

  const validation = useMemo(() => {
    const errors: string[] = [];
    if (!Number.isFinite(weight) || weight < 0) errors.push("Peso precisa ser um número válido (>= 0).");
    if (!Number.isFinite(maxActiveLeads) || maxActiveLeads < 0) errors.push("Máx. leads ativos precisa ser >= 0.");
    const bh = businessHours as any;
    if (bh?.windows && typeof bh.windows === "object") {
      Object.entries(bh.windows).forEach(([k, v]: any) => {
        const start = String(v?.start ?? "").trim();
        const end = String(v?.end ?? "").trim();
        if (!start || !end) errors.push(`Horário inválido no dia ${k}.`);
        if (start && end && start >= end) errors.push(`Horário inválido (início >= fim) no dia ${k}.`);
      });
    }
    return { ok: errors.length === 0, errors };
  }, [businessHours, maxActiveLeads, weight]);

  const requestClose = () => {
    if (saving) return;
    if (isDirty) {
      setConfirmCloseOpen(true);
      return;
    }
    props.onClose();
  };

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
      props.onClose();
    } catch {
      toast.error("Erro ao atualizar corretor");
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      <ExpandableSidePanel
        open={props.open}
        onClose={requestClose}
        defaultSize="2xl"
        expandedSize="full"
        zIndex={200}
        renderHeader={({ expandButton, closeButton }) => (
          <div className="px-6 py-4 border-b border-border flex items-center justify-between bg-card flex-shrink-0">
            <div className="flex items-center gap-3">
              <div className="p-2.5 rounded-xl bg-primary-50">
                <Settings className="w-5 h-5 text-primary-600" />
              </div>
              <div>
                <h2 className="text-lg font-bold text-foreground">Capacidade do corretor</h2>
                <p className="text-xs text-muted-foreground flex items-center gap-2">
                  <User className="w-3.5 h-3.5" />
                  {b?.name || "Selecione um corretor"}
                </p>
              </div>
            </div>
            <div className="flex items-center gap-2">
              {expandButton}
              <div onClick={requestClose}>{closeButton}</div>
            </div>
          </div>
        )}
      >
        <div className="flex-1 min-h-0 overflow-y-auto p-6 space-y-6 bg-muted/20">
          {!b ? (
            <div className="text-sm text-muted-foreground">Selecione um corretor</div>
          ) : (
            <>
              <div className="rounded-2xl border border-border bg-card p-4">
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <div className="text-sm font-bold text-foreground flex items-center gap-2">
                      <User className="w-4 h-4 text-muted-foreground" />
                      {b.name}
                    </div>
                    <div className="text-xs text-muted-foreground mt-1">
                      Papel: <span className="font-bold text-foreground">{b.role}</span>
                      {b.email ? <span className="text-muted-foreground"> • {b.email}</span> : null}
                    </div>
                  </div>
                  {props.onToggleRouletteStatus && props.brokerSummary?.status && (
                    <Button
                      type="button"
                      variant="outline"
                      disabled={props.toggling}
                      onClick={props.onToggleRouletteStatus}
                    >
                      {props.toggling ? (
                        <Loader2 className="w-4 h-4 animate-spin" />
                      ) : props.brokerSummary.status === "paused" ? (
                        "Ativar na roleta"
                      ) : (
                        "Pausar na roleta"
                      )}
                    </Button>
                  )}
                </div>

                <div className="mt-4 grid grid-cols-1 sm:grid-cols-3 gap-3">
                  <div className="rounded-xl border border-border bg-muted/30 p-3">
                    <div className="text-xs font-bold text-muted-foreground flex items-center gap-2">
                      <Activity className="w-3.5 h-3.5" /> Carga
                    </div>
                    <div className="mt-1 text-sm font-bold text-foreground">
                      {typeof props.brokerSummary?.load === "number" ? props.brokerSummary.load : "—"}
                      <span className="text-muted-foreground font-medium">
                        {" "}
                        /{" "}
                        {props.brokerSummary?.maxLoad && props.brokerSummary.maxLoad > 0 ? props.brokerSummary.maxLoad : "∞"}
                      </span>
                    </div>
                  </div>
                  <div className="rounded-xl border border-border bg-muted/30 p-3">
                    <div className="text-xs font-bold text-muted-foreground flex items-center gap-2">
                      <Clock className="w-3.5 h-3.5" /> Último lead
                    </div>
                    <div className="mt-1 text-sm font-bold text-foreground">{props.brokerSummary?.lastAssigned ?? "—"}</div>
                  </div>
                  <div className="rounded-xl border border-border bg-muted/30 p-3">
                    <div className="text-xs font-bold text-muted-foreground flex items-center gap-2">
                      <ShieldAlert className="w-3.5 h-3.5" /> Pool por etapa
                    </div>
                    <div className="mt-1 text-sm font-bold text-foreground">
                      {stageParticipation.length > 0 ? stageParticipation.slice(0, 2).join(", ") : "Padrão"}
                      {stageParticipation.length > 2 ? (
                        <span className="text-muted-foreground font-medium"> +{stageParticipation.length - 2}</span>
                      ) : null}
                    </div>
                    {props.onOpenPoolsConfig && (
                      <button
                        type="button"
                        onClick={props.onOpenPoolsConfig}
                        className="mt-2 text-xs font-bold text-primary-600 hover:text-primary-700"
                      >
                        Ver pools
                      </button>
                    )}
                  </div>
                </div>
              </div>

              {!validation.ok && (
                <div className="rounded-2xl border border-status-critical/30 bg-status-critical-soft p-4">
                  <div className="text-sm font-bold text-status-critical">Ajustes necessários</div>
                  <div className="mt-2 space-y-1 text-xs text-status-critical">
                    {validation.errors.slice(0, 4).map((e, idx) => (
                      <div key={idx}>{e}</div>
                    ))}
                  </div>
                </div>
              )}

              <div className="grid gap-5">
                <MultiSelectTagPicker
                  label="Especialidades"
                  value={specialties}
                  options={props.specialtyOptions ?? []}
                  onChange={setSpecialties}
                  placeholder="Selecionar especialidades..."
                  allowCreate={false}
                />
                <MultiSelectTagPicker
                  label="Regiões"
                  value={regions}
                  options={props.regionOptions ?? []}
                  onChange={setRegions}
                  placeholder="Selecionar regiões..."
                  allowCreate
                />

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <div className="text-xs font-bold text-muted-foreground">Peso</div>
                    <Input type="number" min={0} step={0.1} value={Number.isFinite(weight) ? String(weight) : "1"} onChange={(e) => setWeight(Number(e.target.value))} />
                  </div>
                  <div className="space-y-2">
                    <div className="text-xs font-bold text-muted-foreground">Máx. leads ativos (0 = ilimitado)</div>
                    <Input type="number" min={0} step={1} value={String(maxActiveLeads)} onChange={(e) => setMaxActiveLeads(Number(e.target.value))} />
                  </div>
                </div>

                <div
                  className="flex items-center justify-between rounded-xl border border-border bg-muted/30 p-4 cursor-pointer select-none"
                  role="button"
                  tabIndex={0}
                  onClick={() => setIsPriority((p) => !p)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      setIsPriority((p) => !p);
                    }
                  }}
                >
                  <div>
                    <div className="text-sm font-bold text-foreground">Prioridade</div>
                    <div className="text-xs text-muted-foreground">Recebe leads primeiro quando o método é prioridade</div>
                  </div>
                  <div onClick={(e) => e.stopPropagation()}>
                    <Switch
                      checked={isPriority}
                      onCheckedChange={setIsPriority}
                      className="border border-border data-[state=unchecked]:bg-muted data-[state=checked]:bg-primary"
                    />
                  </div>
                </div>

                <div
                  className="flex items-center justify-between rounded-xl border border-border bg-muted/30 p-4 cursor-pointer select-none"
                  role="button"
                  tabIndex={0}
                  onClick={() => setPaused((p) => !p)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      setPaused((p) => !p);
                    }
                  }}
                >
                  <div>
                    <div className="text-sm font-bold text-foreground">Pausado (SLA)</div>
                    <div className="text-xs text-muted-foreground">Bloqueia elegibilidade na roleta via regra do corretor</div>
                  </div>
                  <div onClick={(e) => e.stopPropagation()}>
                    <Switch checked={paused} onCheckedChange={setPaused} className="border border-border data-[state=unchecked]:bg-muted data-[state=checked]:bg-primary" />
                  </div>
                </div>

                <div
                  className={clsx(
                    "flex items-center justify-between rounded-xl border border-border bg-muted/30 p-4 select-none",
                    props.onToggleRouletteStatus && !props.toggling ? "cursor-pointer" : "opacity-70 cursor-not-allowed",
                  )}
                  role="button"
                  tabIndex={0}
                  onClick={() => {
                    if (props.toggling) return;
                    props.onToggleRouletteStatus?.();
                  }}
                  onKeyDown={(e) => {
                    if (props.toggling) return;
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      props.onToggleRouletteStatus?.();
                    }
                  }}
                >
                  <div>
                    <div className="text-sm font-bold text-foreground">Ativo na roleta</div>
                    <div className="text-xs text-muted-foreground">Controla se o corretor entra na fila de distribuição</div>
                  </div>
                  <div onClick={(e) => e.stopPropagation()}>
                    {props.toggling ? (
                      <Loader2 className="w-5 h-5 animate-spin text-muted-foreground" />
                    ) : (
                      <Switch
                        checked={props.brokerSummary?.status === "online"}
                        onCheckedChange={() => props.onToggleRouletteStatus?.()}
                        className="border border-border data-[state=unchecked]:bg-muted data-[state=checked]:bg-primary"
                        disabled={!props.onToggleRouletteStatus}
                      />
                    )}
                  </div>
                </div>

                <div className="rounded-xl border border-border bg-card p-4">
                  <div className="text-sm font-bold text-foreground">Horário comercial</div>
                  <div className="mt-3">
                    <WeeklyHoursGrid value={businessHours} onChange={setBusinessHours} timezone={props.tenantTimezone} />
                  </div>
                </div>
              </div>
            </>
          )}
        </div>

        <div className="p-4 border-t border-border bg-card flex items-center justify-end gap-2 flex-shrink-0">
          <Button type="button" variant="outline" onClick={requestClose} disabled={saving}>
            Cancelar
          </Button>
          <Button type="button" onClick={save} disabled={saving || !b || !isDirty || !validation.ok} className={clsx(saving && "opacity-80")}>
            {saving ? "Salvando..." : "Salvar"}
          </Button>
        </div>
      </ExpandableSidePanel>
      <ConfirmDialog
        open={confirmCloseOpen}
        title="Descartar alterações?"
        description="Você tem alterações não salvas. Ao fechar, elas serão perdidas."
        confirmLabel="Descartar"
        cancelLabel="Continuar editando"
        variant="warning"
        onConfirm={() => {
          setConfirmCloseOpen(false);
          props.onClose();
        }}
        onCancel={() => setConfirmCloseOpen(false)}
      />
    </>
  );
}

