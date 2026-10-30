import { useState, useCallback, useMemo, useEffect, useRef } from "react";
import { motion, AnimatePresence } from "motion/react";
import { Save } from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";
import { useBlocker } from "react-router";
import { useAuth } from "../context/AuthContext";
import { usePermissionsContext } from "../context/PermissionsContext";
import { Button } from "../ui/button";
import { PageHeader } from "../ui/page-header";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { TABS, type SettingsTab } from "./Settings/_constants";
import { type TabRef } from "./Settings/_shared";
import { ProfileTab } from "./Settings/ProfileTab";
import { CompanyTab } from "./Settings/CompanyTab";
import { AppearanceTab } from "./Settings/AppearanceTab";
import { PermissionsTab } from "./Settings/PermissionsTab";
import { NotificationsTab } from "./Settings/NotificationsTab";
import { IntegrationsTab } from "./Settings/IntegrationsTab";
import { CommissionTab } from "./Settings/CommissionTab";
import { GeneralTab } from "./Settings/GeneralTab";

export { type TabRef } from "./Settings/_shared";

export function Settings() {
  const { refreshUserData, user } = useAuth();
  const { can } = usePermissionsContext();
  const [activeTab, setActiveTab] = useState<SettingsTab>("profile");
  const [isSaving, setIsSaving] = useState(false);
  const [hasChanges, setHasChanges] = useState(false);
  const [pendingTab, setPendingTab] = useState<SettingsTab | null>(null);
  const blocker = useBlocker(hasChanges);
  const tabRef = useRef<TabRef>(null);

  const filteredTabs = useMemo(() => {
    return TABS.filter((tab) => {
      if (user?.permission === "owner" || user?.permission === "admin") return true;
      return !tab.permission || can(tab.permission);
    });
  }, [can, user?.permission]);

  useEffect(() => {
    if (!filteredTabs.find((t) => t.id === activeTab)) {
      setActiveTab(filteredTabs[0]?.id || "profile");
    }
  }, [filteredTabs, activeTab]);

  const markChanged = useCallback(() => setHasChanges(true), []);

  // Trocar de aba desmonta a aba ativa. Sem esta guarda, a edição local era
  // descartada e o botão "Salvar" continuava habilitado, dando a impressão de
  // que o trabalho tinha sido salvo. A troca só acontece depois de uma decisão
  // explícita ou quando não há alterações pendentes.
  const requestTabChange = useCallback(
    (next: SettingsTab) => {
      if (next === activeTab || isSaving) return;
      if (hasChanges) {
        setPendingTab(next);
        return;
      }
      setActiveTab(next);
    },
    [activeTab, hasChanges, isSaving],
  );

  const discardAndSwitch = useCallback(() => {
    if (!pendingTab) return;
    setActiveTab(pendingTab);
    setHasChanges(false);
    setPendingTab(null);
  }, [pendingTab]);

  useEffect(() => {
    if (!hasChanges) return;
    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", onBeforeUnload);
    return () => window.removeEventListener("beforeunload", onBeforeUnload);
  }, [hasChanges]);

  const handleSave = useCallback(async () => {
    if (!tabRef.current) return;
    setIsSaving(true);
    try {
      await tabRef.current.save();
      if (refreshUserData) await refreshUserData();
      setHasChanges(false);
      toast.success("Configurações salvas com sucesso!");
    } catch (error: any) {
      const status: number | undefined = error?.response?.status;
      const fields: Array<{ field: string; message: string }> | undefined = error?.response?.data?.data?.fields;
      const backendMessage: string | undefined = error?.response?.data?.message;
      const isNetworkError = error?.response === undefined || error?.code === "ERR_NETWORK";

      const fieldLabels: Record<string, string> = {
        name: "Nome Fantasia",
        legal_name: "Razão Social",
        tax_id: "CNPJ",
        email: "E-mail",
        phone: "Telefone",
        "address.street": "Logradouro",
        "address.number": "Número",
        "address.district": "Bairro",
        "address.city": "Cidade",
        "address.state": "Estado",
        "address.zip_code": "CEP",
      };
      const friendly = (raw: string): string => {
        if (/cnpj/i.test(raw)) return "CNPJ inválido. Verifique os dígitos verificadores.";
        if (/email/i.test(raw)) return "E-mail inválido.";
        if (/required/i.test(raw)) return "Campo obrigatório.";
        return raw;
      };

      if (status === 400 && Array.isArray(fields) && fields.length > 0) {
        const first = fields[0];
        const label = fieldLabels[first.field] || first.field;
        toast.error(`${label}: ${friendly(first.message)}`);
      } else if (isNetworkError) {
        toast.error("Sem conexão com o servidor. Verifique sua internet e tente novamente.");
      } else if (status && status >= 500) {
        toast.error("Não conseguimos salvar as configurações agora. Tente novamente em alguns instantes.");
      } else if (status === 400 && backendMessage) {
        toast.error(friendly(backendMessage));
      } else {
        toast.error(error?.message || "Erro ao salvar configurações");
      }
    } finally {
      setIsSaving(false);
    }
  }, [refreshUserData]);

  return (
    <div className="h-full flex flex-col bg-muted ">
      <div className="flex-1 overflow-auto p-6 lg:p-8">
        <div className="max-w-6xl mx-auto space-y-6">
          <PageHeader
            title="Configurações"
            description="Gerencie as parametrizações e preferências do seu sistema suitee."
          >
            <div className="flex items-center gap-3">
              {hasChanges && (
                <motion.span
                  initial={{ opacity: 0, scale: 0.9 }}
                  animate={{ opacity: 1, scale: 1 }}
                  className="px-3 py-1.5 bg-status-warning-soft text-status-warning text-xs font-bold rounded-full border border-status-warning/30"
                >
                  Alterações não salvas
                </motion.span>
              )}
              <Button
                onClick={handleSave}
                loading={isSaving}
                loadingText="Salvando..."
                disabled={!hasChanges}
                className="min-w-[160px]"
              >
                <Save className="w-4 h-4" />
                Salvar Alterações
              </Button>
            </div>
          </PageHeader>

          <div className="flex flex-col md:flex-row gap-6 items-start">
            <nav aria-label="Seções de configurações" className="flex flex-row md:flex-col gap-1 w-full md:w-60 shrink-0 overflow-x-auto md:overflow-visible pb-2 md:pb-0 border-b md:border-b-0 border-border">
              {filteredTabs.map((tab) => {
                const Icon = tab.icon;
                const active = activeTab === tab.id;
                return (
                  <button
                    key={tab.id}
                    onClick={() => requestTabChange(tab.id)}
                    className={clsx(
                      "flex items-center gap-3 px-4 py-3 rounded-xl text-sm font-bold whitespace-nowrap transition-colors text-left w-full",
                      active ? "bg-primary-50 text-primary-700" : "text-muted-foreground hover:bg-muted hover:text-foreground",
                    )}
                  >
                    <Icon className="w-[18px] h-[18px] shrink-0" />
                    <span className="truncate">{tab.label}</span>
                  </button>
                );
              })}
            </nav>

            <div className="flex-1 min-w-0 w-full">
              <AnimatePresence mode="wait" initial={false}>
                <motion.div
                  key={activeTab}
                  initial={{ opacity: 0 }}
                  animate={{ opacity: 1 }}
                  exit={{ opacity: 0 }}
                  transition={{ duration: 0.15 }}
                >
                  {activeTab === "profile" && <ProfileTab ref={tabRef} onChange={markChanged} />}
                  {activeTab === "company" && <CompanyTab ref={tabRef} onChange={markChanged} />}
                  {activeTab === "appearance" && <AppearanceTab ref={tabRef} onChange={markChanged} />}
                  {activeTab === "permissions" && <PermissionsTab ref={tabRef} onChange={markChanged} />}
                  {activeTab === "notifications" && <NotificationsTab key={`${user?.id}:${user?.current_company_id}`} ref={tabRef} onChange={markChanged} />}
                  {activeTab === "integrations" && <IntegrationsTab ref={tabRef} onChange={markChanged} />}
                  {activeTab === "commission" && <CommissionTab ref={tabRef} onChange={markChanged} />}
                  {activeTab === "general" && <GeneralTab key={`${user?.id}:${user?.current_company_id}`} ref={tabRef} onChange={markChanged} />}
                </motion.div>
              </AnimatePresence>
            </div>
          </div>
        </div>
      </div>

      <ConfirmDialog
        open={pendingTab !== null}
        title="Descartar alterações não salvas?"
        description={`As alterações em "${filteredTabs.find((tab) => tab.id === activeTab)?.label ?? "Configurações"}" serão perdidas se você trocar de aba. Salve antes de continuar ou descarte para abrir a outra aba.`}
        confirmLabel="Descartar e trocar"
        cancelLabel="Continuar editando"
        variant="warning"
        onCancel={() => setPendingTab(null)}
        onConfirm={discardAndSwitch}
      />

      <ConfirmDialog
        open={blocker.state === "blocked"}
        title="Sair sem salvar?"
        description="Você tem alterações em Configurações que ainda não foram salvas. Se sair agora, elas serão perdidas."
        confirmLabel="Sair sem salvar"
        cancelLabel="Continuar editando"
        variant="warning"
        onCancel={() => blocker.reset?.()}
        onConfirm={() => blocker.proceed?.()}
      />
    </div>
  );
}

