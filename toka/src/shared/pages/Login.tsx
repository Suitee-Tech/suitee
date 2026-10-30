import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router";
import { motion, AnimatePresence } from "motion/react";
import { z } from "zod";
import {
  ArrowRight,
  ArrowLeft,
  Mail,
  Lock,
  User,
  Phone,
  Briefcase,
  CheckCircle,
  CheckCircle2,
  Building2,
  ChevronRight,
  Eye,
  EyeOff,
  AlertCircle
} from "lucide-react";
import { clsx } from "clsx";
import { useTheme, DEFAULT_THEME } from "../context/ThemeContext";
import { useAuth } from "../context/AuthContext";
import { formatPhoneBR, parsePhoneBR } from "../utils/masks";
import { isAbortedError } from "../lib/retry";

import { CompanyLogo } from "../components/CompanyLogo";
import { resolveLoginBanner } from "../loginBanners";
import { Button } from "../ui/button";
// removed suiteeIcon

// Schemas de Validação
const loginSchema = z.object({
 email: z.string().email("E-mail inválido"),
 password: z.string().min(1, "Senha é obrigatória"),
});

const step1Schema = z.object({
 email: z.string().email("E-mail inválido"),
 phone: z.string().refine((v) => v.replace(/\D/g, "").length >= 10, "Telefone inválido (mínimo 10 dígitos)"),
});

const step2Schema = z.object({
 firstName: z.string().min(2, "Nome deve ter pelo menos 2 caracteres"),
 lastName: z.string().min(2, "Sobrenome deve ter pelo menos 2 caracteres"),
 role: z.string().min(1, "Cargo é obrigatório"),
});

const passwordSchema = z.string()
 .min(8, "A senha deve ter pelo menos 8 caracteres")
 .regex(/[A-Z]/, "A senha deve conter pelo menos uma letra maiúscula")
 .regex(/[a-z]/, "A senha deve conter pelo menos uma letra minúscula")
 .regex(/[0-9]/, "A senha deve conter pelo menos um número")
 .regex(/[^A-Za-z0-9]/, "A senha deve conter pelo menos um caractere especial");

const step3Schema = z.object({
 password: passwordSchema,
 confirmPassword: z.string(),
}).refine((data) => data.password === data.confirmPassword, {
 message: "As senhas não coincidem",
 path: ["confirmPassword"],
});

import { forgotPassword } from "../services/auth.service";

const forgotPasswordEmailSchema = z.string().email("E-mail inválido");

