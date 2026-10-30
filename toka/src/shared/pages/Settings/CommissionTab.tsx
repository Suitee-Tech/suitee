import { forwardRef, useImperativeHandle, useState, useCallback } from "react";
import { DollarSign, AlertTriangle } from "lucide-react";
import { toast } from "sonner";
import { Link } from "react-router";
import { Can } from "../../permissions/Can";
import { useAuth } from "../../context/AuthContext";
import { tenantFinancialSettingsService, type TenantFinancialSettings } from "../../services/tenantFinancialSettings.service";
import { commissionRulesService, type CommissionRole, type CommissionRule } from "../../services/commissionRules.service";
import {
  buildCommissionRulesPayload,
  calculateCommissionDistribution,
  commissionRulePercentage,
  COMMISSION_ROLES,
} from "../../lib/commissionRules";
import { SectionCard, FieldRow, type TabRef } from "./_shared";
import { useSettingsLoad, SettingsLoadState } from "./useSettingsLoad";

const ROLE_LABELS: Record<CommissionRole, string> = {
  captador: "Captador",
  co_captador: "Co-captador",
  visitador: "Visitador",
  atendente: "Atendente",
  fechamento: "Fechamento",
  imobiliaria: "Imobiliária",
};

const DEFAULT_SETTINGS: TenantFinancialSettings = {
  tenant_id: "",
  default_late_fee_pct: 2,
  default_monthly_interest_pct: 1,
  default_due_day: 10,
  default_commission_pct_rent: 100,
  default_commission_pct_sale: 6,
  default_currency: "BRL",
  payout_proof_required: true,
  admin_fee_pct: 10,
  default_lease_term_months: 30,
  // As retenções de INSS/ISS do corretor são editadas em Configurações
  // financeiras (aba Valores padrão); aqui só completamos o contrato do tipo.
  default_inss_pct: 0,
  default_iss_pct: 0,
  updated_at: "",
};

interface CommissionTabProps {
  onChange: () => void;
}

