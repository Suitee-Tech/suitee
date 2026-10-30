import { useId, useState } from "react";
import {
  AlertCircle,
  Building2,
  Handshake,
  Loader2,
  MapPin,
  Search,
  X,
} from "lucide-react";
import { toast } from "sonner";
import { Sheet, SheetContent, SheetDescription, SheetTitle } from "../ui/sheet";
import { Button } from "../ui/button";
import { Switch } from "../ui/switch";
import {
  partnershipService,
  effectiveFinancial,
  grantFrom,
  counterpart,
  type AgreementChange,
  type SharingGrant,
  type Partnership,
  type PartnershipType,
  type FinancialConfig,
  type SharingConfig,
  type RoyaltyBase,
  type PermissionScope,
  type CompanyLookup,
} from "../services/partnership.service";
import { isValidCnpj, parseCnpj } from "../utils/masks";
import { DocumentInput } from "../ui/document-input";
import { MoneyInput } from "../ui/money-input";
import { useConfirmAction } from "../hooks/useConfirmAction";
import { Input } from "../ui/input";
import { useAuth } from "../context/AuthContext";
import { PercentInput } from "../ui/percent-input";

const ROYALTY_BASE_OPTIONS: { value: RoyaltyBase; label: string }[] = [
  { value: "gross_commission", label: "Comissão bruta" },
  { value: "net_commission", label: "Comissão líquida" },
  { value: "revenue", label: "Receita" },
];

const PROPERTY_TYPE_OPTIONS: { value: string; label: string }[] = [
  { value: "apartment", label: "Apartamento" },
  { value: "house", label: "Casa" },
  { value: "land", label: "Terreno" },
  { value: "commercial", label: "Comercial" },
];

const LABELED_INPUT_CLS =
  "w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none transition-all disabled:opacity-60 disabled:cursor-not-allowed";

function LabeledInput({
  label,
  value,
  onChange,
  type = "text",
  placeholder,
  disabled,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  type?: "text" | "number" | "percent" | "money";
  placeholder?: string;
  disabled?: boolean;
}) {
  // Bridge string state <-> number for the masked inputs, preserving the
  // external (v: string) => void contract so call sites stay unchanged.
  const inputId = useId();
  const numValue = value === "" ? undefined : Number(value);
  return (
    <div>
      <label
        htmlFor={inputId}
        className="block text-xs font-medium text-foreground mb-1"
      >
        {label}
      </label>
      {type === "percent" ? (
        <PercentInput
          id={inputId}
          value={numValue}
          onChange={(v) => onChange(v == null ? "" : String(v))}
          placeholder={placeholder}
          disabled={disabled}
          className={LABELED_INPUT_CLS}
        />
      ) : type === "money" ? (
        <MoneyInput
          id={inputId}
          withSymbol
          value={numValue}
          onChange={(v) => onChange(v == null ? "" : String(v))}
          placeholder={placeholder}
          disabled={disabled}
          className={LABELED_INPUT_CLS}
        />
      ) : (
        <input
          id={inputId}
          type={type}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={placeholder}
          disabled={disabled}
          className={LABELED_INPUT_CLS}
        />
      )}
    </div>
  );
}

interface NovoParceiroModalProps {
  onClose: () => void;
  onCreated: () => void;
}

