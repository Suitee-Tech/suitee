import { useEffect, useMemo, useState } from "react";
import { Check, ChevronRight, Search, ShieldCheck, Sparkles } from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "../../ui/sheet";
import { Button } from "../../ui/button";
import { settingsService, type CustomRole, type Permission } from "../../services/settings.service";
import { FUNCTIONAL_ROLES, ROLE_LABELS, type FunctionalRole } from "../../../lib/permissions/catalog";
import { COLOR_PRESETS } from "./_constants";
import { SettingsInput, Badge } from "./_shared";
import { permissionsForCatalogRole } from "./accessProfiles";
import { notifyApiError } from "../../services/errorToast";

/**
 * Editor de PERFIL DE ACESSO — o componente único das duas telas.
 *
 * Ele é usado por "Configurações → Permissões de Acesso → Perfis Personalizados"
 * e, embutido, pelo diálogo de "Novo Grupo" (Empresa e Equipe → Grupos de
 * Usuários): criar o perfil e atribuí-lo a um grupo é o MESMO fluxo, com a mesma
 * fonte de dados.
 *
 * O que mudou em relação à tela antiga: o perfil deixou de ser um rótulo com
 * nome/cor. Aqui se escolhe o CONJUNTO EFETIVO de permissões (por módulo, com
 * busca e contadores), e é esse conjunto que o servidor aplica quando alguém
 * recebe o perfil. O "papel base" só semeia/repõe o conjunto — quem decide é o
 * que ficou marcado.
 *
 * A gravação é IMEDIATA (endpoint próprio, auditado, com invalidação do cache
 * de permissões): o perfil não depende do "Salvar" da matriz. Foi de propósito —
 * um documento replace que carregasse os perfis junto apagaria o acesso de quem
 * os usa quando alguém salvasse só a matriz.
 */
