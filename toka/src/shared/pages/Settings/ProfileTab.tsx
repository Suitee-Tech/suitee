import React, { useState, useEffect, useRef, forwardRef, useImperativeHandle, useId } from "react";
import { Upload, Phone, Key } from "lucide-react";
import { toast } from "sonner";
import { useAuth } from "../../context/AuthContext";
import { useAgentsContext } from "../../context/AgentsContext";
import { usePermissionsContext } from "../../context/PermissionsContext";
import { Button } from "../../ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "../../ui/dialog";
import { UserAvatar } from "../../components/UserAvatar";
import { WeeklyHoursGrid, type BusinessHoursValue } from "../../components/WeeklyHoursGrid";
import { MultiSelectTagPicker } from "../../components/MultiSelectTagPicker";
import { DEFAULT_QUAL_FIELDS } from "../../components/QualificationFormBuilder";
import { settingsService } from "../../services/settings.service";
import { leadService, type QualField, type QualRoutingConfig } from "../../services/lead.service";
import { brokerService, type BrokerDTO, type BusinessHoursDTO } from "../../services/broker.service";
import { formatPhoneBR, parsePhoneBR } from "../../utils/masks";
import { SectionCard, SettingsInput, type TabRef } from "./_shared";
import { notifyApiError } from "../../services/errorToast";

