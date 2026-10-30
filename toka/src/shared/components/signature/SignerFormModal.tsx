import { useEffect, useState } from "react";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogTitle } from "../../ui/dialog";
import { Button } from "../../ui/button";

export type SignerFormValues = {
  name: string;
  email: string;
  phone_number?: string;
  documentation?: string;
  birthday?: string;
  role?: string;
  auth?: string[];
};

type Mode = "edit" | "add";

export function SignerFormModal({
  mode,
  open,
  onClose,
  onSubmit,
  initial,
}: {
  mode: Mode;
  open: boolean;
  onClose: () => void;
  onSubmit: (values: SignerFormValues) => Promise<void>;
  initial?: Partial<SignerFormValues>;
}) {
  const [values, setValues] = useState<SignerFormValues>({
    name: "",
    email: "",
    phone_number: "",
    documentation: "",
    birthday: "",
    role: "comprador",
    auth: ["email"],
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setValues({
        name: initial?.name ?? "",
        email: initial?.email ?? "",
        phone_number: initial?.phone_number ?? "",
        documentation: initial?.documentation ?? "",
        birthday: initial?.birthday ?? "",
        role: initial?.role ?? "comprador",
        auth: initial?.auth ?? ["email"],
      });
      setError(null);
    }
  }, [open, initial?.name, initial?.email, initial?.phone_number, initial?.documentation, initial?.birthday, initial?.role]);

  if (!open) return null;

  const handleSubmit = async () => {
    if (busy) return;
    setError(null);
    setBusy(true);
    try {
      await onSubmit(values);
      onClose();
    } catch (e: any) {
      const msg = e?.response?.data?.message ?? e?.message ?? "Não foi possível salvar.";
      setError(String(msg));
    } finally {
      setBusy(false);
    }
  };

  const set = <K extends keyof SignerFormValues>(k: K, v: SignerFormValues[K]) => setValues((prev) => ({ ...prev, [k]: v }));

  return (
    <Dialog
      open
      onOpenChange={(next) => {
        if (!next && !busy) onClose();
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogTitle className="font-bold text-foreground">{mode === "edit" ? "Editar signatário" : "Adicionar signatário"}</DialogTitle>
        <DialogDescription className="sr-only">Formulário de dados do signatário</DialogDescription>

        <div className="space-y-3">
          <Field label="Nome completo" required>
            <input
              type="text"
              value={values.name}
              onChange={(e) => set("name", e.target.value)}
              placeholder="Ex: João da Silva"
              className="w-full px-3 py-2 bg-card border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary"
            />
          </Field>
          <Field label="E-mail" required>
            <input
              type="email"
              value={values.email}
              onChange={(e) => set("email", e.target.value)}
              placeholder="email@exemplo.com"
              className="w-full px-3 py-2 bg-card border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary"
            />
          </Field>
          <Field label="Telefone (opcional)">
            <input
              type="tel"
              value={values.phone_number ?? ""}
              onChange={(e) => set("phone_number", e.target.value)}
              placeholder="(11) 99999-9999"
              className="w-full px-3 py-2 bg-card border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary"
            />
          </Field>
          <Field label="CPF/CNPJ (opcional)">
            <input
              type="text"
              value={values.documentation ?? ""}
              onChange={(e) => set("documentation", e.target.value)}
              placeholder="000.000.000-00"
              className="w-full px-3 py-2 bg-card border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary"
            />
          </Field>
          <Field label="Nascimento (opcional)">
            <input
              type="date"
              value={values.birthday ?? ""}
              onChange={(e) => set("birthday", e.target.value)}
              className="w-full px-3 py-2 bg-card border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary"
            />
          </Field>
          {mode === "add" && (
            <>
              <Field label="Papel">
                <select
                  value={values.role ?? "comprador"}
                  onChange={(e) => set("role", e.target.value)}
                  className="w-full px-3 py-2 bg-card border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                >
                  <option value="comprador">Comprador</option>
                  <option value="vendedor">Vendedor</option>
                  <option value="inquilino">Inquilino</option>
                  <option value="proprietario">Proprietário</option>
                  <option value="fiador">Fiador</option>
                  <option value="avalista">Avalista</option>
                  <option value="testemunha">Testemunha</option>
                  <option value="locador">Locador</option>
                  <option value="locatario">Locatário</option>
                </select>
              </Field>
              <Field label="Autenticação">
                <select
                  value={(values.auth ?? ["email"])[0] ?? "email"}
                  onChange={(e) => set("auth", [e.target.value])}
                  className="w-full px-3 py-2 bg-card border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                >
                  <option value="email">Token por e-mail</option>
                  <option value="sms">Token por SMS</option>
                  <option value="whatsapp">Token por WhatsApp</option>
                  <option value="selfie">Selfie</option>
                  <option value="icp_brasil">ICP-Brasil</option>
                </select>
              </Field>
            </>
          )}
        </div>

        {error && (
          <div className="text-xs font-bold text-status-critical bg-status-critical-soft border border-status-critical/30 rounded-lg p-2">
            {error}
          </div>
        )}

        <DialogFooter>
          <Button variant="ghost" onClick={onClose} disabled={busy}>
            Cancelar
          </Button>
          <Button onClick={handleSubmit} disabled={!values.name || !values.email} loading={busy}>
            {mode === "edit" ? "Salvar" : "Adicionar"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Field({ label, required, children }: { label: string; required?: boolean; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="text-xs font-bold text-muted-foreground mb-1 block">
        {label}
        {required && <span className="text-status-critical"> *</span>}
      </span>
      {children}
    </label>
  );
}

