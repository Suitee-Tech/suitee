import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Building2,
  Copy,
  ExternalLink,
  Eye,
  Globe,
  Loader2,
  Mail,
  Palette,
  RefreshCw,
  Save,
  Settings2,
  ShieldCheck,
  ShieldAlert,
  Users,
} from "lucide-react";
import { Link } from "react-router";
import { toast } from "sonner";

import { useAuth } from "../context/AuthContext";
import { usePermissionsContext } from "../context/PermissionsContext";
import { useTheme } from "../context/ThemeContext";
import { notifyApiError } from "../services/errorToast";
import { settingsService } from "../services/settings.service";
import {
  previewSiteUrl,
  siteService,
  suggestSiteSlug,
  validateSiteSlug,
  type SiteBranding,
  type SiteContact,
  type SiteOverview,
  type SitePage,
  type SitePageKey,
} from "../services/site.service";
import { Button } from "../ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../ui/card";
import { ColorInput } from "../ui/color-input";
import { Input } from "../ui/input";
import { Label } from "../ui/label";
import { PageHeader } from "../ui/page-header";
import { Switch } from "../ui/switch";
import { Textarea } from "../ui/textarea";

const PAGE_TABS: { key: SitePageKey; label: string }[] = [
  { key: "home", label: "Início" },
  { key: "about", label: "Sobre" },
  { key: "contact", label: "Contato" },
];

const EMPTY_CONTACT: SiteContact = {
  email: "",
  phone: "",
  whatsapp: "",
  address_line: "",
  city: "",
  state: "",
  creci: "",
};

const EMPTY_BRANDING: SiteBranding = {
  logo_url: "",
  cover_url: "",
  primary_color: "",
  accent_color: "",
  theme: "light",
  tagline: "",
};

/**
 * Configuração da vitrine pública da imobiliária.
 *
 * A copy usa "site de imóveis" em todo lugar: `sites` é o nome interno do
 * serviço/schema (ADR 0006) e o comprador nunca deve ver um host de produto.
 * Publicar IMÓVEL continua sendo feito em /imoveis — aqui só se configura a
 * vitrine (marca do host, branding e as três páginas institucionais).
 */
