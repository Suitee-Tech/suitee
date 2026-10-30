import { useEffect, useState } from "react";
import { Clock, RotateCcw, Save } from "lucide-react";
import { toast } from "sonner";
import { leadService, type BusinessHours } from "../services/lead.service";
import { WeeklyHoursGrid } from "./WeeklyHoursGrid";
import { clsx } from "clsx";

type Snapshot = { globalBusinessHours: BusinessHours | null; simulationSeed: number };

export function RouletteGlobalConfig() {
  const [globalBusinessHours, setGlobalBusinessHours] = useState<BusinessHours | null>(null);
  const [simulationSeed, setSimulationSeed] = useState<number>(0);
  const [initialSnapshot, setInitialSnapshot] = useState<Snapshot | null>(null);
  const [dirty, setDirty] = useState(false);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    (async () => {
      try {
        const cfg = await leadService.getRouletteConfig();
        const snap: Snapshot = {
          globalBusinessHours: (cfg as any)?.globalBusinessHours ?? null,
          simulationSeed: typeof (cfg as any)?.simulationSeed === "number" ? (cfg as any).simulationSeed : 0,
        };
        setGlobalBusinessHours(snap.globalBusinessHours);
        setSimulationSeed(snap.simulationSeed);
        setInitialSnapshot(snap);
        setDirty(false);
      } catch {
        setInitialSnapshot({ globalBusinessHours: null, simulationSeed: 0 });
      }
    })();
  }, []);

  const handleSave = async () => {
    try {
      setLoading(true);
      await leadService.updateRouletteConfig({
        globalBusinessHours: globalBusinessHours ?? undefined,
        simulationSeed: simulationSeed > 0 ? simulationSeed : undefined,
      });
      setInitialSnapshot({ globalBusinessHours, simulationSeed });
      setDirty(false);
      toast.success("Configuração global salva");
    } catch {
      toast.error("Erro ao salvar configuração global");
    } finally {
      setLoading(false);
    }
  };

  const handleReset = () => {
    if (!initialSnapshot) return;
    setGlobalBusinessHours(initialSnapshot.globalBusinessHours);
    setSimulationSeed(initialSnapshot.simulationSeed);
    setDirty(false);
    toast.info("Configuração restaurada (não salvo)");
  };

  return (
    <div className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden">
      <div className="p-4 border-b border-border bg-muted/50">
        <div className="flex items-center justify-between gap-3">
          <h3 className="text-base font-bold text-foreground flex items-center gap-2">
            <Clock className="w-5 h-5 text-primary-600" /> Horário Global
          </h3>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={handleReset}
              disabled={!dirty || loading}
              className={clsx(
                "inline-flex items-center gap-2 px-3 py-1.5 rounded-lg text-xs font-bold border transition-colors",
                !dirty || loading
                  ? "bg-card border-border text-muted-foreground opacity-60 cursor-not-allowed"
                  : "bg-card border-border text-foreground hover:bg-muted",
              )}
            >
              <RotateCcw className="w-3.5 h-3.5" /> Restaurar
            </button>
            <button
              type="button"
              onClick={handleSave}
              disabled={!dirty || loading}
              className={clsx(
                "inline-flex items-center gap-2 px-3 py-1.5 rounded-lg text-xs font-bold transition-colors",
                !dirty || loading ? "bg-primary/60 text-primary-foreground cursor-not-allowed" : "bg-primary text-primary-foreground hover:bg-primary/90",
              )}
            >
              <Save className="w-3.5 h-3.5" /> Salvar
            </button>
          </div>
        </div>
        <p className="text-xs text-muted-foreground mt-1">
          Define um horário comercial global (fallback quando o corretor não tiver horário próprio) e uma seed padrão para simulações.
        </p>
      </div>
      <div className="p-4 space-y-4">
        <div className="rounded-xl border border-border bg-muted/30 p-4">
          <div className="text-sm font-bold text-foreground mb-3">Horário comercial global</div>
          <WeeklyHoursGrid
            value={globalBusinessHours}
            onChange={(next) => {
              setGlobalBusinessHours(next as any);
              setDirty(true);
            }}
          />
        </div>
        <div className="rounded-xl border border-border bg-muted/30 p-4">
          <div className="text-sm font-bold text-foreground mb-2">Simulation seed (0 = aleatório)</div>
          <input
            type="number"
            min={0}
            value={simulationSeed}
            onChange={(e) => {
              setSimulationSeed(parseInt(e.target.value || "0", 10));
              setDirty(true);
            }}
            className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none"
          />
        </div>
      </div>
    </div>
  );
}