export const ProfileTab = forwardRef<TabRef, { onChange: () => void }>(({ onChange }, ref) => {
  const { user, refreshUserData } = useAuth();
  const { refreshBrokers } = useAgentsContext();
  const { can } = usePermissionsContext();
  const [profile, setProfile] = useState({
    name: user?.fullname || (user as any)?.name || "",
    email: user?.email || "",
    phone: (user as any)?.phone || "",
    bio: (user as any)?.bio || "",
    avatarPreview: (user as any)?.avatar_url || null,
  });

  const canWriteBrokers = can("brokers:write");

  const [brokerQualLoading, setBrokerQualLoading] = useState(false);
  const [brokerQualSaving, setBrokerQualSaving] = useState(false);
  const [brokerQualSnapshot, setBrokerQualSnapshot] = useState("");
  const [brokerQual, setBrokerQual] = useState<{
    specialties: string[];
    regions: string[];
    businessHours: BusinessHoursValue;
  }>({ specialties: [], regions: [], businessHours: null });
  const [brokerQualOptions, setBrokerQualOptions] = useState<{ specialties: string[]; regions: string[] }>({
    specialties: [],
    regions: [],
  });

  const [showPasswordModal, setShowPasswordModal] = useState(false);
  const [passwordData, setPasswordData] = useState({ current: "", new: "", confirm: "" });
  const [passwordError, setPasswordError] = useState("");
  const bioId = useId();
  const passwordCurrentId = useId();
  const passwordNewId = useId();
  const passwordConfirmId = useId();
  const passwordErrorId = useId();

  const handleChangePassword = async () => {
    if (!passwordData.new || passwordData.new !== passwordData.confirm) {
      setPasswordError("As novas senhas não coincidem.");
      return;
    }
    try {
      await settingsService.changePassword({
        current_password: passwordData.current,
        new_password: passwordData.new,
      });
      toast.success("Senha alterada com sucesso!");
      setShowPasswordModal(false);
      setPasswordData({ current: "", new: "", confirm: "" });
      setPasswordError("");
    } catch (e: any) {
      setPasswordError(e.message || "Erro ao alterar a senha.");
    }
  };

  const fileInputRef = useRef<HTMLInputElement>(null);
  // true depois que o usuário edita qualquer campo: o formulário passa a ser
  // a fonte da verdade até desmontar (a aba remonta ao navegar e re-semeia).
  const profileEditedRef = useRef(false);

  const handleAvatarChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    try {
      const result = await settingsService.uploadAvatar(file);
      setProfile((prev) => ({ ...prev, avatarPreview: result.avatar_url }));
      toast.success("Foto de perfil atualizada!");
      if (refreshUserData) await refreshUserData();
      onChange();
    } catch (error: any) {
      notifyApiError(error, { context: "Enviar foto de perfil" });
    }
  };

  const snapshotBrokerQual = (v: { specialties: string[]; regions: string[]; businessHours: BusinessHoursValue }) => {
    const sp = Array.from(new Set((v.specialties ?? []).map((s) => String(s || "").trim()).filter(Boolean))).sort((a, b) =>
      a.localeCompare(b),
    );
    const rg = Array.from(new Set((v.regions ?? []).map((s) => String(s || "").trim()).filter(Boolean))).sort((a, b) =>
      a.localeCompare(b),
    );
    const bh = v.businessHours ?? null;
    return JSON.stringify({ specialties: sp, regions: rg, businessHours: bh });
  };

  useImperativeHandle(ref, () => ({
    save: async () => {
      const [firstname, ...lastnames] = profile.name.split(" ");
      await settingsService.updateProfile({
        firstname: firstname || "",
        lastname: lastnames.join(" ") || "",
        phone: profile.phone,
        bio: profile.bio,
        avatar_url: profile.avatarPreview || undefined,
      });

      if (user?.id && brokerQualSnapshot && brokerQualSnapshot !== snapshotBrokerQual(brokerQual)) {
        const payload = {
          specialties: brokerQual.specialties,
          regions: brokerQual.regions,
          business_hours: (brokerQual.businessHours as BusinessHoursDTO) ?? undefined,
        };
        if (canWriteBrokers) {
          await brokerService.update(user.id, payload);
        } else {
          await brokerService.updateMyQualification(payload);
        }
        setBrokerQualSnapshot(snapshotBrokerQual(brokerQual));
      }
      refreshBrokers().catch(() => {});
    },
  }));

  useEffect(() => {
    let active = true;
    const load = async () => {
      if (!user?.id) return;
      setBrokerQualLoading(true);
      try {
        const [cfg, brokerResp] = await Promise.all([leadService.getRouletteConfig(), brokerService.getById(user.id)]);
        if (!active) return;

        const broker = (brokerResp.data?.data || (brokerResp.data as any) || (brokerResp as any).data) as BrokerDTO | null;
        const fields: QualField[] =
          Array.isArray((cfg as any)?.qualFields) && (cfg as any).qualFields.length > 0
            ? ((cfg as any).qualFields as QualField[])
            : (DEFAULT_QUAL_FIELDS as QualField[]);
        const routing = ((cfg as any)?.qualRouting as QualRoutingConfig | undefined) ?? null;

        const selectableFields = fields.filter((f) => f.active && (f.type === "select" || f.type === "multiselect"));
        const specialtyFieldIds = Array.isArray((routing as any)?.specialtyFieldIds) ? (routing as any).specialtyFieldIds : [];
        const specialtySources = specialtyFieldIds.length > 0 ? selectableFields.filter((f) => specialtyFieldIds.includes(f.id)) : selectableFields;
        const specs = new Set<string>();
        specialtySources.forEach((f) => {
          (Array.isArray(f.options) ? f.options : []).forEach((s) => {
            const v = String(s || "").trim();
            if (v) specs.add(v);
          });
        });

        const regionFieldId = ((routing as any)?.regionFieldId as string | null | undefined) ?? null;
        const regionSources = regionFieldId
          ? fields.filter((f) => f.id === regionFieldId)
          : fields.filter((f) => {
              const label = String(f.label || "").toLowerCase();
              return label.includes("regiao") || label.includes("bairro") || label.includes("local");
            });
        const regs = new Set<string>();
        regionSources.forEach((f) => {
          (Array.isArray(f.options) ? f.options : []).forEach((r) => {
            const v = String(r || "").trim();
            if (v) regs.add(v);
          });
        });

        const next = {
          specialties: Array.isArray(broker?.specialties) ? broker!.specialties : [],
          regions: Array.isArray(broker?.regions) ? broker!.regions : [],
          businessHours: ((broker?.business_hours as BusinessHoursDTO) ?? null) as BusinessHoursValue,
        };
        setBrokerQual(next);
        setBrokerQualSnapshot(snapshotBrokerQual(next));

        const mergedSpecs = new Set<string>(Array.from(specs));
        (next.specialties ?? []).forEach((s) => {
          const v = String(s || "").trim();
          if (v) mergedSpecs.add(v);
        });
        const mergedRegs = new Set<string>(Array.from(regs));
        (next.regions ?? []).forEach((r) => {
          const v = String(r || "").trim();
          if (v) mergedRegs.add(v);
        });
        setBrokerQualOptions({
          specialties: Array.from(mergedSpecs).sort((a, b) => a.localeCompare(b)),
          regions: Array.from(mergedRegs).sort((a, b) => a.localeCompare(b)),
        });
      } catch {
      } finally {
        if (active) setBrokerQualLoading(false);
      }
    };
    load();
    return () => {
      active = false;
    };
  }, [user?.id]);

  useEffect(() => {
    // Semeia o formulário a partir do contexto de auth APENAS até o primeiro
    // toque de edição. Depois disso o objeto `user` fica stale (ele só muda no
    // refresh pós-save) e qualquer re-sync sobrescreveria o que o usuário
    // digitou — era isso que impedia editar o nome completo.
    if (!user || profileEditedRef.current) return;
    setProfile((prev) => ({
      ...prev,
      name: user.fullname || (user as any).name || prev.name,
      email: user.email || prev.email,
    }));
  }, [user]);

  const update = <K extends keyof typeof profile>(key: K, value: (typeof profile)[K]) => {
    profileEditedRef.current = true;
    setProfile((prev) => ({ ...prev, [key]: value }));
    onChange();
  };

  return (
    <div className="space-y-6">
      <SectionCard title="Meu Perfil" subtitle="Informações pessoais e de conta">
        <div className="space-y-6">
          <div className="flex items-center gap-6 pb-6 border-b border-border">
            <UserAvatar
              name={profile.name}
              avatarUrl={profile.avatarPreview || null}
              className="w-24 h-24 border-2 border-border"
              fallbackClassName="bg-muted text-muted-foreground text-2xl font-bold"
            />
            <div className="space-y-2">
              <input
                type="file"
                ref={fileInputRef}
                className="hidden"
                accept="image/png, image/jpeg, image/jpg"
                onChange={handleAvatarChange}
              />
              <Button variant="outline" onClick={() => fileInputRef.current?.click()}>
                <Upload className="w-4 h-4" />
                Trocar Foto
              </Button>
              <p className="text-[11px] text-muted-foreground">JPG ou PNG, max. 2MB.</p>
            </div>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
            <div className="space-y-1.5">
              <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Nome Completo <span aria-hidden="true" className="text-status-critical">*</span></label>
              <SettingsInput ariaLabel="Nome Completo" required value={profile.name} onChange={(v) => update("name", v)} />
            </div>
            <div className="space-y-1.5">
              <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">E-mail (Login) <span aria-hidden="true" className="text-status-critical">*</span></label>
              <SettingsInput ariaLabel="E-mail (Login)" required value={profile.email} onChange={(v) => update("email", v)} type="email" disabled />
            </div>
            <div className="space-y-1.5">
              <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Telefone <span aria-hidden="true" className="text-status-critical">*</span></label>
              <SettingsInput ariaLabel="Telefone" required value={formatPhoneBR(profile.phone)} onChange={(v) => update("phone", parsePhoneBR(v))} icon={<Phone className="w-4 h-4" />} />
            </div>
            <div className="space-y-1.5">
              <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Cargo / Papel</label>
              <div className="px-3 py-2.5 bg-muted border border-border rounded-xl text-sm text-muted-foreground font-medium capitalize">
                {user?.permission === "admin" || user?.permission === "owner" ? "Administrador" : user?.permission || user?.role || "Usuário"}
              </div>
            </div>
            <div className="space-y-1.5 md:col-span-2">
              <label htmlFor={bioId} className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Bio / Descrição Breve</label>
              <textarea
                id={bioId}
                value={profile.bio}
                onChange={(e) => update("bio", e.target.value)}
                className="w-full px-3 py-2.5 bg-card border border-border rounded-xl text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-400 min-h-[80px]"
                placeholder="Um pouco sobre você..."
              />
            </div>
          </div>
        </div>
      </SectionCard>

      <SectionCard title="Qualificação do Corretor" subtitle="Defina seus interesses para receber leads mais alinhados ao seu perfil">
        {brokerQualLoading ? (
          <div className="text-sm text-muted-foreground">Carregando…</div>
        ) : !user?.id ? (
          <div className="text-sm text-muted-foreground">Usuário não identificado</div>
        ) : (
          <div className="space-y-6">
            <div className="grid gap-5">
              <MultiSelectTagPicker
                label="Especialidades"
                value={brokerQual.specialties}
                options={brokerQualOptions.specialties}
                onChange={(next) => {
                  setBrokerQual((prev) => ({ ...prev, specialties: next }));
                  onChange();
                }}
                placeholder="Selecionar especialidades…"
                allowCreate={brokerQualOptions.specialties.length === 0}
              />
              <MultiSelectTagPicker
                label="Regiões"
                value={brokerQual.regions}
                options={brokerQualOptions.regions}
                onChange={(next) => {
                  setBrokerQual((prev) => ({ ...prev, regions: next }));
                  onChange();
                }}
                placeholder="Selecionar regiões…"
                allowCreate
              />
            </div>

            <div className="space-y-2">
              <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Horário para receber leads</label>
              <div className="rounded-2xl border border-border bg-card p-4">
                <WeeklyHoursGrid
                  value={brokerQual.businessHours}
                  onChange={(next) => {
                    setBrokerQual((prev) => ({ ...prev, businessHours: next }));
                    onChange();
                  }}
                />
              </div>
            </div>

            {!canWriteBrokers && (
              <div className="text-xs text-muted-foreground">
                Apenas seus interesses e horário estão disponíveis aqui. Ajustes de capacidade/algoritmo são gerenciados pela administração.
              </div>
            )}

            {brokerQualSnapshot && brokerQualSnapshot !== snapshotBrokerQual(brokerQual) && (
              <div className="flex items-center justify-between gap-4 rounded-xl border border-status-warning/30 bg-status-warning-soft px-4 py-3">
                <div className="text-xs font-bold text-status-warning">Alterações não salvas</div>
                <Button
                  type="button"
                  variant="warning"
                  size="sm"
                  loading={brokerQualSaving}
                  loadingText="Salvando…"
                  onClick={async () => {
                    if (!user?.id) return;
                    if (brokerQualSaving) return;
                    setBrokerQualSaving(true);
                    try {
                      const payload = {
                        specialties: brokerQual.specialties,
                        regions: brokerQual.regions,
                        business_hours: (brokerQual.businessHours as BusinessHoursDTO) ?? undefined,
                      };
                      if (canWriteBrokers) {
                        await brokerService.update(user.id, payload);
                      } else {
                        await brokerService.updateMyQualification(payload);
                      }
                      setBrokerQualSnapshot(snapshotBrokerQual(brokerQual));
                      toast.success("Qualificação atualizada");
                      refreshBrokers().catch(() => {});
                    } catch {
                      toast.error("Erro ao atualizar qualificação");
                    } finally {
                      setBrokerQualSaving(false);
                    }
                  }}
                >
                  Salvar
                </Button>
              </div>
            )}
          </div>
        )}
      </SectionCard>

      <SectionCard title="Segurança da Conta" subtitle="Gerencie sua senha e métodos de acesso">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 py-2">
          <div>
            <p className="text-sm font-bold text-foreground">Senha de Acesso</p>
            <p className="text-xs text-muted-foreground mt-0.5">Última alteração: há 30 dias</p>
          </div>
          <Button variant="outline" onClick={() => setShowPasswordModal(true)} className="w-full sm:w-auto">
            <Key className="w-4 h-4" /> Alterar Senha
          </Button>
        </div>
      </SectionCard>

      <Dialog
        open={showPasswordModal}
        onOpenChange={(next) => {
          if (!next) {
            setShowPasswordModal(false);
            setPasswordError("");
          }
        }}
      >
        <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-md">
          <div className="p-6 border-b border-border pr-12">
            <DialogTitle className="text-lg font-bold text-foreground">Alterar Senha</DialogTitle>
            <DialogDescription className="text-sm mt-1">Insira sua senha atual e a nova senha</DialogDescription>
          </div>

          <div className="p-6 space-y-4">
            {passwordError && (
              <div id={passwordErrorId} role="alert" className="p-3 bg-status-critical-soft text-status-critical text-sm font-medium rounded-xl border border-status-critical/20">
                {passwordError}
              </div>
            )}
            <div>
              <label htmlFor={passwordCurrentId} className="block text-sm font-bold text-foreground mb-1.5">Senha Atual</label>
              <input
                id={passwordCurrentId}
                type="password"
                autoComplete="current-password"
                required
                aria-required="true"
                aria-invalid={passwordError ? true : undefined}
                aria-describedby={passwordError ? passwordErrorId : undefined}
                value={passwordData.current}
                onChange={(e) => setPasswordData((prev) => ({ ...prev, current: e.target.value }))}
                className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all"
              />
            </div>
            <div>
              <label htmlFor={passwordNewId} className="block text-sm font-bold text-foreground mb-1.5">Nova Senha</label>
              <input
                id={passwordNewId}
                type="password"
                autoComplete="new-password"
                required
                aria-required="true"
                aria-invalid={passwordError ? true : undefined}
                aria-describedby={passwordError ? passwordErrorId : undefined}
                value={passwordData.new}
                onChange={(e) => setPasswordData((prev) => ({ ...prev, new: e.target.value }))}
                className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all"
              />
            </div>
            <div>
              <label htmlFor={passwordConfirmId} className="block text-sm font-bold text-foreground mb-1.5">Confirmar Nova Senha</label>
              <input
                id={passwordConfirmId}
                type="password"
                autoComplete="new-password"
                required
                aria-required="true"
                aria-invalid={passwordError ? true : undefined}
                aria-describedby={passwordError ? passwordErrorId : undefined}
                value={passwordData.confirm}
                onChange={(e) => setPasswordData((prev) => ({ ...prev, confirm: e.target.value }))}
                className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all"
              />
            </div>
          </div>

          <div className="p-6 bg-muted border-t border-border flex justify-end gap-3">
            <Button
              variant="ghost"
              onClick={() => {
                setShowPasswordModal(false);
                setPasswordError("");
              }}
            >
              Cancelar
            </Button>
            <Button onClick={handleChangePassword} disabled={!passwordData.current || !passwordData.new || !passwordData.confirm}>
              Salvar Nova Senha
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
});
