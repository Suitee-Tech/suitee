import { useConfirmAction } from "../../../hooks/useConfirmAction";
import { useEffect, useState } from "react";
import { Trash2, RefreshCw, Send, Settings2, User, UserCheck, UserPlus, UserX } from "lucide-react";
import { toast } from "sonner";
import { useAgentsContext } from "../../../context/AgentsContext";
import { FUNCTIONAL_ROLES, ROLE_LABELS } from "../../../../lib/permissions/catalog";
import { Button } from "../../../ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "../../../ui/dialog";
import { getMediaUrl } from "../../../media";
import { settingsService, type CustomRole } from "../../../services/settings.service";
import { SectionCard, SettingsInput, SettingsSelect } from "../_shared";
import { roleDisplayName } from "../accessProfiles";
import { notifyApiError } from "../../../services/errorToast";

/**
 * Chip do papel/perfil do vínculo. Um PERFIL DE ACESSO personalizado (`cr_…`)
 * aparece pelo NOME; se o perfil foi apagado e o vínculo ficou órfão, o chip
 * avisa — antes a tela mostrava o id cru (`cr_36339ef5-…`) cortado pelo CSS e o
 * operador não tinha como saber o que a pessoa podia fazer.
 */
function RoleChip({ role, profiles }: { role?: string | null; profiles: CustomRole[] }) {
  const orphan = Boolean(role && role.startsWith("cr_") && !profiles.some((profile) => profile.id === role));
  const label = role === "owner" || role === "admin" ? "Administrador" : roleDisplayName(role, profiles);
  return (
    <span
      className={
        orphan
          ? "px-2 py-1 bg-status-warning-soft text-status-warning rounded-md text-xs font-medium"
          : "px-2 py-1 bg-muted text-muted-foreground rounded-md text-xs font-medium"
      }
      title={orphan ? "O perfil deste usuário foi removido — ele está sem permissões. Reatribua em Grupos de Usuários." : undefined}
    >
      {label}
    </span>
  );
}

interface Props {
  companyId: string;
  user: any;
  teamMembers: any[];
  invitations: any[];
  groups: any[];
  canManageGroupMembers: boolean;
  onReload: () => Promise<void> | void;
}

