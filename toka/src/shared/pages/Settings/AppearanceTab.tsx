import { useState, useId, useEffect, useRef, forwardRef, useImperativeHandle } from "react";
import {
  Globe,
  Mail,
  CheckCircle2,
  AlertTriangle,
  Type,
  Monitor,
  Sun,
  Moon,
  RotateCcw,
  Settings2,
  Upload,
  ImagePlus,
  Trash2,
  Circle,
  Minus,
  Square,
  Sparkles,
} from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";
import { useTheme, type ThemeSettings, DEFAULT_THEME } from "../../context/ThemeContext";
import { Button } from "../../ui/button";
import { ColorInput } from "../../ui/color-input";
import { readableTextColor } from "../../utils/hexColor";
import { settingsService, type AppearanceSettings } from "../../services/settings.service";
import {
  clearPendingAppearance,
  hasPendingAppearance,
  writePendingAppearance,
} from "../../lib/pendingAppearance";
import { SectionCard, FieldRow, SettingsInput, Toggle, SettingsSelect, type TabRef } from "./_shared";
import { COLOR_PRESETS, ACCENT_PRESETS, FONT_OPTIONS, TENANT_ICON_STYLES } from "./_constants";
import { tenantFontStack, type TenantIconStyle } from "../../lib/tenantAppearance";
import {
  LOGIN_BANNERS,
  LOGIN_BANNER_SOURCE_CUSTOM,
  LOGIN_BANNER_SOURCE_PRESET,
  effectiveLoginBannerSource,
  findLoginBanner,
  hasCustomLoginBanner,
  resolveLoginBanner,
} from "../../loginBanners";