export function NovoParceiroModal({
  onClose,
  onCreated,
}: NovoParceiroModalProps) {
  const { user } = useAuth();
  const [cnpj, setCnpj] = useState("");
  const [searching, setSearching] = useState(false);
  const [found, setFound] = useState<CompanyLookup | null>(null);
  const [lookupError, setLookupError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState({
    type: "partner" as PartnershipType,
    royaltyPercentage: "",
    adminFee: "",
    transactionFee: "",
    feePayer: "" as NonNullable<FinancialConfig["fee_payer"]> | "",
    ownerCompanyPct: "50",
    closingCompanyPct: "50",
  });

  const ownerPct = Number(form.ownerCompanyPct) || 0;
  const closingPct = Number(form.closingCompanyPct) || 0;
  const commissionSum = ownerPct + closingPct;

  const cnpjValid = isValidCnpj(cnpj);

  const resetLookup = () => {
    setFound(null);
    setLookupError(null);
  };

  const handleSearch = async () => {
    if (!cnpjValid || searching) return;
    setSearching(true);
    resetLookup();
    try {
      const company = await partnershipService.lookupByCnpj(parseCnpj(cnpj));
      setFound(company);
    } catch (error: any) {
      if (error?.response?.status === 404) {
        setLookupError("Nenhuma empresa encontrada para este CNPJ.");
      } else {
        setLookupError(
          error?.response?.data?.message ??
            "Não foi possível buscar a empresa.",
        );
      }
    } finally {
      setSearching(false);
    }
  };

  const handleSubmit = async () => {
    if (!found || saving) return;
    if (found.id === user?.current_company_id || !found.is_active) {
      toast.error("Escolha outra empresa ativa.");
      return;
    }
    if (Math.abs(commissionSum - 100) > 0.01) {
      toast.error("A divisão da comissão deve somar 100%.");
      return;
    }
    if (
      (Number(form.adminFee) > 0 || Number(form.transactionFee) > 0) &&
      !form.feePayer
    ) {
      toast.error("Defina quem paga as taxas da parceria.");
      return;
    }
    setSaving(true);
    try {
      await partnershipService.create({
        target_id: found.id,
        type: form.type,
        financial_config: {
          fee_payer: form.feePayer || undefined,
          royalty_percentage: Number(form.royaltyPercentage) || 0,
          admin_fee: Number(form.adminFee) || 0,
          transaction_fee: Number(form.transactionFee) || 0,
          royalty_base: "gross_commission",
          commission_model: {
            owner_company_pct: ownerPct,
            closing_company_pct: closingPct,
          },
        },
        permissions: [{ scope: "shared_portfolio", access_level: "view" }],
      });
      toast.success(
        "Convite enviado. A parceria é criada quando o administrador da empresa aceitar.",
      );
      onCreated();
      onClose();
    } catch (error: any) {
      toast.error(
        error?.response?.data?.message ?? "Não foi possível enviar o convite.",
      );
    } finally {
      setSaving(false);
    }
  };

  const typeNoun = form.type === "franchise" ? "franquia" : "parceria";

  return (
    <Sheet
      open
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
    >
      <SheetContent
        side="right"
        className="w-full gap-0 p-0 sm:max-w-lg flex flex-col"
      >
        <div className="bg-card px-6 py-4 border-b border-border flex items-center gap-3 flex-shrink-0 pr-12">
          <div className="p-2 bg-primary-50 text-primary-700 rounded-lg">
            <Handshake className="w-5 h-5" />
          </div>
          <div>
            <SheetTitle className="text-lg font-bold text-foreground">
              Convidar Parceiro
            </SheetTitle>
            <SheetDescription className="text-sm">
              Busque a empresa pelo CNPJ e envie um convite de parceria ou
              franquia.
            </SheetDescription>
          </div>
        </div>

        <div className="flex-1 overflow-y-auto p-6 space-y-5">
          <div>
            <label className="block text-sm font-bold text-foreground mb-1.5">
              CNPJ da empresa *
            </label>
            <div className="flex gap-2">
              <DocumentInput
                mode="cnpj"
                value={cnpj}
                onChange={(v) => {
                  setCnpj(v);
                  resetLookup();
                }}
                onKeyDown={(e) => {
                  if (e.key === "Enter") handleSearch();
                }}
                placeholder="00.000.000/0000-00"
                className="flex-1 px-3 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none transition-all"
              />
              <Button onClick={handleSearch} disabled={!cnpjValid || searching}>
                {searching ? (
                  <Loader2 className="w-4 h-4 animate-spin" />
                ) : (
                  <Search className="w-4 h-4" />
                )}
                Buscar
              </Button>
            </div>
            {cnpj.length > 0 && !cnpjValid && (
              <p className="text-[11px] text-status-critical mt-1">
                CNPJ inválido.
              </p>
            )}
            {lookupError && (
              <p className="text-xs text-status-critical mt-2 flex items-center gap-1.5">
                <AlertCircle className="w-3.5 h-3.5 shrink-0" /> {lookupError}
              </p>
            )}
          </div>

          {found && (
            <div className="bg-primary-50 border border-primary-200 rounded-xl p-4 flex items-start gap-3">
              <div className="w-10 h-10 rounded-lg bg-card text-primary-600 flex items-center justify-center shrink-0 border border-primary-100">
                <Building2 className="w-5 h-5" />
              </div>
              <div className="min-w-0">
                <p className="text-sm font-bold text-foreground truncate">
                  {found.name}
                </p>
                {found.legal_name && found.legal_name !== found.name && (
                  <p className="text-xs text-muted-foreground truncate">
                    {found.legal_name}
                  </p>
                )}
                <p className="text-xs text-muted-foreground mt-0.5 flex items-center gap-1">
                  <MapPin className="w-3 h-3" />{" "}
                  {[found.city, found.state].filter(Boolean).join("/") ||
                    "Localização não informada"}
                </p>
                {!found.is_active && (
                  <span className="inline-block mt-1 text-caption font-bold uppercase tracking-wider text-status-warning">
                    Empresa inativa
                  </span>
                )}
              </div>
            </div>
          )}

          {found && (
            <>
              <div>
                <label className="block text-sm font-bold text-foreground mb-2">
                  Tipo de vínculo *
                </label>
                <div className="grid grid-cols-2 gap-2">
                  {[
                    {
                      value: "partner" as PartnershipType,
                      label: "Imobiliária Parceira",
                    },
                    {
                      value: "franchise" as PartnershipType,
                      label: "Franquia",
                    },
                  ].map((opt) => (
                    <button
                      key={opt.value}
                      type="button"
                      onClick={() =>
                        setForm((f) => ({ ...f, type: opt.value }))
                      }
                      className={
                        "px-3 py-2.5 rounded-xl border text-sm font-medium transition-all " +
                        (form.type === opt.value
                          ? "bg-primary-50 border-primary-300 text-primary-700 ring-2 ring-primary-600/20"
                          : "bg-card border-border text-muted-foreground hover:bg-muted")
                      }
                    >
                      {opt.label}
                    </button>
                  ))}
                </div>
              </div>

              <div>
                <label className="block text-sm font-bold text-foreground mb-2">
                  Regra Financeira
                </label>
                <div className="grid grid-cols-2 gap-3">
                  <LabeledInput
                    label="Royalty (%)"
                    type="percent"
                    value={form.royaltyPercentage}
                    onChange={(v) =>
                      setForm((f) => ({ ...f, royaltyPercentage: v }))
                    }
                  />
                  <LabeledInput
                    label="Taxa administrativa (R$)"
                    type="money"
                    value={form.adminFee}
                    onChange={(v) => setForm((f) => ({ ...f, adminFee: v }))}
                  />
                  <LabeledInput
                    label="Comissão da parceira de origem (%)"
                    type="percent"
                    value={form.ownerCompanyPct}
                    onChange={(v) =>
                      setForm((f) => ({ ...f, ownerCompanyPct: v }))
                    }
                  />
                  <LabeledInput
                    label="Comissão Fechamento (%)"
                    type="percent"
                    value={form.closingCompanyPct}
                    onChange={(v) =>
                      setForm((f) => ({ ...f, closingCompanyPct: v }))
                    }
                  />
                  <LabeledInput
                    label="Taxa por Transação"
                    type="money"
                    value={form.transactionFee}
                    onChange={(v) =>
                      setForm((f) => ({ ...f, transactionFee: v }))
                    }
                  />
                </div>
                {(Number(form.adminFee) > 0 ||
                  Number(form.transactionFee) > 0) && (
                  <FeePayerSelect
                    value={form.feePayer}
                    onChange={(v) => setForm((f) => ({ ...f, feePayer: v }))}
                  />
                )}
                {commissionSum !== 100 && (
                  <p className="text-[11px] text-status-warning mt-1.5">
                    A soma das comissões é {commissionSum}%. O ideal é que somem
                    100%.
                  </p>
                )}
              </div>

              <p className="text-[11px] text-muted-foreground">
                Um convite será enviado ao administrador da empresa. A{" "}
                {typeNoun} é criada quando o convite for aceito.
              </p>
            </>
          )}
        </div>

        <div className="border-t border-border p-4 flex gap-3 bg-muted flex-shrink-0">
          <Button variant="outline" onClick={onClose} className="flex-1">
            Cancelar
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={
              !found ||
              saving ||
              Math.abs(commissionSum - 100) > 0.01 ||
              ((Number(form.adminFee) > 0 || Number(form.transactionFee) > 0) &&
                !form.feePayer) ||
              !found.is_active ||
              found.id === user?.current_company_id
            }
            className="flex-1"
          >
            {saving && <Loader2 className="w-4 h-4 animate-spin" />}
            Enviar Convite
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  );
}

interface GerenciarAcordoModalProps {
  partnership: Partnership;
  partnerName: string;
  companyId: string;
  canEdit: boolean;
  onClose: () => void;
  onUpdated: () => void;
}
const actionLabels: Record<string, string> = {
  created: "Convite enviado",
  accept: "Convite aceito",
  save: "Compartilhamento atualizado",
  reject: "Convite recusado",
  cancel: "Convite cancelado",
  suspend: "Parceria suspensa",
  terminate: "Parceria encerrada",
  accept_financial: "Proposta financeira aceita",
  reject_financial: "Proposta financeira recusada",
};

function GrantSummary({ grant }: { grant: SharingGrant }) {
  return (
    <div className="space-y-1 text-label">
      <p>
        Portfólio:{" "}
        {grant.permissions.some((p) => p.scope === "shared_portfolio")
          ? "Compartilhado"
          : "Não compartilhado"}
      </p>
      <p>
        Leads:{" "}
        {grant.permissions.some((p) => p.scope === "leads")
          ? "Compartilhados"
          : "Não compartilhados"}
      </p>
      <p>
        Contato de proprietários:{" "}
        {grant.sharing_config.share_owner_contact ? "Visível" : "Oculto"}
      </p>
      <p>
        Contato de leads:{" "}
        {grant.sharing_config.share_lead_contact ? "Visível" : "Oculto"}
      </p>
      <p>
        Co-corretagem:{" "}
        {grant.sharing_config.allow_co_brokering
          ? "Permitida"
          : "Não permitida"}
      </p>
      <p>
        Tipos:{" "}
        {grant.sharing_config.shared_property_types
          ?.map(
            (t) => PROPERTY_TYPE_OPTIONS.find((o) => o.value === t)?.label ?? t,
          )
          .join(", ") || "Todos"}
      </p>
      <p>
        Regiões: {grant.sharing_config.shared_regions?.join(", ") || "Todas"}
      </p>
    </div>
  );
}
function FinancialSummary({ config: rawConfig }: { config: FinancialConfig }) {
  const config = effectiveFinancial(rawConfig);
  return (
    <div className="space-y-1 text-label">
      <p>
        Divisão da comissão (parceira de origem / fechamento):{" "}
        {config.commission_model
          ? `${config.commission_model.owner_company_pct}% / ${config.commission_model.closing_company_pct}%`
          : config.commission_split || "50% / 50%"}
      </p>
      <p>
        Royalties: {config.royalty_percentage}% sobre{" "}
        {ROYALTY_BASE_OPTIONS.find(
          (o) => o.value === (config.royalty_base || "gross_commission"),
        )?.label.toLowerCase()}
      </p>
      <p>
        Taxa administrativa:{" "}
        {Number(config.admin_fee).toLocaleString("pt-BR", {
          style: "currency",
          currency: "BRL",
        })}
      </p>
      <p>
        Taxa por transação:{" "}
        {Number(config.transaction_fee).toLocaleString("pt-BR", {
          style: "currency",
          currency: "BRL",
        })}
      </p>
      {(config.admin_fee > 0 || config.transaction_fee > 0) && (
        <p className="text-caption">
          {config.fee_payer
            ? FEE_PAYER_LABELS[config.fee_payer]
            : "Responsável pelas taxas não definido. Solicite a revisão das condições."}{" "}
          · Valores fixos por negócio, cobrados separadamente na assinatura.
        </p>
      )}
    </div>
  );
}
export function GerenciarAcordoModal({
  partnership: p,
  partnerName,
  companyId,
  canEdit,
  onClose,
  onUpdated,
}: GerenciarAcordoModalProps) {
  const [config, setConfig] = useState<FinancialConfig>(
    effectiveFinancial(p.financial_config),
  );
  const [grant, setGrant] = useState<SharingGrant>(() =>
    grantFrom(p, companyId),
  );
  const [region, setRegion] = useState("");
  const [reason, setReason] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const { confirmAction, confirmationDialog } = useConfirmAction();
  const receiving = grantFrom(p, counterpart(p, companyId));
  const invited = p.status === "pending" && p.target_id === companyId;
  const editable = canEdit && (p.status === "active" || invited) && !saving;
  const model = config.commission_model ?? {
    owner_company_pct: 50,
    closing_company_pct: 50,
  };
  const financialValid =
    [
      config.royalty_percentage,
      config.admin_fee,
      config.transaction_fee,
      model.owner_company_pct,
      model.closing_company_pct,
    ].every((v) => Number.isFinite(v) && v >= 0) &&
    config.royalty_percentage <= 100 &&
    Math.abs(model.owner_company_pct + model.closing_company_pct - 100) <
      0.01 &&
    (!(config.admin_fee > 0 || config.transaction_fee > 0) ||
      Boolean(config.fee_payer));
  const changeSharing = (patch: Partial<SharingConfig>) =>
    setGrant((g) => ({
      ...g,
      sharing_config: { ...g.sharing_config, ...patch },
    }));
  const togglePermission = (scope: PermissionScope, value: boolean) =>
    setGrant((g) => ({
      ...g,
      permissions: value
        ? [
            ...g.permissions.filter((p) => p.scope !== scope),
            { scope, access_level: "view" },
          ]
        : g.permissions.filter((p) => p.scope !== scope),
    }));
  const execute = async (action: AgreementChange["action"]) => {
    if (saving) return;
    if (action === "save" && !financialValid) {
      setError(
        "Verifique as taxas e a divisão da comissão, que deve somar 100%.",
      );
      return;
    }
    if ((action === "suspend" || action === "terminate") && !reason.trim()) {
      setError("Informe o motivo antes de continuar.");
      return;
    }
    setSaving(true);
    setError(null);
    try {
      await partnershipService.change(p.id, {
        expected_version: p.version ?? 0,
        action,
        ...(action === "save" || action === "accept" ? { grant } : {}),
        ...(action === "save" && !p.pending_financial
          ? { financial_config: config }
          : {}),
        reason: reason.trim(),
      });
      toast.success(
        action === "save"
          ? "Compartilhamento salvo. Mudanças financeiras aguardam aceite da outra empresa."
          : "Acordo atualizado.",
      );
      onUpdated();
      onClose();
    } catch (err: unknown) {
      const e = err as { response?: { data?: { message?: string } } };
      setError(
        e.response?.data?.message ??
          "Não foi possível atualizar. Seus dados foram mantidos.",
      );
    } finally {
      setSaving(false);
    }
  };
  const confirmAgreementChange = (action: AgreementChange["action"], label: string) =>
    confirmAction(
      action === "accept"
        ? "Confirme as condições financeiras do convite e o que sua empresa compartilhará com este parceiro."
        : action === "accept_financial"
          ? "Estas condições serão aplicadas aos novos negócios após o aceite. Negócios com termos registrados preservam suas condições."
          : "A ação será registrada no histórico. Suspender ou encerrar bloqueia novos acessos; negócios com termos registrados preservam suas condições.",
      () => execute(action),
      { title: label, confirmLabel: label, variant: "warning" },
    );
  return (
    <>
      <Sheet
        open
        onOpenChange={(open) => {
          if (!open && !saving) onClose();
        }}
      >
        <SheetContent className="flex w-full flex-col overflow-y-auto p-4 sm:max-w-2xl sm:p-6">
          <SheetTitle>
            {invited ? "Revisar convite" : "Acordo de parceria"}
          </SheetTitle>
          <SheetDescription>
            {partnerName} · versão {p.version ?? 0}
          </SheetDescription>
          <section className="space-y-2 rounded-xl border border-border p-4">
            <h3 className="text-label font-bold">
              O que recebo de {partnerName}
            </h3>
            <GrantSummary grant={receiving} />
          </section>
          <section className="space-y-3 rounded-xl border border-border p-4">
            <h3 className="text-label font-bold">
              O que minha empresa compartilha
            </h3>
            <p className="text-caption text-muted-foreground">
              Estas opções controlam somente os seus dados. Cada imóvel e lead
              também precisa estar marcado para compartilhamento.
            </p>
            {(
              [
                ["shared_portfolio", "Compartilhar portfólio"],
                ["leads", "Compartilhar leads"],
              ] as const
            ).map(([scope, label]) => (
              <div
                key={scope}
                className="flex items-center justify-between gap-4 text-label"
              >
                {label}
                <Switch
                  aria-label={label}
                  disabled={!editable}
                  checked={grant.permissions.some(
                    (p) => p.scope === scope && p.access_level === "view",
                  )}
                  onCheckedChange={(v) => togglePermission(scope, v)}
                />
              </div>
            ))}
            {(
              [
                [
                  "share_owner_contact",
                  "Compartilhar contato de proprietários",
                ],
                ["share_lead_contact", "Compartilhar contato de leads"],
                ["allow_co_brokering", "Permitir co-corretagem nos meus dados"],
              ] as const
            ).map(([key, label]) => (
              <div
                key={key}
                className="flex items-center justify-between gap-4 text-label"
              >
                {label}
                <Switch
                  aria-label={label}
                  disabled={!editable}
                  checked={Boolean(grant.sharing_config[key])}
                  onCheckedChange={(v) => changeSharing({ [key]: v })}
                />
              </div>
            ))}
            <p className="text-caption text-muted-foreground">
              Tipos de imóvel. Nenhum selecionado permite todos os tipos.
            </p>
            <div className="flex flex-wrap gap-2">
              {PROPERTY_TYPE_OPTIONS.map((o) => (
                <Button
                  key={o.value}
                  size="sm"
                  variant={
                    grant.sharing_config.shared_property_types?.includes(
                      o.value,
                    )
                      ? "default"
                      : "outline"
                  }
                  disabled={!editable}
                  aria-pressed={grant.sharing_config.shared_property_types?.includes(
                    o.value,
                  )}
                  onClick={() =>
                    changeSharing({
                      shared_property_types:
                        grant.sharing_config.shared_property_types?.includes(
                          o.value,
                        )
                          ? grant.sharing_config.shared_property_types.filter(
                              (v) => v !== o.value,
                            )
                          : [
                              ...(grant.sharing_config.shared_property_types ??
                                []),
                              o.value,
                            ],
                    })
                  }
                >
                  {o.label}
                </Button>
              ))}
            </div>
            <div className="block text-label">
              Regiões (cidade ou bairro)
              <Input
                aria-label="Nova região"
                value={region}
                disabled={!editable}
                onChange={(e) => setRegion(e.target.value)}
              />
            </div>
            <Button
              variant="outline"
              disabled={!editable || !region.trim()}
              onClick={() => {
                changeSharing({
                  shared_regions: [
                    ...new Set([
                      ...(grant.sharing_config.shared_regions ?? []),
                      region.trim(),
                    ]),
                  ],
                });
                setRegion("");
              }}
            >
              Adicionar região
            </Button>
            <p className="text-caption text-muted-foreground">
              Sem regiões, todos os locais são permitidos.
            </p>
            <div className="flex flex-wrap gap-2">
              {grant.sharing_config.shared_regions?.map((r) => (
                <Button
                  key={r}
                  variant="outline"
                  size="sm"
                  disabled={!editable}
                  aria-label={`Remover região ${r}`}
                  onClick={() =>
                    changeSharing({
                      shared_regions:
                        grant.sharing_config.shared_regions.filter(
                          (v) => v !== r,
                        ),
                    })
                  }
                >
                  {r} ×
                </Button>
              ))}
            </div>
          </section>
          <section className="space-y-3 rounded-xl border border-border p-4">
            <h3 className="text-label font-bold">
              Condições financeiras {invited ? "do convite" : "aceitas"}
            </h3>
            <FinancialSummary config={p.financial_config} />
            {p.status === "active" && canEdit && !p.pending_financial && (
              <>
                <p className="text-caption text-muted-foreground">
                  Alterações abaixo geram uma proposta. As condições aceitas
                  continuam vigentes até a confirmação da outra empresa.
                </p>
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  <LabeledInput
                    label="Comissão da parceira de origem (%)"
                    type="percent"
                    value={String(model.owner_company_pct)}
                    onChange={(v) =>
                      setConfig((c) => ({
                        ...c,
                        commission_model: {
                          ...model,
                          owner_company_pct: Number(v),
                        },
                      }))
                    }
                  />
                  <LabeledInput
                    label="Comissão do fechamento (%)"
                    type="percent"
                    value={String(model.closing_company_pct)}
                    onChange={(v) =>
                      setConfig((c) => ({
                        ...c,
                        commission_model: {
                          ...model,
                          closing_company_pct: Number(v),
                        },
                      }))
                    }
                  />
                  <LabeledInput
                    label="Royalties (%)"
                    type="percent"
                    value={String(config.royalty_percentage)}
                    onChange={(v) =>
                      setConfig((c) => ({
                        ...c,
                        royalty_percentage: Number(v),
                      }))
                    }
                  />
                  <label className="text-label">
                    Base dos royalties
                    <select
                      className={LABELED_INPUT_CLS}
                      aria-label="Base dos royalties"
                      value={config.royalty_base}
                      onChange={(e) =>
                        setConfig((c) => ({
                          ...c,
                          royalty_base: e.target.value as RoyaltyBase,
                        }))
                      }
                    >
                      {ROYALTY_BASE_OPTIONS.map((o) => (
                        <option key={o.value} value={o.value}>
                          {o.label}
                        </option>
                      ))}
                    </select>
                  </label>
                  <LabeledInput
                    label="Taxa administrativa"
                    type="money"
                    value={String(config.admin_fee)}
                    onChange={(v) =>
                      setConfig((c) => ({ ...c, admin_fee: Number(v) }))
                    }
                  />
                  <LabeledInput
                    label="Taxa por transação"
                    type="money"
                    value={String(config.transaction_fee)}
                    onChange={(v) =>
                      setConfig((c) => ({ ...c, transaction_fee: Number(v) }))
                    }
                  />
                </div>
                {(config.admin_fee > 0 || config.transaction_fee > 0) && (
                  <FeePayerSelect
                    value={config.fee_payer ?? ""}
                    onChange={(v) =>
                      setConfig((c) => ({ ...c, fee_payer: v || undefined }))
                    }
                  />
                )}
                {!financialValid && (
                  <p role="alert" className="text-label text-status-critical">
                    A divisão deve somar 100%. Para taxas maiores que zero,
                    defina quem paga.
                  </p>
                )}
              </>
            )}
            {p.pending_financial && (
              <div className="space-y-3 rounded-lg border border-border p-3">
                <h4 className="text-label font-bold">
                  Proposta financeira aguardando aceite
                </h4>
                <FinancialSummary
                  config={p.pending_financial.financial_config}
                />
                {canEdit && p.pending_financial.proposed_by !== companyId ? (
                  <div className="flex gap-2">
                    <Button
                      disabled={
                        saving ||
                        ((p.pending_financial.financial_config.admin_fee > 0 ||
                          p.pending_financial.financial_config.transaction_fee >
                            0) &&
                          !p.pending_financial.financial_config.fee_payer)
                      }
                      onClick={() =>
                        confirmAgreementChange(
                          "accept_financial",
                          "Aceitar proposta financeira",
                        )
                      }
                    >
                      Aceitar proposta financeira
                    </Button>
                    <Button
                      variant="outline"
                      disabled={saving}
                      onClick={() =>
                        confirmAgreementChange(
                          "reject_financial",
                          "Recusar proposta financeira",
                        )
                      }
                    >
                      Recusar proposta financeira
                    </Button>
                  </div>
                ) : (
                  <p className="text-caption text-muted-foreground">
                    A outra empresa precisa revisar esta proposta.
                  </p>
                )}
              </div>
            )}
          </section>
          {canEdit && (
            <section className="space-y-3">
              <h3 className="text-label font-bold">Situação da parceria</h3>
              {(p.status === "active" || p.status === "suspended") && (
                <>
                  <div className="block text-label">
                    Motivo da suspensão ou encerramento
                    <Input
                      aria-label="Motivo"
                      value={reason}
                      onChange={(e) => setReason(e.target.value)}
                    />
                  </div>
                  <div className="flex gap-2">
                    {p.status === "active" && (
                      <Button
                        variant="outline"
                        disabled={saving || !reason.trim()}
                        onClick={() => confirmAgreementChange("suspend", "Suspender parceria")}
                      >
                        Suspender parceria
                      </Button>
                    )}
                    <Button
                      variant="outline"
                      disabled={saving || !reason.trim()}
                      onClick={() => confirmAgreementChange("terminate", "Encerrar parceria")}
                    >
                      Encerrar parceria
                    </Button>
                  </div>
                </>
              )}
              {p.status === "pending" && (
                <Button
                  variant="outline"
                  disabled={saving}
                  onClick={() =>
                    confirmAgreementChange(
                      invited ? "reject" : "cancel",
                      invited ? "Recusar convite" : "Cancelar convite",
                    )
                  }
                >
                  {invited ? "Recusar convite" : "Cancelar convite"}
                </Button>
              )}
            </section>
          )}
          <section className="space-y-2">
            <h3 className="text-label font-bold">Histórico</h3>
            {p.audit_logs?.length ? (
              p.audit_logs.map((entry, index) => (
                <p className="text-caption" key={`${entry.timestamp}-${index}`}>
                  {actionLabels[entry.action] ?? entry.action} ·{" "}
                  {new Date(entry.timestamp).toLocaleString("pt-BR")}
                  {entry.details ? ` · ${entry.details}` : ""}
                </p>
              ))
            ) : (
              <p className="text-caption text-muted-foreground">
                Sem histórico registrado.
              </p>
            )}
          </section>
          {error && (
            <p role="alert" className="text-label text-status-critical">
              {error}{" "}
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  onUpdated();
                  onClose();
                }}
              >
                Recarregar acordo
              </Button>
            </p>
          )}
          <div className="flex flex-wrap gap-2 border-t border-border pt-4">
            <Button variant="outline" disabled={saving} onClick={onClose}>
              Fechar
            </Button>
            {editable && (
              <Button
                disabled={saving || !financialValid}
                onClick={() =>
                  invited
                    ? confirmAgreementChange("accept", "Aceitar condições e parceria")
                    : execute("save")
                }
              >
                {invited ? "Aceitar condições e parceria" : "Salvar acordo"}
              </Button>
            )}
          </div>
        </SheetContent>
      </Sheet>
      {confirmationDialog}
    </>
  );
}

const FEE_PAYER_LABELS = {
  closing_pays_source: "Empresa do fechamento paga à parceira de origem",
  source_pays_closing: "Parceira de origem paga à empresa do fechamento",
};
function FeePayerSelect({
  value,
  onChange,
}: {
  value: NonNullable<FinancialConfig["fee_payer"]> | "";
  onChange: (v: NonNullable<FinancialConfig["fee_payer"]> | "") => void;
}) {
  const id = useId();
  return (
    <div className="mt-3 space-y-2">
      <label htmlFor={id} className="text-label font-medium">
        Quem paga as taxas?
      </label>
      <select
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value as typeof value)}
        className="w-full rounded-lg border border-border bg-card p-2 text-label"
      >
        <option value="">Selecione a regra de cobrança</option>
        {Object.entries(FEE_PAYER_LABELS).map(([v, label]) => (
          <option key={v} value={v}>
            {label}
          </option>
        ))}
      </select>
      <p className="text-caption text-muted-foreground">
        Taxas fixas em reais, uma vez por negócio, na assinatura. São obrigações
        separadas: não descontam a comissão nem o repasse do proprietário. A
        outra empresa precisa aceitar esta regra.
      </p>
    </div>
  );
}
