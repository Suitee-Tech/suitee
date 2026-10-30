import { useState, useCallback, forwardRef, useImperativeHandle } from "react";
import { motion } from "motion/react";
import { RotateCcw } from "lucide-react";
import { toast } from "sonner";
import { ConfirmDialog } from "../../components/ConfirmDialog";
import { settingsService } from "../../services/settings.service";
import { SectionCard, FieldRow, Toggle, SettingsSelect, type TabRef } from "./_shared";
import { useSettingsLoad, SettingsLoadState, UnavailableSetting } from "./useSettingsLoad";
import type { GeneralSettings } from "./_constants";

export const GeneralTab = forwardRef<TabRef, { onChange: () => void }>(({ onChange }, ref) => {
  const [settings, setSettings] = useState<GeneralSettings>({
    timezone: "America/Sao_Paulo",
    dateFormat: "dd/MM/yyyy",
    currency: "BRL",
    language: "pt-BR",
    sessionTimeout: 30,
    autoSave: true,
    compactTables: false,
    focusMode: false,
  });

  const [showResetConfirm, setShowResetConfirm] = useState(false);

  const load = useSettingsLoad(useCallback(async () => {
    await Promise.all([settingsService.getPreferences(), settingsService.getCompanySettings()])
      .then(([userPrefs, companySettings]) => {
        setSettings((prev) => ({
          ...prev,
          // O fuso é da IMOBILIÁRIA (company settings): é ele que manda nos
          // lançamentos e relatórios. A preferência por usuário fica como
          // fallback para instalações que gravaram o valor antes dessa mudança.
          timezone: companySettings.timezone || userPrefs.timezone || prev.timezone,
          dateFormat: userPrefs.date_format || prev.dateFormat,
          language: userPrefs.language || prev.language,
          sessionTimeout: userPrefs.session_timeout || prev.sessionTimeout,
          focusMode: userPrefs.focus_mode ?? prev.focusMode,
          currency: companySettings.currency || prev.currency,
          autoSave: companySettings.autoSave ?? prev.autoSave,
          compactTables: companySettings.compactTables ?? prev.compactTables,
        }));
      });
  }, []));

  useImperativeHandle(ref, () => ({
    save: async () => {
      load.requireReady();
      await Promise.all([
        settingsService.updatePreferences({
          timezone: settings.timezone,
          date_format: settings.dateFormat,
          language: settings.language,
          session_timeout: settings.sessionTimeout,
          focus_mode: settings.focusMode,
        }),
        settingsService.updateCompanySettings({
          currency: settings.currency,
          autoSave: settings.autoSave,
          compactTables: settings.compactTables,
          // Fuso da imobiliária: grava no tenant para os fluxos usarem.
          timezone: settings.timezone,
        }),
      ]);
    },
  }));

  const update = <K extends keyof GeneralSettings>(key: K, value: GeneralSettings[K]) => {
    setSettings((prev) => ({ ...prev, [key]: value }));
    onChange();
  };

  const handleResetSettings = () => {
    setSettings({
      timezone: "America/Sao_Paulo",
      dateFormat: "dd/MM/yyyy",
      currency: "BRL",
      language: "pt-BR",
      sessionTimeout: 30,
      autoSave: true,
      compactTables: false,
      focusMode: false,
    });
    setShowResetConfirm(false);
    onChange();
    toast.info("Valores padrão preenchidos. Salve para aplicar as alterações.");
  };

  if (!load.ready) return <SettingsLoadState error={load.error} retry={load.retry} />;

  return (
    <div className="space-y-6">
      <SectionCard title="Localização & Formato" subtitle="Defina o idioma, fuso horário e formato de exibicao">
        <div className="divide-y divide-border">
          <FieldRow label="Idioma do sistema" description="Idioma da interface e mensagens">
            <SettingsSelect
              value={settings.language}
              onChange={(v) => update("language", v)}
              options={[
                { value: "pt-BR", label: "Portugues (Brasil)" },
                { value: "en-US", label: "English (US)" },
                { value: "es-ES", label: "Espanol" },
              ]}
            />
          </FieldRow>
          <FieldRow label="Fuso horário" description="Para agendamentos e datas">
            <SettingsSelect
              value={settings.timezone}
              onChange={(v) => update("timezone", v)}
              options={[
                { value: "America/Sao_Paulo", label: "Brasilia (GMT-3)" },
                { value: "America/Manaus", label: "Manaus (GMT-4)" },
                { value: "America/Recife", label: "Recife (GMT-3)" },
                { value: "America/Belem", label: "Belem (GMT-3)" },
                { value: "America/Cuiaba", label: "Cuiaba (GMT-4)" },
                { value: "America/Rio_Branco", label: "Rio Branco (GMT-5)" },
              ]}
            />
          </FieldRow>
          <FieldRow label="Formato de data" description="Exibicao de datas em todo o sistema">
            <SettingsSelect
              value={settings.dateFormat}
              onChange={(v) => update("dateFormat", v)}
              options={[
                { value: "dd/MM/yyyy", label: "dd/MM/aaaa (31/12/2026)" },
                { value: "MM/dd/yyyy", label: "MM/dd/aaaa (12/31/2026)" },
                { value: "yyyy-MM-dd", label: "aaaa-MM-dd (2026-12-31)" },
              ]}
            />
          </FieldRow>
          <FieldRow label="Moeda" description="Moeda padrão para valores monetarios">
            <SettingsSelect
              value={settings.currency}
              onChange={(v) => update("currency", v)}
              options={[
                { value: "BRL", label: "Real (R$)" },
                { value: "USD", label: "Dolar (US$)" },
                { value: "EUR", label: "Euro" },
              ]}
            />
          </FieldRow>
        </div>
      </SectionCard>

      <SectionCard title="Segurança" subtitle="Configurações de sessão e autenticação" delay={0.05}>
        <div className="divide-y divide-border">
          <FieldRow label="Tempo de sessão (minutos)" description="Desconecta automaticamente após inatividade">
            <div className="flex items-center gap-3">
              <input
                type="range"
                min={5}
                max={120}
                step={5}
                value={settings.sessionTimeout}
                onChange={(e) => update("sessionTimeout", Number(e.target.value))}
                className="w-32 accent-primary-600"
              />
              <span className="text-sm font-bold text-foreground w-12 text-right">{settings.sessionTimeout}min</span>
            </div>
          </FieldRow>
          {/* P2/M12 — o toggle de 2FA foi REMOVIDO: era preferência sem efeito
              (nada acontecia no login, não existe enroll/step-up) e a tela
              chegava a afirmar "2FA ativado". Em vez de manter um controle que
              mente, a linha informa o estado real. */}
          <FieldRow
            label="Autenticação em dois fatores (2FA)"
            description="Ainda não disponível nesta versão. O acesso é protegido por senha e pelas permissões do seu papel."
          >
            <span className="text-xs font-bold text-muted-foreground">Não disponível</span>
          </FieldRow>
        </div>
      </SectionCard>

      <SectionCard title="Preferencias de Uso" subtitle="Comportamento e exibicao do sistema" delay={0.1}>
        <div className="divide-y divide-border">
          <FieldRow label="Salvamento automático" description="Salva rascunhos automaticamente ao editar formulários">
            <Toggle checked={settings.autoSave} onChange={(v) => update("autoSave", v)} />
          </FieldRow>
          <FieldRow label="Tabelas compactas" description="Reduz o espacamento das tabelas para exibir mais dados">
            <Toggle checked={settings.compactTables} onChange={(v) => update("compactTables", v)} />
          </FieldRow>
          <FieldRow label="Modo Foco" description="Oculta os cartoes de KPI nos paineis para uma visualização mais operacional">
            <Toggle checked={settings.focusMode} onChange={(v) => update("focusMode", v)} />
          </FieldRow>
        </div>
      </SectionCard>

      <SectionCard title="Dados & Backup" subtitle="Exportação, importação e restauração" delay={0.15}>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
          <UnavailableSetting title="Exportar dados">A exportação completa da conta ainda não está disponível. Use as exportações nas telas de relatórios.</UnavailableSetting>
          <UnavailableSetting title="Importar dados">A importação de arquivos ainda não está disponível nesta tela.</UnavailableSetting>
          <button
            onClick={() => setShowResetConfirm(true)}
            className="flex flex-col items-center gap-3 p-5 bg-card border border-border rounded-2xl hover:border-status-critical/40 hover:bg-status-critical-soft/30 transition-colors group"
          >
            <div className="w-12 h-12 rounded-xl bg-status-critical-soft text-status-critical flex items-center justify-center group-hover:bg-status-critical-soft transition-colors">
              <RotateCcw className="w-6 h-6" />
            </div>
            <div className="text-center">
              <p className="text-sm font-bold text-foreground">Restaurar Padrão</p>
              <p className="text-caption text-muted-foreground mt-0.5">Resetar configurações</p>
            </div>
          </button>
        </div>
      </SectionCard>

      <ConfirmDialog
        open={showResetConfirm}
        title="Restaurar configurações padrão"
        description="Tem certeza que deseja restaurar todas as configuracoes para o padrao do sistema? Os valores desta aba serão preenchidos com os padrões e só serão aplicados ao salvar."
        confirmLabel="Restaurar Padrão"
        variant="warning"
        doubleConfirm
        doubleConfirmLabel="Esta ação vai reverter todas as preferencias de sistema."
        onConfirm={handleResetSettings}
        onCancel={() => setShowResetConfirm(false)}
      />
    </div>
  );
});
