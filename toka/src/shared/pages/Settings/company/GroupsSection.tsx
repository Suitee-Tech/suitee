import { useCallback, useEffect, useState } from "react";
import { Trash2, Send, Settings2, Users, ShieldCheck, Plus, AlertTriangle } from "lucide-react";
import { toast } from "sonner";
import { Button } from "../../../ui/button";
import { ResponsiveTable } from "../../../ui/responsive-table";
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "../../../ui/sheet";
import { ConfirmDialog } from "../../../components/ConfirmDialog";
import { getMediaUrl } from "../../../media";
import { settingsService, type CustomRole, type Group, type Permission } from "../../../services/settings.service";
import { FUNCTIONAL_ROLES, ROLE_LABELS, type FunctionalRole } from "../../../../lib/permissions/catalog";
import { SectionCard, SettingsInput, Badge } from "../_shared";
import { AccessProfileEditor } from "../AccessProfileEditor";
import { isAccessProfileId, roleDisplayName } from "../accessProfiles";
import { notifyApiError } from "../../../services/errorToast";

interface Props {
  companyId: string;
  groups: Group[];
  canManageGroups: boolean;
  onReload: () => Promise<void> | void;
}

/**
 * Grupos de Usuários — a ATRIBUIÇÃO de perfil de acesso.
 *
 * O grupo aponta para um papel do catálogo OU para um PERFIL DE ACESSO
 * personalizado (`cr_…`, criado em Configurações → Permissões) e é isso que os
 * membros recebem: o `functional_role` do vínculo passa a ser o id do perfil e a
 * autorização sai do conjunto de permissões dele.
 *
 * O que estava errado antes: o diálogo criava grupo só com nome/descrição (sem
 * escolher o que ele concede), a exclusão só aparecia em grupos com `role_id`
 * começando por `custom_` — ou seja, um grupo criado pela tela NÃO podia ser
 * editado nem excluído — e a lista de perfis da outra aba era decorativa, sem
 * nenhuma ligação com este fluxo.
 */
