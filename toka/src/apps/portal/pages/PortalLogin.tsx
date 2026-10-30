import React, { useState, useEffect } from "react";
import { ShieldCheck, Mail, Smartphone, ArrowRight } from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";
import { usePortalAuth } from "../context/PortalAuthContext";
import { useLocation, useNavigate } from "react-router";
import { Input } from "../../../shared/ui/input";
import { Button } from "../../../shared/ui/button";
import { CompanyLogo } from "../../../shared/components/CompanyLogo";

type VerificationMethod = "email" | "whatsapp";
type PortalStep = "identify" | "verify";

export function PortalLogin() {
  const { isAuthenticated, requestOtp, verifyOtp } = usePortalAuth();
  const location = useLocation();
  const navigate = useNavigate();

  const MAX_CODE_LENGTH = 8;
  const RESEND_COOLDOWN_SECONDS = 30;
  const [step, setStep] = useState<PortalStep>("identify");
  const [method, setMethod] = useState<VerificationMethod>("email");
  const [contact, setContact] = useState("");
  const [code, setCode] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const isCodeComplete = /^(?:\d{6}|\d{8})$/.test(code);
  const [resendCooldown, setResendCooldown] = useState(0);

  useEffect(() => {
    const searchParams = new URLSearchParams(window.location.search);
    const emailParam = searchParams.get("email");
    if (emailParam) {
      setContact(emailParam);
      setMethod("email");
    }
  }, []);

  useEffect(() => {
    if (isAuthenticated) {
      const from = (location.state as any)?.from?.pathname || "/portal";
      navigate(from, { replace: true });
    }
  }, [isAuthenticated, location.state, navigate]);

  const handleSendCode = async () => {
    if (!contact.trim()) {
      toast.error("Informe seu e-mail ou telefone.");
      return;
    }

    setIsSubmitting(true);
    try {
      await requestOtp(contact, method);
      const via = method === "email" ? "e-mail" : "SMS/WhatsApp";
      const localMail = typeof window !== "undefined"
        && (window.location.hostname === "localhost" || window.location.hostname === "127.0.0.1");
      toast.success(`Código de verificação enviado via ${via} para ${contact}`, localMail ? {
        description: "No ambiente local, o código fica no Mailpit (não vai para a caixa real).",
        action: {
          label: "Abrir Mailpit",
          onClick: () => window.open("http://localhost:54324", "_blank", "noopener,noreferrer"),
        },
      } : undefined);
      setStep("verify");
      setCode("");
      setResendCooldown(RESEND_COOLDOWN_SECONDS);
    } catch (error: any) {
      const message = error.response?.data?.message || error.response?.data?.error || error.message || "Erro ao solicitar o código.";
      toast.error(message);
    } finally {
      setIsSubmitting(false);
    }
  };

  const submitCode = async (fullCode: string) => {
    if (isSubmitting) return;
    if (!/^(?:\d{6}|\d{8})$/.test(fullCode)) {
      toast.error("Informe os 6 ou 8 dígitos do código recebido.");
      return;
    }
    setIsSubmitting(true);
    try {
      await verifyOtp(contact, fullCode);
      toast.success("Identidade verificada com sucesso!");
    } catch (error: any) {
      const message = error.response?.data?.message || error.response?.data?.error || error.message || "Código inválido ou expirado.";
      toast.error(message);
      setCode("");
    } finally {
      setIsSubmitting(false);
    }
  };

  useEffect(() => {
    if (step !== "verify") return;
    if (resendCooldown <= 0) return;
    const id = window.setInterval(() => {
      setResendCooldown((s) => (s <= 1 ? 0 : s - 1));
    }, 1000);
    return () => window.clearInterval(id);
  }, [resendCooldown, step]);

  const handleBack = () => {
    setStep("identify");
    setCode("");
  };

  return (
    <div className="min-h-screen flex items-center justify-center px-4 py-12 bg-gradient-to-br from-muted via-background to-muted font-sans">
      <div className="w-full max-w-md">
        <div className="text-center mb-8">
          <div className="inline-flex items-center gap-2 mb-2">
            <div className="w-10 h-10 rounded-lg flex items-center justify-center">
              <CompanyLogo className="w-8 h-8 object-contain" />
            </div>
            <span className="text-2xl font-bold text-foreground tracking-tight">suitee</span>
          </div>
          <p className="text-sm text-muted-foreground">Portal Seguro do Cliente</p>
        </div>

        <div className="bg-card rounded-2xl shadow-xl border border-border overflow-hidden">
          <div className="bg-primary px-6 py-4 text-primary-foreground">
            <div className="flex items-center gap-3">
              <ShieldCheck className="w-6 h-6" />
              <div>
                <h2 className="font-bold text-lg">Acesso Seguro</h2>
                <p className="text-primary-200 text-sm">
                  {step === "identify" ? "Informe seus dados para receber o código" : "Digite o código de verificação"}
                </p>
              </div>
            </div>
          </div>

          <div className="p-6">
            {step === "identify" ? (
              <div className="space-y-5">
                <div className="flex bg-muted p-1 rounded-lg">
                  <button
                    onClick={() => setMethod("email")}
                    className={clsx(
                      "flex-1 flex items-center justify-center gap-2 px-3 py-2 rounded-md text-sm font-medium transition-all",
                      method === "email" ? "bg-card text-primary-700 shadow-sm" : "text-muted-foreground",
                    )}
                  >
                    <Mail className="w-4 h-4" /> E-mail
                  </button>
                  <button
                    onClick={() => setMethod("whatsapp")}
                    className={clsx(
                      "flex-1 flex items-center justify-center gap-2 px-3 py-2 rounded-md text-sm font-medium transition-all",
                      method === "whatsapp" ? "bg-card text-primary-700 shadow-sm" : "text-muted-foreground",
                    )}
                  >
                    <Smartphone className="w-4 h-4" /> WhatsApp
                  </button>
                </div>

                <div>
                  <label className="block text-sm font-medium text-foreground mb-1.5">
                    {method === "email" ? "Seu e-mail cadastrado" : "Seu telefone cadastrado"}
                  </label>
                  <input
                    type={method === "email" ? "email" : "tel"}
                    value={contact}
                    onChange={(e) => setContact(e.target.value)}
                    placeholder={method === "email" ? "Ex: carlos@email.com" : "Ex: (11) 98765-4321"}
                    className="w-full px-4 py-3 bg-muted border border-border rounded-xl text-foreground outline-none focus:border-primary-500 focus:ring-2 focus:ring-primary-500/20 transition-all"
                    onKeyDown={(e) => e.key === "Enter" && handleSendCode()}
                    disabled={isSubmitting}
                  />
                  {method === "email" && (
                    <p className="mt-2 text-xs text-muted-foreground">
                      Apenas clientes convidados pela imobiliária recebem acesso. Se você não recebeu o código, entre em contato.
                    </p>
                  )}
                </div>

                <Button onClick={handleSendCode} loading={isSubmitting} loadingText="Enviando..." size="lg" className="w-full">
                  Enviar Código <ArrowRight className="w-4 h-4" />
                </Button>

                <p className="text-xs text-muted-foreground text-center">Você receberá um código para verificar sua identidade.</p>

                <p className="text-[11px] text-muted-foreground text-center">
                  Antes de continuar, leia os{" "}
                  <a href="/portal/terms" className="underline text-primary-600 hover:text-primary-700">
                    Termos de Uso
                  </a>{" "}
                  e a{" "}
                  <a href="/portal/privacy" className="underline text-primary-600 hover:text-primary-700">
                    Política de Privacidade
                  </a>
                  .
                </p>
              </div>
            ) : (
              <div className="space-y-5">
                <p className="text-sm text-muted-foreground text-center">
                  Enviamos um código para <strong className="text-foreground">{contact}</strong>
                </p>
                <p className="text-xs text-muted-foreground text-center">
                  Não chegou em alguns minutos? Confira se este é o e-mail cadastrado no negócio — o código só é
                  enviado para clientes que a imobiliária já cadastrou.
                </p>

                <div className="space-y-2">
                  <label htmlFor="portal-verification-code" className="block text-sm font-medium text-foreground">
                    Código de verificação
                  </label>
                  <Input
                    id="portal-verification-code"
                    type="text"
                    maxLength={MAX_CODE_LENGTH}
                    value={code}
                    autoFocus
                    inputMode="numeric"
                    autoComplete="one-time-code"
                    aria-describedby="portal-verification-code-help"
                    onChange={(event) => setCode(event.target.value.replace(/\D/g, "").slice(0, MAX_CODE_LENGTH))}
                    onPaste={(event) => {
                      event.preventDefault();
                      setCode(event.clipboardData.getData("text").replace(/\D/g, "").slice(0, MAX_CODE_LENGTH));
                    }}
                    onKeyDown={(event) => {
                      if (event.key === "Enter") {
                        event.preventDefault();
                        if (isCodeComplete) void submitCode(code);
                      }
                    }}
                    disabled={isSubmitting}
                    className="h-14 rounded-xl bg-muted text-center font-mono text-xl tracking-widest"
                  />
                  <p id="portal-verification-code-help" className="text-xs text-muted-foreground">
                    Digite ou cole os 6 ou 8 dígitos do código recebido e clique em Verificar.
                  </p>
                </div>

                <Button
                  onClick={() => void submitCode(code)}
                  loading={isSubmitting}
                  loadingText="Verificando..."
                  disabled={!isCodeComplete}
                  size="lg"
                  className="w-full"
                >
                  Verificar
                </Button>

                <Button
                  variant="link"
                  size="sm"
                  onClick={handleSendCode}
                  disabled={isSubmitting || resendCooldown > 0}
                  className="w-full"
                >
                  {resendCooldown > 0 ? `Reenviar código em ${resendCooldown}s` : "Reenviar código"}
                </Button>

                <Button
                  variant="ghost"
                  size="sm"
                  onClick={handleBack}
                  disabled={isSubmitting}
                  className="w-full text-muted-foreground hover:text-foreground"
                >
                  Voltar
                </Button>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
