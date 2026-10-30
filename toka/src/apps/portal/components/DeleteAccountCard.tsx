import { useState } from "react";
import { useNavigate } from "react-router";
import { AlertTriangle, Loader2, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "../../../shared/ui/dialog";
import { Button } from "../../../shared/ui/button";
import { Input } from "../../../shared/ui/input";
import { Textarea } from "../../../shared/ui/textarea";
import { settingsService } from "../../../shared/services/settings.service";
import { usePortalAuth } from "../context/PortalAuthContext";

const EXPECTED_CONFIRM = "EXCLUIR MINHA CONTA";

export function DeleteAccountCard() {
  const navigate = useNavigate();
  const { logout } = usePortalAuth();
  const [open, setOpen] = useState(false);
  const [confirmText, setConfirmText] = useState("");
  const [reason, setReason] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const canSubmit = confirmText.trim() === EXPECTED_CONFIRM && !submitting;

  const handleDelete = async () => {
    if (!canSubmit) return;
    setSubmitting(true);
    try {
      const result = (await settingsService.deleteMyAccount(EXPECTED_CONFIRM, reason.trim() || undefined)) as {
        failed?: string[];
      };
      if (result?.failed?.length) {
        toast.error("O pedido foi processado parcialmente. Contate o canal de privacidade para concluir o atendimento.");
      } else {
        toast.success("A solicitação de anonimização foi registrada. Até logo.");
      }
      await logout();
      navigate("/portal/login");
    } catch (err) {
      const msg =
        (err as { response?: { data?: { message?: string } } })?.response?.data?.message ||
        "Não foi possível concluir a exclusão. Tente novamente em alguns minutos.";
      toast.error(msg);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <>
      <div className="bg-status-critical-soft border border-status-critical/30 rounded-3xl p-6 mb-6">
        <div className="flex items-start gap-3 mb-3">
          <div className="w-10 h-10 rounded-xl bg-status-critical/15 flex items-center justify-center flex-shrink-0 text-status-critical">
            <AlertTriangle className="w-5 h-5" />
          </div>
          <div className="flex-1 min-w-0">
            <p className="text-sm font-bold text-foreground">Excluir minha conta</p>
            <p className="text-xs text-muted-foreground mt-0.5">
              Você pode solicitar a anonimização dos dados pessoais da conta. Registros que precisem ser mantidos por
              obrigação legal ou integridade do negócio podem permanecer com acesso restrito. Esta ação é{" "}
              <span className="font-bold">irreversível</span>.
            </p>
          </div>
        </div>

        <button
          type="button"
          onClick={() => setOpen(true)}
          className="w-full flex items-center justify-center gap-2 px-4 py-2.5 text-status-critical text-sm font-medium rounded-xl border border-status-critical/40 hover:bg-status-critical-soft/80 transition-colors"
        >
          <Trash2 className="w-4 h-4" />
          Excluir minha conta
        </button>
      </div>

      <Dialog
        open={open}
        onOpenChange={(o) => {
          if (!submitting) setOpen(o);
        }}
      >
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle className="text-status-critical">Confirmar exclusão de conta</DialogTitle>
          </DialogHeader>

          <div className="space-y-4">
            <div className="bg-status-critical-soft border border-status-critical/30 rounded-lg p-3 text-sm">
              <p className="font-bold mb-1">Essa ação é irreversível.</p>
              <p>
                O pedido inicia a anonimização dos dados pessoais nos sistemas atendidos pelo recurso. A conclusão pode
                variar por serviço; você não deve presumir que todos os registros de negócio serão apagados.
              </p>
              <p className="mt-2 text-xs text-muted-foreground">
                Cobranças, contratos e documentos podem ser preservados quando houver fundamento legal ou necessidade
                de manter prova da operação. Peça atendimento pelo canal de privacidade se precisar complementar a solicitação.
              </p>
            </div>

            <div>
              <label className="block text-xs font-bold text-foreground uppercase tracking-wider mb-1">
                Para confirmar, digite <span className="font-mono">{EXPECTED_CONFIRM}</span>
              </label>
              <Input
                value={confirmText}
                onChange={(e) => setConfirmText(e.target.value)}
                placeholder={EXPECTED_CONFIRM}
                autoComplete="off"
              />
            </div>

            <div>
              <label className="block text-xs font-bold text-foreground uppercase tracking-wider mb-1">
                Motivo (opcional)
              </label>
              <Textarea
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                placeholder="Ex.: mudei de imobiliária, não preciso mais do serviço, etc."
                rows={3}
                className="resize-none"
              />
              <p className="text-[11px] text-muted-foreground mt-1">
                Não inclua dados sensíveis aqui — usaremos só o comprimento pra análise agregada.
              </p>
            </div>
          </div>

          <DialogFooter className="gap-2">
            <Button variant="outline" onClick={() => setOpen(false)} disabled={submitting}>
              Cancelar
            </Button>
            <Button onClick={handleDelete} disabled={!canSubmit} variant="destructive">
              {submitting ? <Loader2 className="w-4 h-4 animate-spin" /> : "Excluir definitivamente"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