export function GroupsSection({ companyId, groups, canManageGroups, onReload }: Props) {
  const [profiles, setProfiles] = useState<CustomRole[]>([]);
  const [showGroupModal, setShowGroupModal] = useState(false);
  const [editingGroup, setEditingGroup] = useState<Group | null>(null);
  const [groupName, setGroupName] = useState("");
  const [groupDesc, setGroupDesc] = useState("");
  const [groupRole, setGroupRole] = useState<string>("corretor");
  const [saving, setSaving] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<Group | null>(null);
  const [profileEditorOpen, setProfileEditorOpen] = useState(false);
  // Matriz do tenant: o editor de perfil embutido precisa dela para semear as
  // permissões a partir do papel base (o mesmo editor da tela de Permissões).
  const [matrix, setMatrix] = useState<Permission[]>([]);

  const loadProfiles = useCallback(async () => {
    try {
      setProfiles(await settingsService.listAccessProfiles());
    } catch {
      // A lista de perfis é uma melhoria da tela: sem ela o grupo continua
      // funcionando com os papéis do catálogo.
      setProfiles([]);
    }
    try {
      const doc = await settingsService.getPermissions();
      setMatrix((doc.permissions as Permission[]) || []);
    } catch {
      setMatrix([]);
    }
  }, []);

  useEffect(() => {
    void loadProfiles();
  }, [loadProfiles]);

  const openCreate = () => {
    setEditingGroup(null);
    setGroupName("");
    setGroupDesc("");
    setGroupRole("corretor");
    setShowGroupModal(true);
  };

  const openEdit = (group: Group) => {
    setEditingGroup(group);
    setGroupName(group.name);
    setGroupDesc(group.description || "");
    setGroupRole(group.role_id || "corretor");
    setShowGroupModal(true);
  };

  const handleSaveGroup = async () => {
    if (!groupName.trim() || !companyId) return;
    setSaving(true);
    try {
      if (editingGroup) {
        await settingsService.updateGroup(companyId, editingGroup.id, {
          name: groupName.trim(),
          description: groupDesc,
          // Grupo de papel do sistema não troca de perfil (o backend recusa);
          // para os demais, o perfil é o que o grupo concede.
          ...(editingGroup.is_default ? {} : { functional_role: groupRole }),
        });
        toast.success("Grupo atualizado");
      } else {
        await settingsService.createGroup(companyId, {
          name: groupName.trim(),
          description: groupDesc,
          functional_role: groupRole,
        });
        toast.success(`Grupo criado — concede ${roleDisplayName(groupRole, profiles)}`);
      }
      setShowGroupModal(false);
      await onReload();
    } catch (error: any) {
      notifyApiError(error, { context: "Salvar grupo" });
    } finally {
      setSaving(false);
    }
  };

  const handleDeleteGroup = async () => {
    if (!deleteTarget || !companyId) return;
    try {
      await settingsService.deleteGroup(companyId, deleteTarget.id);
      toast.success("Grupo excluído");
      setDeleteTarget(null);
      await onReload();
    } catch (error: any) {
      notifyApiError(error, { context: "Excluir grupo" });
    }
  };

  const handleProfileSaved = async (saved: CustomRole) => {
    await loadProfiles();
    setGroupRole(saved.id);
  };

  if (!canManageGroups) return null;

  const customProfiles = profiles;

  return (
    <SectionCard
      title="Grupos de Usuários"
      subtitle="Cada grupo concede um perfil de acesso (padrão ou personalizado) aos seus membros"
      delay={0.2}
    >
      <div className="space-y-4">
        <div className="flex justify-between items-center">
          <p className="text-sm text-muted-foreground">
            O perfil do grupo é o que a pessoa pode fazer no sistema. Ajuste as permissões em{" "}
            <span className="font-medium text-foreground">Configurações → Permissões de Acesso</span>.
          </p>
          <Button
            onClick={openCreate}
            variant="outline"
            className="border-primary/30 bg-primary/10 text-primary hover:bg-primary/15 hover:text-primary"
          >
            <Users className="w-4 h-4" />
            Novo Grupo
          </Button>
        </div>

        <div className="border border-border rounded-xl overflow-hidden bg-muted">
          <ResponsiveTable
            data={groups}
            caption="Grupos de usuários"
            getRowKey={(group) => group.id}
            empty={(
              <div className="px-4 py-8 text-center text-sm text-muted-foreground">Nenhum grupo criado ainda.</div>
            )}
            columns={[
              {
                key: "name",
                header: "Nome do Grupo",
                primary: true,
                cell: (group) => (
                  <span className="font-bold">
                    {group.name}
                    {group.is_default && <span className="ml-2 text-caption font-bold uppercase text-muted-foreground">padrão</span>}
                  </span>
                ),
              },
              {
                key: "role",
                header: "Perfil de acesso",
                cell: (group) => {
                  const orphan = isAccessProfileId(group.role_id) && !customProfiles.some((p) => p.id === group.role_id);
                  const custom = Boolean(customProfiles.find((p) => p.id === group.role_id));
                  return orphan ? (
                    <span className="inline-flex items-center gap-1 text-status-warning text-xs font-bold">
                      <AlertTriangle className="w-3.5 h-3.5" /> Perfil removido
                    </span>
                  ) : (
                    <Badge text={roleDisplayName(group.role_id, customProfiles)} variant={custom ? "info" : "default"} />
                  );
                },
              },
              { key: "description", header: "Descrição", cell: (group) => group.description || "-" },
              {
                key: "members",
                header: "Membros",
                cell: (group) => (
                  <div className="flex -space-x-2 overflow-hidden items-center">
                    {group.members?.slice(0, 5).map((m, idx) =>
                      m.avatar_url ? (
                        <img key={idx} src={getMediaUrl(m.avatar_url)} alt={m.name} title={m.name} className="inline-block w-7 h-7 rounded-full ring-2 ring-card object-cover shrink-0" />
                      ) : (
                        <div key={idx} className="inline-flex w-7 h-7 rounded-full ring-2 ring-card bg-primary-100 text-primary-700 items-center justify-center font-bold text-xs" title={m.name}>
                          {m.name ? m.name.charAt(0).toUpperCase() : "U"}
                        </div>
                      ),
                    )}
                    {group.members?.length > 5 && (
                      <div className="inline-flex w-7 h-7 rounded-full ring-2 ring-card bg-muted text-muted-foreground items-center justify-center font-bold text-xs">+{group.members.length - 5}</div>
                    )}
                    {(!group.members || group.members.length === 0) && <span className="text-muted-foreground text-xs py-1">Nenhum</span>}
                  </div>
                ),
              },
              {
                key: "actions",
                header: "Ações",
                className: "text-right",
                cell: (group) => (
                  <div className="flex justify-end gap-1">
                    <Button variant="ghost" size="icon" onClick={() => openEdit(group)} className="size-8 text-muted-foreground hover:text-primary-600 hover:bg-primary-50" title="Editar grupo" aria-label={`Editar grupo ${group.name}`}>
                      <Settings2 className="w-4 h-4" />
                    </Button>
                    {!group.is_default && (
                      <Button variant="ghost" size="icon" onClick={() => setDeleteTarget(group)} className="size-8 text-muted-foreground hover:text-status-critical hover:bg-status-critical-soft" title="Excluir grupo" aria-label={`Excluir grupo ${group.name}`}>
                        <Trash2 className="w-4 h-4" />
                      </Button>
                    )}
                  </div>
                ),
              },
            ]}
          />
        </div>
      </div>

      {/* Painel lateral direito: formulário não é alerta. O alert fica para a
          confirmação de exclusão (ação crítica), abaixo. */}
      <Sheet
        open={showGroupModal}
        onOpenChange={(next) => {
          if (!next) setShowGroupModal(false);
        }}
      >
        <SheetContent side="right" className="w-full gap-0 p-0 sm:max-w-md">
          <SheetHeader className="border-b border-border px-6 py-4 pr-14">
            <SheetTitle className="font-bold text-foreground">{editingGroup ? "Editar Grupo" : "Novo Grupo"}</SheetTitle>
            <SheetDescription className="text-xs text-muted-foreground">
              Defina nome, descrição e o perfil de acesso que o grupo concede
            </SheetDescription>
          </SheetHeader>
          <div className="flex-1 overflow-y-auto p-6 space-y-4">
            <div className="space-y-1.5">
              <label className="text-xs font-bold text-muted-foreground uppercase">Nome do Grupo</label>
              <SettingsInput
                value={groupName}
                onChange={setGroupName}
                placeholder="Ex: Corretores Centro"
                disabled={Boolean(editingGroup?.is_default)}
              />
            </div>
            <div className="space-y-1.5">
              <label className="text-xs font-bold text-muted-foreground uppercase">Perfil de acesso</label>
              <select
                value={groupRole}
                onChange={(event) => setGroupRole(event.target.value)}
                disabled={Boolean(editingGroup?.is_default)}
                aria-label="Perfil de acesso do grupo"
                className="w-full px-3 py-2.5 bg-card border border-border rounded-xl text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-primary-500/20 disabled:opacity-60"
              >
                <optgroup label="Perfis padrão">
                  {FUNCTIONAL_ROLES.map((role) => (
                    <option key={role} value={role}>
                      {ROLE_LABELS[role]}
                    </option>
                  ))}
                </optgroup>
                {customProfiles.length > 0 && (
                  <optgroup label="Perfis personalizados">
                    {customProfiles.map((profile) => (
                      <option key={profile.id} value={profile.id}>
                        {profile.name}
                      </option>
                    ))}
                  </optgroup>
                )}
              </select>
              <div className="flex items-center justify-between gap-2">
                <p className="text-[11px] text-muted-foreground">
                  {editingGroup?.is_default
                    ? "Grupo padrão do sistema: o perfil é a identidade dele."
                    : `Os membros deste grupo passam a ter: ${roleDisplayName(groupRole, customProfiles)}`}
                </p>
                {!editingGroup?.is_default && (
                  <button
                    type="button"
                    onClick={() => setProfileEditorOpen(true)}
                    className="text-[11px] font-bold text-primary hover:underline inline-flex items-center gap-1 shrink-0"
                  >
                    <Plus className="w-3 h-3" />
                    Novo perfil
                  </button>
                )}
              </div>
            </div>
            <div className="space-y-1.5">
              <label className="text-xs font-bold text-muted-foreground uppercase">Descrição</label>
              <SettingsInput value={groupDesc} onChange={setGroupDesc} placeholder="Opcional" />
            </div>
          </div>
          <SheetFooter className="flex-row justify-end gap-3 border-t border-border bg-muted px-6 py-4">
            <Button variant="ghost" onClick={() => setShowGroupModal(false)} disabled={saving}>
              Cancelar
            </Button>
            <Button onClick={handleSaveGroup} disabled={saving || !groupName.trim()}>
              <Send className="w-4 h-4" /> {saving ? "Salvando..." : "Salvar Grupo"}
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>

      {/* O MESMO editor da tela de Permissões: criar o perfil aqui já o deixa
          selecionado no grupo (é a unificação dos dois fluxos). */}
      <AccessProfileEditor
        open={profileEditorOpen}
        profile={null}
        matrix={matrix}
        onClose={() => setProfileEditorOpen(false)}
        onSaved={handleProfileSaved}
      />

      <ConfirmDialog
        open={!!deleteTarget}
        title="Excluir grupo"
        description={
          deleteTarget
            ? `${deleteTarget.members?.length || 0} membro(s) deste grupo perdem o perfil "${roleDisplayName(
                deleteTarget.role_id,
                customProfiles,
              )}" e voltam para Corretor. O perfil de acesso em si continua existindo.`
            : ""
        }
        confirmLabel="Excluir Grupo"
        variant="danger"
        onConfirm={handleDeleteGroup}
        onCancel={() => setDeleteTarget(null)}
      />
    </SectionCard>
  );
}
