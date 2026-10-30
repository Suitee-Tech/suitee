import { useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import * as z from "zod";
import { useLocation } from "react-router";
import { User, Mail, Phone, Shield, MapPin, Bell, Landmark, KeyRound } from "lucide-react";
import { toast } from "sonner";
import { Button } from "../../../shared/ui/button";
import portalApi from "../services/portal-api";
import { MarketingConsentCard } from "../components/MarketingConsentCard";
import { LGPDDataExportCard } from "../components/LGPDDataExportCard";
import { DeleteAccountCard } from "../components/DeleteAccountCard";
import { formatCep, formatPhoneBR, parseCep, parsePhoneBR } from "../../../shared/utils/masks";
import { Form } from "../../../shared/ui/form";
import {
  CepField,
  CheckboxField,
  DocumentField,
  PhoneField,
  SelectField,
  TextField,
} from "../../../shared/ui/fields";
import { usePortalAuth } from "../context/PortalAuthContext";
import { PortalPage } from "../shell/PortalPage";
import { PortalPageHeader } from "../shell/PortalPageHeader";

const PIX_KEY_TYPES = [
  { value: "cpf", label: "CPF" },
  { value: "cnpj", label: "CNPJ" },
  { value: "email", label: "E-mail" },
  { value: "phone", label: "Telefone" },
  { value: "evp", label: "Chave aleatória" },
];
const ACCOUNT_TYPES = [
  { value: "checking", label: "Corrente" },
  { value: "savings", label: "Poupança" },
];

const personNameRule = z
  .string()
  .min(2, "Mínimo 2 caracteres")
  .refine((v) => !/\d/.test(v), { message: "Não pode conter números" });

const bankingSchema = z
  .object({
    bank_code: z.string().optional(),
    bank_name: z.string().optional(),
    agency: z.string().optional(),
    agency_digit: z.string().optional(),
    account: z.string().optional(),
    account_digit: z.string().optional(),
    account_type: z.enum(["checking", "savings"]).optional().or(z.literal("")),
    pix_key_type: z.enum(["cpf", "cnpj", "email", "phone", "evp"]).optional().or(z.literal("")),
    pix_key: z.string().optional(),
    holder_name: z.string().optional(),
    holder_document: z.string().optional(),
  })
  .optional();

const profileSchema = z.object({
  firstname: personNameRule,
  lastname: personNameRule,
  phone: z.string().optional(),
  address: z
    .object({
      street: z.string().optional(),
      number: z.string().optional(),
      complement: z.string().optional(),
      district: z.string().optional(),
      city: z.string().optional(),
      state: z.string().optional(),
      zip_code: z.string().optional(),
    })
    .optional(),
  notifications: z
    .object({
      email: z.boolean().optional(),
      sms: z.boolean().optional(),
      push: z.boolean().optional(),
    })
    .optional(),
  banking: bankingSchema,
});

type ProfileFormValues = z.infer<typeof profileSchema>;

export function PortalProfile() {
  const { user, refreshUserData } = usePortalAuth();
  const [isEditing, setIsEditing] = useState(false);
  const location = useLocation();
  // /portal/perfil?tab=seguranca (menu "Segurança e senha") abre o bloco abaixo.
  const isSecurityTab = useMemo(() => {
    try {
      return new URLSearchParams(location.search).get("tab") === "seguranca";
    } catch {
      return false;
    }
  }, [location.search]);
  const [passwordForm, setPasswordForm] = useState({ current: "", next: "", confirm: "" });
  const [passwordSaving, setPasswordSaving] = useState(false);

  const handleChangePassword = async () => {
    if (!passwordForm.current || !passwordForm.next) {
      toast.error("Preencha a senha atual e a nova senha.");
      return;
    }
    if (passwordForm.next.length < 8) {
      toast.error("A nova senha deve ter pelo menos 8 caracteres.");
      return;
    }
    if (passwordForm.next !== passwordForm.confirm) {
      toast.error("A confirmação não confere com a nova senha.");
      return;
    }
    setPasswordSaving(true);
    try {
      await portalApi.post("/settings/change-password", {
        current_password: passwordForm.current,
        new_password: passwordForm.next,
      });
      toast.success("Senha alterada com sucesso!");
      setPasswordForm({ current: "", next: "", confirm: "" });
    } catch (err) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message || "";
      if (/credential not found/i.test(msg)) {
        toast.error(
          "Sua conta entrou por código de verificação e ainda não tem senha. Use \"Esqueci minha senha\" no login para definir uma.",
        );
      } else {
        toast.error(msg || "Não foi possível alterar a senha. Confira a senha atual.");
      }
    } finally {
      setPasswordSaving(false);
    }
  };

  const defaultValues: ProfileFormValues = {
    firstname: user?.firstname || "",
    lastname: user?.lastname || "",
    phone: parsePhoneBR(user?.phone || ""),
    address: {
      street: user?.address?.street || "",
      number: user?.address?.number || "",
      complement: user?.address?.complement || "",
      district: user?.address?.district || "",
      city: user?.address?.city || "",
      state: user?.address?.state || "",
      zip_code: parseCep(user?.address?.zip_code || ""),
    },
    notifications: {
      email: user?.notifications?.email ?? true,
      sms: user?.notifications?.sms ?? false,
      push: user?.notifications?.push ?? false,
    },
    banking: {
      bank_code: user?.banking?.bank_code || "",
      bank_name: user?.banking?.bank_name || "",
      agency: user?.banking?.agency || "",
      agency_digit: user?.banking?.agency_digit || "",
      account: user?.banking?.account || "",
      account_digit: user?.banking?.account_digit || "",
      account_type: user?.banking?.account_type || "",
      pix_key_type: user?.banking?.pix_key_type || "",
      pix_key: user?.banking?.pix_key || "",
      holder_name: user?.banking?.holder_name || "",
      holder_document: user?.banking?.holder_document || "",
    },
  };
  const form = useForm<ProfileFormValues>({
    resolver: zodResolver(profileSchema),
    defaultValues,
  });
  const pixKeyType = form.watch("banking.pix_key_type");
  const pixPlaceholder =
    pixKeyType === "email"
      ? "voce@email.com"
      : pixKeyType === "phone"
        ? "+5511999998888"
        : pixKeyType === "evp"
          ? "UUID gerado pelo banco"
          : "";

  const onSubmit = async (data: ProfileFormValues) => {
    try {
      const cleanedBanking = data.banking
        ? Object.fromEntries(Object.entries(data.banking).filter(([, v]) => v !== "" && v !== undefined && v !== null))
        : undefined;
      const payload: Record<string, unknown> = { ...data };
      if (cleanedBanking && Object.keys(cleanedBanking).length === 0) {
        delete payload.banking;
      } else {
        payload.banking = cleanedBanking;
      }
      await portalApi.patch("/auth/me", payload);
      toast.success("Perfil atualizado!");
      await refreshUserData();
      setIsEditing(false);
    } catch (err) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message;
      toast.error(msg || "Não foi possível atualizar o perfil.");
    }
  };

  return (
    <PortalPage className="px-5">
      <PortalPageHeader title="Perfil" className="-mx-5" />
      <h1 className="text-section font-bold text-foreground tracking-tight mb-6">Dados pessoais</h1>

      {isEditing ? (
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(onSubmit)}
            className="bg-card border border-border rounded-3xl p-6 shadow-sm mb-6 flex flex-col gap-4"
          >
            <div className="grid grid-cols-2 gap-4">
              <TextField control={form.control} name="firstname" label="Nome" required />
              <TextField control={form.control} name="lastname" label="Sobrenome" required />
            </div>
            <PhoneField control={form.control} name="phone" label="Telefone" />

            <div className="border-t pt-4 mt-2">
              <h3 className="text-sm font-bold text-foreground mb-3">Endereço</h3>
              <div className="grid grid-cols-3 gap-3 mb-3">
                <TextField control={form.control} name="address.street" label="Rua" className="col-span-2" />
                <TextField control={form.control} name="address.number" label="Número" />
              </div>
              <div className="grid grid-cols-2 gap-3 mb-3">
                <TextField control={form.control} name="address.complement" label="Complemento" />
                <TextField control={form.control} name="address.district" label="Bairro" />
              </div>
              <div className="grid grid-cols-3 gap-3">
                <TextField control={form.control} name="address.city" label="Cidade" className="col-span-2" />
                <TextField
                  control={form.control}
                  name="address.state"
                  label="UF"
                  inputProps={{ maxLength: 2 }}
                />
              </div>
              <div className="mt-3">
                <CepField control={form.control} name="address.zip_code" label="CEP" />
              </div>
            </div>

            <div className="border-t pt-4 mt-2">
              <h3 className="text-sm font-bold text-foreground mb-3">Notificações</h3>
              <div className="flex flex-col gap-2">
                <CheckboxField control={form.control} name="notifications.email" label="Receber por E-mail" />
                <CheckboxField control={form.control} name="notifications.sms" label="Receber por SMS" />
                <CheckboxField control={form.control} name="notifications.push" label="Receber Notificações Push" />
              </div>
            </div>

            <div className="border-t pt-4 mt-2">
              <h3 className="text-sm font-bold text-foreground mb-1">Dados bancários para repasses</h3>
              <p className="text-xs text-muted-foreground mb-3">
                Preencha PIX (mais rápido) ou conta bancária. Você pode informar os dois — usaremos PIX como preferência
                quando disponível.
              </p>

              <div className="mb-4">
                <h4 className="text-xs font-bold text-foreground/80 mb-2">Chave PIX</h4>
                <div className="grid grid-cols-2 gap-3">
                  <SelectField
                    control={form.control}
                    name="banking.pix_key_type"
                    label="Tipo"
                    placeholder="Selecione…"
                    options={PIX_KEY_TYPES}
                  />
                  {pixKeyType === "cpf" || pixKeyType === "cnpj" ? (
                    <DocumentField control={form.control} name="banking.pix_key" label="Chave" mode={pixKeyType} />
                  ) : (
                    <TextField
                      control={form.control}
                      name="banking.pix_key"
                      label="Chave"
                      type={pixKeyType === "email" ? "email" : "text"}
                      placeholder={pixPlaceholder}
                    />
                  )}
                </div>
              </div>

              <div className="mb-4">
                <h4 className="text-xs font-bold text-foreground/80 mb-2">Conta bancária</h4>
                <div className="grid grid-cols-3 gap-3 mb-3">
                  <TextField control={form.control} name="banking.bank_code" label="Banco (código)" placeholder="341" />
                  <TextField
                    control={form.control}
                    name="banking.bank_name"
                    label="Banco (nome)"
                    placeholder="Itaú"
                    className="col-span-2"
                  />
                </div>
                <div className="grid grid-cols-4 gap-3 mb-3">
                  <TextField
                    control={form.control}
                    name="banking.agency"
                    label="Agência"
                    className="col-span-2"
                    inputProps={{ inputMode: "numeric" }}
                  />
                  <TextField
                    control={form.control}
                    name="banking.agency_digit"
                    label="Dígito"
                    inputProps={{ inputMode: "numeric", maxLength: 2 }}
                  />
                  <SelectField
                    control={form.control}
                    name="banking.account_type"
                    label="Tipo"
                    placeholder="--"
                    options={ACCOUNT_TYPES}
                  />
                </div>
                <div className="grid grid-cols-4 gap-3">
                  <TextField
                    control={form.control}
                    name="banking.account"
                    label="Conta"
                    className="col-span-3"
                    inputProps={{ inputMode: "numeric" }}
                  />
                  <TextField
                    control={form.control}
                    name="banking.account_digit"
                    label="Dígito"
                    inputProps={{ inputMode: "numeric", maxLength: 2 }}
                  />
                </div>
              </div>

              <div className="mb-2">
                <h4 className="text-xs font-bold text-foreground/80 mb-2">Titular da conta</h4>
                <p className="text-[11px] text-muted-foreground mb-2">
                  Deixe em branco para usar seus dados de cadastro. Preencha apenas se a conta for de outra pessoa ou
                  empresa.
                </p>
                <div className="grid grid-cols-2 gap-3">
                  <TextField control={form.control} name="banking.holder_name" label="Nome do titular" />
                  <DocumentField control={form.control} name="banking.holder_document" label="CPF/CNPJ" mode="auto" />
                </div>
              </div>
            </div>

            <div className="flex gap-2 mt-4">
              <Button type="button" variant="outline" onClick={() => setIsEditing(false)}>
                Cancelar
              </Button>
              <Button type="submit" disabled={form.formState.isSubmitting}>
                Salvar
              </Button>
            </div>
          </form>
        </Form>
      ) : (
        <>
          <div className="bg-card border border-border rounded-3xl p-6 shadow-sm mb-6">
            <div className="flex flex-col gap-6">
              <div className="flex items-center gap-4">
                <div className="w-12 h-12 rounded-full bg-status-ok-soft flex items-center justify-center text-status-ok">
                  <User className="w-6 h-6" />
                </div>
                <div>
                  <p className="text-sm text-muted-foreground font-medium">Nome completo</p>
                  <p className="text-base font-bold text-foreground">{user?.name || "Não informado"}</p>
                </div>
              </div>

              <div className="flex items-center gap-4">
                <div className="w-12 h-12 rounded-full bg-muted flex items-center justify-center text-muted-foreground">
                  <Mail className="w-6 h-6" />
                </div>
                <div>
                  <p className="text-sm text-muted-foreground font-medium">E-mail</p>
                  <p className="text-base font-bold text-foreground">{user?.email || "Não informado"}</p>
                </div>
              </div>

              <div className="flex items-center gap-4">
                <div className="w-12 h-12 rounded-full bg-muted flex items-center justify-center text-muted-foreground">
                  <Phone className="w-6 h-6" />
                </div>
                <div>
                  <p className="text-sm text-muted-foreground font-medium">Telefone</p>
                  <p className="text-base font-bold text-foreground">{user?.phone ? formatPhoneBR(user.phone) : "Não informado"}</p>
                </div>
              </div>

              <div className="flex items-center gap-4">
                <div className="w-12 h-12 rounded-full bg-muted flex items-center justify-center text-muted-foreground">
                  <Shield className="w-6 h-6" />
                </div>
                <div>
                  <p className="text-sm text-muted-foreground font-medium">Perfil</p>
                  <p className="text-base font-bold text-foreground capitalize">{user?.role || "Cliente"}</p>
                </div>
              </div>

              {user?.address?.street && (
                <div className="flex items-center gap-4">
                  <div className="w-12 h-12 rounded-full bg-muted flex items-center justify-center text-muted-foreground">
                    <MapPin className="w-6 h-6" />
                  </div>
                  <div>
                    <p className="text-sm text-muted-foreground font-medium">Endereço</p>
                    <p className="text-base font-bold text-foreground">
                      {user.address.street}, {user.address.number}
                    </p>
                    <p className="text-sm text-muted-foreground">
                      {user.address.district}, {user.address.city} - {user.address.state}
                    </p>
                    {user.address.zip_code && <p className="text-sm text-muted-foreground">CEP {formatCep(user.address.zip_code)}</p>}
                  </div>
                </div>
              )}

              <div className="flex items-center gap-4">
                <div className="w-12 h-12 rounded-full bg-muted flex items-center justify-center text-muted-foreground">
                  <Bell className="w-6 h-6" />
                </div>
                <div>
                  <p className="text-sm text-muted-foreground font-medium">Notificações ativas</p>
                  <p className="text-sm font-medium text-foreground mt-1 flex gap-2">
                    {user?.notifications?.email && <span className="bg-muted px-2 py-1 rounded text-xs">E-mail</span>}
                    {user?.notifications?.sms && <span className="bg-muted px-2 py-1 rounded text-xs">SMS</span>}
                    {user?.notifications?.push && <span className="bg-muted px-2 py-1 rounded text-xs">Push</span>}
                    {!user?.notifications?.email && !user?.notifications?.sms && !user?.notifications?.push && (
                      <span className="text-muted-foreground/70">Nenhuma</span>
                    )}
                  </p>
                </div>
              </div>

              <div className="flex items-center gap-4">
                <div className="w-12 h-12 rounded-full bg-muted flex items-center justify-center text-muted-foreground">
                  <Landmark className="w-6 h-6" />
                </div>
                <div>
                  <p className="text-sm text-muted-foreground font-medium">Dados bancários</p>
                  {user?.banking && (user.banking.pix_key || user.banking.account) ? (
                    <div className="text-sm text-foreground mt-1">
                      {user.banking.pix_key && (
                        <p className="font-medium">
                          PIX {user.banking.pix_key_type?.toUpperCase()}:{" "}
                          <span className="font-mono">••••{user.banking.pix_key.slice(-4)}</span>
                        </p>
                      )}
                      {user.banking.account && (
                        <p className="text-xs text-muted-foreground">
                          {user.banking.bank_name || user.banking.bank_code} · Ag {user.banking.agency} · Cc{" "}
                          <span className="font-mono">••••{user.banking.account.slice(-4)}</span>
                        </p>
                      )}
                    </div>
                  ) : (
                    <p className="text-sm text-muted-foreground/70 mt-1">Não informados</p>
                  )}
                </div>
              </div>
            </div>
          </div>

          <MarketingConsentCard />
          <LGPDDataExportCard />

          {isSecurityTab && (
            <div className="bg-card border border-border rounded-3xl p-5 shadow-sm">
              <div className="flex items-center gap-3 mb-1">
                <div className="w-10 h-10 rounded-full bg-muted flex items-center justify-center text-muted-foreground">
                  <KeyRound className="w-5 h-5" />
                </div>
                <div>
                  <p className="text-title font-bold text-foreground">Segurança e senha</p>
                  <p className="text-label text-muted-foreground">Altere a senha de acesso à sua conta.</p>
                </div>
              </div>
              <div className="mt-4 space-y-3">
                <div>
                  <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Senha atual</label>
                  <input
                    type="password"
                    value={passwordForm.current}
                    onChange={(e) => setPasswordForm((p) => ({ ...p, current: e.target.value }))}
                    autoComplete="current-password"
                    placeholder="••••••••"
                    className="mt-1 w-full px-3 py-2.5 bg-background border border-border rounded-xl text-sm"
                  />
                </div>
                <div>
                  <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Nova senha</label>
                  <input
                    type="password"
                    value={passwordForm.next}
                    onChange={(e) => setPasswordForm((p) => ({ ...p, next: e.target.value }))}
                    autoComplete="new-password"
                    placeholder="Mínimo 8 caracteres"
                    className="mt-1 w-full px-3 py-2.5 bg-background border border-border rounded-xl text-sm"
                  />
                </div>
                <div>
                  <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Confirmar nova senha</label>
                  <input
                    type="password"
                    value={passwordForm.confirm}
                    onChange={(e) => setPasswordForm((p) => ({ ...p, confirm: e.target.value }))}
                    autoComplete="new-password"
                    placeholder="Repita a nova senha"
                    className="mt-1 w-full px-3 py-2.5 bg-background border border-border rounded-xl text-sm"
                  />
                </div>
                <Button onClick={handleChangePassword} disabled={passwordSaving} className="w-full h-11 rounded-xl text-sm font-bold">
                  {passwordSaving ? "Salvando..." : "Alterar senha"}
                </Button>
              </div>
            </div>
          )}

          <Button
            onClick={() => {
              form.reset(defaultValues);
              setIsEditing(true);
            }}
            className="w-full h-12 rounded-xl text-base font-bold"
          >
            Editar dados
          </Button>

          <div className="mt-8">
            <DeleteAccountCard />
          </div>
        </>
      )}
    </PortalPage>
  );
}
