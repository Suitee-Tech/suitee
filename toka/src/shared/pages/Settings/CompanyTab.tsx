import React, { useState, useCallback, useMemo, useEffect, useRef, forwardRef, useImperativeHandle } from "react";
import { motion } from "motion/react";
import { Upload, Globe, Mail, CheckCircle2, AlertTriangle, Phone, MapPin, Shield, Hash } from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";
import { useTheme } from "../../context/ThemeContext";
import { usePermissionsContext } from "../../context/PermissionsContext";
import { Button } from "../../ui/button";
import { useAuth } from "../../context/AuthContext";
import { settingsService } from "../../services/settings.service";
import { leadService, type BusinessHours } from "../../services/lead.service";
import suiteeLogo from "../../../assets/logo.png";
import { CompanyLogo } from "../../components/CompanyLogo";
import { type BusinessHoursValue } from "../../components/WeeklyHoursGrid";
import { isValidCnpj, parseCep, parseCnpj, parsePhoneBR } from "../../utils/masks";
import { SectionCard, SettingsInput, type TabRef } from "./_shared";
import { TeamSection } from "./company/TeamSection";
import { GroupsSection } from "./company/GroupsSection";
import { BusinessHoursSection } from "./company/BusinessHoursSection";
import { notifyApiError } from "../../services/errorToast";

interface CompanyFormState {
  razaoSocial: string;
  nomeFantasia: string;
  cnpj: string;
  creci: string;
  email: string;
  phone: string;
  website: string;
  endereco: string;
  numero: string;
  complemento: string;
  bairro: string;
  cidade: string;
  estado: string;
  cep: string;
  horarioAbertura: string;
  horarioFechamento: string;
  diasOperacao: string[];
  businessHours: BusinessHoursValue;
  logoPreview: string | undefined;
}