export function AccessProfileEditor({
  open,
  profile,
  matrix,
  usageCount,
  onClose,
  onSaved,
}: {
  open: boolean;
  /** `null` = criar; perfil existente = editar. */
  profile: CustomRole | null;
  /** Matriz do tenant — usada para semear o conjunto a partir do papel base. */
  matrix: Permission[];
  /** Quantos usuários usam o perfil (mostrado no aviso de impacto). */
  usageCount?: number;
  onClose: () => void;
  onSaved: (profile: CustomRole, created: boolean) => void | Promise<void>;
}) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [baseRole, setBaseRole] = useState<string>("corretor");
  const [color, setColor] = useState(COLOR_PRESETS[0].value);
  const [granted, setGranted] = useState<Set<string>>(new Set());
  const [search, setSearch] = useState("");
  const [expandedModule, setExpandedModule] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  /** Marca se o conjunto foi tocado à mão — trocar o papel base não sobrescreve escolha manual sem aviso. */
  const [touched, setTouched] = useState(false);

  useEffect(() => {
    if (!open) return;
    if (profile) {
      setName(profile.name);
      setDescription(profile.description || "");
      setBaseRole(profile.baseRole || "corretor");
      setColor(profile.color || COLOR_PRESETS[0].value);
      setGranted(new Set(profile.permissions || []));
      setTouched(true);
    } else {
      setName("");
      setDescription("");
      const seedRole = "corretor";
      setBaseRole(seedRole);
      setColor(COLOR_PRESETS[0].value);
      setGranted(permissionsForCatalogRole(seedRole, matrix));
      setTouched(false);
    }
    setSearch("");
    setExpandedModule(null);
    setSaving(false);
  }, [open, profile, matrix]);

  const modules = useMemo(() => [...new Set(matrix.map((p) => p.module))], [matrix]);

  const visible = useMemo(() => {
    const term = search.trim().toLowerCase();
    if (!term) return matrix;
    return matrix.filter((p) => p.label.toLowerCase().includes(term) || p.module.toLowerCase().includes(term));
  }, [matrix, search]);

  const toggle = (permissionId: string) => {
    setTouched(true);
    setGranted((current) => {
      const next = new Set(current);
      if (next.has(permissionId)) next.delete(permissionId);
      else next.add(permissionId);
      return next;
    });
  };

  const toggleModule = (module: string, enable: boolean) => {
    setTouched(true);
    setGranted((current) => {
      const next = new Set(current);
      matrix
        .filter((p) => p.module === module)
        .forEach((p) => (enable ? next.add(p.id) : next.delete(p.id)));
      return next;
    });
  };

  const applyBaseRole = (role: string) => {
    setBaseRole(role);
    setGranted(permissionsForCatalogRole(role, matrix));
    setTouched(false);
  };

  const handleSave = async () => {
    const trimmed = name.trim();
    if (!trimmed) {
      toast.error("Dê um nome ao perfil de acesso");
      return;
    }
    setSaving(true);
    try {
      const payload = {
        name: trimmed,
        description: description.trim(),
        baseRole,
        color,
        permissions: [...granted],
      };
      const saved = profile
        ? await settingsService.updateAccessProfile(profile.id, payload)
        : await settingsService.createAccessProfile(payload);
      toast.success(
        profile
          ? `Perfil "${saved?.name || trimmed}" atualizado — vale para quem já o usa`
          : `Perfil "${saved?.name || trimmed}" criado com ${granted.size} permissões`,
      );
      await onSaved(saved, !profile);
      onClose();
    } catch (error: any) {
      notifyApiError(error, { context: "Salvar perfil de acesso" });
    } finally {
      setSaving(false);
    }
  };

  return (
    // Painel lateral direito (padrão da casa para formulários: `Sheet`). Alert
    // (modal) fica reservado a CONFIRMAÇÃO de ação crítica — o editor de perfil
    // é formulário, e o painel lateral ainda deixa a lista de perfis visível
    // enquanto se edita.
    <Sheet
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
    >
      <SheetContent side="right" className="w-full gap-0 p-0 sm:max-w-3xl">
        <SheetHeader className="border-b border-border px-6 py-4 pr-14">
          <SheetTitle className="flex items-center gap-2 font-bold text-foreground">
            <ShieldCheck className="w-4 h-4 text-primary" />
            {profile ? "Editar perfil de acesso" : "Novo perfil de acesso"}
          </SheetTitle>
          <SheetDescription className="text-xs text-muted-foreground">
            O perfil define o que a pessoa pode fazer. Ele é atribuído a uma ou mais pessoas por um Grupo de Usuários.
          </SheetDescription>
        </SheetHeader>

        <div className="flex-1 overflow-y-auto">
          <div className="p-6 space-y-4">
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <label className="text-xs font-bold text-muted-foreground uppercase">Nome do perfil</label>
                <SettingsInput value={name} onChange={setName} placeholder="Ex: Corretor Sênior" ariaLabel="Nome do perfil" />
              </div>
              <div className="space-y-1.5">
                <label className="text-xs font-bold text-muted-foreground uppercase">Baseado em</label>
                <select
                  value={baseRole}
                  onChange={(event) => applyBaseRole(event.target.value)}
                  aria-label="Perfil base"
                  className="w-full px-3 py-2.5 bg-card border border-border rounded-xl text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-primary-500/20"
                >
                  {FUNCTIONAL_ROLES.map((role) => (
                    <option key={role} value={role}>
                      {ROLE_LABELS[role]}
                    </option>
                  ))}
                </select>
              </div>
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-bold text-muted-foreground uppercase">Descrição</label>
              <SettingsInput
                value={description}
                onChange={setDescription}
                placeholder="Para que serve este perfil (opcional)"
                ariaLabel="Descrição do perfil"
              />
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-bold text-muted-foreground uppercase">Cor</label>
              <div className="flex flex-wrap items-center gap-2">
                {COLOR_PRESETS.map((preset) => (
                  <button
                    key={preset.value}
                    type="button"
                    title={preset.name}
                    aria-label={`Cor ${preset.name}`}
                    onClick={() => setColor(preset.value)}
                    className={clsx(
                      "w-7 h-7 rounded-full border-2 transition-transform",
                      color === preset.value ? "border-foreground scale-110" : "border-transparent",
                    )}
                    style={{ backgroundColor: preset.value }}
                  />
                ))}
              </div>
            </div>

            <div className="rounded-xl border border-border overflow-hidden">
              <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 px-4 py-3 bg-muted border-b border-border">
                <div className="flex items-center gap-2 whitespace-nowrap">
                  <Sparkles className="w-4 h-4 text-primary shrink-0" />
                  <span className="text-sm font-bold text-foreground">Permissões do perfil</span>
                  <Badge
                    text={`${granted.size} de ${matrix.length}`}
                    variant={granted.size === 0 ? "warning" : "success"}
                  />
                </div>
                <div className="flex items-center gap-2">
                  {touched && (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => applyBaseRole(baseRole)}
                      className="text-xs whitespace-nowrap shrink-0"
                    >
                      Repor do perfil base
                    </Button>
                  )}
                  <div className="relative">
                    <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground" />
                    <input
                      value={search}
                      onChange={(event) => setSearch(event.target.value)}
                      placeholder="Buscar permissão..."
                      aria-label="Buscar permissão"
                      className="pl-8 pr-3 py-2 w-full sm:w-56 bg-card border border-border rounded-xl text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-primary-500/20"
                    />
                  </div>
                </div>
              </div>

              {granted.size === 0 && (
                <p className="px-4 py-3 text-xs text-status-warning bg-status-warning-soft border-b border-border">
                  Nenhuma permissão marcada: quem receber este perfil não conseguirá abrir nenhuma tela do sistema.
                </p>
              )}

              <div className="divide-y divide-border max-h-[38vh] overflow-y-auto">
                {modules.map((module) => {
                  const modulePerms = visible.filter((p) => p.module === module);
                  if (modulePerms.length === 0) return null;
                  const allModule = matrix.filter((p) => p.module === module);
                  const grantedCount = allModule.filter((p) => granted.has(p.id)).length;
                  const expanded = expandedModule === module || Boolean(search.trim());

                  return (
                    <div key={module}>
                      <div className="flex items-center justify-between gap-3 px-4 py-2.5 hover:bg-muted/40">
                        <button
                          type="button"
                          onClick={() => setExpandedModule(expandedModule === module ? null : module)}
                          className="flex items-center gap-2 min-w-0"
                          aria-expanded={expanded}
                        >
                          <ChevronRight className={clsx("w-4 h-4 text-muted-foreground transition-transform", expanded && "rotate-90")} />
                          <span className="text-sm font-bold text-foreground truncate">{module}</span>
                          <Badge text={`${grantedCount}/${allModule.length}`} variant={grantedCount === allModule.length ? "success" : "default"} />
                        </button>
                        <button
                          type="button"
                          onClick={() => toggleModule(module, grantedCount !== allModule.length)}
                          className="text-xs font-bold text-primary hover:underline shrink-0"
                        >
                          {grantedCount === allModule.length ? "Desmarcar" : "Marcar todas"}
                        </button>
                      </div>
                      {expanded && (
                        <div className="px-4 pb-3 space-y-1">
                          {modulePerms.map((permission) => {
                            const checked = granted.has(permission.id);
                            // O estado DESMARCADO precisa ser visível nos dois temas:
                            // o affordance é a BORDA (token), não o preenchimento —
                            // `bg-muted` sumia sobre a linha em `hover:bg-muted` e no
                            // modo noturno.
                            return (
                              <button
                                key={permission.id}
                                type="button"
                                onClick={() => toggle(permission.id)}
                                aria-pressed={checked}
                                className="group/perm w-full flex items-center justify-between gap-3 px-3 py-2 rounded-lg hover:bg-muted text-left"
                              >
                                <span className="text-sm text-foreground min-w-0 truncate">{permission.label}</span>
                                <span
                                  aria-hidden="true"
                                  className={clsx(
                                    "w-5 h-5 rounded-md flex items-center justify-center shrink-0 border-2 transition-colors",
                                    checked
                                      ? "border-status-ok bg-status-ok text-white"
                                      : "border-muted-foreground/45 bg-background text-transparent group-hover/perm:border-primary/70",
                                  )}
                                >
                                  <Check className="w-3 h-3" />
                                </span>
                              </button>
                            );
                          })}
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            </div>

            {profile && typeof usageCount === "number" && usageCount > 0 && (
              <p className="text-xs text-muted-foreground">
                {usageCount === 1 ? "1 pessoa usa" : `${usageCount} pessoas usam`} este perfil agora. Alterar as permissões
                vale imediatamente para elas.
              </p>
            )}
          </div>
        </div>

        <SheetFooter className="flex-row justify-end gap-3 border-t border-border bg-muted px-6 py-4">
          <Button variant="ghost" onClick={onClose} disabled={saving}>
            Cancelar
          </Button>
          <Button onClick={handleSave} disabled={saving || !name.trim()}>
            {saving ? "Salvando..." : profile ? "Salvar perfil" : "Criar perfil"}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}