export function TeamSection({ companyId, user, teamMembers, invitations, groups, canManageGroupMembers, onReload }: Props) {
  const { confirmAction, confirmationDialog } = useConfirmAction();
  const { refreshBrokers } = useAgentsContext();
  const [showInviteModal, setShowInviteModal] = useState(false);
  const [inviteEmail, setInviteEmail] = useState("");
  const [inviteRole, setInviteRole] = useState("corretor");
  const [showMoveModal, setShowMoveModal] = useState(false);
  const [movingMemberId, setMovingMemberId] = useState<string | null>(null);
  const [targetGroupId, setTargetGroupId] = useState("");
  // Perfis de acesso do tenant: podem ser atribuídos direto a um membro ou
  // convidado, do mesmo jeito que os papéis do catálogo.
  const [profiles, setProfiles] = useState<CustomRole[]>([]);

  useEffect(() => {
    settingsService
      .getPermissions()
      .then((apiSettings) => {
        if (apiSettings.defaultRole) {
          setInviteRole(apiSettings.defaultRole);
        }
        if (apiSettings.customRoles) {
          setProfiles(apiSettings.customRoles);
        }
      })
      .catch(console.error);
  }, []);

  const handleInviteUser = async () => {
    if (!inviteEmail) return;
    if (!companyId) {
      toast.error("Empresa não encontrada.");
      return;
    }
    try {
      await settingsService.createInvitation(companyId, inviteEmail, inviteRole);
      toast.success(`Convite enviado para ${inviteEmail}`);
      setShowInviteModal(false);
      setInviteEmail("");
      setInviteRole("corretor");
      await onReload();
      refreshBrokers().catch(() => {});
    } catch (error: any) {
      notifyApiError(error, { context: "Enviar convite" });
    }
  };

  const handleResendInvite = async (invitationId: string) => {
    if (!companyId) return;
    try {
      await settingsService.resendInvitation(companyId, invitationId);
      toast.success("Convite reenviado com sucesso!");
      await onReload();
    } catch (error: any) {
      notifyApiError(error, { context: "Reenviar convite" });
    }
  };

  const handleCancelInvite = async (invitationId: string) => {
    if (!companyId) return;
    try {
      await settingsService.cancelInvitation(companyId, invitationId);
      toast.success("Convite cancelado!");
      await onReload();
    } catch (error: any) {
      notifyApiError(error, { context: "Cancelar convite" });
    }
  };

  const handleRemoveMember = (memberId: string) => {
    if (!companyId) return;
    confirmAction("Remover este membro da empresa?", async () => {
      try {
        await settingsService.removeCompanyMember(companyId, memberId);
        toast.success("Membro removido com sucesso!");
        await Promise.resolve().then(onReload).catch(() => { toast.error("A alteração foi salva, mas a lista não atualizou. Recarregue a página."); });
        void refreshBrokers().catch(() => {});
      } catch (error: any) {
        notifyApiError(error, { context: "Remover membro" });
        throw error;
      }
    }, { title: "Remover membro", confirmLabel: "Remover", variant: "danger" });
  };

  const handleToggleMemberActive = async (member: any) => {
    if (!companyId) return;
    const willSuspend = member.is_active !== false;
    const apply = async () => {
      try {
        await settingsService.updateCompanyMember(companyId, member.account_id, { is_active: !willSuspend });
        toast.success(willSuspend ? "Acesso suspenso!" : "Acesso reativado!");
        await Promise.resolve().then(onReload).catch(() => { toast.error("A alteração foi salva, mas a lista não atualizou. Recarregue a página."); });
        void refreshBrokers().catch(() => {});
      } catch (error: any) {
        notifyApiError(error, { context: "Alterar acesso do membro" });
        throw error;
      }
    };
    if (willSuspend) {
      confirmAction("Suspender o acesso deste membro? Ele continua na empresa, mas perde o acesso imediatamente.", apply, { title: "Suspender acesso", confirmLabel: "Suspender", variant: "danger" });
    } else { await apply().catch(() => {}); }
  };

  const handleMoveMember = async () => {
    if (!movingMemberId || !targetGroupId) return;
    if (!companyId) return;
    try {
      await settingsService.moveMember(companyId, "dummy", movingMemberId, targetGroupId);
      toast.success("Membro movido com sucesso!");
      setShowMoveModal(false);
      setMovingMemberId(null);
      setTargetGroupId("");
      await onReload();
    } catch (error: any) {
      notifyApiError(error, { context: "Mover membro" });
    }
  };

  return (
    <>
      {confirmationDialog}
      <SectionCard title="Equipe da Empresa" subtitle="Gerencie os usuários que têm acesso a esta empresa" delay={0.15}>
        <div className="space-y-4">
          <div className="flex justify-between items-center">
            <p className="text-sm text-muted-foreground">Convide corretores, gerentes e assistentes para colaborar.</p>
            <Button
              onClick={() => setShowInviteModal(true)}
              variant="outline"
              className="border-primary/30 bg-primary/10 text-primary hover:bg-primary/15 hover:text-primary"
            >
              <UserPlus className="w-4 h-4" />
              Convidar Membro
            </Button>
          </div>

          <div className="border border-border rounded-xl overflow-hidden bg-muted">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border bg-card">
                  <th className="text-left px-4 py-3 text-xs font-bold text-muted-foreground uppercase">Usuário</th>
                  <th className="text-left px-4 py-3 text-xs font-bold text-muted-foreground uppercase">Papel</th>
                  <th className="text-left px-4 py-3 text-xs font-bold text-muted-foreground uppercase">Status</th>
                  <th className="text-right px-4 py-3 text-xs font-bold text-muted-foreground uppercase">Ações</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                <tr className="bg-card">
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-3">
                      {user?.avatar_url ? (
                        <img
                          src={getMediaUrl(user.avatar_url)}
                          alt={user?.fullname || (user as any)?.name}
                          className="w-8 h-8 rounded-full object-cover shrink-0"
                        />
                      ) : (
                        <div className="w-8 h-8 rounded-full bg-primary-100 text-primary-700 flex items-center justify-center font-bold text-xs">
                          {(user?.fullname || (user as any)?.name)
                            ? ((user?.fullname || (user as any)?.name) as string).charAt(0).toUpperCase()
                            : "U"}
                        </div>
                      )}
                      <div>
                        <p className="font-bold text-foreground">{user?.fullname || (user as any)?.name} (Você)</p>
                        <p className="text-xs text-muted-foreground">{user?.email}</p>
                      </div>
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    <span className="px-2 py-1 bg-muted text-muted-foreground rounded-md text-xs font-medium capitalize">
                      {user?.permission === "admin" || user?.permission === "owner"
                        ? "Administrador"
                        : user?.permission || "Corretor"}
                    </span>
                  </td>
                  <td className="px-4 py-3">
                    <span className="flex items-center gap-1.5 text-xs font-medium text-status-ok">
                      <div className="w-1.5 h-1.5 rounded-full bg-status-ok"></div> Ativo
                    </span>
                  </td>
                  <td className="px-4 py-3 text-right">
                    <Button variant="ghost" size="icon" className="size-8 text-muted-foreground hover:text-primary-600" disabled title="Não é possível remover a si mesmo">
                      <Settings2 className="w-4 h-4" />
                    </Button>
                  </td>
                </tr>

                {teamMembers
                  .filter((m: any) => m.account_id !== user?.id)
                  .map((member: any, idx: number) => (
                    <tr key={`member-${idx}`} className="bg-card">
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-3">
                          {member.avatar_url ? (
                            <img src={getMediaUrl(member.avatar_url)} alt={member.name} className="w-8 h-8 rounded-full object-cover shrink-0" />
                          ) : (
                            <div className="w-8 h-8 rounded-full bg-muted text-foreground/80 flex items-center justify-center font-bold text-xs">
                              {member.name ? member.name.charAt(0).toUpperCase() : <User className="w-4 h-4" />}
                            </div>
                          )}
                          <div>
                            <p className="font-bold text-foreground">{member.name || "Membro da Equipe"}</p>
                            <p className="text-xs text-muted-foreground">
                              {member.email || `ID: ${member.account_id.substring(0, 8)}...`}
                            </p>
                          </div>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <RoleChip role={member.functional_role || member.profile} profiles={profiles} />
                      </td>
                      <td className="px-4 py-3">
                        {member.is_active === false ? (
                          <span className="flex items-center gap-1.5 text-xs font-medium text-status-warning">
                            <div className="w-1.5 h-1.5 rounded-full bg-status-warning"></div> Suspenso
                          </span>
                        ) : (
                          <span className="flex items-center gap-1.5 text-xs font-medium text-status-ok">
                            <div className="w-1.5 h-1.5 rounded-full bg-status-ok"></div> Ativo
                          </span>
                        )}
                      </td>
                      <td className="px-4 py-3 text-right flex justify-end gap-1">
                        {member.profile !== "owner" && member.profile !== "admin" && canManageGroupMembers && (
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => handleToggleMemberActive(member)}
                            className="size-8 text-muted-foreground hover:text-status-warning hover:bg-status-warning-soft"
                            title={member.is_active === false ? "Reativar acesso" : "Suspender acesso"}
                          >
                            {member.is_active === false ? <UserCheck className="w-4 h-4" /> : <UserX className="w-4 h-4" />}
                          </Button>
                        )}
                        {member.profile !== "owner" && member.profile !== "admin" && canManageGroupMembers && (
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => {
                              setMovingMemberId(member.account_id);
                              setShowMoveModal(true);
                            }}
                            className="size-8 text-muted-foreground hover:text-status-info hover:bg-status-info-soft"
                            title="Alterar Grupo/Perfil"
                          >
                            <Settings2 className="w-4 h-4" />
                          </Button>
                        )}
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => handleRemoveMember(member.account_id)}
                          className="size-8 text-muted-foreground hover:text-status-critical hover:bg-status-critical-soft"
                          title="Remover Membro"
                        >
                          <Trash2 className="w-4 h-4" />
                        </Button>
                      </td>
                    </tr>
                  ))}

                {invitations
                  .filter((inv: any) => inv.status === "pending")
                  .map((inv: any) => (
                    <tr key={inv.id} className="bg-card">
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-3">
                          <div className="w-8 h-8 rounded-full bg-status-warning-soft text-status-warning flex items-center justify-center font-bold text-xs">
                            {inv.email.charAt(0).toUpperCase()}
                          </div>
                          <div>
                            <p className="font-bold text-foreground">Convidado</p>
                            <p className="text-xs text-muted-foreground">{inv.email}</p>
                          </div>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <RoleChip role={inv.role} profiles={profiles} />
                      </td>
                      <td className="px-4 py-3">
                        <span
                          className={`flex items-center gap-1.5 text-xs font-medium ${
                            inv.status === "pending" ? "text-status-warning" : inv.status === "accepted" ? "text-status-ok" : "text-status-critical"
                          }`}
                        >
                          <div
                            className={`w-1.5 h-1.5 rounded-full ${
                              inv.status === "pending" ? "bg-status-warning" : inv.status === "accepted" ? "bg-status-ok" : "bg-status-critical"
                            }`}
                          ></div>
                          {inv.status === "pending" ? "Pendente" : inv.status === "accepted" ? "Aceito" : "Cancelado"}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-right flex justify-end gap-1">
                        {inv.status === "pending" && (
                          <>
                            <Button
                              variant="ghost"
                              size="icon"
                              onClick={() => handleResendInvite(inv.id)}
                              className="size-8 text-muted-foreground hover:text-status-info hover:bg-status-info-soft"
                              title="Reenviar Convite"
                            >
                              <RefreshCw className="w-4 h-4" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon"
                              onClick={() => handleCancelInvite(inv.id)}
                              className="size-8 text-muted-foreground hover:text-status-critical hover:bg-status-critical-soft"
                              title="Cancelar Convite"
                            >
                              <Trash2 className="w-4 h-4" />
                            </Button>
                          </>
                        )}
                      </td>
                    </tr>
                  ))}
              </tbody>
            </table>
          </div>
        </div>

        <Dialog
          open={showInviteModal}
          onOpenChange={(next) => {
            if (!next) setShowInviteModal(false);
          }}
        >
          <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-md">
            <div className="px-6 py-4 border-b border-border pr-12">
              <DialogTitle className="font-bold text-foreground">Convidar novo membro</DialogTitle>
              <DialogDescription className="sr-only">Convide um novo membro para esta empresa</DialogDescription>
            </div>
            <div className="p-6 space-y-4">
              <div className="space-y-1.5">
                <label className="text-xs font-bold text-muted-foreground uppercase">E-mail do usuário</label>
                <SettingsInput value={inviteEmail} onChange={setInviteEmail} placeholder="exemplo@imobiliaria.com" type="email" />
              </div>
              <div className="space-y-1.5">
                <label className="text-xs font-bold text-muted-foreground uppercase">Perfil de Acesso</label>
                <SettingsSelect
                  value={inviteRole}
                  onChange={setInviteRole}
                  options={[
                    ...FUNCTIONAL_ROLES.map((role) => ({ value: role, label: ROLE_LABELS[role] })),
                    ...profiles.map((profile) => ({ value: profile.id, label: `${profile.name} (personalizado)` })),
                  ]}
                />
              </div>
            </div>
            <div className="px-6 py-4 bg-muted border-t border-border flex justify-end gap-3">
              <Button variant="ghost" onClick={() => setShowInviteModal(false)}>
                Cancelar
              </Button>
              <Button onClick={handleInviteUser} disabled={!inviteEmail}>
                <Send className="w-4 h-4" /> Enviar Convite
              </Button>
            </div>
          </DialogContent>
        </Dialog>
      </SectionCard>

      <Dialog
        open={showMoveModal}
        onOpenChange={(next) => {
          if (!next) {
            setShowMoveModal(false);
            setMovingMemberId(null);
            setTargetGroupId("");
          }
        }}
      >
        <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-md">
          <div className="px-6 py-4 border-b border-border pr-12">
            <DialogTitle className="font-bold text-foreground">Alterar Grupo/Perfil do Usuário</DialogTitle>
            <DialogDescription className="sr-only">Selecione o novo grupo ou perfil do usuário</DialogDescription>
          </div>
          <div className="p-6 space-y-4">
            <div className="space-y-1.5">
              <label className="text-xs font-bold text-muted-foreground uppercase">Selecione o novo Grupo/Perfil</label>
              <SettingsSelect
                value={targetGroupId}
                onChange={setTargetGroupId}
                options={[{ value: "", label: "Selecione um grupo..." }, ...groups.map((g) => ({ value: g.id, label: g.name }))]}
              />
            </div>
          </div>
          <div className="px-6 py-4 bg-muted border-t border-border flex justify-end gap-3">
            <Button
              variant="ghost"
              onClick={() => {
                setShowMoveModal(false);
                setMovingMemberId(null);
                setTargetGroupId("");
              }}
            >
              Cancelar
            </Button>
            <Button onClick={handleMoveMember} disabled={!targetGroupId}>
              <Send className="w-4 h-4" /> Salvar Alteração
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}