export const CompanyTab = forwardRef<TabRef, { onChange: () => void }>(({ onChange }, ref) => {
  const { user, companies, refreshUserData } = useAuth();
  const { updateTheme } = useTheme();
  const { can } = usePermissionsContext();

  // Grupos são governados por PERFIL no backend
  // (toca-auth/internal/service/group_service.go: checkCompanyAdmin exige
  // proprietário ou profile admin). Não existem permissões de catálogo para
  // isso — os ids antigos (`company:manage_groups`,
  // `company:manage_group_members`) nunca existiram no backend e davam a
  // impressão de que a matriz controlava o acesso a grupos.
  const canManageGroups = user?.permission === "owner" || user?.permission === "admin";
  const canManageGroupMembers = canManageGroups;

  const [invitations, setInvitations] = useState<any[]>([]);
  const [teamMembers, setTeamMembers] = useState<any[]>([]);
  const [groups, setGroups] = useState<any[]>([]);

  const currentCompany = useMemo(
    () => companies.find((c) => c.id === user?.current_company_id) || companies[0],
    [companies, user?.current_company_id],
  );
  const currentCompanyId = currentCompany?.id || "";

  const loadGroups = useCallback(async () => {
    if (!currentCompany) return;
    try {
      const data = await settingsService.listGroups(currentCompany.id);
      setGroups(data || []);
    } catch (err) {
      console.error("Failed to load groups", err);
    }
  }, [currentCompany]);

  const loadInvitations = useCallback(async () => {
    if (!currentCompany) return;
    try {
      const [invData, memData] = await Promise.all([
        settingsService.listInvitations(currentCompany.id),
        settingsService.listCompanyMembers(currentCompany.id),
      ]);
      setInvitations(invData || []);
      setTeamMembers(memData || []);
    } catch (err) {
      console.error("Failed to load invitations or members", err);
    }
  }, [currentCompany]);

  const loadCompanyMembers = useCallback(async () => {
    try {
      if (currentCompany) {
        const members = await settingsService.listCompanyMembers(currentCompany.id);
        setTeamMembers(members);
      }
    } catch (error) {
      console.error("Erro ao carregar membros da empresa:", error);
    }
  }, [currentCompany]);

  const reloadTeamAndGroups = useCallback(async () => {
    await Promise.all([loadGroups(), loadInvitations()]);
    await loadCompanyMembers();
  }, [loadGroups, loadInvitations, loadCompanyMembers]);

  const [company, setCompany] = useState<CompanyFormState>({
    razaoSocial: "",
    nomeFantasia: "",
    cnpj: "",
    creci: "",
    email: "",
    phone: "",
    website: "",
    endereco: "",
    numero: "",
    complemento: "",
    bairro: "",
    cidade: "",
    estado: "",
    cep: "",
    horarioAbertura: "08:00",
    horarioFechamento: "18:00",
    diasOperacao: ["Seg", "Ter", "Qua", "Qui", "Sex"],
    businessHours: {
      timezone: "America/Sao_Paulo",
      windows: {
        "1": { start: "08:00", end: "18:00" },
        "2": { start: "08:00", end: "18:00" },
        "3": { start: "08:00", end: "18:00" },
        "4": { start: "08:00", end: "18:00" },
        "5": { start: "08:00", end: "18:00" },
      },
    },
    logoPreview: undefined,
  });

  useEffect(() => {
    let active = true;
    if (user && companies.length > 0 && active) {
      const cc = companies.find((c) => c.id === user.current_company_id) || companies[0];
      if (cc) {
        setCompany((prev) => {
          const razaoSocial = cc.legal_name || cc.name;
          const nomeFantasia = cc.name;
          const email = cc.email || "";
          const cnpj = parseCnpj(cc.tax_id || "");
          const phone = parsePhoneBR(cc.phone || "");
          const endereco = cc.address?.street || "";
          const numero = cc.address?.number || "";
          const complemento = cc.address?.complement || "";
          const bairro = cc.address?.district || "";
          const cidade = cc.address?.city || "";
          const estado = cc.address?.state || "";
          const cep = parseCep(cc.address?.zip_code || "");

          if (
            prev.razaoSocial !== razaoSocial ||
            prev.nomeFantasia !== nomeFantasia ||
            prev.email !== email ||
            prev.cnpj !== cnpj
          ) {
            return {
              ...prev,
              razaoSocial,
              nomeFantasia,
              cnpj,
              email,
              phone,
              endereco,
              numero,
              complemento,
              bairro,
              cidade,
              estado,
              cep,
            };
          }
          return prev;
        });

        settingsService
          .getCompanySettings()
          .then((settings) => {
            if (!active) return;
            setCompany((prev) => {
              const horarioAbertura = settings.horarioAbertura || prev.horarioAbertura;
              const horarioFechamento = settings.horarioFechamento || prev.horarioFechamento;
              const diasOperacao = settings.diasOperacao?.length ? settings.diasOperacao : prev.diasOperacao;
              const mapDiaToWeekday: Record<string, string> = {
                Dom: "0",
                Seg: "1",
                Ter: "2",
                Qua: "3",
                Qui: "4",
                Sex: "5",
                Sab: "6",
              };
              const legacyWindows: Record<string, { start: string; end: string }> = {};
              (diasOperacao || []).forEach((d) => {
                const key = mapDiaToWeekday[d];
                if (!key) return;
                if (!horarioAbertura || !horarioFechamento) return;
                legacyWindows[key] = { start: horarioAbertura, end: horarioFechamento };
              });
              const businessHours = settings.businessHours
                ? settings.businessHours
                : { timezone: "America/Sao_Paulo", windows: legacyWindows };

              return {
                ...prev,
                creci: settings.creci || prev.creci,
                website: settings.website || prev.website,
                horarioAbertura,
                horarioFechamento,
                diasOperacao,
                businessHours,
                logoPreview: settings.logoPreview || prev.logoPreview,
              };
            });
          })
          .catch((err) => console.error("Error loading company settings:", err));

        loadInvitations();
        loadGroups();
      }
    }
    return () => {
      active = false;
    };
  }, [user, companies, loadInvitations, loadGroups]);

  const REQUIRED_FIELDS: { key: keyof CompanyFormState; label: string }[] = useMemo(
    () => [
      { key: "razaoSocial", label: "Razão Social" },
      { key: "nomeFantasia", label: "Nome Fantasia" },
      { key: "cnpj", label: "CNPJ" },
      { key: "creci", label: "CRECI" },
      { key: "email", label: "E-mail de Contato" },
      { key: "phone", label: "Telefone" },
      { key: "endereco", label: "Logradouro" },
      { key: "numero", label: "Número" },
      { key: "bairro", label: "Bairro" },
      { key: "cidade", label: "Cidade" },
      { key: "estado", label: "Estado" },
      { key: "cep", label: "CEP" },
    ],
    [],
  );

  const getFieldError = useCallback(
    (key: keyof CompanyFormState): string | undefined => {
      const val = company[key];
      if (typeof val === "string" && val.trim() === "") {
        return "Campo obrigatório";
      }
      if (key === "email" && typeof val === "string" && val.trim() !== "") {
        const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
        if (!emailRegex.test(val)) return "E-mail inválido";
      }
      if (key === "cnpj" && typeof val === "string" && val.trim() !== "") {
        const cnpjClean = val.replace(/\D/g, "");
        if (cnpjClean.length !== 14) return "CNPJ deve ter 14 dígitos";
        if (!isValidCnpj(cnpjClean)) return "CNPJ inválido (dígitos verificadores)";
      }
      if (key === "cep" && typeof val === "string" && val.trim() !== "") {
        const cepClean = val.replace(/\D/g, "");
        if (cepClean.length !== 8) return "CEP deve ter 8 dígitos";
      }
      return undefined;
    },
    [company],
  );

  const [touched, setTouched] = useState<Record<string, boolean>>({});
  const [showAllErrors, setShowAllErrors] = useState(false);

  const hasAnyError = useMemo(() => REQUIRED_FIELDS.some((f) => getFieldError(f.key)), [getFieldError, REQUIRED_FIELDS]);

  useImperativeHandle(ref, () => ({
    save: async () => {
      if (!currentCompany) return;

      const hasErrors = REQUIRED_FIELDS.some((f) => getFieldError(f.key));
      if (hasErrors) {
        setShowAllErrors(true);
        const allTouched: Record<string, boolean> = {};
        REQUIRED_FIELDS.forEach((f) => {
          allTouched[f.key] = true;
        });
        setTouched((prev) => ({ ...prev, ...allTouched }));
        throw new Error("Preencha todos os campos obrigatórios do Perfil da Empresa antes de salvar.");
      }

      await settingsService.updateCompanyAccount(currentCompany.id, {
        name: company.nomeFantasia,
        legal_name: company.razaoSocial,
        tax_id: company.cnpj,
        email: company.email || "contato@empresa.com",
        phone: company.phone,
        address: {
          street: company.endereco,
          number: company.numero,
          complement: company.complemento,
          district: company.bairro,
          city: company.cidade,
          state: company.estado,
          zip_code: company.cep,
        },
      });

      const bh = company.businessHours;
      const windows: Record<string, { start: string; end: string }> = bh?.windows ?? {};
      const mapWeekdayToDia: Record<string, string> = {
        "0": "Dom",
        "1": "Seg",
        "2": "Ter",
        "3": "Qua",
        "4": "Qui",
        "5": "Sex",
        "6": "Sab",
      };
      const diasOperacao = Object.keys(windows)
        .map((k) => mapWeekdayToDia[k])
        .filter(Boolean);
      const pickKey = ["1", "2", "3", "4", "5", "6", "0"].find((k) => !!windows[k]);
      const horarioAbertura = pickKey ? windows[pickKey].start : company.horarioAbertura;
      const horarioFechamento = pickKey ? windows[pickKey].end : company.horarioFechamento;

      await settingsService.updateCompanySettings({
        creci: company.creci,
        website: company.website,
        horarioAbertura,
        horarioFechamento,
        diasOperacao,
        businessHours: bh ?? undefined,
        logoPreview: company.logoPreview || "",
      });

      try {
        const rouletteBH: BusinessHours | undefined = bh ? { timezone: bh.timezone, windows: bh.windows } : undefined;
        await leadService.updateRouletteConfig({ globalBusinessHours: rouletteBH });
      } catch {
      }
    },
  }));

  const shouldShowError = (key: keyof CompanyFormState): string | undefined => {
    const isRequired = REQUIRED_FIELDS.some((f) => f.key === key);
    if (!isRequired && !touched[key as keyof typeof touched]) return undefined;
    if (!touched[key as keyof typeof touched] && !showAllErrors) return undefined;
    return getFieldError(key);
  };

  const update = <K extends keyof CompanyFormState>(key: K, value: CompanyFormState[K]) => {
    setCompany((prev) => ({ ...prev, [key]: value }));
    setTouched((prev) => ({ ...prev, [key]: true }));
    onChange();
  };

  const logoInputRef = useRef<HTMLInputElement>(null);

  const handleLogoUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    try {
      const result = await settingsService.uploadCompanyLogo(file);
      update("logoPreview", result.logo_url);
      updateTheme("logoUrl", result.logo_url);
      toast.success("Logo carregado com sucesso!");
      if (refreshUserData) await refreshUserData();
    } catch (error: any) {
      notifyApiError(error, { context: "Enviar logo" });
    }
  };

  const handleValidateAll = () => {
    setShowAllErrors(true);
    const allTouched: Record<string, boolean> = {};
    REQUIRED_FIELDS.forEach((f) => {
      allTouched[f.key] = true;
    });
    setTouched((prev) => ({ ...prev, ...allTouched }));

    if (hasAnyError) {
      toast.error("Preencha todos os campos obrigatórios antes de salvar.");
    } else {
      toast.success("Todos os campos estao preenchidos corretamente!");
    }
  };

  return (
    <div className="space-y-6">
      {showAllErrors && hasAnyError && (
        <motion.div
          initial={{ opacity: 0, y: -8 }}
          animate={{ opacity: 1, y: 0 }}
          className="flex items-start gap-3 p-4 bg-status-critical-soft border border-status-critical/30 rounded-2xl"
        >
          <AlertTriangle className="w-5 h-5 text-status-critical shrink-0 mt-0.5" />
          <div>
            <p className="text-sm font-bold text-status-critical">Campos obrigatorios pendentes</p>
            <p className="text-xs text-status-critical mt-0.5">
              Preencha os campos destacados em vermelho:{" "}
              {REQUIRED_FIELDS.filter((f) => getFieldError(f.key))
                .map((f) => f.label)
                .join(", ")}
              .
            </p>
          </div>
        </motion.div>
      )}

      <SectionCard
        title="Perfil da Empresa"
        subtitle="Informações basicas que aparecerao em contratos e propostas"
        actions={
          <Button
            onClick={handleValidateAll}
            size="sm"
            variant="secondary"
            className={clsx(
              "text-xs font-bold",
              showAllErrors && !hasAnyError && "bg-status-ok-soft text-status-ok border border-status-ok/30 hover:bg-status-ok-soft",
            )}
          >
            {showAllErrors && !hasAnyError ? (
              <>
                <CheckCircle2 className="w-3.5 h-3.5" /> Validado
              </>
            ) : (
              <>
                <AlertTriangle className="w-3.5 h-3.5" /> Validar Campos
              </>
            )}
          </Button>
        }
      >
        <div className="space-y-6">
          <div className="flex items-center gap-6 pb-6 border-b border-border">
            <div className="w-24 h-24 rounded-2xl bg-muted border-2 border-dashed border-border flex items-center justify-center shrink-0 overflow-hidden">
              {company.logoPreview ? (
                <div className="w-full h-full flex items-center justify-center p-2 bg-card">
                  <CompanyLogo logoUrl={company.logoPreview} alt="Logo da Empresa" className="w-full h-full object-contain" />
                </div>
              ) : (
                <div className="w-full h-full bg-muted flex items-center justify-center opacity-50 p-4">
                  <img src={suiteeLogo} alt="Logo padrão Suitee" className="w-full h-full object-contain opacity-60 grayscale" />
                </div>
              )}
            </div>
            <div className="space-y-2">
              <input
                type="file"
                ref={logoInputRef}
                className="hidden"
                accept="image/png, image/jpeg, image/svg+xml"
                onChange={handleLogoUpload}
              />
              <Button onClick={() => logoInputRef.current?.click()} variant="outline">
                <Upload className="w-4 h-4" />
                {company.logoPreview ? "Trocar Logo" : "Enviar Logo"}
              </Button>
              <p className="text-[11px] text-muted-foreground">PNG ou SVG, max. 2MB. Fundo transparente recomendado.</p>
              {company.logoPreview && (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    update("logoPreview", undefined);
                    updateTheme("logoUrl", undefined);
                  }}
                  className="h-auto px-2 py-1 text-[11px] font-bold text-status-critical hover:text-status-critical"
                >
                  Remover logo
                </Button>
              )}
            </div>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
            <div className="space-y-1.5">
              <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">
                Razão Social <span className="text-status-critical">*</span>
              </label>
              <SettingsInput ariaLabel="Razão Social" required value={company.razaoSocial} onChange={(v) => update("razaoSocial", v)} error={shouldShowError("razaoSocial")} />
            </div>
            <div className="space-y-1.5">
              <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">
                Nome Fantasia <span className="text-status-critical">*</span>
              </label>
              <SettingsInput ariaLabel="Nome Fantasia" required value={company.nomeFantasia} onChange={(v) => update("nomeFantasia", v)} error={shouldShowError("nomeFantasia")} />
            </div>
            <div className="space-y-1.5">
              <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">
                CNPJ <span className="text-status-critical">*</span>
              </label>
              <SettingsInput ariaLabel="CNPJ" required value={company.cnpj} onChange={(v) => update("cnpj", v)} type="cnpj" icon={<Hash className="w-4 h-4" />} error={shouldShowError("cnpj")} />
            </div>
            <div className="space-y-1.5">
              <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">
                CRECI <span className="text-status-critical">*</span>
              </label>
              <SettingsInput ariaLabel="CRECI" required value={company.creci} onChange={(v) => update("creci", v)} icon={<Shield className="w-4 h-4" />} error={shouldShowError("creci")} />
            </div>
            <div className="space-y-1.5">
              <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">
                E-mail de Contato <span className="text-status-critical">*</span>
              </label>
              <SettingsInput ariaLabel="E-mail de Contato" required value={company.email} onChange={(v) => update("email", v)} type="email" icon={<Mail className="w-4 h-4" />} error={shouldShowError("email")} />
            </div>
            <div className="space-y-1.5">
              <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">
                Telefone <span className="text-status-critical">*</span>
              </label>
              <SettingsInput ariaLabel="Telefone" required value={company.phone} onChange={(v) => update("phone", v)} type="phone" icon={<Phone className="w-4 h-4" />} error={shouldShowError("phone")} />
            </div>
            <div className="space-y-1.5 md:col-span-2">
              <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Website</label>
              <SettingsInput ariaLabel="Website" value={company.website} onChange={(v) => update("website", v)} icon={<Globe className="w-4 h-4" />} />
            </div>
          </div>
        </div>
      </SectionCard>

      <SectionCard title="Endereço" subtitle="Localização física da sede da imobiliária" delay={0.05}>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
          <div className="space-y-1.5 md:col-span-2">
            <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">
              Logradouro <span className="text-status-critical">*</span>
            </label>
            <SettingsInput ariaLabel="Logradouro" required value={company.endereco} onChange={(v) => update("endereco", v)} icon={<MapPin className="w-4 h-4" />} error={shouldShowError("endereco")} />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-1.5">
              <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">
                Número <span className="text-status-critical">*</span>
              </label>
              <SettingsInput ariaLabel="Número" required value={company.numero} onChange={(v) => update("numero", v)} error={shouldShowError("numero")} />
            </div>
            <div className="space-y-1.5">
              <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Complemento</label>
              <SettingsInput ariaLabel="Complemento" value={company.complemento} onChange={(v) => update("complemento", v)} />
            </div>
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">
              Bairro <span className="text-status-critical">*</span>
            </label>
            <SettingsInput ariaLabel="Bairro" required value={company.bairro} onChange={(v) => update("bairro", v)} error={shouldShowError("bairro")} />
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">
              Cidade <span className="text-status-critical">*</span>
            </label>
            <SettingsInput ariaLabel="Cidade" required value={company.cidade} onChange={(v) => update("cidade", v)} error={shouldShowError("cidade")} />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-1.5">
              <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">
                Estado <span className="text-status-critical">*</span>
              </label>
              <SettingsInput ariaLabel="Estado" required value={company.estado} onChange={(v) => update("estado", v)} error={shouldShowError("estado")} />
            </div>
            <div className="space-y-1.5">
              <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">
                CEP <span className="text-status-critical">*</span>
              </label>
              <SettingsInput ariaLabel="CEP" required value={company.cep} onChange={(v) => update("cep", v)} type="cep" error={shouldShowError("cep")} />
            </div>
          </div>
        </div>
      </SectionCard>

      <BusinessHoursSection
        horarioAbertura={company.horarioAbertura}
        horarioFechamento={company.horarioFechamento}
        businessHours={company.businessHours}
        onChangeHorarioAbertura={(v) => update("horarioAbertura", v)}
        onChangeHorarioFechamento={(v) => update("horarioFechamento", v)}
        onChangeBusinessHours={(v) => update("businessHours", v)}
      />

      <TeamSection
        companyId={currentCompanyId}
        user={user}
        teamMembers={teamMembers}
        invitations={invitations}
        groups={groups}
        canManageGroupMembers={canManageGroupMembers}
        onReload={reloadTeamAndGroups}
      />

      <GroupsSection companyId={currentCompanyId} groups={groups} canManageGroups={canManageGroups} onReload={loadGroups} />
    </div>
  );
});
