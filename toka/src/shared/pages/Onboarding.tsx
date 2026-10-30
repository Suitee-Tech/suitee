import { useState } from "react";
import { useNavigate } from "react-router";
import { motion } from "motion/react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { ArrowRight, ArrowLeft, MapPin, Users, CheckCircle, AlertCircle } from "lucide-react";
import { clsx } from "clsx";
import { createCompany } from "../services/auth.service";
import { useAuth } from "../context/AuthContext";
import { formatCep } from "../utils/masks";
import { Form } from "../ui/form";
import { CepField, DocumentField, PhoneField, SelectField, TextField } from "../ui/fields";
import { zCep, zDocument, zPhone, zUF } from "../validation/fields";
import suiteeLogo from "../../assets/logo.png";
import { CompanyLogo } from "../components/CompanyLogo";
import { Button } from "../ui/button";

const step1Schema = z.object({
  name: z.string().min(2, "Nome da empresa é obrigatório"),
  legal_name: z.string().min(2, "Razão social é obrigatória"),
  tax_id: zDocument({ mode: "cnpj", required: true }),
  phone: zPhone({ required: true }),
});

const addressSchema = z.object({
  street: z.string().min(1, "Rua é obrigatória"),
  number: z.string().min(1, "Número é obrigatório"),
  complement: z.string().optional(),
  district: z.string().min(1, "Bairro é obrigatório"),
  city: z.string().min(1, "Cidade é obrigatória"),
  state: zUF({ required: true }),
  zip_code: zCep({ required: true }),
});

const companySchema = step1Schema.extend({
  address: addressSchema,
  size: z.string(),
  type: z.string(),
});
type CompanyForm = z.infer<typeof companySchema>;

const UF_OPTIONS = [
  { value: "AC", label: "AC" },
  { value: "AL", label: "AL" },
  { value: "AP", label: "AP" },
  { value: "AM", label: "AM" },
  { value: "BA", label: "BA" },
  { value: "CE", label: "CE" },
  { value: "DF", label: "DF" },
  { value: "ES", label: "ES" },
  { value: "GO", label: "GO" },
  { value: "MA", label: "MA" },
  { value: "MT", label: "MT" },
  { value: "MS", label: "MS" },
  { value: "MG", label: "MG" },
  { value: "PA", label: "PA" },
  { value: "PB", label: "PB" },
  { value: "PR", label: "PR" },
  { value: "PE", label: "PE" },
  { value: "PI", label: "PI" },
  { value: "RJ", label: "RJ" },
  { value: "RN", label: "RN" },
  { value: "RS", label: "RS" },
  { value: "RO", label: "RO" },
  { value: "RR", label: "RR" },
  { value: "SC", label: "SC" },
  { value: "SP", label: "SP" },
  { value: "SE", label: "SE" },
  { value: "TO", label: "TO" },
];

const SIZE_OPTIONS = ["1-10", "11-50", "51-200", "200+"];