export const AppearanceTab = forwardRef<TabRef, { onChange: () => void }>(({ onChange }, ref) => {
  const fieldId = useId();
  const { theme: themeCtx, updateTheme } = useTheme();

  const [settings, setSettings] = useState<AppearanceSettings>({
    tenant_id: "",
    primaryColor: themeCtx.primaryColor,
    accentColor: themeCtx.accentColor,
    sidebarStyle: themeCtx.sidebarStyle,
    theme: themeCtx.theme,
    showLogoLogin: themeCtx.showLogoLogin,
    darkModeEnabled: themeCtx.darkModeEnabled,
    customDomain: themeCtx.customDomain,
    domainVerified: themeCtx.domainVerified,
    loginTagline: themeCtx.loginTagline,
    loginSubtitle: themeCtx.loginSubtitle,
    loginBannerPreset: themeCtx.loginBannerPreset,
    loginBannerUrl: themeCtx.loginBannerUrl,
    loginBannerSource: themeCtx.loginBannerSource,
    emailFooter: themeCtx.emailFooter,
    fontFamily: themeCtx.fontFamily,
    iconStyle: themeCtx.iconStyle,
  });

  useEffect(() => {
    settingsService
      .getAppearance()
      .then((apiSettings) => {
        setSettings((prev) => ({
          ...prev,
          ...apiSettings,
        }));
      })
      .catch(console.error);
  }, []);

  useImperativeHandle(ref, () => ({
    save: async () => {
      await settingsService.updateAppearance(settings);
      // Só limpa o rascunho depois do servidor aceitar: se o PUT falhar, o que
      // está aplicado continua sendo o que o usuário escolheu.
      clearPendingAppearance();
      dirtyRef.current = false;
      setHasPending(false);
    },
  }));

  const [verifyingDomain, setVerifyingDomain] = useState(false);
  // Há alterações aplicadas neste navegador que ainda não foram para o servidor?
  // O rascunho é gravado em `pendingAppearance` e sobrevive à recarga (o
  // ThemeProvider o aplica por cima do valor do tenant); salvar limpa.
  const [hasPending, setHasPending] = useState(false);
  const dirtyRef = useRef(false);

  useEffect(() => {
    const pending = hasPendingAppearance();
    setHasPending(pending);
    // Rascunho de uma sessão anterior: a página de Configurações precisa abrir
    // já marcada como "alterações não salvas", senão o aviso some na recarga.
    if (pending) onChange();
  }, [onChange]);

  const update = <K extends keyof AppearanceSettings>(key: K, value: AppearanceSettings[K]) => {
    setSettings((prev) => ({ ...prev, [key]: value }));
    updateTheme(key as keyof ThemeSettings, value as any);
    dirtyRef.current = true;
    onChange();
  };

  // Trocar de imagem mexe em DOIS campos (a escolha e a fonte) e o tema do
  // localStorage precisa dos dois para a tela de login resolver igual. Sem isto,
  // dois `update` seguidos deixariam o tema intermediário gravado.
  const updateMany = (patch: Partial<AppearanceSettings>) => {
    setSettings((prev) => ({ ...prev, ...patch }));
    (Object.keys(patch) as (keyof ThemeSettings)[]).forEach((key) => {
      updateTheme(key, patch[key as keyof AppearanceSettings] as any);
    });
    dirtyRef.current = true;
    onChange();
  };

  const handleVerifyDomain = () => {
    setVerifyingDomain(true);
    setTimeout(() => {
      setVerifyingDomain(false);
      update("domainVerified", true);
      toast.success("Dominio verificado com sucesso!");
    }, 2000);
  };

  // O rascunho acompanha o estado da aba. Só depois de uma edição do usuário:
  // gravar no primeiro render marcaria como pendente o que veio do servidor.
  useEffect(() => {
    if (!dirtyRef.current) return;
    writePendingAppearance(settings as unknown as Record<string, unknown>);
    setHasPending(true);
  }, [settings]);

  const [uploadingBanner, setUploadingBanner] = useState(false);
  const bannerInputRef = useRef<HTMLInputElement>(null);

  // Prévia: usa a MESMA resolução da tela de login (`resolveLoginBanner`), então
  // o que aparece aqui é o que a equipe verá.
  //
  // A imagem enviada e os presets ficam os DOIS guardados; `activeBannerSource`
  // diz qual está em uso e é o que a troca rápida muda. Antes, escolher um
  // preset apagava a imagem enviada e obrigava a subir o arquivo de novo.
  const hasUploadedBanner = hasCustomLoginBanner(settings.loginBannerUrl);
  const activeBannerSource = effectiveLoginBannerSource(settings.loginBannerSource, settings.loginBannerUrl);
  const resolvedBanner = resolveLoginBanner(
    settings.loginBannerPreset,
    settings.loginBannerUrl,
    activeBannerSource,
  );
  const previewTitle = settings.loginTagline || DEFAULT_THEME.loginTagline;
  const previewSubtitle = settings.loginSubtitle || DEFAULT_THEME.loginSubtitle;
  const previewSourceLabel =
    activeBannerSource === LOGIN_BANNER_SOURCE_CUSTOM && hasUploadedBanner
      ? "Imagem enviada pela imobiliária"
      : `Modelo: ${findLoginBanner(settings.loginBannerPreset)?.label ?? "padrao"}`;
  // Guarda o `src` que falhou (e não um booleano) para a prévia se recuperar
  // sozinha quando o usuário troca de imagem, sem precisar de useEffect.
  const [failedBannerSrc, setFailedBannerSrc] = useState<string | null>(null);
  const bannerPreviewFailed = resolvedBanner !== undefined && failedBannerSrc === resolvedBanner;
  const [failedTileSrc, setFailedTileSrc] = useState<string | null>(null);

  // Troca rápida: nenhuma das duas apaga a outra.
  const useUploadedBanner = () => updateMany({ loginBannerSource: LOGIN_BANNER_SOURCE_CUSTOM });
  const applyPresetBanner = (id: string) =>
    updateMany({ loginBannerPreset: id, loginBannerSource: LOGIN_BANNER_SOURCE_PRESET });
  // Única ação destrutiva, explícita: sem a URL não há o que alternar, então a
  // fonte volta para o preset escolhido.
  const removeUploadedBanner = () =>
    updateMany({ loginBannerUrl: "", loginBannerSource: LOGIN_BANNER_SOURCE_PRESET });

  // O upload grava a imagem no storage e devolve a URL; ela vira
  // `loginBannerUrl` e passa a ser a imagem ATIVA (quem envia quer usá-la). O
  // preset escolhido continua guardado. Só o "Salvar" do rodapé persiste a
  // escolha para a empresa inteira.
  const handleBannerFile = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    // Limpa o input para que reenviar o MESMO arquivo volte a disparar onChange.
    event.target.value = "";
    if (!file) return;

    setUploadingBanner(true);
    try {
      const { banner_url } = await settingsService.uploadLoginBanner(file);
      // O backend grava a URL e já marca a fonte como `custom`; aqui o estado
      // local acompanha o mesmo par para o preview não depender do reload.
      updateMany({ loginBannerUrl: banner_url, loginBannerSource: LOGIN_BANNER_SOURCE_CUSTOM });
      toast.success("Imagem enviada. Clique em Salvar para aplicar a toda a equipe.");
    } catch {
      // O backend valida o MIME real do conteúdo e o limite de 5MB do bucket
      // `toca-logos`; a mensagem evita expor o erro cru da API.
      toast.error("Não foi possível enviar a imagem. Use PNG, JPG ou WebP de ate 5MB.");
    } finally {
      setUploadingBanner(false);
    }
  };

  const resetAppearance = () => {
    setSettings((prev) => ({ ...prev, ...DEFAULT_THEME }));
    Object.entries(DEFAULT_THEME).forEach(([key, value]) => {
      updateTheme(key as keyof ThemeSettings, value as any);
    });
    dirtyRef.current = true;
    onChange();
    toast.success("Configurações de aparência restauradas para o padrão.");
  };

  return (
    <div className="space-y-6">
      {hasPending && (
        <div
          role="status"
          data-testid="appearance-pending"
          className="flex items-start gap-2 rounded-xl border border-status-warning/30 bg-status-warning-soft px-4 py-3 text-xs text-status-warning"
        >
          <AlertTriangle className="w-4 h-4 shrink-0 mt-0.5" />
          <p>
            Estas alterações estao aplicadas neste navegador, mas ainda não foram salvas.
            Clique em <strong>Salvar Alterações</strong> para valer para toda a equipe.
          </p>
        </div>
      )}

      <SectionCard title="Paleta de Cores" subtitle="Personalize as cores do painel para combinar com sua marca">
        <div className="flex justify-end mb-4">
          <Button
            variant="ghost"
            size="sm"
            onClick={resetAppearance}
            className="h-auto px-2 py-1 text-xs font-bold text-muted-foreground"
          >
            <RotateCcw className="w-3.5 h-3.5" /> Restaurar Padrão
          </Button>
        </div>
        <div className="space-y-6">
          <div>
            <label htmlFor={`${fieldId}-primaryColor`} className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-3 block">
              Cor Principal (Primary)
            </label>
            <div className="flex items-center gap-4 mb-4">
              <ColorInput
                value={settings.primaryColor}
                onChange={(hex) => update("primaryColor", hex)}
                label="Cor principal"
              />
              <div className="space-y-1">
                <SettingsInput
                  id={`${fieldId}-primaryColor`} value={settings.primaryColor}
                  onChange={(v) => update("primaryColor", v)}
                  mono
                  className="w-36"
                />
                <p className="text-[11px] text-muted-foreground">Botoes, links e destaques</p>
                <p className="text-caption text-muted-foreground">Clique no quadrado para escolher a cor</p>
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              {COLOR_PRESETS.map((c) => (
                <button
                  key={c.value}
                  aria-pressed={settings.primaryColor === c.value}
                  onClick={() => update("primaryColor", c.value)}
                  className={clsx(
                    "group flex items-center gap-2 px-3 py-2 rounded-xl border text-xs font-bold transition-all",
                    settings.primaryColor === c.value
                      ? "border-primary-400 bg-primary-50 text-primary-700 ring-2 ring-primary-200"
                      : "border-border bg-card text-muted-foreground hover:border-border",
                  )}
                >
                  <span
                    className="w-4 h-4 rounded-full shrink-0 border border-black/10"
                    style={{ backgroundColor: c.value }}
                  />
                  {c.name}
                </button>
              ))}
            </div>
          </div>

          <div>
            <label htmlFor={`${fieldId}-accentColor`} className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-3 block">
              Cor de Acento (Accent)
            </label>
            <div className="flex items-center gap-4 mb-4">
              <ColorInput
                value={settings.accentColor}
                onChange={(hex) => update("accentColor", hex)}
                label="Cor de acento"
              />
              <div className="space-y-1">
                <SettingsInput
                  id={`${fieldId}-accentColor`} value={settings.accentColor}
                  onChange={(v) => update("accentColor", v)}
                  mono
                  className="w-36"
                />
                <p className="text-[11px] text-muted-foreground">Sucesso, confirmacoes e badges</p>
                <p className="text-caption text-muted-foreground">Clique no quadrado para escolher a cor</p>
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              {ACCENT_PRESETS.map((c) => (
                <button
                  key={c.value}
                  aria-pressed={settings.accentColor === c.value}
                  onClick={() => update("accentColor", c.value)}
                  className={clsx(
                    "group flex items-center gap-2 px-3 py-2 rounded-xl border text-xs font-bold transition-all",
                    settings.accentColor === c.value
                      ? "border-primary-400 bg-primary-50 text-primary-700 ring-2 ring-primary-200"
                      : "border-border bg-card text-muted-foreground hover:border-border",
                  )}
                >
                  <span className="w-4 h-4 rounded-full shrink-0 border border-black/10" style={{ backgroundColor: c.value }} />
                  {c.name}
                </button>
              ))}
            </div>
          </div>

          <div className="p-4 bg-muted rounded-2xl border border-border">
            <p className="text-xs font-bold text-muted-foreground mb-3">Pré-visualização</p>
            <div className="flex flex-wrap items-center gap-3">
              <button
                className="px-4 py-2 rounded-xl text-sm font-bold"
                style={{
                  backgroundColor: settings.primaryColor,
                  color: readableTextColor(settings.primaryColor),
                }}
              >
                Botao Primario
              </button>
              <button
                className="px-4 py-2 rounded-xl text-sm font-bold"
                style={{
                  backgroundColor: settings.accentColor,
                  color: readableTextColor(settings.accentColor),
                }}
              >
                Botao Acento
              </button>
              <span
                className="px-3 py-1.5 rounded-lg text-xs font-bold"
                style={{ backgroundColor: `${settings.primaryColor}15`, color: settings.primaryColor }}
              >
                Badge Primaria
              </span>
              <span
                className="px-3 py-1.5 rounded-lg text-xs font-bold"
                style={{ backgroundColor: `${settings.accentColor}15`, color: settings.accentColor }}
              >
                Badge Acento
              </span>
            </div>
          </div>
        </div>
      </SectionCard>

      <SectionCard title="Tipografia & Layout" subtitle="Fonte e estilo visual do painel" delay={0.05}>
        <div className="space-y-5">
          <div>
            <label htmlFor={`${fieldId}-fontFamily`} className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-2 block">
              Familia Tipografica
            </label>
            <div className="max-w-xs">
              <SettingsSelect id={`${fieldId}-fontFamily`} value={settings.fontFamily} onChange={(v) => update("fontFamily", v)} options={FONT_OPTIONS} />
            </div>
            <p className="mt-2 text-caption text-muted-foreground">
              Aplicada ao painel inteiro, à tela de login e aos componentes com tipografia própria.
            </p>
            <div
              data-testid="font-preview"
              className="mt-4 rounded-2xl border border-border bg-muted/50 px-5 py-4"
              style={{ fontFamily: tenantFontStack(settings.fontFamily) }}
            >
              <div className="flex items-baseline gap-3">
                <span className="text-3xl font-semibold tracking-tight text-foreground">Aa</span>
                <span className="text-sm font-bold text-foreground">Acompanhe leads, contratos e resultados.</span>
              </div>
              <p className="mt-2 text-xs text-muted-foreground">
                {settings.fontFamily} · {settings.fontFamily === "Inter" ? "fonte padrão da Suitee" : "fonte da sua marca"} · 0123456789
              </p>
            </div>
          </div>

          <div>
            <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-3 block">
              Estilo da Sidebar
            </p>
            <div className="flex gap-3">
              {([
                { value: "default", label: "Padrão", icon: Monitor, desc: "Completa com rótulos" },
                { value: "compact", label: "Compacta", icon: Settings2, desc: "Icones com tooltip" },
                { value: "minimal", label: "Minimal", icon: Type, desc: "Apenas icones" },
              ] as const).map((opt) => (
                <button
                  key={opt.value}
                  aria-label={`${opt.label}: ${opt.desc}`}
                  aria-pressed={settings.sidebarStyle === opt.value}
                  onClick={() => update("sidebarStyle", opt.value)}
                  className={clsx(
                    "flex-1 flex flex-col items-center gap-2 p-4 rounded-2xl border transition-all text-center",
                    settings.sidebarStyle === opt.value
                      ? "border-primary-400 bg-primary-50 ring-2 ring-primary-200"
                      : "border-border bg-card hover:border-border",
                  )}
                >
                  <opt.icon className={clsx("w-5 h-5", settings.sidebarStyle === opt.value ? "text-primary-600" : "text-muted-foreground")} />
                  <span className={clsx("text-xs font-bold", settings.sidebarStyle === opt.value ? "text-primary-700" : "text-muted-foreground")}>
                    {opt.label}
                  </span>
                  <span className="text-caption text-muted-foreground">{opt.desc}</span>
                </button>
              ))}
            </div>
          </div>

          <div>
            <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-1 block">
              Traço dos ícones
            </p>
            <p className="mb-3 text-caption text-muted-foreground">
              Ajusta o peso e o acabamento dos ícones em todo o painel.
            </p>
            <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
              {TENANT_ICON_STYLES.map((style) => {
                const selected = settings.iconStyle === style.value;
                const Icon = style.value === "light" ? Minus : style.value === "sharp" ? Square : style.value === "bold" ? Sparkles : Circle;
                return (
                  <button
                    key={style.value}
                    type="button"
                    aria-label={`Ícones ${style.label}: ${style.description}`}
                    aria-pressed={selected}
                    onClick={() => update("iconStyle", style.value as TenantIconStyle)}
                    className={clsx(
                      "flex items-center gap-3 rounded-xl border px-3 py-2.5 text-left transition-all",
                      selected
                        ? "border-primary-400 bg-primary-50 ring-2 ring-primary-200"
                        : "border-border bg-card hover:border-border",
                    )}
                  >
                    <Icon
                      className={clsx("h-4 w-4 shrink-0", selected ? "text-primary-600" : "text-muted-foreground")}
                      strokeWidth={Number(style.strokeWidth)}
                      strokeLinecap={style.linecap as "round" | "butt"}
                      strokeLinejoin={style.linejoin as "round" | "miter"}
                    />
                    <span className="min-w-0">
                      <span className={clsx("block text-xs font-bold", selected ? "text-primary-700" : "text-foreground")}>{style.label}</span>
                      <span className="block text-caption text-muted-foreground">{style.description}</span>
                    </span>
                  </button>
                );
              })}
            </div>
          </div>

          <div className="divide-y divide-border">
            <FieldRow label="Tema do painel" description="Aparência visual claro, escuro ou automático">
              <div className="flex items-center gap-1 bg-muted rounded-xl p-1">
                {([
                  { value: "light", icon: Sun },
                  { value: "dark", icon: Moon },
                  { value: "auto", icon: Monitor },
                ] as const).map(({ value, icon: Icon }) => (
                  <button
                    key={value}
                    onClick={() => update("theme", value)}
                    className={clsx(
                      "p-2 rounded-lg transition-colors",
                      settings.theme === value ? "bg-card shadow-sm text-primary-600" : "text-muted-foreground hover:text-muted-foreground",
                    )}
                  >
                    <Icon className="w-4 h-4" />
                  </button>
                ))}
              </div>
            </FieldRow>
            <FieldRow label="Exibir logotipo na tela de Login" description="Substitui o logo suitee pelo da sua empresa">
              <Toggle checked={settings.showLogoLogin} onChange={(v) => update("showLogoLogin", v)} />
            </FieldRow>
            <FieldRow label="Habilitar modo escuro para usuários" description="Permitir que cada usuário ative o tema escuro">
              <Toggle checked={settings.darkModeEnabled} onChange={(v) => update("darkModeEnabled", v)} />
            </FieldRow>
          </div>
        </div>
      </SectionCard>

      <SectionCard title="Dominio Personalizado" subtitle="Acesse o CRM atraves do seu proprio dominio" delay={0.1}>
        <div className="space-y-4">
          <div className="flex flex-col sm:flex-row gap-3">
            <div className="flex-1">
              <SettingsInput ariaLabel="Domínio personalizado" value={settings.customDomain} onChange={(v) => update("customDomain", v)} icon={<Globe className="w-4 h-4" />} />
            </div>
            <Button
              onClick={handleVerifyDomain}
              disabled={verifyingDomain || settings.domainVerified}
              loading={verifyingDomain}
              loadingText="Verificando..."
              className={clsx(
                "shrink-0",
                settings.domainVerified &&
                  "bg-status-ok-soft text-status-ok border border-status-ok/30 hover:bg-status-ok-soft",
              )}
            >
              {settings.domainVerified ? (
                <>
                  <CheckCircle2 className="w-4 h-4" /> Verificado
                </>
              ) : (
                "Verificar Dominio"
              )}
            </Button>
          </div>
          {!settings.domainVerified && (
            <div className="flex items-start gap-2 bg-status-warning-soft text-status-warning p-4 rounded-xl border border-status-warning/30">
              <AlertTriangle className="w-5 h-5 shrink-0 mt-0.5" />
              <p className="text-sm">
                Configure o registro CNAME no seu provedor de dominio (Registro.br, Cloudflare, etc) apontando para{" "}
                <strong>cname.suiteecrm.com</strong>.
              </p>
            </div>
          )}
        </div>
      </SectionCard>

      <SectionCard title="Personalizacao da Marca" subtitle="Textos e identidade em telas publicas e e-mails" delay={0.15}>
        <div className="space-y-5">
          <div className="space-y-1.5">
            <label htmlFor={`${fieldId}-loginTagline`} className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Título do Login</label>
            <SettingsInput
              id={`${fieldId}-loginTagline`} value={settings.loginTagline}
              onChange={(v) => update("loginTagline", v)}
              placeholder="Texto exibido na tela de login"
              icon={<Type className="w-4 h-4" />}
            />
          </div>
          <div className="space-y-1.5">
            <label htmlFor={`${fieldId}-loginSubtitle`} className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Subtítulo do Login</label>
            <SettingsInput
              id={`${fieldId}-loginSubtitle`} value={settings.loginSubtitle}
              onChange={(v) => update("loginSubtitle", v)}
              placeholder="Segunda linha, abaixo do título"
              icon={<Type className="w-4 h-4" />}
            />
          </div>
          <div className="space-y-1.5">
            <label htmlFor={`${fieldId}-emailFooter`} className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Rodape dos E-mails</label>
            <SettingsInput
              id={`${fieldId}-emailFooter`} value={settings.emailFooter}
              onChange={(v) => update("emailFooter", v)}
              placeholder="Texto do rodape nos e-mails enviados"
              icon={<Mail className="w-4 h-4" />}
            />
          </div>
        </div>
      </SectionCard>

      <SectionCard
        title="Imagem da Tela de Login"
        subtitle="Aparece no painel a esquerda do formulário, no desktop, para toda a equipe"
        delay={0.2}
      >
        <div className="space-y-5">
          {resolvedBanner && (
            <div className="space-y-2">
              <div className="flex items-center justify-between gap-3">
                <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider">
                  Pré-visualização no desktop
                </p>
                <span className="text-[11px] font-medium text-muted-foreground">{previewSourceLabel}</span>
              </div>
              {/* Espelha o painel esquerdo de `Login.tsx`: mesma imagem, mesmo
                  gradiente e mesmos textos, para a previa não mentir. */}
              <div className="relative overflow-hidden rounded-2xl border border-border bg-slate-900 aspect-[16/9] max-w-lg">
                {bannerPreviewFailed ? (
                  <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 px-6 text-center">
                    <AlertTriangle className="w-5 h-5 text-status-warning" />
                    <p className="text-xs text-white/80">
                      Não foi possível carregar esta imagem. Envie outra ou escolha um modelo abaixo —
                      no login o painel ficaria sem imagem.
                    </p>
                  </div>
                ) : (
                  <img
                    src={resolvedBanner}
                    alt=""
                    data-testid="login-banner-preview"
                    className="absolute inset-0 h-full w-full object-cover opacity-60"
                    onError={() => setFailedBannerSrc(resolvedBanner)}
                  />
                )}
                <div className="absolute inset-0 bg-gradient-to-t from-slate-900 via-slate-900/60 to-transparent" />
                <div className="relative z-10 flex h-full flex-col justify-end p-5">
                  <h3 className="text-lg font-bold leading-tight text-white">{previewTitle}</h3>
                  <p className="mt-1 line-clamp-2 text-xs text-white/70">{previewSubtitle}</p>
                </div>
              </div>
            </div>
          )}

          {/* Uma lista só: a imagem enviada entra como a primeira opção, ao lado
              dos modelos do sistema. Clicar em qualquer opção TROCA a imagem
              ativa sem apagar a outra — é a troca simplificada. */}
          <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
            {hasUploadedBanner && (
              <button
                type="button"
                data-testid="login-banner-tile-custom"
                aria-pressed={activeBannerSource === LOGIN_BANNER_SOURCE_CUSTOM}
                aria-label="Usar imagem enviada pela imobiliária"
                onClick={useUploadedBanner}
                className={clsx(
                  "relative overflow-hidden rounded-xl border-2 aspect-[4/3] text-left transition-colors",
                  activeBannerSource === LOGIN_BANNER_SOURCE_CUSTOM
                    ? "border-primary-500"
                    : "border-border hover:border-primary-300",
                )}
              >
                {failedTileSrc === settings.loginBannerUrl ? (
                  <span className="flex h-full w-full items-center justify-center bg-muted text-[11px] font-medium text-muted-foreground">
                    Imagem indisponivel
                  </span>
                ) : (
                  <img
                    src={settings.loginBannerUrl}
                    alt=""
                    className="w-full h-full object-cover"
                    onError={() => setFailedTileSrc(settings.loginBannerUrl)}
                  />
                )}
                {activeBannerSource === LOGIN_BANNER_SOURCE_CUSTOM && (
                  <span className="absolute top-2 right-2 rounded-full bg-primary p-1 text-primary-foreground">
                    <CheckCircle2 className="w-3.5 h-3.5" />
                  </span>
                )}
                <span className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/70 to-transparent px-2 py-1.5 text-[11px] font-medium text-white">
                  Sua imagem
                </span>
              </button>
            )}
            {LOGIN_BANNERS.map((banner) => {
              // A seleção segue a FONTE ativa, não a existência da imagem
              // enviada: com a imagem guardada, o preset escolhido continua
              // marcado quando é ele que está em uso.
              const selected =
                activeBannerSource === LOGIN_BANNER_SOURCE_PRESET &&
                settings.loginBannerPreset === banner.id;
              return (
                <button
                  key={banner.id}
                  type="button"
                  aria-pressed={selected}
                  aria-label={`Usar imagem: ${banner.label}`}
                  onClick={() => applyPresetBanner(banner.id)}
                  className={clsx(
                    "relative overflow-hidden rounded-xl border-2 aspect-[4/3] text-left transition-colors",
                    selected ? "border-primary-500" : "border-border hover:border-primary-300",
                  )}
                >
                  <img src={banner.src} alt="" className="w-full h-full object-cover" />
                  {selected && (
                    <span className="absolute top-2 right-2 rounded-full bg-primary p-1 text-primary-foreground">
                      <CheckCircle2 className="w-3.5 h-3.5" />
                    </span>
                  )}
                  <span className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/70 to-transparent px-2 py-1.5 text-[11px] font-medium text-white">
                    {banner.label}
                  </span>
                </button>
              );
            })}
          </div>

          <div className="flex flex-wrap items-center gap-3">
            <input
              ref={bannerInputRef}
              type="file"
              accept="image/png,image/jpeg,image/webp"
              className="hidden"
              onChange={handleBannerFile}
            />
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => bannerInputRef.current?.click()}
              loading={uploadingBanner}
              loadingText="Enviando..."
            >
              <Upload className="w-4 h-4" />
              {hasUploadedBanner ? "Trocar imagem" : "Enviar imagem da imobiliária"}
            </Button>
            {hasUploadedBanner && (
              <>
                <span className="inline-flex items-center gap-1.5 text-xs font-medium text-primary-600">
                  <ImagePlus className="w-3.5 h-3.5" />
                  {activeBannerSource === LOGIN_BANNER_SOURCE_CUSTOM
                    ? "Imagem enviada em uso"
                    : "Imagem enviada guardada"}
                </span>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={removeUploadedBanner}
                  className="text-muted-foreground"
                >
                  <Trash2 className="w-3.5 h-3.5" /> Remover imagem
                </Button>
              </>
            )}
          </div>

          <p className="text-[11px] text-muted-foreground">
            {hasUploadedBanner
              ? "Clique em qualquer imagem para trocar a que aparece no login. A sua imagem continua guardada — nada e apagado sem clicar em Remover imagem."
              : "Escolha um dos modelos do sistema ou envie a imagem da sua imobiliária."}
          </p>
        </div>
      </SectionCard>
    </div>
  );
});