const ForgotPasswordForm = ({ onBack }: { onBack: () => void }) => {
  const [email, setEmail] = useState("");
  const [status, setStatus] = useState<"idle" | "loading" | "success" | "error">("idle");
  const [emailError, setEmailError] = useState("");
  const [errorMsg, setErrorMsg] = useState("");

  const submitDisabled = status === "loading" || !forgotPasswordEmailSchema.safeParse(email).success;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const parsed = forgotPasswordEmailSchema.safeParse(email);
    if (!parsed.success) {
      setEmailError(parsed.error.issues[0].message);
      return;
    }

    setStatus("loading");
    setErrorMsg("");
    try {
      await forgotPassword(email);
      setStatus("success");
    } catch (err: any) {
      console.error(err);
      const httpStatus: number | undefined = err?.response?.status;
      const isNetworkError = !err?.response || isAbortedError(err);
      if (isNetworkError) {
        setErrorMsg('Sem conexão com o servidor. Verifique sua internet e tente novamente.');
      } else if (httpStatus && httpStatus >= 500) {
        setErrorMsg('Não conseguimos enviar o e-mail agora. Tente novamente em alguns instantes.');
      } else {
        setErrorMsg(err?.response?.data?.message || 'Não foi possível enviar o link. Tente novamente.');
      }
      setStatus("error");
    }
  };

  return (
    <motion.div
      key="forgot-password"
      initial={{ opacity: 0, x: 20 }}
      animate={{ opacity: 1, x: 0 }}
      exit={{ opacity: 0, x: 20 }}
      transition={{ duration: 0.2 }}
    >
      <div className="text-center mb-8">
        <h1 className="text-2xl font-bold text-foreground mb-2">Recuperar Senha</h1>
        <p className="text-sm text-muted-foreground">Informe seu e-mail e enviaremos um link para redefinir sua senha.</p>
      </div>

      {status === "success" ? (
        <div className="text-center space-y-6">
          <div className="w-16 h-16 bg-status-ok-soft text-status-ok rounded-full flex items-center justify-center mx-auto mb-4">
            <CheckCircle2 className="w-8 h-8" />
          </div>
          <h3 className="text-lg font-bold text-foreground">E-mail enviado!</h3>
          <p className="text-sm text-muted-foreground">
            Verifique sua caixa de entrada para o endereço <span className="font-semibold text-foreground">{email}</span> e siga as instruções.
          </p>
          <Button
            type="button"
            variant="outline"
            size="lg"
            onClick={onBack}
            className="w-full mt-6"
          >
            Voltar para o Login
          </Button>
        </div>
      ) : (
        <form onSubmit={handleSubmit} className="space-y-5" noValidate>
          {status === "error" && errorMsg && (
            <div className="p-3 bg-status-critical-soft border border-status-critical/30 text-status-critical text-sm rounded-lg flex items-center gap-2">
              <AlertCircle className="w-4 h-4 shrink-0" />
              {errorMsg}
            </div>
          )}

          <div>
            <label htmlFor="forgot-email" className="block text-xs font-bold text-foreground uppercase tracking-wider mb-2">E-mail da Conta</label>
            <div className="relative">
              <Mail className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-muted-foreground" />
              <input
                id="forgot-email"
                type="email"
                value={email}
                onChange={(e) => {
                  setEmail(e.target.value);
                  if (emailError) setEmailError("");
                }}
                onBlur={() => {
                  const parsed = forgotPasswordEmailSchema.safeParse(email);
                  if (!parsed.success) {
                    setEmailError(parsed.error.issues[0].message);
                  }
                }}
                placeholder="nome@imobiliaria.com.br"
                className={clsx(
                  "w-full pl-10 pr-4 py-3 bg-card border rounded-xl text-sm focus:ring-2 focus:ring-primary-500 focus:border-transparent outline-none transition-all placeholder:text-muted-foreground font-medium",
                  emailError ? "border-status-critical/40 focus:ring-status-critical/30" : "border-border"
                )}
              />
            </div>
            {emailError && <p className="mt-1 text-xs text-status-critical font-medium">{emailError}</p>}
          </div>

          <Button
            type="submit"
            size="lg"
            aria-label="Enviar Link de Recuperação"
            loading={status === "loading"}
            disabled={submitDisabled}
            className="w-full mt-2"
          >
            Enviar Link de Recuperação
          </Button>

          <Button
            type="button"
            variant="ghost"
            size="lg"
            onClick={onBack}
            className="w-full mt-2 text-muted-foreground hover:text-foreground"
          >
            Voltar para o Login
          </Button>
        </form>
      )}
    </motion.div>
  );
};

