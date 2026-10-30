import { useEffect, useState } from "react";
import { Loader2, Mail, MessageCircle, Phone } from "lucide-react";
import { toast } from "sonner";
import { portalService } from "../services/portal.service";
import type { UserConsent } from "../../../shared/services/settings.service";

const EMPTY_CONSENT: UserConsent = {
  marketing_emails: false,
  marketing_sms: false,
  marketing_whatsapp: false,
};

export function MarketingConsentCard() {
  const [consent, setConsent] = useState<UserConsent>(EMPTY_CONSENT);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState<keyof UserConsent | null>(null);

  useEffect(() => {
    let cancelled = false;
    portalService
      .getPreferences()
      .then((p: any) => {
        if (cancelled) return;
        setConsent(p.consent ?? EMPTY_CONSENT);
      })
      .catch(() => {})
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const toggle = async (key: "marketing_emails" | "marketing_sms" | "marketing_whatsapp", next: boolean) => {
    const previous = consent[key];
    setConsent((c) => ({ ...c, [key]: next }));
    setSaving(key);
    try {
      await portalService.patchPreferences({
        consent: { ...consent, [key]: next } as UserConsent,
      });
      toast.success(next ? "Aceite registrado. Você passa a receber comunicações nesse canal." : "Aceite revogado.");
    } catch {
      setConsent((c) => ({ ...c, [key]: previous }));
      toast.error("Não foi possível salvar a preferência. Tente novamente.");
    } finally {
      setSaving(null);
    }
  };

  if (loading) {
    return (
      <div className="bg-card border border-border rounded-3xl p-6 shadow-sm mb-6 flex items-center gap-2 text-muted-foreground">
        <Loader2 className="w-4 h-4 animate-spin" />
        <span className="text-sm">Carregando consentimentos...</span>
      </div>
    );
  }

  return (
    <div className="bg-card border border-border rounded-3xl p-6 shadow-sm mb-6">
      <h2 className="text-base font-bold text-foreground mb-1">Consentimentos de marketing</h2>
      <p className="text-xs text-muted-foreground mb-4">
        Escolha por quais canais aceita receber novidades, ofertas e materiais promocionais. Você pode mudar a qualquer
        momento — não afeta comunicações operacionais (cobranças, contratos, suporte). LGPD Art. 8.
      </p>

      <div className="flex flex-col gap-3">
        <ConsentToggle
          icon={Mail}
          label="E-mail"
          description="Receber e-mails de marketing"
          checked={consent.marketing_emails}
          saving={saving === "marketing_emails"}
          onChange={(v) => toggle("marketing_emails", v)}
        />
        <ConsentToggle
          icon={MessageCircle}
          label="WhatsApp"
          description="Mensagens promocionais por WhatsApp"
          checked={consent.marketing_whatsapp}
          saving={saving === "marketing_whatsapp"}
          onChange={(v) => toggle("marketing_whatsapp", v)}
        />
        <ConsentToggle
          icon={Phone}
          label="SMS"
          description="Mensagens promocionais por SMS"
          checked={consent.marketing_sms}
          saving={saving === "marketing_sms"}
          onChange={(v) => toggle("marketing_sms", v)}
        />
      </div>
    </div>
  );
}

function ConsentToggle({
  icon: Icon,
  label,
  description,
  checked,
  saving,
  onChange,
}: {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  description: string;
  checked: boolean;
  saving: boolean;
  onChange: (next: boolean) => void;
}) {
  return (
    <label className="flex items-start gap-3 cursor-pointer">
      <div className="w-10 h-10 rounded-xl bg-muted flex items-center justify-center flex-shrink-0 text-muted-foreground">
        <Icon className="w-5 h-5" />
      </div>
      <div className="flex-1 min-w-0">
        <p className="text-sm font-bold text-foreground">{label}</p>
        <p className="text-xs text-muted-foreground">{description}</p>
      </div>
      <div className="flex items-center gap-2 mt-1">
        {saving && <Loader2 className="w-3 h-3 animate-spin text-muted-foreground" />}
        <input
          type="checkbox"
          checked={checked}
          disabled={saving}
          onChange={(e) => onChange(e.target.checked)}
          className="w-5 h-5"
        />
      </div>
    </label>
  );
}

