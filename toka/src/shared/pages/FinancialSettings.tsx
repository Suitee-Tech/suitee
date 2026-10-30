import { useEffect, useId, useState } from 'react';
import { notifyApiError } from "../services/errorToast";
import { useSessionRecovery } from '../../shared/context/SessionRecoveryContext';
import { useSearchParams } from 'react-router';
import { Loader2, Save, Percent, Calendar, DollarSign, FileCheck } from 'lucide-react';
import { toast } from 'sonner';
import { Card } from '../ui/card';
import { Button } from '../ui/button';
import { ChartOfAccountsSection } from '../components/finance/ChartOfAccountsSection';
import { FinancialFlowSummary } from '../components/finance/FinancialFlowSummary';
import { tenantFinancialSettingsService, type TenantFinancialSettings } from '../services/tenantFinancialSettings.service';
import { commissionRulesService, type CommissionRole, type CommissionRule } from '../services/commissionRules.service';
import {
  buildCommissionRulesPayload,
  calculateCommissionDistribution,
  commissionRulePercentage,
  COMMISSION_ROLES,
} from '../lib/commissionRules';

const ROLE_LABELS: Record<CommissionRole, string> = {
  captador: 'Captador',
  co_captador: 'Co-captador',
  visitador: 'Visitador',
  atendente: 'Atendente',
  fechamento: 'Fechamento',
  imobiliaria: 'Imobiliária',
};

const TABS = [
  { id: 'defaults', label: 'Valores padrão' },
  { id: 'rules', label: 'Comissão por papel' },
  { id: 'payout', label: 'Repasse' },
  { id: 'accounts', label: 'Plano de contas' },
] as const;

type TabId = (typeof TABS)[number]['id'];

/**
 * As retenções sobre a comissão do corretor são alíquotas: 0 desliga a
 * retenção (a imobiliária não retém) e 100 é o teto — o backend recusa
 * qualquer valor fora de 0–100 (tenant_financial_settings/settings.go).
 */
type RetentionKey = 'default_inss_pct' | 'default_iss_pct';

const RETENTION_FIELDS = [
  {
    key: 'default_inss_pct',
    label: 'INSS retido do autônomo (%)',
    help: '0 = não retém. Corretor autônomo contribui tipicamente com 20%.',
  },
  {
    key: 'default_iss_pct',
    label: 'ISS retido (%)',
    help: 'ISS varia por município, tipicamente 2% a 5%. 0 = não retém.',
  },
] as const satisfies readonly { key: RetentionKey; label: string; help: string }[];

const isValidRetentionPct = (value: number) => Number.isFinite(value) && value >= 0 && value <= 100;

const RETENTION_ERROR = 'Informe uma alíquota entre 0 e 100.';

