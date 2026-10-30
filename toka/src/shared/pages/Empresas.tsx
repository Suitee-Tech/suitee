import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { motion, AnimatePresence } from "motion/react";
import { z } from "zod";
import {
  Building2,
  Plus,
  Check,
  ArrowRight,
  ArrowLeft,
  MapPin,
  Users,
  CheckCircle,
  AlertCircle,
  Phone,
  Mail,
  Globe,
  Shield,
} from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";

import { EmptyState } from "../ui/EmptyState";
import { Button } from "../ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "../ui/dialog";
import { useAuth } from "../context/AuthContext";
import { useTheme } from "../context/ThemeContext";
import { CompanyLogo } from "../components/CompanyLogo";
import { createCompany } from "../services/auth.service";
import { formatCep, formatCnpj } from "../utils/masks";
import { Form } from "../ui/form";
import { CepField, DocumentField, PhoneField, SelectField, TextField } from "../ui/fields";
import { zCep, zDocument, zPhone, zUF } from "../validation/fields";

const companySchema = z.object({
  name: z.string().min(2, "Nome da empresa é obrigatório"),
  legal_name: z.string().min(2, "Razão social é obrigatória"),
  tax_id: zDocument({ mode: "cnpj", required: true }),
  email: z.string().min(1, "E-mail é obrigatório").email("E-mail inválido"),
  phone: zPhone({ required: true }),
  address: z.object({
    street: z.string().min(1, "Rua é obrigatória"),
    number: z.string().min(1, "Número é obrigatório"),
    complement: z.string().optional(),
    district: z.string().min(1, "Bairro é obrigatório"),
    city: z.string().min(1, "Cidade é obrigatória"),
    state: zUF({ required: true }),
    zip_code: zCep({ required: true }),
  }),
  size: z.string(),
  type: z.string(),
});
type CompanyForm = z.infer<typeof companySchema>;

const STATES = [
  "AC","AL","AP","AM","BA","CE","DF","ES","GO","MA","MT","MS","MG",
  "PA","PB","PR","PE","PI","RJ","RN","RS","RO","RR","SC","SP","SE","TO",
];

const emptyForm = {
  name: "",
  legal_name: "",
  tax_id: "",
  phone: "",
  email: "",
  address: {
    street: "",
    number: "",
    complement: "",
    district: "",
    city: "",
    state: "",
    zip_code: "",
  },
  size: "1-10",
  type: "imobiliaria",
};

