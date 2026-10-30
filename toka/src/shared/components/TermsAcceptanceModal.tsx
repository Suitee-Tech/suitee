import { useEffect, useState } from "react";
import { Link } from "react-router";
import { Loader2, Shield, ExternalLink } from "lucide-react";
import { toast } from "sonner";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from "../ui/dialog";
import { Button } from "../ui/button";
import { settingsService, type UserConsent } from "../services/settings.service";
import { CURRENT_TERMS_VERSION, TERMS_SUMMARY } from "../config/lgpd";

interface TermsAcceptanceModalProps {
  policyPath: string;
  termsPath: string;
  onLogout?: () => void | Promise<void>;
  /** Cliente de settings injetado — o PORTAL passa o portalService (token do
   *  portal) para NÃO tocar na sessão/API do painel (auth separada). */
  settings?: {
    getPreferences: () => Promise<Record<string, unknown>>;
    patchPreferences: (data: Record<string, unknown>) => Promise<void>;
  };
}

export function TermsAcceptanceModal({ policyPath, termsPath, onLogout, settings }: TermsAcceptanceModalProps) {
  const settingsClient = settings ?? settingsService as unknown as { getPreferences: () => Promise<Record<string, unknown>>; patchPreferences: (data: Record<string, unknown>) => Promise<void> };
  const [needsAccept, setNeedsAccept] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    let cancelled = false;
    settingsClient
      .getPreferences()
      .then((prefs) => {
        if (cancelled) return;
        const accepted = (prefs as any)?.consent?.terms_version ?? "";
        if (accepted !== CURRENT_TERMS_VERSION) {
          setNeedsAccept(true);
        }
      })
      .catch(() => {
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const handleAccept = async () => {
    setSubmitting(true);
    try {
      const consent: Partial<UserConsent> = {
        terms_version: CURRENT_TERMS_VERSION,
      };
      await settingsClient.patchPreferences({ consent: consent as UserConsent });
      setNeedsAccept(false);
      toast.success("Aceite dos Termos de Uso registrado.");
    } catch {
      toast.error("Não foi possível registrar o aceite. Tente novamente.");
    } finally {
      setSubmitting(false);
    }
  };

  const handleLogout = async () => {
    if (onLogout) await onLogout();
  };

  return (
    <Dialog open={needsAccept}>
      <DialogContent
        className="max-w-md"
        onPointerDownOutside={(e) => e.preventDefault()}
        onEscapeKeyDown={(e) => e.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Shield className="w-5 h-5 text-primary-600" />
            Revise os Termos de Uso e a Política de Privacidade
          </DialogTitle>
          <DialogDescription>
            Para continuar usando o Suitee Inteligence, leia o resumo e os dois documentos. Os Termos regulam o uso do
            produto; a Política explica o tratamento de dados pessoais.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <div className="bg-muted/40 rounded-xl p-3">
            <p className="text-xs font-bold text-foreground uppercase tracking-wider mb-2">Resumo (TL;DR)</p>
            <ul className="space-y-1.5">
              {TERMS_SUMMARY.map((line) => (
                <li key={line} className="text-sm text-foreground/90 flex gap-2">
                  <span className="text-primary-600">•</span>
                  <span>{line}</span>
                </li>
              ))}
            </ul>
          </div>

          <div className="flex flex-col gap-2">
            <Link
              to={termsPath}
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-1 text-sm text-primary-600 hover:text-primary-700"
            >
              Ler Termos de Uso
              <ExternalLink className="w-3.5 h-3.5" />
            </Link>
            <Link
              to={policyPath}
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-1 text-sm text-primary-600 hover:text-primary-700"
            >
              Ler Política de Privacidade
              <ExternalLink className="w-3.5 h-3.5" />
            </Link>
          </div>

          <p className="text-[11px] text-muted-foreground border-t border-border pt-3">
            Ao clicar em <span className="font-bold">"Aceitar Termos e continuar"</span>, você aceita os Termos de Uso
            e confirma que teve acesso à Política de Privacidade. Isso não substitui consentimentos específicos, que
            podem ser revogados nas preferências quando aplicável.
          </p>
        </div>

        <DialogFooter className="flex flex-col sm:flex-row gap-2">
          {onLogout && (
            <Button variant="outline" onClick={handleLogout} disabled={submitting}>
              Sair sem aceitar
            </Button>
          )}
          <Button onClick={handleAccept} disabled={submitting}>
            {submitting ? <Loader2 className="w-4 h-4 animate-spin" /> : "Aceitar Termos e continuar"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
