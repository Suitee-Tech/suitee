import { useEffect, useMemo, useState } from "react";
import {
  ArrowRight,
  ChevronDown,
  ChevronRight,
  ImageIcon,
  MinusCircle,
  PlusCircle,
  ShieldCheck,
  TrendingDown,
  TrendingUp,
} from "lucide-react";
import { clsx } from "clsx";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "../../ui/dialog";
import { Button } from "../../ui/button";
import { inspectionService, type Inspection, type Finding } from "../../services/inspection.service";
import { MediaImage } from "../MediaImage";
import { diffInspections, groupByRoom, summarize, type DiffPair, type DiffStatus } from "./inspectionDiff";

interface Props {
  entryId: string;
  exitId: string;
  open: boolean;
  onClose: () => void;
}

export function InspectionDiffDialog({ entryId, exitId, open, onClose }: Props) {
  const [entry, setEntry] = useState<Inspection | null>(null);
  const [exitInsp, setExitInsp] = useState<Inspection | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [lightbox, setLightbox] = useState<{ url: string; alt: string } | null>(null);
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setLoading(true);
    setError(null);
    Promise.all([inspectionService.getById(entryId), inspectionService.getById(exitId)])
      .then(([a, b]) => {
        if (cancelled) return;
        setEntry(a?.data || null);
        setExitInsp(b?.data || null);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(err?.message || "Erro ao carregar vistorias.");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [open, entryId, exitId]);

  const pairs = useMemo(() => diffInspections(entry?.findings || [], exitInsp?.findings || []), [entry, exitInsp]);
  const summary = useMemo(() => summarize(pairs), [pairs]);
  const groups = useMemo(() => groupByRoom(pairs), [pairs]);

  const toggleRoom = (room: string) => setCollapsed((c) => ({ ...c, [room]: !c[room] }));

  return (
    <>
      <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
        <DialogContent className="max-w-6xl max-h-[92vh] flex flex-col overflow-hidden">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              Comparação Entrada × Saída
              {entry && exitInsp && (
                <span className="text-caption font-mono text-muted-foreground">
                  {entry.id.slice(0, 6)} → {exitInsp.id.slice(0, 6)}
                </span>
              )}
            </DialogTitle>
            <p className="text-caption text-muted-foreground">
              Itens em "Piorou" indicam danos atribuíveis ao locatário. "Sem par" exige revisão do checklist (item esquecido
              ou novo dano).
            </p>
          </DialogHeader>

          <div className="flex-1 overflow-y-auto -mx-6 px-6 py-4 space-y-4">
            {loading && <div className="text-center py-12 text-sm text-muted-foreground">Carregando vistorias…</div>}

            {error && (
              <div className="rounded-xl border border-status-critical/30 bg-status-critical-soft p-4 text-sm">
                <p className="font-bold text-status-critical">Falha ao carregar</p>
                <p className="text-caption text-status-critical/85 mt-0.5">{error}</p>
              </div>
            )}

            {!loading && !error && entry && exitInsp && (
              <>
                <DiffSummaryCards summary={summary} />
                {groups.length === 0 ? (
                  <div className="text-center py-8 text-sm text-muted-foreground">Nenhum item nas vistorias para comparar.</div>
                ) : (
                  groups.map((g) => (
                    <RoomDiffBlock
                      key={g.room}
                      room={g.room}
                      pairs={g.pairs}
                      collapsed={!!collapsed[g.room]}
                      onToggle={() => toggleRoom(g.room)}
                      onOpenPhoto={setLightbox}
                    />
                  ))
                )}
              </>
            )}
          </div>

          <DialogFooter className="pt-3 border-t border-border">
            <Button variant="outline" onClick={onClose}>
              Fechar
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {lightbox && (
        <Dialog open onOpenChange={(o) => !o && setLightbox(null)}>
          <DialogContent className="max-w-5xl p-0 overflow-hidden bg-black border-0">
            <MediaImage src={lightbox.url} alt={lightbox.alt} className="w-full h-auto max-h-[85vh] object-contain bg-black" />
          </DialogContent>
        </Dialog>
      )}
    </>
  );
}

function DiffSummaryCards({ summary }: { summary: ReturnType<typeof summarize> }) {
  return (
    <div className="grid grid-cols-2 sm:grid-cols-5 gap-3">
      <SummaryCard
        label="Piorou"
        count={summary.worsened}
        icon={<TrendingDown className="w-4 h-4" />}
        tone="critical"
        hint="Itens com condição menor que a vistoria de entrada — base para cálculo de multa."
      />
      <SummaryCard
        label="Inalterado"
        count={summary.unchanged}
        icon={<ShieldCheck className="w-4 h-4" />}
        tone="ok"
        hint="Estado preservado durante a locação."
      />
      <SummaryCard
        label="Melhorou"
        count={summary.improved}
        icon={<TrendingUp className="w-4 h-4" />}
        tone="info"
        hint="Reformas / melhorias pelo locatário durante o contrato."
      />
      <SummaryCard
        label="Sem par (entrada)"
        count={summary.missingInExit}
        icon={<MinusCircle className="w-4 h-4" />}
        tone="warning"
        hint="Existia na entrada mas não na saída — item esquecido?"
      />
      <SummaryCard
        label="Sem par (saída)"
        count={summary.addedInExit}
        icon={<PlusCircle className="w-4 h-4" />}
        tone="warning"
        hint="Apareceu só na saída — provavelmente dano novo."
      />
    </div>
  );
}

interface SummaryCardProps {
  label: string;
  count: number;
  icon: React.ReactNode;
  tone: "critical" | "ok" | "info" | "warning";
  hint: string;
}

function SummaryCard({ label, count, icon, tone, hint }: SummaryCardProps) {
  const palette = {
    critical: "bg-status-critical-soft text-status-critical border-status-critical/30",
    ok: "bg-status-ok-soft text-status-ok border-status-ok/30",
    info: "bg-status-info-soft text-status-info border-status-info/30",
    warning: "bg-status-warning-soft text-status-warning border-status-warning/30",
  }[tone];

  return (
    <div className={clsx("rounded-xl border p-3", palette)} title={hint}>
      <div className="flex items-center gap-1.5 text-micro font-bold uppercase tracking-wide">
        {icon}
        {label}
      </div>
      <div className="text-2xl font-bold tabular-nums mt-1">{count}</div>
    </div>
  );
}

interface RoomDiffBlockProps {
  room: string;
  pairs: DiffPair[];
  collapsed: boolean;
  onToggle: () => void;
  onOpenPhoto: (p: { url: string; alt: string }) => void;
}

function RoomDiffBlock({ room, pairs, collapsed, onToggle, onOpenPhoto }: RoomDiffBlockProps) {
  const worsenedCount = pairs.filter((p) => p.status === "worsened" || p.status === "added_in_exit").length;

  return (
    <section className="rounded-xl border border-border overflow-hidden">
      <header className="flex items-center justify-between px-4 py-2.5 bg-muted/40 border-b border-border">
        <button onClick={onToggle} className="flex items-center gap-2 flex-1 text-left">
          {collapsed ? (
            <ChevronRight className="w-4 h-4 text-muted-foreground" />
          ) : (
            <ChevronDown className="w-4 h-4 text-muted-foreground" />
          )}
          <h4 className="text-sm font-bold text-foreground">{room}</h4>
          <span
            className={clsx(
              "text-micro font-bold px-1.5 py-0.5 rounded-full tabular-nums",
              worsenedCount > 0 ? "bg-status-critical-soft text-status-critical" : "bg-status-ok-soft text-status-ok",
            )}
          >
            {worsenedCount === 0 ? `${pairs.length} itens` : `${worsenedCount}/${pairs.length} c/ problema`}
          </span>
        </button>
      </header>

      {!collapsed && (
        <div className="divide-y divide-border bg-card">
          {pairs.map((p) => (
            <PairRow key={p.key} pair={p} onOpenPhoto={onOpenPhoto} />
          ))}
        </div>
      )}
    </section>
  );
}

function PairRow({ pair, onOpenPhoto }: { pair: DiffPair; onOpenPhoto: (p: { url: string; alt: string }) => void }) {
  return (
    <div className="px-4 py-3">
      <div className="flex items-center justify-between gap-3 mb-2">
        <h5 className="text-sm font-bold text-foreground">{pair.description}</h5>
        <DiffStatusBadge status={pair.status} />
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <SidePanel label="Entrada" finding={pair.entry} missing={!pair.entry} tone="info" onOpenPhoto={onOpenPhoto} />
        <SidePanel
          label="Saída"
          finding={pair.exit}
          missing={!pair.exit}
          tone={pair.status === "worsened" || pair.status === "added_in_exit" ? "critical" : "info"}
          onOpenPhoto={onOpenPhoto}
        />
      </div>
    </div>
  );
}

function DiffStatusBadge({ status }: { status: DiffStatus }) {
  const cfg: Record<DiffStatus, { label: string; cls: string; Icon: React.ComponentType<{ className?: string }> }> = {
    worsened: { label: "Piorou", cls: "bg-status-critical text-white", Icon: TrendingDown },
    unchanged: { label: "Inalterado", cls: "bg-status-ok-soft text-status-ok border border-status-ok/30", Icon: ShieldCheck },
    improved: { label: "Melhorou", cls: "bg-status-info-soft text-status-info border border-status-info/30", Icon: TrendingUp },
    missing_in_exit: {
      label: "Sem par na saída",
      cls: "bg-status-warning-soft text-status-warning border border-status-warning/30",
      Icon: MinusCircle,
    },
    added_in_exit: {
      label: "Novo na saída",
      cls: "bg-status-warning-soft text-status-warning border border-status-warning/30",
      Icon: PlusCircle,
    },
  };
  const { label, cls, Icon } = cfg[status];
  return (
    <span className={clsx("inline-flex items-center gap-1 px-2 py-0.5 rounded text-micro font-bold tabular-nums", cls)}>
      <Icon className="w-3.5 h-3.5" /> {label}
    </span>
  );
}

interface SidePanelProps {
  label: string;
  finding?: Finding;
  missing: boolean;
  tone: "info" | "critical";
  onOpenPhoto: (p: { url: string; alt: string }) => void;
}

function SidePanel({ label, finding, missing, tone, onOpenPhoto }: SidePanelProps) {
  return (
    <div className={clsx("rounded-lg border p-3", missing ? "border-dashed border-border bg-muted/30" : "border-border bg-muted/20")}>
      <div className="flex items-center justify-between mb-2">
        <span className="text-micro font-bold text-muted-foreground uppercase tracking-wide">{label}</span>
        {!missing && finding?.condition && (
          <span className={clsx("text-micro font-bold px-1.5 py-0.5 rounded border tabular-nums", conditionStyle(finding.condition))}>
            {finding.condition}
          </span>
        )}
      </div>

      {missing ? (
        <p className="text-caption text-muted-foreground italic">
          {label === "Entrada" ? "Sem registro na vistoria de entrada." : "Sem registro na vistoria de saída."}
        </p>
      ) : (
        <>
          {finding?.notes && <p className="text-caption text-foreground italic mb-2">"{finding.notes}"</p>}
          {finding && finding.photos && finding.photos.length > 0 ? (
            <div className="flex flex-wrap gap-2">
              {finding.photos.map((url) => (
                <button
                  key={url}
                  type="button"
                  onClick={() => onOpenPhoto({ url, alt: `${label}: ${finding.description}` })}
                  className="w-14 h-14 rounded-md overflow-hidden border border-border bg-muted hover:opacity-90"
                >
                  <MediaImage src={url} alt="" className="w-full h-full object-cover" loading="lazy" />
                </button>
              ))}
            </div>
          ) : (
            <p className="text-caption text-muted-foreground inline-flex items-center gap-1">
              <ImageIcon className="w-3.5 h-3.5" /> Sem fotos
            </p>
          )}
        </>
      )}
      <ArrowRight className={clsx("w-0 h-0", tone === "critical" ? "opacity-100" : "opacity-0")} />
    </div>
  );
}

function conditionStyle(condition: string): string {
  switch (condition) {
    case "Ótimo":
      return "bg-status-ok-soft text-status-ok border-status-ok/30";
    case "Bom":
      return "bg-status-info-soft text-status-info border-status-info/30";
    case "Regular":
      return "bg-status-warning-soft text-status-warning border-status-warning/30";
    case "Ruim":
      return "bg-status-critical-soft text-status-critical border-status-critical/30";
    case "Danificado":
      return "bg-status-critical text-white border-status-critical";
    default:
      return "bg-muted text-muted-foreground border-border";
  }
}