export function Login() {
  const navigate = useNavigate();
  const { login, register, isAuthenticated, isLoading: authLoading, user, companies } = useAuth();
  const { theme: themeSettings } = useTheme();
  // White label da tela de login. Esta tela é pré-autenticação, então não há
  // chamada ao servidor aqui: os valores vêm do tema persistido em localStorage
  // pela última sessão (ThemeContext), que é o mesmo caminho das cores.
  const loginTitle = themeSettings.loginTagline || DEFAULT_THEME.loginTagline;
  const loginSubtitle = themeSettings.loginSubtitle || DEFAULT_THEME.loginSubtitle;
  const loginBanner = resolveLoginBanner(
    themeSettings.loginBannerPreset,
    themeSettings.loginBannerUrl,
    themeSettings.loginBannerSource,
  );
  // `showLogoLogin` é o toggle "Exibir logotipo na tela de Login" e
  // `CompanyLogo` já cai no logo suitee quando não há logo (ou se o logo falhar).
  const loginLogoUrl = themeSettings.showLogoLogin ? themeSettings.logoUrl : undefined;
  const [isLogin, setIsLogin] = useState(true);
  const [isForgotPassword, setIsForgotPassword] = useState(false);
  const [error, setError] = useState("");
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
 const [isLoading, setIsLoading] = useState(false);

 useEffect(() => {
  try {
    const u = new URL(window.location.href);
    const hasRecoveryHash = u.hash.includes("type=recovery") && (u.hash.includes("access_token=") || u.hash.includes("refresh_token=") || u.hash.includes("token_hash="));
    const hasRecoveryCode = u.searchParams.has("code") || u.searchParams.get("type") === "recovery";
    if (hasRecoveryHash || hasRecoveryCode) {
      navigate({ pathname: "/reset-password", search: u.search, hash: u.hash }, { replace: true });
    }
  } catch {}
 }, [navigate]);

 useEffect(() => {
  if (authLoading) return;
  if (!isAuthenticated) return;
  if (!companies?.length) {
    navigate("/onboarding", { replace: true });
    return;
  }
  if (!user?.current_company_id) {
    navigate("/company-selection", { replace: true });
    return;
  }
  navigate("/", { replace: true });
 }, [authLoading, companies, isAuthenticated, navigate, user?.current_company_id]);

 // Login State
 const [loginEmail, setLoginEmail] = useState("");
 const [loginPassword, setLoginPassword] = useState("");
 const [showPassword, setShowPassword] = useState(false);

 // Register State
 const [step, setStep] = useState(1);
 const [regData, setRegData] = useState({
 email: "",
 phone: "",
 firstName: "",
 lastName: "",
 role: "Corretor",
 password: "",
 confirmPassword: ""
 });

 const loginDisabled = useMemo(() => {
   if (isLoading) return true;
   return !loginSchema.safeParse({ email: loginEmail, password: loginPassword }).success;
 }, [isLoading, loginEmail, loginPassword]);

 // Whether the "Continuar" button should be disabled. Gates Steps 1, 2 and 3 of the signup wizard.
 const continueDisabled = useMemo(() => {
   if (isLoading) return true;
   if (isLogin) return false;
   if (step === 1) {
     return !step1Schema.safeParse({ email: regData.email, phone: regData.phone }).success;
   }
   if (step === 2) {
     return !step2Schema.safeParse({
       firstName: regData.firstName,
       lastName: regData.lastName,
       role: regData.role,
     }).success;
   }
   if (step === 3) {
     return !step3Schema.safeParse({
       password: regData.password,
       confirmPassword: regData.confirmPassword,
     }).success;
   }
   return false;
 }, [isLoading, isLogin, step, regData.email, regData.phone, regData.firstName, regData.lastName, regData.role, regData.password, regData.confirmPassword]);

 const validateLogin = () => {
 try {
 loginSchema.parse({ email: loginEmail, password: loginPassword });
 setFieldErrors({});
 return true;
 } catch (err) {
 if (err instanceof z.ZodError) {
 const errors: Record<string, string> = {};
 err.issues.forEach((issue) => {
 if (issue.path[0]) errors[issue.path[0] as string] = issue.message;
 });
 setFieldErrors(errors);
 }
 return false;
 }
 };

 const validateStep = (currentStep: number) => {
 try {
 if (currentStep === 1) {
 step1Schema.parse({ email: regData.email, phone: regData.phone });
 } else if (currentStep === 2) {
 step2Schema.parse({
 firstName: regData.firstName,
 lastName: regData.lastName,
 role: regData.role
 });
 } else if (currentStep === 3) {
 step3Schema.parse({
 password: regData.password,
 confirmPassword: regData.confirmPassword
 });
 }
 setFieldErrors({});
 return true;
 } catch (err) {
 if (err instanceof z.ZodError) {
 const errors: Record<string, string> = {};
 err.issues.forEach((issue) => {
 if (issue.path[0]) errors[issue.path[0] as string] = issue.message;
 });
 setFieldErrors(errors);
 }
 return false;
 }
 };

 const handleLogin = async (e: React.FormEvent) => {
 e.preventDefault();
 if (!validateLogin()) return;

 setError("");
 setIsLoading(true);
 try {
 await login(loginEmail, loginPassword);
 navigate("/");
 } catch (err: any) {
   console.error(err);
   const status: number | undefined = err?.response?.status;
   const isNetworkError = !err?.response || isAbortedError(err);
   if (isNetworkError) {
     setError('Sem conexão com o servidor. Verifique sua internet e tente novamente.');
   } else if (status === 401 || status === 403) {
     setError('E-mail ou senha incorretos.');
   } else if (status && status >= 500) {
     setError('Não conseguimos te autenticar no momento. Tente novamente em alguns instantes.');
   } else {
     setError(err?.response?.data?.message || 'Não foi possível entrar. Verifique suas credenciais.');
   }
 } finally {
 setIsLoading(false);
 }
 };

 const handleRegister = async () => {
 // Validate all steps before submission just in case
 const isStep1Valid = step1Schema.safeParse({ email: regData.email, phone: regData.phone }).success;
 const isStep2Valid = step2Schema.safeParse({ firstName: regData.firstName, lastName: regData.lastName, role: regData.role }).success;
 const isStep3Valid = step3Schema.safeParse({ password: regData.password, confirmPassword: regData.confirmPassword }).success;

 if (!isStep1Valid || !isStep2Valid || !isStep3Valid) {
 setError("Por favor, corrija os erros nos passos anteriores.");
 return;
 }

 setError("");
 setIsLoading(true);
 try {
 await register({
 email: regData.email,
 password: regData.password,
 confirm_password: regData.confirmPassword,
 type: 'individual',
 firstname: regData.firstName,
 lastname: regData.lastName,
 phone: regData.phone,
 role: regData.role,
 });
 // After successful registration, redirect to onboarding or dashboard
 navigate("/onboarding");
 } catch (err: any) {
        console.error(err);
        const status: number | undefined = err.response?.status;
        const backendMessage: string | undefined = err.response?.data?.message;
        const isNetworkError = !err.response || isAbortedError(err);
        const bm = backendMessage ?? '';
        const bmLower = bm.toLowerCase();

        let message: string;

        if (status === 409) {
          // Conflito de cadastro: o backend responde genérico de propósito para
          // não confirmar qual identificador já existe. Ignoramos até a message
          // do backend no 409 para que uma resposta antiga/específica nunca
          // reintroduza o oráculo. Voltamos ao Passo 1 sem erro de campo.
          setFieldErrors({});
          setStep(1);
          message = 'Não foi possível concluir o cadastro com os dados informados.';
        } else if (bmLower.includes('password must')) {
          // Erro técnico de senha (em inglês) vindo do backend → leva ao Passo 3.
          setFieldErrors(prev => ({ ...prev, password: bm }));
          setStep(3);
          message = 'Sua senha não atende aos requisitos. Ajuste-a no Passo 3.';
        } else if (isNetworkError) {
          message = 'Sem conexão com o servidor. Verifique sua internet e tente novamente.';
        } else if (status && status >= 500) {
          message = 'Não conseguimos criar sua conta no momento. Tente novamente em alguns instantes.';
        } else if (backendMessage) {
          message = backendMessage;
        } else {
          message = 'Não foi possível concluir o cadastro. Verifique os dados e tente novamente.';
        }

        setError(message);
      } finally {
 setIsLoading(false);
 }
 };

 const updateRegData = (field: string, value: string) => {
 setRegData(prev => ({ ...prev, [field]: value }));
 if (fieldErrors[field]) {
 setFieldErrors(prev => {
 const newErrors = { ...prev };
 delete newErrors[field];
 return newErrors;
 });
 }
 };

 const updateLoginData = (field: 'email' | 'password', value: string) => {
 if (field === 'email') setLoginEmail(value);
 if (field === 'password') setLoginPassword(value);

 if (fieldErrors[field]) {
 setFieldErrors(prev => {
 const newErrors = { ...prev };
 delete newErrors[field];
 return newErrors;
 });
 }
 };

 return (
 <div className="min-h-screen flex bg-card font-sans text-foreground">

 {/* Left side - Banner (Desktop only) */}
 <div className="hidden lg:flex lg:w-1/2 relative bg-slate-900 overflow-hidden">
 {/* Imagem escolhida pela imobiliária (Configurações > Aparência). São assets
     locais: a antiga vinha de um host externo e a requisição vazava do gate e2e. */}
 {loginBanner && (
 <img
 src={loginBanner}
 alt=""
 data-testid="login-banner"
 className="absolute inset-0 w-full h-full object-cover opacity-60"
 />
 )}
 <div className="absolute inset-0 bg-gradient-to-t from-slate-900 via-slate-900/60 to-transparent"></div>
 <div className="relative z-10 p-16 flex flex-col justify-end h-full">
 <div className="flex items-center text-white mb-8">
 <CompanyLogo logoUrl={loginLogoUrl} className="h-10 object-contain" />
 </div>
 <h2 className="text-4xl font-bold text-white leading-tight mb-4">
 {loginTitle}
 </h2>
 <p className="text-white/70 text-lg max-w-md">
 {loginSubtitle}
 </p>
 </div>
 </div>

 {/* Right side - Form Area */}
 <div className="w-full lg:w-1/2 flex flex-col items-center justify-center p-8 sm:p-12 lg:p-20 overflow-y-auto relative bg-muted lg:bg-card">

 {/* Mobile Header */}
 <div className="flex lg:hidden items-center justify-center mb-10 w-full">
 <CompanyLogo logoUrl={loginLogoUrl} className="h-8 object-contain" />
 </div>

 <div className="w-full max-w-md">
 {!isForgotPassword && (
 <div className="bg-muted p-1.5 rounded-xl flex mb-10 relative shadow-inner">
 <div
 className={clsx(
 "absolute top-1.5 bottom-1.5 w-[calc(50%-6px)] bg-card rounded-lg shadow-sm transition-all duration-300 ease-out",
 isLogin ? "left-1.5" : "left-[calc(50%+1.5px)]"
 )}
 ></div>
 <button
 onClick={() => { setIsLogin(true); setStep(1); setFieldErrors({}); setError(""); }}
 className={clsx(
 "flex-1 py-2.5 text-sm font-bold z-10 transition-colors",
 isLogin ? "text-primary-600" : "text-muted-foreground hover:text-foreground"
 )}
 >
 Entrar
 </button>
 <button
 onClick={() => { setIsLogin(false); setFieldErrors({}); setError(""); }}
 className={clsx(
 "flex-1 py-2.5 text-sm font-bold z-10 transition-colors",
 !isLogin ? "text-primary-600" : "text-muted-foreground hover:text-foreground"
 )}
 >
 Cadastre-se
 </button>
 </div>
 )}

 <AnimatePresence mode="wait">
 {isForgotPassword ? (
 <ForgotPasswordForm onBack={() => setIsForgotPassword(false)} />
 ) : isLogin ? (
 <motion.div
 key="login"
 initial={{ opacity: 0, x: -20 }}
 animate={{ opacity: 1, x: 0 }}
 exit={{ opacity: 0, x: -20 }}
 transition={{ duration: 0.2 }}
 >
 <div className="text-center mb-8">
 <h1 className="text-2xl font-bold text-foreground mb-2">Bem-vindo de volta!</h1>
 <p className="text-sm text-muted-foreground">Insira suas credenciais para acessar sua conta.</p>
 </div>

 {error && (
 <div className="mb-6 p-3 bg-status-critical-soft border border-status-critical/30 text-status-critical text-sm rounded-lg flex items-center gap-2">
 <AlertCircle className="w-4 h-4 shrink-0" />
 {error}
 </div>
 )}

 <form onSubmit={handleLogin} className="space-y-5">
 <div>
 <label htmlFor="login-email" className="block text-xs font-bold text-foreground uppercase tracking-wider mb-2">E-mail Corporativo</label>
 <div className="relative">
 <Mail className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-muted-foreground" />
 <input
 id="login-email"
 type="email"
 value={loginEmail}
 onChange={(e) => updateLoginData('email', e.target.value)}
 onBlur={() => {
   const result = loginSchema.shape.email.safeParse(loginEmail);
   if (!result.success) {
     setFieldErrors(prev => ({ ...prev, email: result.error.issues[0].message }));
   }
 }}
 placeholder="nome@imobiliaria.com.br"
 className={clsx(
 "w-full pl-10 pr-4 py-3 bg-card border rounded-xl text-sm focus:ring-2 focus:ring-primary-500 focus:border-transparent outline-none transition-all placeholder:text-muted-foreground font-medium",
 fieldErrors.email ? "border-status-critical/40 focus:ring-status-critical/30" : "border-border"
 )}
 />
 </div>
 {fieldErrors.email && <p className="mt-1 text-xs text-status-critical font-medium">{fieldErrors.email}</p>}
 </div>

 <div>
 <div className="flex items-center justify-between mb-2">
                      <label htmlFor="login-password" className="block text-xs font-bold text-foreground uppercase tracking-wider">Senha</label>
                      <Button
                        type="button"
                        variant="link"
                        onClick={() => setIsForgotPassword(true)}
                        className="h-auto p-0 text-xs font-bold"
                      >
                        Esqueci minha senha
                      </Button>
                    </div>
 <div className="relative">
 <Lock className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-muted-foreground" />
 <input
 id="login-password"
 type={showPassword ? "text" : "password"}
 value={loginPassword}
 onChange={(e) => updateLoginData('password', e.target.value)}
 placeholder="••••••••"
 className={clsx(
 "w-full pl-10 pr-10 py-3 bg-card border rounded-xl text-sm focus:ring-2 focus:ring-primary-500 focus:border-transparent outline-none transition-all placeholder:text-muted-foreground font-medium",
 fieldErrors.password ? "border-status-critical/40 focus:ring-status-critical/30" : "border-border"
 )}
 />
 <button
 type="button"
 aria-label="Alternar visibilidade da senha"
 onClick={() => setShowPassword(!showPassword)}
 className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-muted-foreground"
 >
 {showPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
 </button>
 </div>
 {fieldErrors.password && <p className="mt-1 text-xs text-status-critical font-medium">{fieldErrors.password}</p>}
 </div>

 <Button
 type="submit"
 size="lg"
 aria-label="Entrar na conta"
 loading={isLoading}
 disabled={loginDisabled}
 className="w-full mt-2 group"
 >
 Entrar <ArrowRight className="w-4 h-4 group-hover:translate-x-1 transition-transform" />
 </Button>
 </form>

 <div className="mt-8 bg-primary-50 border border-primary-100 rounded-xl p-4 flex gap-3">
 <div className="mt-1 shrink-0">
 <CompanyLogo logoUrl={themeSettings?.logoUrl} className="w-5 h-5 object-contain" />
 </div>
 <div>
 <h4 className="text-sm font-bold text-primary-900">Recebeu um convite?</h4>
 <p className="text-xs text-primary-700 mt-0.5">Se a sua imobiliária já usa o suitee, peça o link de convite para ingressar no time.</p>
 </div>
 </div>
 </motion.div>
 ) : (
 <motion.div
 key="register"
 initial={{ opacity: 0, x: 20 }}
 animate={{ opacity: 1, x: 0 }}
 exit={{ opacity: 0, x: 20 }}
 transition={{ duration: 0.2 }}
 >
 <div className="mb-6 flex items-center justify-between">
 <div className="flex gap-1.5">
 {[1, 2, 3, 4].map(s => (
 <div key={s} className={clsx("h-1.5 rounded-full transition-all duration-300", step >= s ? "bg-primary-600" : "bg-muted", step === s ? "w-8" : "w-4")} />
 ))}
 </div>
 <span className="text-xs font-bold text-muted-foreground">Passo {step} de 4</span>
 </div>

 <div className="text-center mb-8">
 <h1 className="text-2xl font-bold text-foreground mb-2">
 {step === 1 && "Seus dados de contato"}
 {step === 2 && "Identificação e Perfil"}
 {step === 3 && "Crie sua Senha"}
 {step === 4 && "Revise seus dados"}
 </h1>
 <p className="text-sm text-muted-foreground">
 {step === 1 && "Vamos começar com o básico para sua conta."}
 {step === 2 && "Como devemos chamar você na plataforma?"}
 {step === 3 && "Escolha uma senha forte e segura (mín. 8 caracteres)."}
 {step === 4 && "Tudo certo? Confirme para criar sua conta."}
 </p>
 </div>

 {error && (
 <div className="mb-6 p-3 bg-status-critical-soft border border-status-critical/30 text-status-critical text-sm rounded-lg flex items-center gap-2">
 <AlertCircle className="w-4 h-4 shrink-0" />
 {error}
 </div>
 )}

 <div className="space-y-5 min-h-[220px]">
 {step === 1 && (
 <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="space-y-5">
 <div>
 <label className="block text-xs font-bold text-foreground uppercase tracking-wider mb-2">E-mail Corporativo</label>
 <div className="relative">
 <Mail className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-muted-foreground" />
 <input
 type="email"
 value={regData.email}
 onChange={(e) => updateRegData('email', e.target.value)}
 onBlur={() => {
   const result = step1Schema.shape.email.safeParse(regData.email);
   if (!result.success) {
     setFieldErrors((prev) => ({ ...prev, email: result.error.issues[0].message }));
   }
 }}
 placeholder="nome@suaimobiliaria.com.br"
 className={clsx(
 "w-full pl-10 pr-4 py-3 bg-card border rounded-xl text-sm focus:ring-2 focus:ring-primary-500 outline-none transition-all placeholder:text-muted-foreground",
 fieldErrors.email ? "border-status-critical/40 focus:ring-status-critical/30" : "border-border"
 )}
 />
 </div>
 {fieldErrors.email && <p className="mt-1 text-xs text-status-critical font-medium">{fieldErrors.email}</p>}
 </div>
 <div>
 <label className="block text-xs font-bold text-foreground uppercase tracking-wider mb-2">Telefone / Celular</label>
 <div className="relative">
 <Phone className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-muted-foreground" />
 <input
 type="tel"
 value={formatPhoneBR(regData.phone)}
 onChange={(e) => updateRegData('phone', parsePhoneBR(e.target.value))}
 onBlur={() => {
   const result = step1Schema.shape.phone.safeParse(regData.phone);
   if (!result.success) {
     setFieldErrors((prev) => ({ ...prev, phone: result.error.issues[0].message }));
   }
 }}
 placeholder="(00) 00000-0000"
 className={clsx(
 "w-full pl-10 pr-4 py-3 bg-card border rounded-xl text-sm focus:ring-2 focus:ring-primary-500 outline-none transition-all placeholder:text-muted-foreground",
 fieldErrors.phone ? "border-status-critical/40 focus:ring-status-critical/30" : "border-border"
 )}
 />
 </div>
 {fieldErrors.phone && <p className="mt-1 text-xs text-status-critical font-medium">{fieldErrors.phone}</p>}
 </div>
 </motion.div>
 )}

 {step === 2 && (
 <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="space-y-5">
 <div className="grid grid-cols-2 gap-4">
 <div>
 <label htmlFor="signup-firstname" className="block text-xs font-bold text-foreground uppercase tracking-wider mb-2">Nome</label>
 <div className="relative">
 <User className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
 <input
 id="signup-firstname"
 type="text"
 value={regData.firstName}
 onChange={(e) => updateRegData('firstName', e.target.value)}
 onBlur={() => {
   const result = step2Schema.shape.firstName.safeParse(regData.firstName);
   if (!result.success) {
     setFieldErrors((prev) => ({ ...prev, firstName: result.error.issues[0].message }));
   }
 }}
 className={clsx(
 "w-full pl-9 pr-3 py-3 bg-card border rounded-xl text-sm focus:ring-2 focus:ring-primary-500 outline-none transition-all",
 fieldErrors.firstName ? "border-status-critical/40 focus:ring-status-critical/30" : "border-border"
 )}
 />
 </div>
 {fieldErrors.firstName && <p className="mt-1 text-xs text-status-critical font-medium">{fieldErrors.firstName}</p>}
 </div>
 <div>
 <label htmlFor="signup-lastname" className="block text-xs font-bold text-foreground uppercase tracking-wider mb-2">Sobrenome</label>
 <input
 id="signup-lastname"
 type="text"
 value={regData.lastName}
 onChange={(e) => updateRegData('lastName', e.target.value)}
 onBlur={() => {
   const result = step2Schema.shape.lastName.safeParse(regData.lastName);
   if (!result.success) {
     setFieldErrors((prev) => ({ ...prev, lastName: result.error.issues[0].message }));
   }
 }}
 className={clsx(
 "w-full px-4 py-3 bg-card border rounded-xl text-sm focus:ring-2 focus:ring-primary-500 outline-none transition-all",
 fieldErrors.lastName ? "border-status-critical/40 focus:ring-status-critical/30" : "border-border"
 )}
 />
 {fieldErrors.lastName && <p className="mt-1 text-xs text-status-critical font-medium">{fieldErrors.lastName}</p>}
 </div>
 </div>
 <div>
 <label htmlFor="signup-role" className="block text-xs font-bold text-foreground uppercase tracking-wider mb-2">Cargo / Função</label>
 <div className="relative">
 <Briefcase className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-muted-foreground" />
 <select
 id="signup-role"
 value={regData.role}
 onChange={(e) => updateRegData('role', e.target.value)}
 className={clsx(
 "w-full pl-10 pr-4 py-3 bg-card border rounded-xl text-sm focus:ring-2 focus:ring-primary-500 outline-none transition-all appearance-none",
 fieldErrors.role ? "border-status-critical/40 focus:ring-status-critical/30" : "border-border"
 )}
 >
 <option value="Administrador">Administrador da Imobiliária</option>
 <option value="Gerente">Gerente de Vendas / Locação</option>
 <option value="Corretor">Corretor / Captador</option>
 <option value="Secretário">Secretário / Administrativo</option>
 </select>
 <ChevronRight className="absolute right-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground rotate-90" />
 </div>
 {fieldErrors.role && <p className="mt-1 text-xs text-status-critical font-medium">{fieldErrors.role}</p>}
 </div>
 </motion.div>
 )}

 {step === 3 && (
 <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="space-y-5">
 <div>
 <label htmlFor="signup-password" className="block text-xs font-bold text-foreground uppercase tracking-wider mb-2">Senha</label>
 <div className="relative">
 <Lock className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-muted-foreground" />
 <input
 id="signup-password"
 type={showPassword ? "text" : "password"}
 value={regData.password}
 onChange={(e) => updateRegData('password', e.target.value)}
 onBlur={() => {
   const result = passwordSchema.safeParse(regData.password);
   if (!result.success) {
     setFieldErrors((prev) => ({ ...prev, password: result.error.issues[0].message }));
   }
 }}
 className={clsx(
 "w-full pl-10 pr-10 py-3 bg-card border rounded-xl text-sm focus:ring-2 focus:ring-primary-500 outline-none transition-all",
 fieldErrors.password ? "border-status-critical/40 focus:ring-status-critical/30" : "border-border"
 )}
 />
 <button
 type="button"
 aria-label="Alternar visibilidade da senha"
 onClick={() => setShowPassword(!showPassword)}
 className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-muted-foreground"
 >
 {showPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
 </button>
 </div>
 {fieldErrors.password && <p className="mt-1 text-xs text-status-critical font-medium">{fieldErrors.password}</p>}
 </div>
 <div>
 <label htmlFor="signup-confirm-password" className="block text-xs font-bold text-foreground uppercase tracking-wider mb-2">Confirme a Senha</label>
 <div className="relative">
 <Lock className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-muted-foreground" />
 <input
 id="signup-confirm-password"
 type="password"
 value={regData.confirmPassword}
 onChange={(e) => updateRegData('confirmPassword', e.target.value)}
 onBlur={() => {
   if (regData.confirmPassword && regData.password !== regData.confirmPassword) {
     setFieldErrors((prev) => ({ ...prev, confirmPassword: 'As senhas não coincidem' }));
   }
 }}
 className={clsx(
 "w-full pl-10 pr-4 py-3 bg-card border rounded-xl text-sm focus:ring-2 focus:ring-primary-500 outline-none transition-all",
 fieldErrors.confirmPassword ? "border-status-critical/40 focus:ring-status-critical/30" : "border-border"
 )}
 />
 </div>
 {fieldErrors.confirmPassword && <p className="mt-1 text-xs text-status-critical font-medium">{fieldErrors.confirmPassword}</p>}
 </div>
 </motion.div>
 )}

 {step === 4 && (
 <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="bg-card rounded-xl border border-border p-5 space-y-4 shadow-sm">
 <div className="flex items-center gap-4 border-b border-border pb-4">
 <div className="w-12 h-12 rounded-full bg-primary-100 text-primary-600 flex items-center justify-center font-bold text-lg">
 {regData.firstName.charAt(0) || 'U'}
 </div>
 <div>
 <p className="font-bold text-foreground">{regData.firstName} {regData.lastName}</p>
 <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">{regData.role}</p>
 </div>
 </div>
 <div className="space-y-2 pt-2">
 <div className="flex justify-between items-center text-sm">
 <span className="text-muted-foreground">E-mail:</span>
 <span className="font-medium text-foreground">{regData.email}</span>
 </div>
 <div className="flex justify-between items-center text-sm">
 <span className="text-muted-foreground">Telefone:</span>
 <span className="font-medium text-foreground">{formatPhoneBR(regData.phone)}</span>
 </div>
 </div>
 <div className="bg-status-ok-soft text-status-ok text-xs font-medium p-3 rounded-lg flex items-start gap-2 mt-4">
 <CheckCircle className="w-4 h-4 shrink-0 mt-0.5" />
 Ao clicar em "Criar conta", você aceita os{" "}
 <a href="/terms" target="_blank" rel="noreferrer" className="underline hover:text-primary-700">Termos de Uso</a>
 e confirma que teve acesso à{" "}
 <a href="/privacy" target="_blank" rel="noreferrer" className="underline hover:text-primary-700">Política de Privacidade</a>.
 </div>
 </motion.div>
 )}
 </div>

 <div className="flex gap-3 mt-8">
 {step > 1 && (
 <Button
 type="button"
 variant="outline"
 size="icon"
 aria-label="Voltar ao passo anterior"
 onClick={() => setStep(step - 1)}
 className="h-11 w-11 shrink-0"
 >
 <ArrowLeft className="w-4 h-4" />
 </Button>
 )}
 <Button
 type="button"
 size="lg"
 aria-label={step === 4 ? 'Criar minha conta' : 'Continuar'}
 onClick={() => {
 if (step < 4) {
 if (validateStep(step)) setStep(step + 1);
 }
 else handleRegister();
 }}
 loading={isLoading}
 disabled={continueDisabled}
 className="flex-1 group"
 >
 {step === 4 ? "Criar minha conta" : "Continuar"}
 {step < 4 && <ArrowRight className="w-4 h-4 group-hover:translate-x-1 transition-transform" />}
 </Button>
 </div>
 </motion.div>
 )}
 </AnimatePresence>
 </div>
 </div>
 </div>
 );
}