export function Onboarding() {
  const navigate = useNavigate();
  const { user, logout, refreshUserData } = useAuth();
  const [step, setStep] = useState(1);
  const [isLoading, setIsLoading] = useState(false);
  const [isLoggingOut, setIsLoggingOut] = useState(false);
  const [error, setError] = useState("");

  const form = useForm<CompanyForm>({
    resolver: zodResolver(companySchema),
    mode: "onTouched",
    defaultValues: {
      name: "",
      legal_name: "",
      tax_id: "",
      phone: "",
      address: { street: "", number: "", complement: "", district: "", city: "", state: "", zip_code: "" },
      size: "1-10",
      type: "imobiliaria",
    },
  });

  const handleSair = async () => {
    if (isLoggingOut) return;
    setIsLoggingOut(true);
    try {
      await logout();
    } finally {
      navigate("/login", { replace: true });
    }
  };

  const onSubmit = async (data: CompanyForm) => {
    setIsLoading(true);
    setError("");
    try {
      const ownerEmail = user?.email;
      if (!ownerEmail) {
        setError("Não foi possível identificar seu e-mail. Faça login novamente.");
        return;
      }
      z.string().email().parse(ownerEmail);

      await createCompany({
        name: data.name,
        legal_name: data.legal_name || data.name,
        tax_id: data.tax_id,
        email: ownerEmail,
        phone: data.phone,
        address: { ...data.address, zip_code: formatCep(data.address.zip_code) },
        type: data.type,
      });

      await refreshUserData();
      navigate("/");
    } catch (err: any) {
      console.error(err);
      if (err instanceof z.ZodError) {
        setError(err.issues[0].message);
        return;
      }
      const status: number | undefined = err?.response?.status;
      const backendMessage: string | undefined = err?.response?.data?.message;
      const isNetworkError = !err?.response || err?.code === "ERR_NETWORK";

      if (status === 409) {
        setError("Já existe uma empresa cadastrada com esse CNPJ. Se essa empresa é sua, solicite acesso ao administrador.");
      } else if (status === 403) {
        // Negação de autorização (ex.: papel `cliente`, que fica fora da
        // superfície de operador). Sem este ramo a tela caía no fallback abaixo
        // e exibia a mensagem crua do backend — "permission denied: insufficient
        // permissions" — em inglês, no meio de uma UI em português.
        setError("Sua conta não tem permissão para criar a imobiliária. Fale com o administrador.");
      } else if (isNetworkError) {
        setError("Sem conexão com o servidor. Verifique sua internet e tente novamente.");
      } else if (status && status >= 500) {
        setError("Não conseguimos criar sua imobiliária no momento. Tente novamente em alguns instantes.");
      } else {
        setError(backendMessage || err.message || "Falha ao criar empresa. Tente novamente.");
      }
    } finally {
      setIsLoading(false);
    }
  };

  // Gating dos passos: valida só os campos do passo atual antes de avançar.
  const w = form.watch();
  const nextDisabled =
    isLoading ||
    (step === 1 &&
      !step1Schema.safeParse({ name: w.name, legal_name: w.legal_name, tax_id: w.tax_id, phone: w.phone }).success) ||
    (step === 2 && !addressSchema.safeParse(w.address).success);

  const handleNext = async () => {
    if (step === 1) {
      if (await form.trigger(["name", "legal_name", "tax_id", "phone"])) setStep(2);
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
    <Form {...form}>
      <div className="min-h-screen bg-muted flex flex-col font-sans">
        <div className="flex items-center justify-between px-6 lg:px-10 py-5 bg-card border-b border-border">
          <div className="flex items-center gap-2 text-foreground">
            <img src={suiteeLogo} alt="suitee Logo" className="h-6 object-contain" />
          </div>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={handleSair}
            loading={isLoggingOut}
            loadingText="Saindo..."
            className="font-bold text-muted-foreground hover:text-foreground"
          >
            Sair
          </Button>
        </div>

        <div className="flex-1 flex flex-col items-center justify-center p-6 sm:p-10">
          <div className="w-full max-w-xl">
            <div className="mb-10 flex items-center justify-center">
              <div className="flex items-center gap-2">
                <div className={clsx("w-8 h-2 rounded-full transition-colors", step >= 1 ? "bg-primary-600" : "bg-muted")} />
                <div className={clsx("w-8 h-2 rounded-full transition-colors", step >= 2 ? "bg-primary-600" : "bg-muted")} />
                <div className={clsx("w-8 h-2 rounded-full transition-colors", step >= 3 ? "bg-primary-600" : "bg-muted")} />
              </div>
            </div>

            <motion.div
              key={step}
              initial={{ opacity: 0, y: 20 }}
              animate={{ opacity: 1, y: 0 }}
              className="bg-card rounded-3xl border border-border p-8 sm:p-12 shadow-xl shadow-slate-200/40"
            >
              {error && (
                <div className="mb-6 p-3 bg-status-critical-soft border border-status-critical/30 text-status-critical text-sm rounded-lg flex items-center gap-2">
                  <AlertCircle className="w-4 h-4 shrink-0" />
                  {error}
                </div>
              )}

              {step === 1 && (
                <div className="space-y-6">
                  <div className="text-center mb-8">
                    <div className="w-16 h-16 rounded-full flex items-center justify-center mx-auto mb-6">
                      <CompanyLogo className="w-10 h-10 object-contain opacity-80" />
                    </div>
                    <h1 className="text-2xl font-bold text-foreground mb-2">Bem-vindo(a) ao suitee!</h1>
                    <p className="text-muted-foreground">Para começarmos, qual o nome da sua imobiliária?</p>
                  </div>

                  <TextField
                    control={form.control}
                    name="name"
                    label="Nome Fantasia"
                    required
                    placeholder="Ex: Lumière Imóveis"
                  />
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
                    <PhoneField control={form.control} name="phone" label="Telefone Comercial" required />
                  </div>
                </div>
              )}

              {step === 2 && (
                <div className="space-y-6">
                  <div className="text-center mb-8">
                    <div className="w-16 h-16 bg-primary-50 text-primary-600 rounded-full flex items-center justify-center mx-auto mb-6">
                      <MapPin className="w-8 h-8" />
                    </div>
                    <h1 className="text-2xl font-bold text-foreground mb-2">Endereço Comercial</h1>
                    <p className="text-muted-foreground">Onde fica a matriz da sua operação?</p>
                  </div>

                  <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
                    <CepField control={form.control} name="address.zip_code" label="CEP" required className="col-span-1" />
                    <TextField
                      control={form.control}
                      name="address.street"
                      label="Rua / Logradouro"
                      required
                      className="col-span-2"
                    />
                  </div>

                  <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
                    <TextField
                      control={form.control}
                      name="address.number"
                      label="Número"
                      required
                      className="col-span-1"
                    />
                    <TextField
                      control={form.control}
                      name="address.complement"
                      label="Complemento"
                      className="col-span-2"
                    />
                  </div>

                  <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
                    <TextField control={form.control} name="address.district" label="Bairro" required className="col-span-1" />
                    <TextField control={form.control} name="address.city" label="Cidade" required className="col-span-1" />
                    <SelectField
                      control={form.control}
                      name="address.state"
                      label="Estado"
                      required
                      placeholder="UF"
                      options={UF_OPTIONS}
                      className="col-span-1"
                    />
                  </div>
                </div>
              )}

              {step === 3 && (
                <div className="space-y-6">
                  <div className="text-center mb-8">
                    <div className="w-16 h-16 bg-primary-50 text-primary-600 rounded-full flex items-center justify-center mx-auto mb-6">
                      <Users className="w-8 h-8" />
                    </div>
                    <h1 className="text-2xl font-bold text-foreground mb-2">Tamanho da Equipe</h1>
                    <p className="text-muted-foreground">Quantas pessoas usarão o sistema?</p>
                  </div>

                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                    {SIZE_OPTIONS.map((size) => (
                      <button
                        key={size}
                        type="button"
                        onClick={() => form.setValue("size", size)}
                        className={clsx(
                          "py-4 rounded-xl border text-sm font-bold transition-all",
                          w.size === size
                            ? "bg-primary-50 border-primary-600 text-primary-700 shadow-sm"
                            : "bg-card border-border text-muted-foreground hover:border-border",
                        )}
                      >
                        {size}
                      </button>
                    ))}
                  </div>

                  <div className="mt-8 bg-status-ok-soft text-status-ok p-4 rounded-xl flex items-start gap-3">
                    <CheckCircle className="w-5 h-5 shrink-0 mt-0.5 text-status-ok" />
                    <div>
                      <h4 className="text-sm font-bold">Pronto para decolar!</h4>
                      <p className="text-xs mt-1 text-status-ok">Ao finalizar, criaremos seu ambiente seguro e isolado para sua equipe.</p>
                    </div>
                  </div>
                </div>
              )}

              <div className="flex gap-3 mt-10">
                {step > 1 && (
                  <Button
                    type="button"
                    variant="outline"
                    size="icon"
                    aria-label="Voltar ao passo anterior"
                    onClick={() => setStep(step - 1)}
                    disabled={isLoading}
                    className="h-14 w-14 shrink-0"
                  >
                    <ArrowLeft className="w-5 h-5" />
                  </Button>
                )}
                <Button
                  type="button"
                  size="lg"
                  aria-label={step === 3 ? "Finalizar e Acessar CRM" : "Próximo Passo"}
                  onClick={handleNext}
                  loading={isLoading}
                  disabled={nextDisabled}
                  className="flex-1 h-14 text-base group"
                >
                  {step === 3 ? "Finalizar e Acessar CRM" : "Próximo Passo"}
                  {step < 3 && <ArrowRight className="w-5 h-5 group-hover:translate-x-1 transition-transform" />}
                </Button>
              </div>
            </motion.div>
          </div>
        </div>
      </div>
    </Form>
  );
}
