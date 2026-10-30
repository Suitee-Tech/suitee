import { useEffect, useState } from "react";
import { Lock, Mail } from "lucide-react";
import { Switch } from "../ui/switch";
import type { EmailPref } from "../services/email-prefs.service";

export interface EmailPrefsServiceLike {
  list: () => Promise<EmailPref[]>;
  update: (categories: Array<{ category: string; enabled: boolean }>) => Promise<EmailPref[]>;
}

export interface EmailPreferencesPanelProps {
  /** Serviço do app anfitrião (admin usa o axios do painel; portal usa o próprio). */
  service: EmailPrefsServiceLike;
}

/**
 * Preferências de e-mail transacional no dropdown do sino (beta F4).
 * Lista as categorias com toggle imediato (PUT por mudança); categorias
 * travadas para a persona aparecem desabilitadas com cadeado.
 */
export function EmailPreferencesPanel({ service }: EmailPreferencesPanelProps) {
  const [prefs, setPrefs] = useState<EmailPref[] | null>(null);
  const [saving, setSaving] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    service
      .list()
      .then((list) => {
        if (!cancelled) setPrefs(list);
      })
      .catch(() => {
        if (!cancelled) setError("Não foi possível carregar suas preferências de e-mail.");
      });
    return () => {
      cancelled = true;
    };
    // O serviço é estável por mount (instância de módulo) — intencional.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleToggle = async (pref: EmailPref, checked: boolean) => {
    if (pref.locked) return;
    setSaving(pref.category);
    setError(null);
    // Otimista: atualiza imediatamente e reverte em caso de erro.
    setPrefs((current) =>
      current ? current.map((p) => (p.category === pref.category ? { ...p, enabled: checked } : p)) : current,
    );
    try {
      const updated = await service.update([{ category: pref.category, enabled: checked }]);
      setPrefs(updated);
    } catch {
      setPrefs((current) =>
        current ? current.map((p) => (p.category === pref.category ? { ...p, enabled: !checked } : p)) : current,
      );
      setError("Não foi possível salvar a preferência. Tente novamente.");
    } finally {
      setSaving(null);
    }
  };

  if (prefs === null) {
    return (
      <div className="border-t border-border bg-muted/50">
        {error ? (
          <p className="px-4 py-3 text-xs text-status-error leading-tight">{error}</p>
        ) : (
          <div className="flex items-center gap-2 px-4 py-3 text-xs text-muted-foreground">
            <Mail className="w-4 h-4 shrink-0" />
            <span>Carregando preferências de e-mail…</span>
          </div>
        )}
      </div>
    );
  }

  return (
    <div className="border-t border-border bg-muted/50">
      <div className="px-4 pt-3 pb-1">
        <p className="text-xs font-semibold text-foreground leading-tight">E-mails</p>
        <p className="text-micro text-muted-foreground leading-tight">
          Escolha quais notificações você também recebe por e-mail
        </p>
      </div>
      {prefs.map((pref) => (
        <div key={pref.category} className="flex items-center justify-between gap-3 px-4 py-2">
          <div className="min-w-0">
            <p className="text-xs font-medium text-foreground leading-tight flex items-center gap-1.5">
              {pref.label}
              {pref.locked && <Lock className="w-3 h-3 text-muted-foreground shrink-0" aria-label="Não pode ser desligado" />}
            </p>
          </div>
          <Switch
            checked={pref.enabled}
            disabled={pref.locked || saving === pref.category}
            onCheckedChange={(checked) => void handleToggle(pref, checked)}
            aria-label={pref.label}
          />
        </div>
      ))}
      {error && <p className="px-4 pb-3 text-xs text-status-error leading-tight">{error}</p>}
    </div>
  );
}