export function Empresas() {
  const { user, companies, selectCompany, refreshUserData } = useAuth();
  const { theme } = useTheme();

  const [showModal, setShowModal] = useState(false);
  const [step, setStep] = useState(1);
  const [isLoading, setIsLoading] = useState(false);
  const [isSwitching, setSwitching] = useState<string | null>(null);
  const [error, setError] = useState("");

  const form = useForm<CompanyForm>({
    resolver: zodResolver(companySchema),
    mode: "onTouched",
    defaultValues: { ...emptyForm, email: user?.email || "" },
  });
  const sizeValue = form.watch("size");

  const currentCompanyId = user?.current_company_id;

  const handleSwitch = async (companyId: string) => {
    if (companyId === currentCompanyId) return;
    setSwitching(companyId);
    try {
      await selectCompany(companyId);
      toast.success("Empresa alterada com sucesso!");
    } catch {
      toast.error("Erro ao trocar de empresa.");
    } finally {
      setSwitching(null);
    }
  };

  const openModal = () => {
    form.reset({ ...emptyForm, email: user?.email || "" });
    setStep(1);
    setError("");
    setShowModal(true);
  };

  const onSubmit = async (data: CompanyForm) => {
    setIsLoading(true);
    setError("");
    try {
      await createCompany({
        name: data.name,
        legal_name: data.legal_name || data.name,
        tax_id: data.tax_id,
        email: data.email,
        phone: data.phone,
        address: { ...data.address, zip_code: formatCep(data.address.zip_code) },
        type: data.type,
      });
      await refreshUserData();
      setShowModal(false);
      toast.success("Empresa criada com sucesso!");
    } catch (err: any) {
      const status = err?.response?.status;
      if (status === 409) {
        setError("Já existe uma empresa cadastrada com esse CNPJ ou e-mail.");
      } else {
        setError(err.response?.data?.message || err.message || "Falha ao criar empresa.");
      }
    } finally {
      setIsLoading(false);
    }
  };

  const handleNext = async () => {
    if (step === 1) {
      if (await form.trigger(["name", "legal_name", "tax_id", "phone", "email"])) setStep(2);
      return;
    }
    if (step === 2) {
      if (
        await form.trigger([
          "address.street",
          "address.number",
          "address.district",
          "address.city",
          "address.state",
          "address.zip_code",
        ])
      )
        setStep(3);
      return;
    }
    await form.handleSubmit(onSubmit)();
  };

  return (
    <div className="p-3 sm:p-6 lg:p-8 max-w-5xl mx-auto w-full overflow-x-hidden">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6 sm:mb-8">
        <div>
          <h1 className={clsx("text-2xl font-bold", "text-foreground")}>
            Empresas
          </h1>
          <p className={clsx("text-sm mt-1", "text-muted-foreground")}>
            Gerencie suas imobiliárias e alterne entre workspaces.
          </p>
        </div>
        <Button onClick={openModal} className="w-full sm:w-auto">
          <Plus className="w-4 h-4" />
          Nova Empresa
        </Button>
      </div>

      {currentCompanyId && (() => {
        const current = companies.find((c) => c.id === currentCompanyId);
        if (!current) return null;
        return (
          <motion.div
            initial={{ opacity: 0, y: 10 }}
            animate={{ opacity: 1, y: 0 }}
            className={clsx(
              "mb-6 sm:mb-8 p-4 sm:p-5 rounded-2xl border-2 border-primary-500/30 overflow-hidden",
              "bg-primary/10"
            )}
          >
            <div className="flex items-center gap-2 mb-3">
              <Shield className="w-4 h-4 text-primary-500 shrink-0" />
              <span className={clsx("text-xs font-bold uppercase tracking-wider truncate", "text-primary")}>
                Workspace Ativo
              </span>
            </div>
            <div className="flex flex-col sm:flex-row sm:items-center gap-4 min-w-0">
              <div className="flex items-center gap-3 sm:gap-4 flex-1 min-w-0">
                <div className={clsx(
                  "w-12 h-12 sm:w-14 sm:h-14 rounded-xl flex items-center justify-center shrink-0",
                  "bg-card border border-border"
                )}>
                  <CompanyLogo logoUrl={theme?.logoUrl} className="w-6 h-6 sm:w-8 sm:h-8 object-contain opacity-80" />
                </div>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center justify-between gap-2 sm:block">
                    <h3 className={clsx("text-base sm:text-lg font-bold truncate", "text-foreground")}>
                      {current.name}
                    </h3>
                    <span className={clsx(
                      "inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full text-caption font-bold sm:hidden shrink-0",
                      "bg-status-ok-soft text-status-ok"
                    )}>
                      <Check className="w-3 h-3" /> Conectado
                    </span>
                  </div>
                  <div className="flex flex-wrap items-center gap-x-3 sm:gap-x-4 gap-y-1 mt-1 min-w-0">
                    {current.email && (
                      <span className={clsx("flex items-center gap-1 text-xs min-w-0 max-w-full", "text-muted-foreground")}>
                        <Mail className="w-3 h-3 shrink-0" /> <span className="truncate">{current.email}</span>
                      </span>
                    )}
                    {current.phone && (
                      <span className={clsx("flex items-center gap-1 text-xs min-w-0", "text-muted-foreground")}>
                        <Phone className="w-3 h-3 shrink-0" /> <span className="truncate">{current.phone}</span>
                      </span>
                    )}
                  </div>
                </div>
              </div>
              <div className="hidden sm:flex justify-end shrink-0">
                <span className={clsx(
                  "inline-flex items-center gap-1 px-3 py-1 rounded-full text-xs font-bold",
                  "bg-status-ok-soft text-status-ok"
                )}>
                  <Check className="w-3 h-3" /> Conectado
                </span>
              </div>
            </div>
          </motion.div>
        );
      })()}

      <div className="grid gap-4">
        {companies.map((company, index) => {
          const isActive = company.id === currentCompanyId;
          return (
            <motion.div
              key={company.id}
              initial={{ opacity: 0, y: 10 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ delay: index * 0.05 }}
              className={clsx(
                "p-4 sm:p-5 rounded-2xl border transition-all group bg-card overflow-hidden",
                isActive
                  ? "border-primary/40 shadow-sm"
                  : "border-border hover:border-border hover:shadow-sm"
              )}
            >
              <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 min-w-0">
                <div className="flex items-start sm:items-center gap-3 sm:gap-4 flex-1 min-w-0">
                  <div className={clsx(
                    "w-12 h-12 rounded-xl flex items-center justify-center shrink-0 overflow-hidden",
                    isActive
                      ? "bg-card border border-border"
                      : "bg-muted border border-border"
                  )}>
                    <CompanyLogo
                      logoUrl={isActive ? theme?.logoUrl : undefined}
                      className={clsx(
                        "w-6 h-6 object-contain",
                        isActive ? "opacity-80" : "opacity-40 grayscale"
                      )}
                    />
                  </div>

                  <div className="flex-1 min-w-0 space-y-1.5">
                    <div className="flex items-center gap-2 flex-wrap">
                      <h3 className={clsx("text-base font-bold truncate", "text-foreground")}>
                        {company.name}
                      </h3>
                      {isActive && (
                        <span className={clsx(
                          "inline-flex items-center px-2 py-0.5 rounded text-caption font-bold uppercase tracking-wider shrink-0",
                          "bg-primary/10 text-primary"
                        )}>
                          Ativo
                        </span>
                      )}
                    </div>
                    {company.legal_name && (
                      <p className={clsx("text-sm truncate", "text-muted-foreground")}>
                        {company.legal_name}
                      </p>
                    )}
                    {company.tax_id && (
                      <p className={clsx("text-xs truncate", "text-muted-foreground")}>
                        CNPJ: {formatCnpj(company.tax_id)}
                      </p>
                    )}
                    <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5 mt-2 pt-3 sm:pt-1 border-t sm:border-t-0 border-border/50 min-w-0">
                      {company.email && (
                        <span className={clsx("flex items-center gap-1.5 text-xs min-w-0 max-w-full", "text-muted-foreground")}>
                          <Mail className="w-3.5 h-3.5 shrink-0" /> <span className="truncate">{company.email}</span>
                        </span>
                      )}
                      {company.address?.city && company.address?.state && (
                        <span className={clsx("flex items-center gap-1.5 text-xs min-w-0 max-w-full", "text-muted-foreground")}>
                          <Globe className="w-3.5 h-3.5 shrink-0" /> <span className="truncate">{company.address.city}/{company.address.state}</span>
                        </span>
                      )}
                      <span className={clsx(
                        "inline-flex items-center px-2 py-0.5 rounded text-caption font-bold uppercase tracking-wider shrink-0",
                        "bg-muted text-muted-foreground"
                      )}>
                        {company.profile === "owner" ? "Dono" : "Membro"}
                      </span>
                    </div>
                  </div>
                </div>

                <div className="flex sm:shrink-0 w-full sm:w-auto mt-3 sm:mt-0">
                  {isActive ? (
                    <div className={clsx(
                      "w-full sm:w-10 h-10 rounded-xl sm:rounded-full flex items-center justify-center shrink-0 gap-2",
                      "bg-status-ok-soft text-status-ok"
                    )}>
                      <Check className="w-5 h-5" />
                      <span className="sm:hidden font-bold text-sm">Workspace Atual</span>
                    </div>
                  ) : (
                    <Button
                      variant="secondary"
                      onClick={() => handleSwitch(company.id)}
                      loading={isSwitching === company.id}
                      disabled={isSwitching !== null}
                      className="w-full sm:w-auto"
                    >
                      <ArrowRight className="w-4 h-4" />
                      Acessar
                    </Button>
                  )}
                </div>
              </div>
            </motion.div>
          );
        })}

        {companies.length === 0 && (
          <EmptyState
            icon={Building2}
            title="Nenhuma empresa cadastrada"
            message="Crie sua primeira imobiliária para começar a usar o sistema."
            action={
              <Button onClick={openModal}>
                <Plus className="w-4 h-4" />
                Cadastrar Empresa
              </Button>
            }
            className={clsx(
              "border-2 border-dashed",
              "border-border bg-card"
            )}
          />
        )}

        {companies.length > 0 && (
          <Button
            variant="outline"
            onClick={openModal}
            className="h-auto p-5 rounded-2xl border-2 border-dashed text-muted-foreground hover:border-primary/40 hover:text-primary hover:bg-primary/5 font-bold"
          >
            <Plus className="w-5 h-5" />
            Cadastrar Nova Empresa
          </Button>
        )}
      </div>

      <Dialog open={showModal} onOpenChange={(next) => { if (!next && !isLoading) setShowModal(false); }}>
        <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-xl">
          <Form {...form}>
          <div className="flex items-center gap-3 px-8 py-5 border-b border-border pr-12">
            <div className="w-10 h-10 rounded-xl flex items-center justify-center bg-muted">
              <CompanyLogo className="w-5 h-5 object-contain opacity-50 grayscale" />
            </div>
            <div>
              <DialogTitle className="text-lg font-bold text-foreground">
                Nova Empresa
              </DialogTitle>
              <DialogDescription className="text-xs">
                Passo {step} de 3
              </DialogDescription>
            </div>
          </div>

          <div className="px-8 pt-5">
            <div className="flex gap-2">
              {[1, 2, 3].map((s) => (
                <div
                  key={s}
                  className={clsx(
                    "flex-1 h-1.5 rounded-full transition-colors",
                    s <= step
                      ? "bg-primary-600"
                      : "bg-muted"
                  )}
                />
              ))}
            </div>
          </div>

          <div className="px-8 py-6 max-h-[60vh] overflow-y-auto">
            {error && (
              <div className="mb-5 p-3 bg-status-critical-soft border border-status-critical/30 text-status-critical text-sm rounded-lg flex items-center gap-2">
                <AlertCircle className="w-4 h-4 shrink-0" />
                {error}
              </div>
            )}

            <AnimatePresence mode="wait">
              <motion.div
                key={step}
                initial={{ opacity: 0, x: 20 }}
                animate={{ opacity: 1, x: 0 }}
                exit={{ opacity: 0, x: -20 }}
                transition={{ duration: 0.2 }}
              >
                {step === 1 && (
                  <div className="space-y-5">
                    <div className="text-center mb-6">
                      <div className={clsx(
                        "w-14 h-14 rounded-full flex items-center justify-center mx-auto mb-4",
                        "bg-muted"
                      )}>
                        <CompanyLogo className="w-8 h-8 object-contain opacity-50 grayscale" />
                      </div>
                      <h3 className={clsx("text-xl font-bold mb-1", "text-foreground")}>
                        Dados da Empresa
                      </h3>
                      <p className={clsx("text-sm", "text-muted-foreground")}>
                        Informações básicas da sua imobiliária
                      </p>
                    </div>

                    <TextField control={form.control} name="name" label="Nome Fantasia" required placeholder="Ex: Lumière Imóveis" />
                    <TextField
                      control={form.control}
                      name="legal_name"
                      label="Razão Social"
                      required
                      placeholder="Ex: Lumière Negócios Imobiliários Ltda"
                    />
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                      <DocumentField
                        control={form.control}
                        name="tax_id"
                        label="CNPJ"
                        mode="cnpj"
                        required
                        placeholder="00.000.000/0000-00"
                      />
                      <PhoneField control={form.control} name="phone" label="Telefone" required />
                    </div>
                    <TextField
                      control={form.control}
                      name="email"
                      label="E-mail da Empresa"
                      type="email"
                      required
                      placeholder="contato@empresa.com"
                    />
                  </div>
                )}

                {step === 2 && (
                  <div className="space-y-5">
                    <div className="text-center mb-6">
                      <div className={clsx(
                        "w-14 h-14 rounded-full flex items-center justify-center mx-auto mb-4",
                        "bg-primary/10 text-primary"
                      )}>
                        <MapPin className="w-7 h-7" />
                      </div>
                      <h3 className={clsx("text-xl font-bold mb-1", "text-foreground")}>
                        Endereço Comercial
                      </h3>
                      <p className={clsx("text-sm", "text-muted-foreground")}>
                        Onde fica a matriz da operação?
                      </p>
                    </div>

                    <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
                      <CepField control={form.control} name="address.zip_code" label="CEP" required />
                      <TextField
                        control={form.control}
                        name="address.street"
                        label="Rua / Logradouro"
                        required
                        className="col-span-2"
                      />
                    </div>
                    <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
                      <TextField control={form.control} name="address.number" label="Número" required />
                      <TextField
                        control={form.control}
                        name="address.complement"
                        label="Complemento"
                        className="col-span-2"
                      />
                    </div>
                    <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
                      <TextField control={form.control} name="address.district" label="Bairro" required />
                      <TextField control={form.control} name="address.city" label="Cidade" required />
                      <SelectField
                        control={form.control}
                        name="address.state"
                        label="Estado"
                        required
                        placeholder="UF"
                        options={STATES.map((s) => ({ value: s, label: s }))}
                      />
                    </div>
                  </div>
                )}

                {step === 3 && (
                  <div className="space-y-5">
                    <div className="text-center mb-6">
                      <div className={clsx(
                        "w-14 h-14 rounded-full flex items-center justify-center mx-auto mb-4",
                        "bg-primary/10 text-primary"
                      )}>
                        <Users className="w-7 h-7" />
                      </div>
                      <h3 className={clsx("text-xl font-bold mb-1", "text-foreground")}>
                        Tamanho da Equipe
                      </h3>
                      <p className={clsx("text-sm", "text-muted-foreground")}>
                        Quantas pessoas usarão o sistema?
                      </p>
                    </div>

                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                      {["1-10", "11-50", "51-200", "200+"].map((size) => (
                        <button
                          key={size}
                          type="button"
                          onClick={() => form.setValue("size", size)}
                          className={clsx(
                            "py-4 rounded-xl border text-sm font-bold transition-all",
                            sizeValue === size
                              ? "bg-primary/10 border-primary text-primary shadow-sm"
                              : "bg-card border-border text-muted-foreground hover:border-muted-foreground/40"
                          )}
                        >
                          {size}
                        </button>
                      ))}
                    </div>

                    <div className={clsx(
                      "mt-6 p-4 rounded-xl flex items-start gap-3",
                      "bg-status-ok-soft text-status-ok"
                    )}>
                      <CheckCircle className={clsx("w-5 h-5 shrink-0 mt-0.5", "text-status-ok")} />
                      <div>
                        <h4 className="text-sm font-bold">Pronto para decolar!</h4>
                        <p className={clsx("text-xs mt-1", "text-status-ok")}>
                          Ao finalizar, criaremos seu ambiente seguro e isolado.
                        </p>
                      </div>
                    </div>
                  </div>
                )}
              </motion.div>
            </AnimatePresence>
          </div>

          <div className={clsx(
            "px-8 py-5 border-t flex gap-3",
            "border-border"
          )}>
            {step > 1 && (
              <Button
                variant="outline"
                size="icon"
                onClick={() => setStep(step - 1)}
                disabled={isLoading}
                className="h-11 w-11 shrink-0"
              >
                <ArrowLeft className="w-5 h-5" />
              </Button>
            )}
            <Button
              size="lg"
              onClick={handleNext}
              loading={isLoading}
              disabled={isLoading}
              className="flex-1 group"
            >
              {step === 3 ? (
                <>
                  <CheckCircle className="w-4 h-4" />
                  Criar Empresa
                </>
              ) : (
                <>
                  Próximo
                  <ArrowRight className="w-4 h-4 group-hover:translate-x-0.5 transition-transform" />
                </>
              )}
            </Button>
          </div>
          </Form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
