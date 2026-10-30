import { useState, useMemo, useEffect, forwardRef, useImperativeHandle } from "react";
import { motion, AnimatePresence } from "motion/react";
import { Plus, Trash2, Check, Pencil, ChevronRight, Users, AlertTriangle } from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";
import {
  FUNCTIONAL_ROLES,
  ROLE_LABELS,
  PERMISSION_CATALOG,
  DEFAULT_MATRIX,
  type FunctionalRole,
} from "../../../lib/permissions/catalog";
import { useAgentsContext } from "../../context/AgentsContext";
import { useAuth } from "../../context/AuthContext";
import { useIsMobile } from "../../ui/use-mobile";
import { Button } from "../../ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "../../ui/dialog";
import { settingsService, type CustomRole, type Group } from "../../services/settings.service";
import { SectionCard, SettingsInput, SettingsSelect, Badge, type TabRef } from "./_shared";
import { AccessProfileEditor } from "./AccessProfileEditor";
import { assignableRoleOptions, grantedCount, roleDisplayName } from "./accessProfiles";
import { notifyApiError } from "../../services/errorToast";

export const PermissionsTab = forwardRef<TabRef, { onChange: () => void }>(({ onChange }, ref) => {
  const isMobile = useIsMobile();
  const { refreshBrokers } = useAgentsContext();
  const [permissions, setPermissions] = useState<Record<string, Record<FunctionalRole, boolean>>>(DEFAULT_MATRIX);
  const [defaultRole, setDefaultRole] = useState<string>("corretor");
  const [mobileRole, setMobileRole] = useState<FunctionalRole>("corretor");
  const [customRoles, setCustomRoles] = useState<CustomRole[]>([]);
  const [expandedModule, setExpandedModule] = useState<string | null>(null);
  // Editor de perfil (o MESMO componente do diálogo de Grupo de Usuários).
  const [editorProfile, setEditorProfile] = useState<CustomRole | null>(null);
  const [editorOpen, setEditorOpen] = useState(false);
  // Impacto: quem usa cada perfil hoje (Equipe + Grupos). Sem isso, excluir um
  // perfil deixaria gente sem permissão nenhuma sem o operador perceber.
  const [members, setMembers] = useState<Array<{ account_id: string; name?: string; functional_role?: string }>>([]);
  const [groups, setGroups] = useState<Group[]>([]);
  const [deleteProfile, setDeleteProfile] = useState<CustomRole | null>(null);
  const [reassignTo, setReassignTo] = useState("corretor");
  const [deleting, setDeleting] = useState(false);
  const { user } = useAuth();
  const companyId = user?.current_company_id || "";
  // A6/P0.2: a tela NÃO pode salvar antes de saber o que o servidor tem.
  // Antes, o estado iniciava em DEFAULT_MATRIX e a carga era `.catch(console.error)`;
  // uma falha transitória no GET + "Salvar" persistia a matriz inteira de
  // defaults — que, com os catálogos divergentes, ESCALAVA privilégios (ex.:
  // `cliente` ganhava `documents:write`). Agora o save é recusado até a carga
  // concluir com sucesso, e o motivo fica visível para o operador.
  const [loadState, setLoadState] = useState<"loading" | "ready" | "failed">("loading");

  useEffect(() => {
    settingsService
      .getPermissions()
      .then((apiSettings) => {
        if (apiSettings.permissions && apiSettings.permissions.length > 0) {
          const matrix = { ...DEFAULT_MATRIX };
          apiSettings.permissions.forEach((p) => {
            if (matrix[p.id]) {
              matrix[p.id] = { ...matrix[p.id], ...p.roles } as Record<FunctionalRole, boolean>;
            }
          });
          setPermissions(matrix);
        }
        if (apiSettings.defaultRole) {
          setDefaultRole(apiSettings.defaultRole);
        }
        if (apiSettings.customRoles) {
          setCustomRoles(apiSettings.customRoles);
        }
        setLoadState("ready");
      })
      .catch((error) => {
        console.error(error);
        setLoadState("failed");
      });
  }, []);

  // Uso dos perfis (para o aviso de impacto e a exclusão segura). Best-effort:
  // a tela de permissões continua funcionando se a lista de equipe falhar.
  const loadUsage = useMemo(
    () => async () => {
      if (!companyId) return;
      const [membersResult, groupsResult] = await Promise.allSettled([
        settingsService.listCompanyMembers(companyId),
        settingsService.listGroups(companyId),
      ]);
      if (membersResult.status === "fulfilled") setMembers(membersResult.value || []);
      if (groupsResult.status === "fulfilled") setGroups((groupsResult.value as Group[]) || []);
    },
    [companyId],
  );

  useEffect(() => {
    void loadUsage();
  }, [loadUsage]);

  const usageOf = (profileId: string) => ({
    members: members.filter((member) => member.functional_role === profileId),
    groups: groups.filter((group) => group.role_id === profileId),
  });

  useImperativeHandle(ref, () => ({
    save: async () => {
      if (loadState !== "ready") {
        throw new Error(
          loadState === "failed"
            ? "Não foi possível carregar a matriz de permissões do servidor. Recarregue a página antes de salvar — salvar agora sobrescreveria a configuração atual com os padrões."
            : "A matriz de permissões ainda está carregando. Aguarde e tente novamente.",
        );
      }
      // `customRoles` NÃO vai aqui de propósito: a rota da matriz é um replace do
      // documento e não grava perfis (o servidor preserva os persistidos). Perfis
      // são salvos pelo editor, em endpoint próprio e auditado.
      await settingsService.updatePermissions({
        defaultRole,
        permissions: PERMISSION_CATALOG.map((p) => ({
          id: p.id,
          label: p.label,
          module: p.module,
          roles: permissions[p.id],
        })),
      });
      refreshBrokers().catch(() => {});
    },
  }));

  const modules = useMemo(() => [...new Set(PERMISSION_CATALOG.map((p) => p.module))], []);

  const togglePermission = (id: string, role: FunctionalRole) => {
    setPermissions((prev) => ({
      ...prev,
      [id]: {
        ...prev[id],
        [role]: !prev[id][role],
      },
    }));
    onChange();
  };

  const openCreateProfile = () => {
    setEditorProfile(null);
    setEditorOpen(true);
  };

  const openEditProfile = (profile: CustomRole) => {
    setEditorProfile(profile);
    setEditorOpen(true);
  };

  /** O editor grava no servidor; aqui só refletimos na lista local. */
  const handleProfileSaved = async (saved: CustomRole, created: boolean) => {
    setCustomRoles((prev) => (created ? [...prev, saved] : prev.map((p) => (p.id === saved.id ? saved : p))));
    await loadUsage();
  };

  /**
   * Excluir perfil é operação de IMPACTO: quem o usa fica sem permissão nenhuma
   * (fail-closed). Antes de excluir, reatribuímos os usuários e os grupos que
   * apontavam para ele — e só então removemos o perfil.
   */
  const handleDeleteProfile = async () => {
    if (!deleteProfile || !companyId) return;
    setDeleting(true);
    try {
      const { members: affectedMembers, groups: affectedGroups } = usageOf(deleteProfile.id);
      for (const member of affectedMembers) {
        await settingsService.updateCompanyMember(companyId, member.account_id, { functional_role: reassignTo });
      }
      for (const group of affectedGroups) {
        await settingsService.updateGroup(companyId, group.id, {
          name: group.name,
          description: group.description,
          functional_role: reassignTo,
        });
      }
      await settingsService.deleteAccessProfile(deleteProfile.id);
      setCustomRoles((prev) => prev.filter((p) => p.id !== deleteProfile.id));
      toast.success(
        affectedMembers.length + affectedGroups.length > 0
          ? `Perfil removido — ${affectedMembers.length} usuário(s) e ${affectedGroups.length} grupo(s) passaram para ${roleDisplayName(reassignTo, customRoles)}.`
          : `Perfil "${deleteProfile.name}" removido.`,
      );
      setDeleteProfile(null);
      await loadUsage();
      onChange();
    } catch (error: any) {
      notifyApiError(error, { context: "Excluir perfil" });
    } finally {
      setDeleting(false);
    }
  };

  return (
    <div className="space-y-6">
      {loadState !== "ready" && (
        <div
          role="alert"
          className="rounded-lg border border-amber-500/40 bg-amber-500/10 px-4 py-3 text-sm text-amber-700 dark:text-amber-300"
        >
          {loadState === "failed"
            ? "Não foi possível carregar a matriz de permissões desta empresa. Salvar está bloqueado para não sobrescrever a configuração atual com os padrões — recarregue a página."
            : "Carregando a matriz de permissões do servidor..."}
        </div>
      )}
      <SectionCard title="Controle de Acessos" subtitle="Defina as permissões por perfil de usuário">
        <div className="space-y-6">
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-4 border-b border-border">
            <div>
              <p className="text-sm font-bold text-foreground">Perfil padrão para novos usuários</p>
              <p className="text-xs text-muted-foreground">Atribuído automaticamente ao criar um novo usuário</p>
            </div>
            <div className="w-52">
              <SettingsSelect
                value={defaultRole}
                onChange={(v) => {
                  setDefaultRole(v);
                  onChange();
                }}
                options={FUNCTIONAL_ROLES.map((role) => ({ value: role, label: ROLE_LABELS[role] }))}
              />
            </div>
          </div>

          <div className="space-y-3">
            {modules.map((module) => {
              const expanded = expandedModule === module || expandedModule === null;
              const modulePerms = PERMISSION_CATALOG.filter((p) => p.module === module);
              return (
                <div key={module} className="border border-border rounded-xl overflow-hidden">
                  <button
                    onClick={() => setExpandedModule(expandedModule === module ? null : module)}
                    className="w-full flex items-center justify-between px-4 py-3 bg-muted hover:bg-muted transition-colors"
                  >
                    <div className="flex items-center gap-2">
                      <ChevronRight
                        className={clsx("w-4 h-4 text-muted-foreground transition-transform", expanded && "rotate-90")}
                      />
                      <span className="text-sm font-bold text-foreground">{module}</span>
                      <Badge text={`${modulePerms.length} regras`} variant="info" />
                    </div>
                  </button>
                  <AnimatePresence initial={false}>
                    {expanded && (
                      <motion.div
                        initial={{ height: 0, opacity: 0 }}
                        animate={{ height: "auto", opacity: 1 }}
                        exit={{ height: 0, opacity: 0 }}
                        transition={{ duration: 0.2 }}
                        className={clsx("overflow-hidden", !isMobile && "overflow-x-auto")}
                      >
                        {isMobile ? (
                          <div className="p-4 space-y-3">
                            <div className="flex items-center justify-between gap-3">
                              <div className="text-[11px] font-bold text-muted-foreground uppercase tracking-wider">
                                Editar perfil
                              </div>
                              <div className="w-44">
                                <SettingsSelect
                                  value={mobileRole}
                                  onChange={(v) => setMobileRole(v as FunctionalRole)}
                                  options={FUNCTIONAL_ROLES.map((role) => ({ value: role, label: ROLE_LABELS[role] }))}
                                />
                              </div>
                            </div>
                            <div className="space-y-2 max-h-[60vh] overflow-y-auto pr-1">
                              {modulePerms.map((perm) => (
                                <div
                                  key={perm.id}
                                  className="flex items-center justify-between gap-3 p-3 bg-muted/40 rounded-xl border border-border"
                                >
                                  <div className="text-sm font-medium text-foreground min-w-0">{perm.label}</div>
                                  <button
                                    onClick={() => togglePermission(perm.id, mobileRole)}
                                    className="shrink-0 group/perm"
                                    aria-pressed={Boolean(permissions[perm.id]?.[mobileRole])}
                                    aria-label={`${permissions[perm.id]?.[mobileRole] ? "Remover" : "Conceder"} ${perm.label}`}
                                  >
                                    {/* Estado NÃO marcado com borda em token: `bg-muted` sobre a
                                        linha em `bg-muted/40` (e no modo noturno) sumia — o
                                        operador não via que a permissão existia e estava desligada. */}
                                    <div
                                      className={clsx(
                                        "w-6 h-6 rounded-md border-2 flex items-center justify-center transition-colors",
                                        permissions[perm.id]?.[mobileRole]
                                          ? "border-status-ok bg-status-ok text-white"
                                          : "border-muted-foreground/45 bg-background text-transparent group-hover/perm:border-primary/70",
                                      )}
                                    >
                                      <Check className="w-3.5 h-3.5" />
                                    </div>
                                  </button>
                                </div>
                              ))}
                            </div>
                          </div>
                        ) : (
                          <table className="w-full text-sm min-w-[800px]">
                            <thead>
                              <tr className="border-b border-border bg-card">
                                <th className="text-left px-4 py-2.5 text-[11px] font-bold text-muted-foreground uppercase tracking-wider sticky left-0 bg-card z-10">
                                  Permissão
                                </th>
                                {FUNCTIONAL_ROLES.map((role) => (
                                  <th
                                    key={role}
                                    className="px-3 py-2.5 text-center text-[11px] font-bold text-muted-foreground uppercase tracking-wider min-w-[100px]"
                                  >
                                    {ROLE_LABELS[role]}
                                  </th>
                                ))}
                              </tr>
                            </thead>
                            <tbody className="divide-y divide-border">
                              {modulePerms.map((perm) => (
                                <tr key={perm.id} className="hover:bg-muted/50 transition-colors">
                                  <td className="px-4 py-3 text-foreground text-sm sticky left-0 bg-card/90 backdrop-blur-sm z-10">
                                    {perm.label}
                                  </td>
                                  {FUNCTIONAL_ROLES.map((role) => (
                                    <td key={role} className="px-3 py-3 text-center">
                                      <button
                                        onClick={() => togglePermission(perm.id, role)}
                                        className="mx-auto flex justify-center group/perm"
                                        aria-pressed={Boolean(permissions[perm.id]?.[role])}
                                        aria-label={`${permissions[perm.id]?.[role] ? "Remover" : "Conceder"} ${perm.label} — ${ROLE_LABELS[role]}`}
                                      >
                                        {/* Mesmo contraste do editor de perfil: o affordance do
                                            estado desligado é a BORDA (token), visível nos dois temas. */}
                                        <div
                                          className={clsx(
                                            "w-5 h-5 rounded-md border-2 flex items-center justify-center transition-colors",
                                            permissions[perm.id]?.[role]
                                              ? "border-status-ok bg-status-ok text-white"
                                              : "border-muted-foreground/45 bg-background text-transparent group-hover/perm:border-primary/70",
                                          )}
                                        >
                                          <Check className="w-3 h-3" />
                                        </div>
                                      </button>
                                    </td>
                                  ))}
                                </tr>
                              ))}
                            </tbody>
                          </table>
                        )}
                      </motion.div>
                    )}
                  </AnimatePresence>
                </div>
              );
            })}
          </div>
        </div>
      </SectionCard>

      <SectionCard
        title="Perfis de Acesso Personalizados"
        subtitle="Crie perfis com as permissões que quiser e atribua-os pelo Grupo de Usuários"
        delay={0.05}
        actions={
          <Button
            onClick={openCreateProfile}
            variant="outline"
            size="sm"
            className="border-primary/30 bg-primary/10 text-primary hover:bg-primary/15 hover:text-primary"
          >
            <Plus className="w-3.5 h-3.5" />
            Novo Perfil
          </Button>
        }
      >
        <div className="space-y-3">
          {customRoles.map((role) => {
            const usage = usageOf(role.id);
            const grantedQty = grantedCount(role.id, PERMISSION_CATALOG.map((p) => ({
              id: p.id,
              label: p.label,
              module: p.module,
              roles: permissions[p.id],
            })), customRoles);
            return (
              <div
                key={role.id}
                className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 p-4 bg-muted rounded-xl border border-border"
              >
                <div className="flex items-start gap-3 min-w-0">
                  <div className="w-3 h-3 rounded-full mt-1.5 shrink-0" style={{ backgroundColor: role.color }} />
                  <div className="min-w-0">
                    <p className="text-sm font-bold text-foreground truncate">{role.name}</p>
                    <p className="text-[11px] text-muted-foreground">
                      Baseado em: {ROLE_LABELS[role.baseRole as FunctionalRole] || role.baseRole} ·{" "}
                      {grantedQty} de {PERMISSION_CATALOG.length} permissões
                      {role.description ? ` · ${role.description}` : ""}
                    </p>
                    <p className="text-[11px] text-muted-foreground mt-0.5 flex items-center gap-1">
                      <Users className="w-3 h-3" />
                      {usage.members.length === 0 && usage.groups.length === 0
                        ? "Ninguém usa este perfil ainda — atribua-o em um Grupo de Usuários"
                        : `${usage.members.length} usuário(s)` +
                          (usage.groups.length > 0 ? ` · grupo(s): ${usage.groups.map((g) => g.name).join(", ")}` : "")}
                    </p>
                  </div>
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <Badge
                    text={grantedQty === 0 ? "sem permissões" : `${grantedQty} permissões`}
                    variant={grantedQty === 0 ? "warning" : "info"}
                  />
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => openEditProfile(role)}
                    className="size-8 text-muted-foreground hover:text-primary-600 hover:bg-primary-50"
                    title="Editar permissões do perfil"
                    aria-label={`Editar perfil ${role.name}`}
                  >
                    <Pencil className="w-3.5 h-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => {
                      setReassignTo(defaultRole && defaultRole !== role.id ? defaultRole : "corretor");
                      setDeleteProfile(role);
                    }}
                    className="size-8 text-muted-foreground hover:text-status-critical hover:bg-status-critical-soft"
                    title="Excluir perfil"
                    aria-label={`Excluir perfil ${role.name}`}
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </Button>
                </div>
              </div>
            );
          })}
          {customRoles.length === 0 && (
            <div className="text-center py-6 space-y-1">
              <p className="text-sm text-muted-foreground">Nenhum perfil de acesso criado.</p>
              <p className="text-xs text-muted-foreground">
                Um perfil define um conjunto próprio de permissões (ex.: "Corretor Sênior" com Financeiro de leitura) e é
                atribuído às pessoas por um Grupo de Usuários.
              </p>
            </div>
          )}
        </div>
      </SectionCard>

      <AccessProfileEditor
        open={editorOpen}
        profile={editorProfile}
        matrix={PERMISSION_CATALOG.map((p) => ({
          id: p.id,
          label: p.label,
          module: p.module,
          roles: permissions[p.id],
        }))}
        usageCount={editorProfile ? usageOf(editorProfile.id).members.length : undefined}
        onClose={() => setEditorOpen(false)}
        onSaved={handleProfileSaved}
      />

      <Dialog
        open={!!deleteProfile}
        onOpenChange={(next) => {
          if (!next && !deleting) setDeleteProfile(null);
        }}
      >
        <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-lg">
          <div className="px-6 py-4 border-b border-border pr-12">
            <DialogTitle className="font-bold text-foreground">Excluir perfil de acesso</DialogTitle>
            <DialogDescription className="text-xs text-muted-foreground">
              O acesso concedido por este perfil deixa de existir imediatamente.
            </DialogDescription>
          </div>
          <div className="p-6 space-y-4">
            {deleteProfile && (
              <>
                {(usageOf(deleteProfile.id).members.length > 0 || usageOf(deleteProfile.id).groups.length > 0) && (
                  <div className="flex gap-2 rounded-xl border border-status-warning/30 bg-status-warning-soft p-3 text-xs text-status-warning">
                    <AlertTriangle className="w-4 h-4 shrink-0" />
                    <div>
                      <p className="font-bold">
                        {usageOf(deleteProfile.id).members.length} usuário(s) e{" "}
                        {usageOf(deleteProfile.id).groups.length} grupo(s) usam "{deleteProfile.name}".
                      </p>
                      <p>Sem reatribuição eles ficariam sem nenhuma permissão. Escolha para onde mover:</p>
                    </div>
                  </div>
                )}
                <div className="space-y-1.5">
                  <label className="text-xs font-bold text-muted-foreground uppercase">Reatribuir para</label>
                  <SettingsSelect
                    value={reassignTo}
                    onChange={setReassignTo}
                    options={assignableRoleOptions(customRoles.filter((p) => p.id !== deleteProfile.id))}
                  />
                </div>
              </>
            )}
          </div>
          <div className="px-6 py-4 bg-muted border-t border-border flex justify-end gap-3">
            <Button variant="ghost" onClick={() => setDeleteProfile(null)} disabled={deleting}>
              Cancelar
            </Button>
            <Button
              onClick={handleDeleteProfile}
              disabled={deleting}
              className="bg-status-critical text-white hover:bg-status-critical/90"
            >
              {deleting ? "Excluindo..." : "Excluir e reatribuir"}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
});
