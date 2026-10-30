import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { CheckCircle2, Clock, Loader2, Tag, UserPlus, XCircle } from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "../ui/dialog";
import { Button } from "../ui/button";
import { useWsSubscription } from "../ws/useWsSubscription";
import { leadService, type ApiLead } from "../services/lead.service";
import { notifyApiError } from "../services/errorToast";

const ASSIGNMENT_SLA_SECONDS = 5 * 60;
const SLA_THRESHOLD_WARNING = 180;
const SLA_THRESHOLD_CRITICAL = 60;

interface LeadAssignedPayload {
  lead_id?: string;
  broker_id?: string;
  stage?: string;
  status?: string;
}

interface MatchReason {
  reason?: string;
  matched_specialties?: string[];
  qualification_score?: number;
}

type AcceptLeadModalProps = {
  userId?: string | null;
  isAuthenticated?: boolean;
};

export function AcceptLeadModal({ userId, isAuthenticated }: AcceptLeadModalProps) {
  const authenticated = typeof isAuthenticated === "boolean" ? isAuthenticated : Boolean(userId);

  const [open, setOpen] = useState(false);
  const [lead, setLead] = useState<ApiLead | null>(null);
  const [matchInfo, setMatchInfo] = useState<MatchReason>({});
  const [timeLeft, setTimeLeft] = useState(ASSIGNMENT_SLA_SECONDS);
  const [accepting, setAccepting] = useState(false);
  const [rejecting, setRejecting] = useState(false);
  const [showRejectForm, setShowRejectForm] = useState(false);
  const [rejectReason, setRejectReason] = useState("");

  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const clearTimer = useCallback(() => {
    if (timerRef.current) {
      clearInterval(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  const closeModal = useCallback(() => {
    clearTimer();
    setOpen(false);
    setLead(null);
    setMatchInfo({});
    setTimeLeft(ASSIGNMENT_SLA_SECONDS);
    setShowRejectForm(false);
    setRejectReason("");
    setAccepting(false);
    setRejecting(false);
  }, [clearTimer]);

  useEffect(() => {
    if (!open) return;
    if (timeLeft > 0) return;
    clearTimer();
    toast.info("Tempo de aceitação expirou. O lead voltou para a roleta.");
    closeModal();
  }, [open, timeLeft, clearTimer, closeModal]);

  const startCountdown = useCallback(() => {
    clearTimer();
    setTimeLeft(ASSIGNMENT_SLA_SECONDS);
    timerRef.current = setInterval(() => {
      setTimeLeft((prev) => Math.max(0, prev - 1));
    }, 1000);
  }, [clearTimer]);

  useEffect(() => () => clearTimer(), [clearTimer]);

  const handleLeadAssigned = useCallback(
    (event: unknown) => {
      if (!authenticated || !userId) return;
      if (!event || typeof event !== "object") return;
      const evt = event as { type?: string; payload?: unknown };
      if (evt.type !== "lead_assigned") return;
      const payload = (evt.payload || {}) as LeadAssignedPayload & MatchReason;

      if (typeof payload.broker_id !== "string" || payload.broker_id !== userId) return;
      const leadId = payload.lead_id;
      if (typeof leadId !== "string" || !leadId) return;

      setMatchInfo({
        reason: payload.reason,
        matched_specialties: payload.matched_specialties,
        qualification_score: payload.qualification_score,
      });

      leadService
        .getById(leadId)
        .then((fetched) => {
          if (!fetched) return;
          setLead(fetched);
          setOpen(true);
          startCountdown();
        })
        .catch(() => {
          setLead({ id: leadId, name: "Lead atribuído", email: "", phone: "" } as ApiLead);
          setOpen(true);
          startCountdown();
        });
    },
    [authenticated, startCountdown, userId],
  );

  useWsSubscription("lead_assigned", handleLeadAssigned);

  const handleAccept = useCallback(async () => {
    if (!lead) return;
    setAccepting(true);
    try {
      await leadService.acceptAssignment(lead.id);
      toast.success("Lead aceito. Bom trabalho!");
      closeModal();
    } catch (err: any) {
      const status = err?.response?.status;
      if (status === 409) {
        toast.error("Lead já não está disponível (outro corretor pegou ou expirou).");
        closeModal();
      } else if (status === 403) {
        toast.error("Você não tem permissão para aceitar este lead.");
      } else {
        notifyApiError(err, { context: "Aceitar lead" });
      }
    } finally {
      setAccepting(false);
    }
  }, [lead, closeModal]);

  const handleReject = useCallback(async () => {
    if (!lead) return;
    const trimmed = rejectReason.trim();
    if (trimmed.length < 2) {
      toast.error("Informe um motivo (mínimo 2 caracteres).");
      return;
    }
    setRejecting(true);
    try {
      await leadService.rejectAssignment(lead.id, trimmed);
      toast.success("Lead recusado. Voltou para a roleta.");
      closeModal();
    } catch (err: any) {
      const status = err?.response?.status;
      if (status === 409) {
        toast.error("Lead já não está disponível.");
        closeModal();
      } else {
        notifyApiError(err, { context: "Recusar lead" });
      }
    } finally {
      setRejecting(false);
    }
  }, [lead, rejectReason, closeModal]);

  const progressPct = useMemo(() => Math.max(0, Math.min(100, (timeLeft / ASSIGNMENT_SLA_SECONDS) * 100)), [timeLeft]);

  const isCritical = timeLeft <= SLA_THRESHOLD_CRITICAL;
  const isWarning = timeLeft <= SLA_THRESHOLD_WARNING && !isCritical;
  const progressClass = isCritical
    ? "bg-status-critical motion-safe:animate-pulse"
    : isWarning
      ? "bg-status-warning"
      : "bg-status-ok";

  const mm = Math.floor(timeLeft / 60);
  const ss = (timeLeft % 60).toString().padStart(2, "0");
  const countdownLabel = `${mm}:${ss}`;

  if (!open || !lead) return null;

  return (
    <Dialog open={open} onOpenChange={(o) => !o && closeModal()}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <UserPlus className="w-5 h-5 text-primary" />
            Novo Lead Atribuído
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-2">
          <div className="flex items-center justify-between text-sm">
            <span className="flex items-center gap-1.5 text-muted-foreground">
              <Clock className="w-4 h-4" aria-hidden="true" /> Tempo para aceitar
            </span>
            <span
              className={clsx(
                "font-bold tabular-nums",
                isCritical ? "text-status-critical" : isWarning ? "text-status-warning" : "text-status-ok",
              )}
            >
              {countdownLabel}
            </span>
          </div>
          <div className="w-full h-2 bg-muted rounded-full overflow-hidden">
            <div
              role="progressbar"
              aria-valuenow={Math.round(progressPct)}
              aria-valuemin={0}
              aria-valuemax={100}
              className={clsx("h-full rounded-full transition-all duration-500 ease-out", progressClass)}
              style={{ width: `${progressPct}%` }}
            />
          </div>
        </div>

        <div className="rounded-xl border border-border bg-muted/40 p-3 space-y-2">
          <div>
            <div className="text-micro font-bold text-muted-foreground uppercase tracking-wide">Cliente</div>
            <div className="text-sm font-bold text-foreground">{lead.name || "Sem nome"}</div>
            {lead.email && <div className="text-caption text-muted-foreground truncate">{lead.email}</div>}
            {lead.phone && <div className="text-caption text-muted-foreground">{lead.phone}</div>}
          </div>

          {typeof matchInfo.qualification_score === "number" && (
            <div>
              <div className="text-micro font-bold text-muted-foreground uppercase tracking-wide">Score de qualificação</div>
              <div className="text-sm font-bold text-foreground tabular-nums">{Math.round(matchInfo.qualification_score)} / 100</div>
            </div>
          )}

          {Array.isArray(matchInfo.matched_specialties) && matchInfo.matched_specialties.length > 0 && (
            <div>
              <div className="text-micro font-bold text-muted-foreground uppercase tracking-wide mb-1">Match por especialidade</div>
              <div className="flex flex-wrap gap-1">
                {matchInfo.matched_specialties.map((s) => (
                  <span
                    key={s}
                    className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md bg-primary/10 text-primary text-caption font-medium"
                  >
                    <Tag className="w-3 h-3" />
                    {s}
                  </span>
                ))}
              </div>
            </div>
          )}

          {matchInfo.reason && !matchInfo.matched_specialties?.length && (
            <div>
              <div className="text-micro font-bold text-muted-foreground uppercase tracking-wide">Motivo do match</div>
              <div className="text-caption text-foreground">{matchInfo.reason}</div>
            </div>
          )}
        </div>

        {showRejectForm && (
          <div className="space-y-2">
            <label htmlFor="reject-reason" className="text-micro font-bold text-foreground uppercase tracking-wide">
              Motivo da recusa (obrigatório)
            </label>
            <textarea
              id="reject-reason"
              value={rejectReason}
              onChange={(e) => setRejectReason(e.target.value)}
              maxLength={500}
              rows={3}
              placeholder="Ex.: sem capacidade no momento, conflito de interesse, fora da minha especialidade…"
              className="w-full px-3 py-2 text-sm bg-card border border-border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary resize-none"
              autoFocus
            />
            <div className="text-caption text-muted-foreground text-right">{rejectReason.length}/500</div>
          </div>
        )}

        <DialogFooter className="gap-2">
          {showRejectForm ? (
            <>
              <Button
                variant="outline"
                onClick={() => {
                  setShowRejectForm(false);
                  setRejectReason("");
                }}
                disabled={rejecting}
              >
                Voltar
              </Button>
              <Button variant="destructive" onClick={handleReject} disabled={rejecting || rejectReason.trim().length < 2}>
                {rejecting ? <Loader2 className="w-4 h-4 animate-spin mr-2" /> : <XCircle className="w-4 h-4 mr-2" />}
                Confirmar recusa
              </Button>
            </>
          ) : (
            <>
              <Button
                variant="outline"
                onClick={() => setShowRejectForm(true)}
                disabled={accepting}
                className="text-status-critical border-status-critical/30 hover:bg-status-critical-soft"
              >
                <XCircle className="w-4 h-4 mr-2" />
                Recusar
              </Button>
              <Button onClick={handleAccept} disabled={accepting} className="bg-status-ok hover:bg-status-ok/90 text-white">
                {accepting ? <Loader2 className="w-4 h-4 animate-spin mr-2" /> : <CheckCircle2 className="w-4 h-4 mr-2" />}
                Aceitar
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