export const CommissionTab = forwardRef<TabRef, CommissionTabProps>(function CommissionTab({ onChange }, ref) {
  const { user } = useAuth();
  const [settings, setSettings] = useState<TenantFinancialSettings>(DEFAULT_SETTINGS);
  const [rules, setRules] = useState<CommissionRule[]>([]);

  // Fail-closed: as duas leituras precisam concluir para o formulário existir.
  // Antes, um GET falho deixava os defaults editáveis e o Salvar podia gravar
  // 6%/100%/10% por cima das regras reais da imobiliária.
  const load = useSettingsLoad(useCallback(async () => {
    const [nextSettings, nextRules] = await Promise.all([
      tenantFinancialSettingsService.get(),
      commissionRulesService.get(),
    ]);
    if (!nextSettings) throw new Error("Configurações financeiras não retornadas.");
    if (!Array.isArray(nextRules)) throw new Error("Regras de comissão não retornadas.");
    setSettings(nextSettings);
    setRules(nextRules);
  }, []));

  const updateRule = useCallback((role: CommissionRole, pct: number) => {
    setRules((prev) => {
      const exists = prev.find((r) => r.role === role);
      if (exists) {
        return prev.map((r) => (r.role === role ? { ...r, default_percentage: pct } : r));
      }
      return [...prev, { tenant_id: settings?.tenant_id || "", role, default_percentage: pct }];
    });
    onChange();
  }, [settings?.tenant_id, onChange]);

  const updateField = useCallback(<K extends keyof TenantFinancialSettings>(key: K, value: TenantFinancialSettings[K]) => {
    setSettings((prev) => ({ ...prev, [key]: value }));
    onChange();
  }, [onChange]);

  const save = useCallback(async () => {
    load.requireReady();
    try {
      const distribution = calculateCommissionDistribution(rules);
      if (distribution.overallocated) {
        throw new Error("A soma dos papéis, sem a imobiliária, não pode passar de 100%.");
      }

      // Salva defaults de comissão (SALE/RENT)
      await tenantFinancialSettingsService.update({
        default_late_fee_pct: settings.default_late_fee_pct,
        default_monthly_interest_pct: settings.default_monthly_interest_pct,
        default_due_day: settings.default_due_day,
        default_commission_pct_rent: settings.default_commission_pct_rent,
        default_commission_pct_sale: settings.default_commission_pct_sale,
        default_currency: settings.default_currency,
        payout_proof_required: settings.payout_proof_required,
        admin_fee_pct: settings.admin_fee_pct,
      });

      // Salva regras de distribuição por papel
      const updatedRules = await commissionRulesService.update({
        rules: buildCommissionRulesPayload(rules),
      });
      setRules(updatedRules);

      toast.success("Configurações de comissão salvas com sucesso");
    } catch (err: any) {
      console.error("Failed to save commission settings", err);
      const msg = err?.response?.data?.message || err?.message || "Erro ao salvar configurações de comissão";
      toast.error(msg);
      throw err;
    }
  }, [settings, rules, load]);

  useImperativeHandle(ref, () => ({ save }), [save]);

  if (!load.ready) return <SettingsLoadState error={load.error} retry={load.retry} />;

  const distribution = calculateCommissionDistribution(rules);
  const hasCompleteDistribution = Math.abs(distribution.totalPercentage - 100) < 0.01;

  return (
    <div className="space-y-6">
      <Can permission="finance:write">
        <div className="rounded-xl border border-border bg-card p-4 text-sm">
          <p className="text-muted-foreground">Consulte como as comissões e os repasses de vendas e locações são classificados.</p>
          <Link to="/configuracoes/financeiro?tab=accounts" className="mt-2 inline-block font-semibold text-primary-600 hover:underline">
            Ver plano de contas e fluxo financeiro
          </Link>
        </div>
      </Can>
      {/* ── Comissão padrão (SALE / RENT) ── */}
      <SectionCard title="Percentual de comissão padrão" subtitle="Aplicado automaticamente quando um contrato ou negócio não define um percentual próprio.">
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <FieldRow label="Comissão padrão VENDA (%)" description="Sobre o valor total da venda">
            <div className="flex items-center gap-2">
              <DollarSign className="w-4 h-4 text-muted-foreground" />
              <input
                type="number"
                min={0}
                max={100}
                step={0.1}
                value={settings.default_commission_pct_sale}
                onChange={(e) => updateField("default_commission_pct_sale", parseFloat(e.target.value) || 0)}
                className="w-28 px-3 py-2 bg-card border border-border rounded-xl text-sm text-right"
              />
              <span className="text-sm text-muted-foreground">%</span>
            </div>
          </FieldRow>
          <FieldRow label="Comissão padrão LOCAÇÃO (%)" description="Sobre o primeiro aluguel (intermediação)">
            <div className="flex items-center gap-2">
              <DollarSign className="w-4 h-4 text-muted-foreground" />
              <input
                type="number"
                min={0}
                max={100}
                step={0.1}
                value={settings.default_commission_pct_rent}
                onChange={(e) => updateField("default_commission_pct_rent", parseFloat(e.target.value) || 0)}
                className="w-28 px-3 py-2 bg-card border border-border rounded-xl text-sm text-right"
              />
              <span className="text-sm text-muted-foreground">%</span>
            </div>
          </FieldRow>
        </div>
        <p className="text-xs text-muted-foreground mt-3">
          Ex: 6% de comissão sobre R$ 500.000 = R$ 30.000 a distribuir entre os corretores.
        </p>
      </SectionCard>

      {/* ── Distribuição por papel (regras de comissão) ── */}
      <SectionCard
        title="Distribuição por papel do corretor"
        subtitle="A imobiliária recebe automaticamente o restante. Uma configuração zerada mantém a comissão consolidada, sem divisão por corretor."
      >
        <div className="space-y-3">
          {COMMISSION_ROLES.map((role) => {
            const isImobiliaria = role === "imobiliaria";
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

          {/* Soma + validação */}
          <div className="flex items-center justify-between pt-3 border-t border-border">
            <span className="text-sm font-bold">Soma</span>
            <span
              className={`text-sm font-bold flex items-center gap-1.5 ${
                hasCompleteDistribution
                  ? "text-status-success"
                  : distribution.overallocated
                    ? "text-status-critical"
                    : "text-muted-foreground"
              }`}
            >
              {distribution.overallocated && <AlertTriangle className="w-3.5 h-3.5" />}
              {distribution.totalPercentage.toFixed(1)}%
            </span>
          </div>

          {distribution.overallocated && (
            <p className="text-xs text-status-critical">
              A soma dos papéis, sem a imobiliária, não pode passar de 100%.
            </p>
          )}
          {distribution.nonAgencyPercentage < 100 && distribution.nonAgencyPercentage > 0 && (
            <p className="text-xs text-muted-foreground">
              Os papéis somam {distribution.nonAgencyPercentage.toFixed(1)}%. Os {distribution.agencyPercentage.toFixed(1)}% restantes
              serão atribuídos à imobiliária como receita retida.
            </p>
          )}
          {distribution.nonAgencyPercentage === 0 && (
            <p className="text-xs text-muted-foreground">
              Nenhum papel configurado. A comissão será provisionada como valor total
              sem distribuição por corretor.
            </p>
          )}
        </div>
      </SectionCard>

      {/* ── Nota de permissionamento ── */}
      {user?.permission !== "owner" && user?.permission !== "admin" && (
        <div className="px-4 py-3 bg-status-warning-soft rounded-xl border border-status-warning/20">
          <p className="text-sm text-status-warning flex items-center gap-2">
            <AlertTriangle className="w-4 h-4 shrink-0" />
            As alterações nas regras de comissão afetam todos os novos contratos.
            Revise com atenção antes de salvar.
          </p>
        </div>
      )}
    </div>
  );
});