export function Site() {
  const queryClient = useQueryClient();
  const { can } = usePermissionsContext();
  const canWrite = can("sites:write");
  const { theme } = useTheme();
  const { companies, user } = useAuth();
  const canRead = can("sites:read");
  const canLeads = can("leads:read");
  const canReadProperties = can("properties:read");
  const currentCompanyId = user?.current_company_id || companies[0]?.id || "";
  const company =
    companies.find((item) => item.id === currentCompanyId) ?? companies[0];

  const [enabled, setEnabled] = useState(false);
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [branding, setBranding] = useState<SiteBranding>(EMPTY_BRANDING);
  const [contact, setContact] = useState<SiteContact>(EMPTY_CONTACT);
  const [pages, setPages] = useState<SitePage[]>([]);
  const [slugServerError, setSlugServerError] = useState<string | null>(null);
  const [brandingServerError, setBrandingServerError] = useState<string | null>(null);
  const [activePage, setActivePage] = useState<SitePageKey>("home");
  // A hidratação é por empresa. Sem isso, trocar de imobiliária mantém o
  // formulário/slug/leads da empresa anterior no cache da mesma chave.
  const [hydratedTenant, setHydratedTenant] = useState<string | null>(null);
  const hydrated = Boolean(currentCompanyId) && hydratedTenant === currentCompanyId;

  const overview = useQuery({
    queryKey: ["site", "me", currentCompanyId],
    queryFn: () => siteService.getOverview(),
    enabled: Boolean(currentCompanyId),
  });

  // Contato vem de Configurações da empresa (fonte canônica no Toka). O site
  // guarda um snapshot para não depender de uma chamada cross-service no SSR.
  const companySettings = useQuery({
    queryKey: ["company-settings", currentCompanyId],
    queryFn: () => settingsService.getCompanySettings(),
    enabled: Boolean(currentCompanyId),
    staleTime: 5 * 60_000,
    retry: false,
  });

  const companyContact = useMemo<SiteContact>(() => {
    const address = company?.address;
    return {
      email: company?.email?.trim() ?? "",
      phone: company?.phone?.trim() ?? "",
      whatsapp: company?.phone?.trim() ?? "",
      address_line: [address?.street, address?.number, address?.district]
        .filter(Boolean)
        .join(", "),
      city: address?.city?.trim() ?? "",
      state: address?.state?.trim() ?? "",
      creci: companySettings.data?.creci?.trim() ?? "",
    };
  }, [company, companySettings.data?.creci]);

  const companyLogo = (companySettings.data?.logoPreview ?? theme.logoUrl ?? "").trim();
  const siteExists = Boolean(overview.data?.site?.id);
  const recentLeads = useQuery({
    queryKey: ["site", "leads", currentCompanyId],
    queryFn: () => siteService.leads({ limit: 5 }),
    enabled: Boolean(currentCompanyId) && siteExists && canRead,
    retry: false,
  });

  useEffect(() => {
    setActivePage("home");
    setSlugServerError(null);
    logoAutoApplied.current = null;
  }, [currentCompanyId]);

  const hydrate = (data: SiteOverview) => {
    setEnabled(Boolean(data.site?.enabled));
    setName(data.site?.name || company?.name || "");
    setSlug(data.site?.slug || "");
    setBranding({ ...EMPTY_BRANDING, ...(data.site?.branding ?? {}) });
    setContact({ ...EMPTY_CONTACT, ...(data.site?.contact ?? {}) });
    setPages(data.pages?.length ? data.pages : PAGE_TABS.map((tab) => ({ key: tab.key, title: "" })));
  };

  useEffect(() => {
    if (!overview.data || !currentCompanyId || hydratedTenant === currentCompanyId) return;
    hydrate(overview.data);
    setHydratedTenant(currentCompanyId);
    // `company` chega depois da sessão; o primeiro carregamento preenche o
    // nome. Depois disso o formulário é do usuário.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [overview.data, currentCompanyId, hydratedTenant]);

  // Se o site ainda não tem contato salvo, preenche com a empresa assim que
  // as Configurações carregam. Campos já preenchidos nunca são sobrescritos.
  // O logo da empresa é a identidade canônica; se a vitrine ainda não tem um
  // logo próprio, preenche o campo para que o próximo save o publique no
  // Payload (cabeçalho, rodapé, favicon e card da imobiliária).
  useEffect(() => {
    if (!hydrated || !companyLogo) return;
    setBranding((prev) => (prev.logo_url?.trim() ? prev : { ...prev, logo_url: companyLogo }));
  }, [hydrated, companyLogo]);

  useEffect(() => {
    if (!hydrated) return;
    setContact((prev) => {
      const next = { ...prev };
      let changed = false;
      (Object.keys(EMPTY_CONTACT) as Array<keyof SiteContact>).forEach((key) => {
        if (!String(next[key] ?? "").trim() && companyContact[key]) {
          next[key] = companyContact[key];
          changed = true;
        }
      });
      return changed ? next : prev;
    });
  }, [hydrated, companyContact]);

  const suffix = overview.data?.host_suffix ?? "";
  const scheme = overview.data?.public_scheme ?? "https";
  const effectiveSlug = slug || suggestSiteSlug(name);
  const previewUrl = previewSiteUrl(scheme, effectiveSlug, suffix, overview.data?.public_port);
  const savedUrl = overview.data?.site?.public_base_url ?? "";
  const persistedEnabled = Boolean(overview.data?.site?.enabled);
  const readiness = useQuery({
    queryKey: ["site", "readiness", currentCompanyId, savedUrl],
    queryFn: () => siteService.readiness(),
    enabled: Boolean(currentCompanyId && siteExists && persistedEnabled && savedUrl),
    // Enquanto o ACME/publicação não termina, o painel acompanha o status sem
    // oferecer um link que o Chrome abriria com certificado inválido.
    refetchInterval: (query) => (query.state.data?.ready ? false : 4000),
    retry: 1,
    staleTime: 0,
  });
  const siteReady = readiness.data?.ready === true;
  const slugError = (slug ? validateSiteSlug(slug) : null) ?? slugServerError;
  const brandingFieldError = (() => {
    const color = (value?: string) =>
      value && !/^#[0-9a-fA-F]{6}$/.test(value.trim()) ? "Use uma cor no formato #RRGGBB." : null;
    return (
      color(branding.primary_color) ||
      color(branding.accent_color) ||
      ((branding.tagline ?? "").length > 160 ? "A frase de destaque não pode passar de 160 caracteres." : null) ||
      brandingServerError
    );
  })();
  const canPublishPage = canWrite && siteExists;

  const currentPage = useMemo(
    () => pages.find((page) => page.key === activePage) ?? { key: activePage, title: "" },
    [pages, activePage],
  );

  const saveSite = useMutation({
    mutationFn: () =>
      siteService.save({
        name: name.trim(),
        slug: slug.trim(),
        enabled,
        branding,
        contact,
      }),
    onSuccess: (data) => {
      setSlugServerError(null);
      setBrandingServerError(null);
      queryClient.setQueryData(["site", "me", currentCompanyId], data);
      void queryClient.invalidateQueries({ queryKey: ["site", "readiness", currentCompanyId] });
      hydrate(data);
      toast.success(enabled ? "Site de imóveis atualizado." : "Site de imóveis desligado.");
    },
    onError: (error) => {
      const response = (error as {
        response?: {
          status?: number;
          data?: { message?: unknown; data?: { fields?: Array<{ field?: string; message?: string }> } };
        };
      })?.response;
      const status = response?.status;
      if (status === 409) {
        setSlugServerError("Essa marca já está em uso por outra imobiliária. Escolha outra.");
      } else if (status === 400) {
        const fields = response?.data?.data?.fields;
        const brandingField = fields?.find((field) => field.field?.startsWith("branding."));
        if (brandingField?.message) setBrandingServerError(brandingField.message);
        const fieldMessage = fields?.find((field) => field.field === "slug")?.message;
        const message =
          fieldMessage ||
          (typeof response?.data?.message === "string"
            ? response.data.message
            : "Slug inválido. Use letras minúsculas, números e hífen.");
        setSlugServerError(message);
      }
      notifyApiError(error, { context: "Salvar site de imóveis" });
    },
  });

  // Se a empresa já tem logo em Configurações e a vitrine ainda está sem logo,
  // aplicamos uma única vez o logo canônico. Sem logo da empresa, o site usa o
  // fallback da Suitee (não um logo/mock da Exemplo).
  const logoAutoApplied = useRef<string | null>(null);
  const applyCompanyLogo = useMutation({
    mutationFn: (logoUrl: string) =>
      siteService.save({
        name: name.trim(),
        slug: slug.trim(),
        enabled,
        branding: { ...branding, logo_url: logoUrl },
        contact,
      }),
    onSuccess: (data, logoUrl) => {
      queryClient.setQueryData(["site", "me", currentCompanyId], data);
      setBranding((prev) => ({ ...prev, logo_url: logoUrl }));
    },
    // Silencioso: o campo continua preenchido e o usuário pode salvar manualmente.
    onError: () => undefined,
  });
  useEffect(() => {
    if (!canWrite || !hydrated || !overview.data?.site?.id || !companyLogo) return;
    if (branding.logo_url?.trim() || logoAutoApplied.current === currentCompanyId) return;
    logoAutoApplied.current = currentCompanyId;
    setBranding((prev) => ({ ...prev, logo_url: companyLogo }));
    applyCompanyLogo.mutate(companyLogo);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [canWrite, hydrated, overview.data?.site?.id, companyLogo, branding.logo_url, currentCompanyId]);

  const savePage = useMutation({
    mutationFn: () => siteService.savePage(activePage, currentPage),
    onSuccess: (saved) => {
      setPages((prev) => prev.map((page) => (page.key === activePage ? { ...page, ...saved } : page)));
      void queryClient.invalidateQueries({ queryKey: ["site", "me", currentCompanyId] });
      toast.success("Página publicada no site.");
    },
    onError: (error) => notifyApiError(error, { context: "Salvar página do site" }),
  });

  const syncSite = useMutation({
    mutationFn: () => siteService.reconcile(),
    onSuccess: () => {
      toast.success("Sincronização da vitrine iniciada.");
      void queryClient.invalidateQueries({ queryKey: ["site", "me", currentCompanyId] });
    },
    onError: (error) => notifyApiError(error, { context: "Sincronizar vitrine" }),
  });

  const copyPublicUrl = async () => {
    if (!savedUrl) return;
    if (!siteReady) {
      toast.info("O site ainda está em validação. O link será liberado automaticamente.");
      return;
    }
    try {
      await navigator.clipboard.writeText(savedUrl);
      toast.success("Link do site copiado.");
    } catch {
      toast.error("Não foi possível copiar o link.");
    }
  };

  const updatePage = (patch: Partial<SitePage>) => {
    setPages((prev) =>
      prev.map((page) => (page.key === activePage ? { ...page, ...patch } : page)),
    );
  };

  const applyCompanyContact = () => {
    setContact({ ...EMPTY_CONTACT, ...companyContact });
  };

  const applyPanelIdentity = () => {
    setBranding((prev) => ({
      ...prev,
      primary_color: theme.primaryColor,
      accent_color: theme.accentColor,
      logo_url: theme.logoUrl ?? "",
      tagline: theme.loginTagline ?? "",
      theme: theme.theme === "dark" ? "dark" : "light",
    }));
  };

  const stats = overview.data?.stats;

  if (overview.isLoading) {
    return (
      <div className="flex items-center justify-center gap-2 p-10 text-sm text-muted-foreground" role="status">
        <Loader2 className="size-5 animate-spin" aria-hidden="true" />
        Carregando o site de imóveis…
      </div>
    );
  }

  if (overview.isError) {
    return (
      <div className="mx-auto max-w-4xl space-y-3 p-6" role="alert">
        <p className="text-sm text-foreground">Não foi possível carregar o site de imóveis.</p>
        <Button variant="outline" onClick={() => void overview.refetch()}>
          Tentar novamente
        </Button>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-5xl space-y-6 p-6">
      <PageHeader
        icon={<Globe className="size-6" aria-hidden="true" />}
        title="Site de imóveis"
        description={`Vitrine de ${company?.name || "sua imobiliária"}: endereço próprio, identidade visual e páginas institucionais. Os imóveis publicados em Imóveis aparecem aqui automaticamente.`}
      >
        {savedUrl && enabled ? (
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              onClick={() => void copyPublicUrl()}
              disabled={!siteReady}
              title={siteReady ? "Copiar link do site" : "Aguarde a validação do certificado"}
            >
              <Copy className="size-4" aria-hidden="true" />
              Copiar link
            </Button>
            {canWrite ? (
              <Button
                variant="outline"
                onClick={() => syncSite.mutate()}
                disabled={syncSite.isPending}
              >
                <RefreshCw
                  className={syncSite.isPending ? "size-4 animate-spin" : "size-4"}
                  aria-hidden="true"
                />
                Sincronizar
              </Button>
            ) : null}
            {siteReady ? (
              <Button variant="outline" asChild>
                <a href={savedUrl} target="_blank" rel="noreferrer">
                  <ExternalLink className="size-4" aria-hidden="true" />
                  Abrir site
                </a>
              </Button>
            ) : (
              <Button
                variant="outline"
                disabled
                title="O link será liberado quando o certificado HTTPS estiver válido"
              >
                {readiness.isLoading ? (
                  <Loader2 className="size-4 animate-spin" aria-hidden="true" />
                ) : (
                  <ShieldAlert className="size-4" aria-hidden="true" />
                )}
                {readiness.data?.status === "awaiting_publication"
                  ? "Publicando conteúdo…"
                  : "Validando certificado…"}
              </Button>
            )}
          </div>
        ) : null}
      </PageHeader>

      {siteExists && persistedEnabled ? (
        <div
          className={`flex flex-col gap-3 rounded-xl border px-4 py-3 sm:flex-row sm:items-center sm:justify-between ${
            siteReady
              ? "border-emerald-200 bg-emerald-50/70"
              : "border-amber-200 bg-amber-50/70"
          }`}
          role="status"
          aria-live="polite"
        >
          <div className="flex items-start gap-3">
            <span className={`mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-full ${siteReady ? "bg-emerald-100 text-emerald-700" : "bg-amber-100 text-amber-700"}`}>
              {siteReady ? (
                <ShieldCheck className="size-5" aria-hidden="true" />
              ) : (
                <Loader2 className="size-5 animate-spin" aria-hidden="true" />
              )}
            </span>
            <div>
              <p className="text-sm font-bold text-foreground">
                {siteReady
                  ? "Site publicado com HTTPS válido"
                  : "Publicando seu site com segurança"}
              </p>
              <p className="mt-0.5 text-xs text-muted-foreground">
                {siteReady
                  ? "O certificado de segurança foi validado. Você já pode abrir e compartilhar o site."
                  : readiness.isError
                    ? "Não foi possível confirmar o certificado agora. Estamos tentando novamente."
                    : readiness.data?.status === "awaiting_publication"
                      ? "O certificado já foi emitido. Estamos finalizando a publicação do conteúdo."
                      : "Estamos emitindo o certificado de segurança do seu domínio. O botão Abrir site será liberado automaticamente."}
              </p>
            </div>
          </div>
          {!siteReady && readiness.isError ? (
            <Button variant="outline" size="sm" onClick={() => void readiness.refetch()}>
              Verificar novamente
            </Button>
          ) : null}
        </div>
      ) : null}

      {stats ? (
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <Metric icon={<Eye className="size-4" aria-hidden="true" />} label="Visualizações (7d)" value={stats.views_7d} />
          <Metric icon={<Users className="size-4" aria-hidden="true" />} label="Leads (7d)" value={stats.leads_7d} />
          <Metric icon={<Building2 className="size-4" aria-hidden="true" />} label="Imóveis publicados" value={stats.published_listings} />
          <Metric icon={<Globe className="size-4" aria-hidden="true" />} label="Imóveis na vitrine" value={stats.total_listings} />
        </div>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base font-bold text-foreground">
            <Globe className="size-4 text-primary-600" aria-hidden="true" />
            Endereço do site
          </CardTitle>
          <CardDescription>
            Escolha a marca que aparece antes de <span className="font-semibold">imoveis</span> no
            endereço. Ela é única, estável e não muda quando o nome do imóvel muda.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border bg-muted/40 px-4 py-3">
            <div>
              <p className="text-sm font-bold text-foreground">Site de imóveis ativo</p>
              <p className="text-xs text-muted-foreground">
                Desligado, o endereço responde como inexistente e os anúncios saem da vitrine.
              </p>
            </div>
            <Switch
              checked={enabled}
              disabled={!canWrite}
              onCheckedChange={setEnabled}
              aria-label="Ativar site de imóveis"
            />
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="site-name">Nome da imobiliária</Label>
              <Input
                id="site-name"
                value={name}
                disabled={!canWrite}
                placeholder={companies[0]?.name || "Imobiliária"}
                onChange={(event) => setName(event.target.value)}
              />
              <p className="text-xs text-muted-foreground">Aparece no título das páginas e no rodapé.</p>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="site-slug">Marca do endereço</Label>
              <Input
                id="site-slug"
                value={slug}
                disabled={!canWrite}
                placeholder="sua-imobiliaria"
                autoCapitalize="none"
                spellCheck={false}
                aria-invalid={Boolean(slugError)}
                onChange={(event) => {
                  setSlugServerError(null);
                  setSlug(event.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ""));
                }}
              />
              {slugError ? (
                <p className="text-xs text-destructive" role="alert">
                  {slugError}
                </p>
              ) : (
                <p className="text-xs text-muted-foreground">
                  Use de 3 a 48 caracteres: letras minúsculas, números e hífen. Precisa ter ao menos
                  uma letra. Se ficar vazio, geramos a marca a partir do nome.
                </p>
              )}
              {canWrite ? (
                <button
                  type="button"
                  className="w-fit text-xs font-bold text-primary-700 underline underline-offset-4"
                  onClick={() => {
                    setSlugServerError(null);
                    setSlug(suggestSiteSlug(name));
                  }}
                >
                  Sugerir pelo nome da imobiliária
                </button>
              ) : null}
            </div>
          </div>

          <div className="rounded-xl border border-dashed border-border bg-muted/30 px-4 py-3">
            <p className="text-caption font-bold uppercase tracking-wider text-muted-foreground">
              Endereço do site
            </p>
            <p className="mt-1 break-all text-sm font-semibold text-foreground">
              {previewUrl || "Escolha uma marca para ver o endereço"}
            </p>
          </div>

          {canWrite ? (
            <div className="flex justify-end">
              <Button
                onClick={() => saveSite.mutate()}
                disabled={saveSite.isPending || Boolean(slugError) || Boolean(brandingFieldError) || !name.trim()}
              >
                {saveSite.isPending ? (
                  <Loader2 className="size-4 animate-spin" aria-hidden="true" />
                ) : (
                  <Save className="size-4" aria-hidden="true" />
                )}
                Salvar site
              </Button>
            </div>
          ) : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base font-bold text-foreground">
            <Palette className="size-4 text-primary-600" aria-hidden="true" />
            Identidade visual
          </CardTitle>
          <CardDescription>
            Cores e logo aplicados ao site público. Comece pela identidade do painel e ajuste o que
            for diferente.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          {brandingFieldError ? (
            <p className="rounded-xl border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs font-semibold text-destructive" role="alert">
              {brandingFieldError}
            </p>
          ) : null}
          <div className="flex flex-wrap items-end gap-6">
            <div className="flex items-center gap-3">
              <ColorInput
                label="Cor principal do site"
                value={branding.primary_color ?? ""}
                onChange={(value) => setBranding((prev) => ({ ...prev, primary_color: value }))}
              />
              <div className="space-y-1">
                <Label htmlFor="site-primary">Cor principal</Label>
                <Input
                  id="site-primary"
                  value={branding.primary_color ?? ""}
                  disabled={!canWrite}
                  maxLength={7}
                  aria-invalid={Boolean(brandingFieldError)}
                  onChange={(event) => {
                    setBrandingServerError(null);
                    setBranding((prev) => ({ ...prev, primary_color: event.target.value }));
                  }}
                />
              </div>
            </div>
            <div className="flex items-center gap-3">
              <ColorInput
                label="Cor de destaque do site"
                value={branding.accent_color ?? ""}
                onChange={(value) => setBranding((prev) => ({ ...prev, accent_color: value }))}
              />
              <div className="space-y-1">
                <Label htmlFor="site-accent">Cor de destaque</Label>
                <Input
                  id="site-accent"
                  value={branding.accent_color ?? ""}
                  disabled={!canWrite}
                  maxLength={7}
                  aria-invalid={Boolean(brandingFieldError)}
                  onChange={(event) => {
                    setBrandingServerError(null);
                    setBranding((prev) => ({ ...prev, accent_color: event.target.value }));
                  }}
                />
              </div>
            </div>
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="site-logo">URL do logo</Label>
              <Input
                id="site-logo"
                value={branding.logo_url ?? ""}
                disabled={!canWrite}
                placeholder="https://…"
                onChange={(event) => setBranding((prev) => ({ ...prev, logo_url: event.target.value }))}
              />
              {companyLogo ? (
                <button
                  type="button"
                  className="text-xs font-bold text-primary-700 underline underline-offset-4"
                  onClick={() => setBranding((prev) => ({ ...prev, logo_url: companyLogo }))}
                >
                  Usar logo da empresa
                </button>
              ) : null}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="site-tagline">Frase de destaque</Label>
              <Input
                id="site-tagline"
                value={branding.tagline ?? ""}
                disabled={!canWrite}
                maxLength={160}
                placeholder="Encontre o imóvel certo para a sua próxima etapa"
                onChange={(event) => {
                  setBrandingServerError(null);
                  setBranding((prev) => ({ ...prev, tagline: event.target.value }));
                }}
              />
              <p className="text-xs text-muted-foreground">
                {(branding.tagline ?? "").length}/160
              </p>
            </div>
            <div className="space-y-1.5 md:col-span-2">
              <Label htmlFor="site-cover">URL da imagem de capa</Label>
              <Input
                id="site-cover"
                value={branding.cover_url ?? ""}
                disabled={!canWrite}
                placeholder="https://…"
                onChange={(event) => setBranding((prev) => ({ ...prev, cover_url: event.target.value }))}
              />
              <p className="text-xs text-muted-foreground">
                Usada no hero da home e como imagem de compartilhamento quando não houver uma imagem específica.
              </p>
            </div>
          </div>

          {canWrite ? (
            <div className="flex flex-wrap justify-end gap-2">
              <Button variant="outline" onClick={applyPanelIdentity}>
                Usar identidade do painel
              </Button>
              <Button
                onClick={() => saveSite.mutate()}
                disabled={saveSite.isPending || Boolean(brandingFieldError)}
              >
                {saveSite.isPending ? (
                  <Loader2 className="size-4 animate-spin" aria-hidden="true" />
                ) : (
                  <Save className="size-4" aria-hidden="true" />
                )}
                Salvar identidade
              </Button>
            </div>
          ) : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base font-bold text-foreground">
            <Mail className="size-4 text-primary-600" aria-hidden="true" />
            Contato no site
          </CardTitle>
          <CardDescription>
            Aparece no rodapé, no card da imobiliária e nos botões de WhatsApp. Preenchemos com os
            dados da empresa; ajuste aqui se a vitrine usar outro canal.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="site-contact-email">E-mail</Label>
              <Input
                id="site-contact-email"
                type="email"
                value={contact.email ?? ""}
                disabled={!canWrite}
                placeholder="contato@imobiliaria.com.br"
                onChange={(event) => setContact((prev) => ({ ...prev, email: event.target.value }))}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="site-contact-creci">CRECI</Label>
              <Input
                id="site-contact-creci"
                value={contact.creci ?? ""}
                disabled={!canWrite}
                placeholder="CRECI-SP 00000-J"
                onChange={(event) => setContact((prev) => ({ ...prev, creci: event.target.value }))}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="site-contact-phone">Telefone</Label>
              <Input
                id="site-contact-phone"
                type="tel"
                value={contact.phone ?? ""}
                disabled={!canWrite}
                placeholder="(11) 3333-0000"
                onChange={(event) => setContact((prev) => ({ ...prev, phone: event.target.value }))}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="site-contact-whatsapp">WhatsApp</Label>
              <Input
                id="site-contact-whatsapp"
                type="tel"
                value={contact.whatsapp ?? ""}
                disabled={!canWrite}
                placeholder="(11) 99999-0000"
                onChange={(event) =>
                  setContact((prev) => ({ ...prev, whatsapp: event.target.value }))
                }
              />
            </div>
            <div className="space-y-1.5 md:col-span-2">
              <Label htmlFor="site-contact-address">Endereço</Label>
              <Input
                id="site-contact-address"
                value={contact.address_line ?? ""}
                disabled={!canWrite}
                placeholder="Rua, número e bairro"
                onChange={(event) =>
                  setContact((prev) => ({ ...prev, address_line: event.target.value }))
                }
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="site-contact-city">Cidade</Label>
              <Input
                id="site-contact-city"
                value={contact.city ?? ""}
                disabled={!canWrite}
                onChange={(event) => setContact((prev) => ({ ...prev, city: event.target.value }))}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="site-contact-state">UF</Label>
              <Input
                id="site-contact-state"
                value={contact.state ?? ""}
                disabled={!canWrite}
                maxLength={2}
                onChange={(event) =>
                  setContact((prev) => ({ ...prev, state: event.target.value.toUpperCase() }))
                }
              />
            </div>
          </div>

          {canWrite ? (
            <div className="flex flex-wrap items-center justify-between gap-2">
              <Button variant="outline" asChild>
                <Link to="/configuracoes">
                  <Settings2 className="size-4" aria-hidden="true" />
                  Editar em Configurações
                </Link>
              </Button>
              <div className="flex flex-wrap gap-2">
                <Button variant="outline" onClick={applyCompanyContact}>
                  Usar dados da empresa
                </Button>
                <Button onClick={() => saveSite.mutate()} disabled={saveSite.isPending}>
                  {saveSite.isPending ? (
                    <Loader2 className="size-4 animate-spin" aria-hidden="true" />
                  ) : (
                    <Save className="size-4" aria-hidden="true" />
                  )}
                  Salvar contato
                </Button>
              </div>
            </div>
          ) : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base font-bold text-foreground">
            <Users className="size-4 text-primary-600" aria-hidden="true" />
            Leads recentes
          </CardTitle>
          <CardDescription>
            Contatos enviados pelo formulário do site. O lead canônico fica no CRM; aqui você vê os
            últimos cinco.
          </CardDescription>
          {canLeads ? (
            <Button variant="outline" asChild>
              <Link to="/leads">Ver todos os leads</Link>
            </Button>
          ) : null}
        </CardHeader>
        <CardContent>
          {recentLeads.isLoading ? (
            <p className="text-sm text-muted-foreground">Carregando leads…</p>
          ) : recentLeads.data?.leads.length ? (
            <ul className="divide-y divide-border">
              {recentLeads.data.leads.map((lead) => (
                <li
                  key={lead.id}
                  className="flex flex-wrap items-start justify-between gap-3 py-3 first:pt-0 last:pb-0"
                >
                  <div className="min-w-0">
                    <p className="truncate text-sm font-bold text-foreground">{lead.name}</p>
                    <p className="truncate text-xs text-muted-foreground">
                      {[lead.email, lead.phone].filter(Boolean).join(" · ") || "Sem contato informado"}
                    </p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {formatLeadDate(lead.created_at)}
                      {lead.property_id ? " · imóvel do site" : ""}
                    </p>
                  </div>
                  <div className="flex flex-wrap items-center gap-3">
                    <span
                      className={
                        lead.status === "processed"
                          ? "rounded-full bg-emerald-50 px-2.5 py-1 text-caption font-bold text-emerald-700"
                          : "rounded-full bg-amber-50 px-2.5 py-1 text-caption font-bold text-amber-700"
                      }
                    >
                      {lead.status === "processed" ? "No CRM" : "CRM pendente"}
                    </span>
                    {canReadProperties && lead.property_id ? (
                      <Link
                        to={`/imoveis/${lead.property_id}`}
                        className="text-xs font-bold text-primary-700 underline underline-offset-4"
                      >
                        Ver imóvel
                      </Link>
                    ) : null}
                    {canLeads && lead.crm_lead_id ? (
                      <Link
                        to={`/leads/${lead.crm_lead_id}`}
                        className="text-xs font-bold text-primary-700 underline underline-offset-4"
                      >
                        Abrir no CRM
                      </Link>
                    ) : null}
                  </div>
                </li>
              ))}
            </ul>
          ) : (
            <p className="rounded-xl border border-dashed border-border bg-muted/30 px-4 py-3 text-sm text-muted-foreground">
              Nenhum lead recebido pelo site ainda. Compartilhe o endereço da vitrine para começar.
            </p>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base font-bold text-foreground">Páginas do site</CardTitle>
          <CardDescription>
            Textos de Início, Sobre e Contato. A vitrine de imóveis é gerada automaticamente.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex gap-1 overflow-x-auto border-b border-border" role="tablist" aria-label="Páginas do site">
            {PAGE_TABS.map((tab) => (
              <button
                key={tab.key}
                type="button"
                role="tab"
                aria-selected={activePage === tab.key}
                onClick={() => setActivePage(tab.key)}
                className={
                  activePage === tab.key
                    ? "whitespace-nowrap border-b-2 border-primary-600 px-4 py-2 text-sm font-bold text-primary-700"
                    : "whitespace-nowrap border-b-2 border-transparent px-4 py-2 text-sm font-medium text-muted-foreground hover:text-foreground"
                }
              >
                {tab.label}
              </button>
            ))}
          </div>

          <div className="grid gap-4">
            <div className="space-y-1.5">
              <Label htmlFor="page-title">Título</Label>
              <Input
                id="page-title"
                value={currentPage.title ?? ""}
                disabled={!canWrite}
                onChange={(event) => updatePage({ title: event.target.value })}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="page-subtitle">Subtítulo</Label>
              <Input
                id="page-subtitle"
                value={currentPage.subtitle ?? ""}
                disabled={!canWrite}
                onChange={(event) => updatePage({ subtitle: event.target.value })}
              />
            </div>
            <div className="grid gap-4 md:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="page-seo-title">Título de SEO</Label>
                <Input
                  id="page-seo-title"
                  value={currentPage.seo_title ?? ""}
                  disabled={!canWrite}
                  maxLength={70}
                  placeholder={currentPage.title || "Título para buscadores"}
                  onChange={(event) => updatePage({ seo_title: event.target.value })}
                />
                <p className="text-xs text-muted-foreground">
                  {(currentPage.seo_title ?? "").length}/70 · se vazio, usamos o título da página.
                </p>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="page-seo-description">Descrição de SEO</Label>
                <Textarea
                  id="page-seo-description"
                  rows={2}
                  value={currentPage.seo_description ?? ""}
                  disabled={!canWrite}
                  maxLength={180}
                  placeholder={currentPage.subtitle || "Resumo para Google e compartilhamento"}
                  onChange={(event) => updatePage({ seo_description: event.target.value })}
                />
                <p className="text-xs text-muted-foreground">
                  {(currentPage.seo_description ?? "").length}/180 · se vazio, usamos o subtítulo.
                </p>
              </div>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="page-body">Conteúdo</Label>
              <Textarea
                id="page-body"
                rows={6}
                value={currentPage.body ?? ""}
                disabled={!canWrite}
                placeholder="Conte em poucas linhas o que a sua imobiliária faz e por que confiar nela."
                onChange={(event) => updatePage({ body: event.target.value })}
              />
            </div>
          </div>

          {!siteExists ? (
            <p className="rounded-xl border border-border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
              Salve o endereço do site primeiro. Depois de criar o site, as páginas ficam disponíveis para publicação.
            </p>
          ) : null}

          {canWrite ? (
            <div className="flex justify-end">
              <Button
                onClick={() => savePage.mutate()}
                disabled={savePage.isPending || !currentPage.title?.trim() || !canPublishPage}
              >
                {savePage.isPending ? (
                  <Loader2 className="size-4 animate-spin" aria-hidden="true" />
                ) : (
                  <Save className="size-4" aria-hidden="true" />
                )}
                Publicar página
              </Button>
            </div>
          ) : null}
        </CardContent>
      </Card>
    </div>
  );
}

function Metric({ icon, label, value }: { icon: ReactNode; label: string; value: number }) {
  return (
    <div className="rounded-2xl border border-border bg-card px-4 py-3">
      <div className="flex items-center gap-2 text-xs font-bold uppercase tracking-wider text-muted-foreground">
        {icon}
        {label}
      </div>
      <p className="mt-1 text-2xl font-bold text-foreground">{value}</p>
    </div>
  );
}

function formatLeadDate(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "Data indisponível";
  return new Intl.DateTimeFormat("pt-BR", {
    day: "2-digit",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}
