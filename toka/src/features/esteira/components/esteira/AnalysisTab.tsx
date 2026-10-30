import { useEffect, useRef, useState } from "react";
import { Briefcase, CheckCircle, CheckCircle2, Clock, ShieldCheck, UserCheck } from "lucide-react";
import { toast } from "sonner";
import { flags } from "../../../../shared/config/flags";
import { useDealsContext } from "../../../../shared/context/DealsContext";
import { useRequestScopeGuard } from "../../../../shared/hooks/useRequestScopeGuard";
import { notifyApiError } from "../../../../shared/services/errorToast";
import type { DealDTO, DueDiligenceCriterionStatus } from "../../../../shared/services/deal.service";
import { dealService } from "../../../../shared/services/deal.service";
import type { AnalysisCriterion } from "../../../../shared/documents/types";
import { Button } from "../../../../shared/ui/button";
import { StatusDot, StatusPill } from "./PanelPrimitives";
import { MultiPartyAnalysis } from "./MultiPartyAnalysis";

export function AnalysisTab({ dealDetails }: { dealDetails: DealDTO | null }) {
  const { updateDueDiligence, updatePartyDueDiligence, fetchDealById } = useDealsContext();
  const dd = dealDetails?.due_diligence;
  const parties = dealDetails?.parties ?? [];
  const [editing, setEditing] = useState(false);
  const [scoreSerasa, setScoreSerasa] = useState(dd?.score_serasa != null ? String(dd.score_serasa) : "");
  const [cndStatus, setCndStatus] = useState(dd?.cnd_status ?? "");
  const [legalOpinion, setLegalOpinion] = useState(dd?.legal_opinion ?? "");
  const [monthlyIncome, setMonthlyIncome] = useState(
    dd?.monthly_income != null ? String(dd.monthly_income) : "",
  );
  const [status, setStatus] = useState((dd?.status ?? "PENDING").toUpperCase());
  const [saving, setSaving] = useState(false);
  const saveRequestPending = useRef(false);
  const beginScopedRequest = useRequestScopeGuard(dealDetails?.id ?? "");

  useEffect(() => {
    saveRequestPending.current = false;
    setSaving(false);
    setEditing(false);
  }, [dealDetails?.id]);

  useEffect(() => {
    if (editing || saving) return;
    setScoreSerasa(dd?.score_serasa != null ? String(dd.score_serasa) : "");
    setCndStatus(dd?.cnd_status ?? "");
    setLegalOpinion(dd?.legal_opinion ?? "");
    setMonthlyIncome(dd?.monthly_income != null ? String(dd.monthly_income) : "");
    setStatus((dd?.status ?? "PENDING").toUpperCase());
  }, [dd?.cnd_status, dd?.legal_opinion, dd?.score_serasa, dd?.monthly_income, dd?.status, editing, saving]);

  const handleSaveAggregate = async () => {
    if (!dealDetails || saveRequestPending.current) return;
    const isCurrent = beginScopedRequest();
    saveRequestPending.current = true;
    setSaving(true);
    try {
      const n = scoreSerasa.trim() ? Number(scoreSerasa.trim()) : undefined;
      const income = monthlyIncome.trim() ? Number(monthlyIncome.trim()) : undefined;
      await updateDueDiligence(dealDetails.id, {
        score_serasa: Number.isFinite(n as any) ? (n as number) : undefined,
        cnd_status: cndStatus.trim() || undefined,
        legal_opinion: legalOpinion.trim() || undefined,
        monthly_income: Number.isFinite(income as any) ? (income as number) : undefined,
        status,
      });
      if (!isCurrent()) return;
      toast.success("Análise atualizada!");
      setEditing(false);
    } catch (error) {
      if (!isCurrent()) return;
      notifyApiError(error, { context: "Atualizar análise" });
    } finally {
      if (isCurrent()) {
        saveRequestPending.current = false;
        setSaving(false);
      }
    }
  };

  const handleUpdateCriterion = async (
    partyId: string,
    criterion: AnalysisCriterion,
    status: DueDiligenceCriterionStatus,
  ) => {
    if (!dealDetails) return;
    try {
      await dealService.updatePartyDueDiligenceCriterion(dealDetails.id, partyId, criterion, {
        status,
      });
      await fetchDealById(dealDetails.id);
      toast.success("Status do critério atualizado.");
    } catch (error) {
      notifyApiError(error, { context: "Atualizar critério da análise" });
    }
  };

  if (dealDetails && flags.multiPartyUI && parties.length > 0) {
    return (
      <MultiPartyAnalysis
        dealDetails={dealDetails}
        aggregateDueDiligence={dd}
        onUpdatePartyDueDiligence={updatePartyDueDiligence}
        onUpdateCriterion={handleUpdateCriterion}
      />
    );
  }

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <section className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden">
          <header className="px-5 py-3 border-b border-border flex items-center justify-between gap-3">
            <div className="flex items-center gap-2.5 min-w-0">
              <UserCheck className="w-4 h-4 text-primary" aria-hidden="true" />
              <h3 className="text-label font-bold text-foreground uppercase tracking-wide">Consulta CPF/CNPJ</h3>
            </div>
            <Button
              size="sm"
              variant="ghost"
              className="h-8 -my-1 -mr-2 text-primary hover:bg-primary/10"
              onClick={() => setEditing((v) => !v)}
              disabled={saving}
            >
              {editing ? "Fechar" : "Editar"}
            </Button>
          </header>

          <div className="p-5">
          {dd?.score_serasa ? (
            <div className="p-4 bg-status-ok-soft border border-status-ok/20 rounded-xl">
              <div className="flex items-center gap-2 mb-1">
                <CheckCircle2 className="w-5 h-5 text-status-ok" />
                <span className="font-bold text-status-ok">Nada Consta (Serasa / SPC)</span>
              </div>
              <p className="text-caption text-status-ok">Score: {dd.score_serasa} (Excelente). Sem restrições financeiras.</p>
            </div>
          ) : (
            <div className="p-4 bg-muted border border-border rounded-xl text-center">
              <p className="text-sm text-muted-foreground">Nenhuma análise realizada.</p>
            </div>
          )}

          {editing && dealDetails && (
            <div className="space-y-3 mt-3">
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <div className="space-y-1">
                  <div className="text-xs font-semibold text-muted-foreground">Status</div>
                  <select
                    value={status}
                    onChange={(e) => setStatus(e.target.value)}
                    className="w-full px-3 py-2 bg-card border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                  >
                    <option value="PENDING">Pendente</option>
                    <option value="APPROVED">Aprovado</option>
                    <option value="REJECTED">Reprovado</option>
                  </select>
                </div>
                <div className="space-y-1">
                  <div className="text-xs font-semibold text-muted-foreground">Score Serasa</div>
                  <input
                    value={scoreSerasa}
                    onChange={(e) => setScoreSerasa(e.target.value)}
                    placeholder="Ex: 750"
                    className="w-full px-3 py-2 bg-card border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                  />
                </div>
              </div>
              <div className="space-y-1">
                <div className="text-xs font-semibold text-muted-foreground">CND</div>
                <input
                  value={cndStatus}
                  onChange={(e) => setCndStatus(e.target.value)}
                  placeholder="Ex: OK / PENDENTE"
                  className="w-full px-3 py-2 bg-card border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                />
              </div>
              <div className="space-y-1">
                <div className="text-xs font-semibold text-muted-foreground">Renda mensal (R$)</div>
                <input
                  value={monthlyIncome}
                  onChange={(e) => setMonthlyIncome(e.target.value.replace(/\D/g, ""))}
                  placeholder="Ex: 5000"
                  inputMode="numeric"
                  className="w-full px-3 py-2 bg-card border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                />
              </div>
              <div className="space-y-1">
                <div className="text-xs font-semibold text-muted-foreground">Parecer jurídico</div>
                <textarea
                  value={legalOpinion}
                  onChange={(e) => setLegalOpinion(e.target.value)}
                  rows={3}
                  placeholder="Descreva o parecer..."
                  className="w-full px-3 py-2 bg-card border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                />
              </div>
              <div className="flex justify-end gap-2 pt-1">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setEditing(false)}
                  disabled={saving}
                  className="text-xs font-bold"
                >
                  Cancelar
                </Button>
                <Button
                  size="sm"
                  className="text-xs font-bold"
                  onClick={handleSaveAggregate}
                  loading={saving}
                  loadingText="Salvando…"
                >
                  Salvar
                </Button>
              </div>
            </div>
          )}
          </div>
        </section>

        <section className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden">
          <header className="px-5 py-3 border-b border-border flex items-center justify-between gap-3">
            <div className="flex items-center gap-2.5 min-w-0">
              <Briefcase className="w-4 h-4 text-primary" aria-hidden="true" />
              <h3 className="text-label font-bold text-foreground uppercase tracking-wide">Certidões (Despachante)</h3>
            </div>
            <StatusPill tone={dd?.cnd_status === "OK" ? "ok" : "warning"}>
              {dd?.cnd_status === "OK" ? "Concluído" : "Pendente"}
            </StatusPill>
          </header>
          <div className="p-5">
            <div className="flex items-center justify-between text-sm py-1">
              <span className="text-muted-foreground">CND Federal</span>
              <span className="inline-flex items-center gap-1.5 font-semibold">
                <StatusDot tone={dd?.cnd_status === "OK" ? "ok" : "neutral"} />
                {dd?.cnd_status === "OK" ? (
                  <>
                    <CheckCircle className="w-3 h-3 text-status-ok" /> <span className="text-status-ok">Emitida</span>
                  </>
                ) : (
                  <>
                    <Clock className="w-3 h-3 text-muted-foreground" /> <span className="text-muted-foreground">Aguardando</span>
                  </>
                )}
              </span>
            </div>
          </div>
        </section>
      </div>

      <section className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden">
        <header className="px-5 py-3 border-b border-border flex items-center justify-between gap-3">
          <div className="flex items-center gap-2.5 min-w-0">
            <ShieldCheck className="w-4 h-4 text-primary" aria-hidden="true" />
            <h3 className="text-label font-bold text-foreground uppercase tracking-wide">Parecer Jurídico</h3>
          </div>
          <StatusPill tone={dd?.status === "APPROVED" ? "ok" : "pending"}>
            {dd?.status === "APPROVED" ? "Viável" : "Pendente"}
          </StatusPill>
        </header>
        <p className="px-5 py-4 text-sm text-foreground/80 leading-relaxed">{dd?.legal_opinion || "Aguardando análise jurídica..."}</p>
      </section>
    </div>
  );
}