export default function FinancialSettings() {
  const [searchParams, setSearchParams] = useSearchParams();
  const requestedTab = searchParams.get('tab');
  const tab: TabId = TABS.find((item) => item.id === requestedTab)?.id ?? 'defaults';
  const setTab = (nextTab: TabId) => setSearchParams((params) => {
    params.set('tab', nextTab);
    return params;
  });
  const [settings, setSettings] = useState<TenantFinancialSettings | null>(null);
  const [rules, setRules] = useState<CommissionRule[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [loadFailed, setLoadFailed] = useState(false);
  const [loadAttempt, setLoadAttempt] = useState(0);
  const { isRecovering: sessionRecovering } = useSessionRecovery();

  useEffect(() => {
    let active = true;
    setLoading(true);
    setLoadFailed(false);
    (async () => {
      try {
        const [s, r] = await Promise.all([
          tenantFinancialSettingsService.get(),
          commissionRulesService.get(),
        ]);
        if (!active) return;
        setSettings(s);
        setRules(r);
      } catch (err) {
        if (!active) return;
        setLoadFailed(true);
        console.error(err);
        toast.error('Não foi possível carregar as configurações financeiras.');
      } finally {
        if (active) setLoading(false);
      }
    })();
    return () => {
      active = false;
    };
  }, [loadAttempt]);

  const updateField = <K extends keyof TenantFinancialSettings>(key: K, value: TenantFinancialSettings[K]) => {
    setSettings((prev) => (prev ? { ...prev, [key]: value } : prev));
  };

  // Alíquota fora de 0–100 é recusada pelo backend: barrar aqui evita um PUT
  // perdido e diz no próprio campo o que corrigir.
  const invalidRetentions = settings
    ? RETENTION_FIELDS.filter((field) => !isValidRetentionPct(settings[field.key]))
    : [];

  const updateRule = (role: CommissionRole, pct: number) => {
    setRules((prev) => {
      const exists = prev.find((r) => r.role === role);
      if (exists) {
        return prev.map((r) => (r.role === role ? { ...r, default_percentage: pct } : r));
      }
      return [...prev, { tenant_id: settings?.tenant_id || '', role, default_percentage: pct }];
    });
  };

  const handleSave = async () => {
    if (!settings) return;
    if (invalidRetentions.length > 0) {
      toast.error(`Revise ${invalidRetentions.map((field) => field.label).join(' e ')}: ${RETENTION_ERROR}`);
      return;
    }
    setSaving(true);
    try {
      if (tab === 'rules') {
        const distribution = calculateCommissionDistribution(rules);
        if (distribution.overallocated) {
          throw new Error('A soma dos papéis, sem a imobiliária, não pode passar de 100%.');
        }
        const updated = await commissionRulesService.update({
          rules: buildCommissionRulesPayload(rules),
        });
        setRules(updated);
        toast.success('Regras de comissão atualizadas');
      } else {
        const updated = await tenantFinancialSettingsService.update({
          default_late_fee_pct: settings.default_late_fee_pct,
          default_monthly_interest_pct: settings.default_monthly_interest_pct,
          default_due_day: settings.default_due_day,
          default_commission_pct_rent: settings.default_commission_pct_rent,
          default_commission_pct_sale: settings.default_commission_pct_sale,
          default_currency: settings.default_currency,
          payout_proof_required: settings.payout_proof_required,
          admin_fee_pct: settings.admin_fee_pct,
          default_lease_term_months: settings.default_lease_term_months,
          default_inss_pct: settings.default_inss_pct,
          default_iss_pct: settings.default_iss_pct,
        });
        setSettings(updated);
        toast.success('Configurações salvas');
      }
    } catch (err) {
      console.error(err);
      notifyApiError(err, { context: "Salvar configurações financeiras" });
    } finally {
      setSaving(false);
    }
  };

  const distribution = calculateCommissionDistribution(rules);

  return (
    <div className="p-6 max-w-4xl mx-auto space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-foreground">Configurações financeiras</h1>
        <p className="text-sm text-muted-foreground mt-1">
          Valores padrão, divisão de comissões e contas usadas nos contratos de venda e locação.
        </p>
      </div>

      <div className="flex gap-1 overflow-x-auto border-b border-border" role="group" aria-label="Seções das configurações financeiras">
        {TABS.map((t) => (
          <button
            key={t.id}
            type="button"
            aria-pressed={tab === t.id}
            onClick={() => setTab(t.id)}
            className={`whitespace-nowrap px-4 py-2 text-sm font-medium border-b-2 transition-colors ${
              tab === t.id
                ? 'border-primary-600 text-primary-600'
                : 'border-transparent text-muted-foreground hover:text-foreground'
            }`}
          >
            {t.label}
          </button>
        ))}
      </div>

      {tab === 'accounts' && <ChartOfAccountsSection />}

      {tab !== 'accounts' && loading && (
        <div role="status" className="flex items-center justify-center gap-2 p-8 text-sm text-muted-foreground">
          <Loader2 className="size-5 animate-spin" />Carregando configurações…
        </div>
      )}

      {/* Enquanto a sessão é restaurada, "não foi possível carregar" seria um
          erro falso: a requisição falhou com credencial vencida e será refeita. */}
      {tab !== 'accounts' && !loading && (loadFailed || !settings) && sessionRecovering && (
        <Card className="flex items-center gap-2 p-6 text-sm text-muted-foreground" role="status">
          <Loader2 className="w-4 h-4 animate-spin" aria-hidden="true" />
          Restaurando a sessão...
        </Card>
      )}

      {tab !== 'accounts' && !loading && (loadFailed || !settings) && !sessionRecovering && (
        <Card className="space-y-3 p-6" role="alert">
          <p className="text-sm">Não foi possível carregar as configurações financeiras.</p>
          <Button variant="outline" onClick={() => setLoadAttempt((attempt) => attempt + 1)}>Tentar novamente</Button>
        </Card>
      )}

      {tab === 'defaults' && !loading && settings && (
        <Card className="p-6 space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <NumberField
              label="Multa (%)"
              icon={<Percent className="w-4 h-4" />}
              value={settings.default_late_fee_pct}
              onChange={(v) => updateField('default_late_fee_pct', v)}
              help="Aplicada ao atraso (ex.: 2 = 2%)"
            />
            <NumberField
              label="Juros mensais (%)"
              icon={<Percent className="w-4 h-4" />}
              value={settings.default_monthly_interest_pct}
              onChange={(v) => updateField('default_monthly_interest_pct', v)}
              help="Pro-rata diário a partir do vencimento"
            />
            <NumberField
              label="Dia padrão de vencimento"
              icon={<Calendar className="w-4 h-4" />}
              value={settings.default_due_day}
              onChange={(v) => updateField('default_due_day', v)}
              help="1 a 28 (dia do mês)"
              max={28}
              step={1}
            />
            <NumberField
              label="Comissão padrão de venda (%)"
              icon={<DollarSign className="w-4 h-4" />}
              value={settings.default_commission_pct_sale}
              onChange={(v) => updateField('default_commission_pct_sale', v)}
              help="Sobre o valor total da venda"
            />
            <NumberField
              label="Comissão padrão de locação (%)"
              icon={<DollarSign className="w-4 h-4" />}
              value={settings.default_commission_pct_rent}
              onChange={(v) => updateField('default_commission_pct_rent', v)}
              help="Sobre o primeiro aluguel (intermediação)"
            />
            <NumberField
              label="Taxa de administração (%)"
              icon={<Percent className="w-4 h-4" />}
              value={settings.admin_fee_pct}
              onChange={(v) => updateField('admin_fee_pct', v)}
              help="Retida mensalmente antes do repasse ao proprietário"
            />
            <NumberField
              label="Prazo padrão da locação (meses)"
              icon={<Calendar className="w-4 h-4" />}
              value={settings.default_lease_term_months}
              onChange={(v) => updateField('default_lease_term_months', v)}
              help="Sugerido no plano pré-contrato; calcula o término e o residual do último aluguel"
              max={120}
              min={1}
              step={1}
            />
            {/* Retenções sobre a comissão do corretor: as alíquotas padrão que o
                lançamento de desconto pré-preenche (INSS de autônomo / ISS). */}
            {RETENTION_FIELDS.map((field) => (
              <NumberField
                key={field.key}
                label={field.label}
                icon={<Percent className="w-4 h-4" />}
                value={settings[field.key]}
                onChange={(v) => updateField(field.key, v)}
                help={field.help}
                error={isValidRetentionPct(settings[field.key]) ? undefined : RETENTION_ERROR}
              />
            ))}
          </div>
        </Card>
      )}

      {tab === 'rules' && !loading && settings && (
        <Card className="p-6 space-y-4">
          <div>
            <p className="text-sm font-semibold mb-2">Divisão da comissão por papel do corretor</p>
            <p className="text-xs text-muted-foreground">
              O restante é atribuído à imobiliária. Papéis com 0% não participam.
              A assinatura provisiona a comissão, e sua liberação depende da cobertura pelos recebimentos.
            </p>
          </div>
          <div className="space-y-3">
            {COMMISSION_ROLES.map((role) => {
              const isImobiliaria = role === 'imobiliaria';
              const pct = isImobiliaria
                ? distribution.agencyPercentage
                : commissionRulePercentage(rules, role);
              return (
                <div key={role} className="flex items-center gap-3">
                  <label className="w-40 text-sm font-medium flex items-center gap-1.5">
                    {ROLE_LABELS[role]}
                    {isImobiliaria && (
                      <span className="text-caption text-muted-foreground bg-muted px-1.5 py-0.5 rounded">
                        resto
                      </span>
                    )}
                  </label>
                  <input
                    type="number"
                    min={0}
                    max={100}
                    step={0.1}
                    value={pct}
                    onChange={(e) => {
                      if (!isImobiliaria) {
                        updateRule(role, parseFloat(e.target.value) || 0);
                      }
                    }}
                    className="w-24 px-3 py-2 bg-card border border-border rounded-xl text-sm text-right"
                    disabled={isImobiliaria}
                  />
                  <span className="text-sm text-muted-foreground">%</span>
                  {isImobiliaria && (
                    <span className="text-xs text-muted-foreground ml-1">
                      (calculado automaticamente)
                    </span>
                  )}
                </div>
              );
            })}
            <div className="flex items-center justify-between pt-3 border-t border-border">
              <span className="text-sm font-bold">Soma</span>
              <span
                className={`text-sm font-bold ${
                  Math.abs(distribution.totalPercentage - 100) < 0.01
                    ? 'text-status-success'
                    : distribution.overallocated
                    ? 'text-status-critical'
                    : 'text-muted-foreground'
                }`}
              >
                {distribution.totalPercentage.toFixed(1)}%
              </span>
            </div>
            {distribution.overallocated && (
              <p className="text-xs text-status-critical">
                A soma dos papéis, sem a imobiliária, não pode passar de 100%.
              </p>
            )}
            {distribution.nonAgencyPercentage > 0 && distribution.nonAgencyPercentage < 100 && (
              <p className="text-xs text-muted-foreground">
                Os papéis somam {distribution.nonAgencyPercentage.toFixed(1)}%. Os{' '}
                {distribution.agencyPercentage.toFixed(1)}% restantes serão atribuídos à imobiliária.
              </p>
            )}
            {distribution.nonAgencyPercentage === 0 && (
              <p className="text-xs text-muted-foreground">
                Nenhum papel configurado. A comissão será provisionada como valor total sem distribuição por corretor.
              </p>
            )}
          </div>
        </Card>
      )}

      {tab === 'payout' && !loading && settings && (
        <div className="space-y-5">
        <FinancialFlowSummary />
        <Card className="p-6 space-y-4">
          <label className="flex items-center gap-3 cursor-pointer">
            <input
              type="checkbox"
              checked={settings.payout_proof_required}
              onChange={(e) => updateField('payout_proof_required', e.target.checked)}
            />
            <FileCheck className="w-4 h-4" />
            <div>
              <p className="text-sm font-medium">Exigir comprovante nas confirmações manuais</p>
              <p className="text-xs text-muted-foreground">
                Quando ligado, confirmar um pagamento ou recebimento só finaliza após anexar o comprovante
                (PDF/imagem ou URL). Vale para ordens de pagamento, comissões, repasses e baixas de lançamentos —
                todos registram uma operação feita fora do sistema, e o comprovante é a evidência dela.
              </p>
            </div>
          </label>
        </Card>
        </div>
      )}

      {tab !== 'accounts' && !loading && settings && !loadFailed && <div className="flex justify-end">
        <Button
          onClick={handleSave}
          disabled={saving || invalidRetentions.length > 0 || (tab === 'rules' && distribution.overallocated)}
        >
          {saving ? <Loader2 className="w-4 h-4 animate-spin mr-2" /> : <Save className="w-4 h-4 mr-2" />}
          Salvar alterações
        </Button>
      </div>}
    </div>
  );
}

function NumberField({
  label,
  icon,
  value,
  onChange,
  help,
  error,
  max = 100,
  min = 0,
  step = 0.1,
}: {
  label: string;
  icon?: React.ReactNode;
  value: number;
  onChange: (v: number) => void;
  help?: string;
  /** Mensagem de validação do próprio campo (pt-BR), exibida sob o input. */
  error?: string;
  max?: number;
  min?: number;
  step?: number;
}) {
  // htmlFor/id: sem a associação o leitor de tela lê o campo sem nome — e o
  // teste não consegue mirar o input pelo rótulo.
  const inputId = useId();
  return (
    <div>
      <label htmlFor={inputId} className="flex items-center gap-2 text-sm font-medium mb-1">
        {icon}
        {label}
      </label>
      <input
        id={inputId}
        type="number"
        min={min}
        max={max}
        step={step}
        value={value}
        onChange={(e) => onChange(parseFloat(e.target.value) || 0)}
        aria-invalid={error ? true : undefined}
        className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm"
      />
      {error ? (
        <p role="alert" className="text-xs font-medium text-status-critical mt-1">
          {error}
        </p>
      ) : (
        help && <p className="text-xs text-muted-foreground mt-1">{help}</p>
      )}
    </div>
  );
}
