import api, { unwrapApiData } from './api';

/**
 * Contrato do toca-sites (ADR 0006). A vitrine é vendida como "site de
 * imóveis"; `sites` é o nome interno do serviço/schema e não deve aparecer na
 * copy. O host comercial vem montado do servidor (`public_base_url`) — o
 * frontend nunca concatena o sufixo de produção.
 */

export type SitePageKey = 'home' | 'about' | 'contact';

export interface SiteContact {
  email?: string;
  phone?: string;
  whatsapp?: string;
  address_line?: string;
  city?: string;
  state?: string;
  creci?: string;
}

export interface SiteBranding {
  logo_url?: string;
  cover_url?: string;
  primary_color?: string;
  accent_color?: string;
  theme?: string;
  tagline?: string;
}

export interface SiteRecord {
  id: string;
  tenant_id: string;
  slug: string;
  name: string;
  enabled: boolean;
  /** URL canônica já montada pelo servidor (`https://{marca}.imoveis...`). */
  public_base_url: string;
  branding: SiteBranding;
  contact: SiteContact;
  created_at?: string;
  updated_at?: string;
}

export interface SitePage {
  id?: string;
  tenant_id?: string;
  site_id?: string;
  key: SitePageKey;
  title: string;
  subtitle?: string;
  /** Texto simples. O corpo rico fica para uma fase posterior. */
  body?: string;
  seo_title?: string;
  seo_description?: string;
  updated_at?: string;
}

export interface SiteStats {
  views_7d: number;
  views_30d: number;
  leads_7d: number;
  leads_30d: number;
  published_listings: number;
  total_listings: number;
  unpublished_listings: number;
}

export interface SiteOverview {
  site: SiteRecord;
  pages: SitePage[];
  stats: SiteStats;
  host_suffix: string;
  public_scheme: string;
  /** Vazio em produção; no local acompanha a porta do Next (3002). */
  public_port?: string;
}

export type SiteReadinessStatus =
  | 'not_configured'
  | 'disabled'
  | 'awaiting_certificate'
  | 'awaiting_publication'
  | 'ready';

export interface SiteReadiness {
  ready: boolean;
  status: SiteReadinessStatus;
  public_url?: string;
  checked_at?: string;
}

export interface SaveSiteInput {
  name: string;
  slug: string;
  enabled: boolean;
  branding: SiteBranding;
  contact: SiteContact;
  pages?: SitePage[];
}

export interface ListingSiteUrl {
  published: boolean;
  public_url?: string;
  seo_slug?: string;
  status?: string;
}

export interface ListingSiteStats {
  property_id: string;
  listing_id?: string;
  seo_slug?: string;
  public_url?: string;
  status?: string;
  views_7d: number;
  views_30d: number;
  leads_7d: number;
  leads_30d: number;
  last_view_at?: string;
  published_at?: string;
}

export interface PortalSiteInfo {
  name: string;
  slug: string;
  public_url: string;
  branding?: SiteBranding;
  contact_phone?: string;
}

export interface SiteLead {
  id: string;
  name: string;
  email?: string;
  phone?: string;
  message?: string;
  property_id?: string;
  status: string;
  crm_lead_id?: string;
  origin_host: string;
  created_at: string;
}

class SiteService {
  private readonly baseUrl = '/sites';

  async getOverview(): Promise<SiteOverview> {
    const response = await api.get(this.baseUrl + '/me');
    return unwrapApiData<SiteOverview>(response);
  }

  async save(input: SaveSiteInput): Promise<SiteOverview> {
    const response = await api.put(this.baseUrl + '/me', input);
    return unwrapApiData<SiteOverview>(response);
  }

  async readiness(): Promise<SiteReadiness> {
    const response = await api.get(this.baseUrl + '/me/readiness');
    return unwrapApiData<SiteReadiness>(response);
  }

  async reconcile(): Promise<void> {
    await api.post(this.baseUrl + '/me/reconcile');
  }

  async savePage(key: SitePageKey, page: Omit<SitePage, 'key'>): Promise<SitePage> {
    const response = await api.put(`${this.baseUrl}/pages/${key}`, {
      title: page.title,
      subtitle: page.subtitle ?? '',
      // O backend guarda o corpo como JSON (jsonb); uma string JSON é
      // renderizada como texto pelo site público.
      body: page.body ?? '',
      seo_title: page.seo_title ?? '',
      seo_description: page.seo_description ?? '',
    });
    return unwrapApiData<SitePage>(response);
  }

  async listingUrl(propertyId: string): Promise<ListingSiteUrl> {
    const response = await api.get(`${this.baseUrl}/listings/${propertyId}/url`);
    return unwrapApiData<ListingSiteUrl>(response);
  }

  async listingStats(propertyId: string): Promise<ListingSiteStats> {
    const response = await api.get(`${this.baseUrl}/listings/${propertyId}/stats`);
    return unwrapApiData<ListingSiteStats>(response);
  }

  async resync(propertyId: string): Promise<ListingSiteStats> {
    const response = await api.post(`${this.baseUrl}/listings/${propertyId}/resync`);
    return unwrapApiData<ListingSiteStats>(response);
  }

  async stats(): Promise<SiteStats> {
    const response = await api.get(this.baseUrl + '/stats');
    return unwrapApiData<SiteStats>(response);
  }

  async leads(params?: { limit?: number; offset?: number }): Promise<{ leads: SiteLead[]; total: number }> {
    const response = await api.get(this.baseUrl + '/leads', { params });
    const envelope = response.data as { data?: SiteLead[]; meta?: { total?: number } };
    return { leads: envelope?.data ?? [], total: envelope?.meta?.total ?? 0 };
  }

  /**
   * Portal do cliente: devolve a URL pública SÓ quando o imóvel está publicado
   * na vitrine da própria imobiliária do deal. O tenant sai do JWT.
   */
  async portalListingUrl(propertyId: string): Promise<ListingSiteUrl> {
    return this.portalListingUrlForTenant(propertyId);
  }

  /**
   * O portal do cliente precisa do tenant da IMOBILIÁRIA do deal: o JWT dele
   * carrega o tenant da própria conta. O backend só devolve URL de anúncio
   * publicado — nunca rascunho/métrica.
   */
  async portalSite(tenantId?: string): Promise<PortalSiteInfo> {
    const response = await api.get(this.baseUrl + '/portal/site', {
      params: tenantId ? { tenant_id: tenantId } : undefined,
    });
    return unwrapApiData<PortalSiteInfo>(response);
  }

  async portalListingUrlForTenant(propertyId: string, tenantId?: string): Promise<ListingSiteUrl> {
    const response = await api.get(this.baseUrl + '/portal/listing-url', {
      params: { property_id: propertyId, ...(tenantId ? { tenant_id: tenantId } : {}) },
    });
    return unwrapApiData<ListingSiteUrl>(response);
  }
}

export const siteService = new SiteService();

/** Preview do host comercial durante a edição (antes do primeiro save). */
export function previewSiteUrl(scheme: string, slug: string, suffix: string, port?: string): string {
  const cleanSlug = slug.trim().toLowerCase();
  if (!cleanSlug || !suffix) return '';
  const cleanPort = (port ?? '').trim();
  const host = cleanPort ? `${cleanSlug}.${suffix}:${cleanPort}` : `${cleanSlug}.${suffix}`;
  return `${scheme || 'https'}://${host}`;
}

/** Validação local espelhando o contrato do servidor (feedback imediato). */
const RESERVED_SITE_SLUGS = [
  'www', 'app', 'api', 'admin', 'portal', 'sites', 'mail', 'cdn', 'imoveis', 'suitee',
  'blog', 'docs', 'status', 'suporte', 'atendimento', 'conta', 'login', 'painel',
  'sistema', 'checkout', 'assets', 'static', 'media', 'storage', 'webhook', 'webhooks',
  'root', 'null', 'undefined', 'test', 'teste', 'demo', 'dev', 'staging', 'beta',
  'imobiliaria', 'imovel',
] as const;

/** Sugestão determinística; não grava nada e não garante unicidade global. */
export function suggestSiteSlug(name: string): string {
  return name
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 48)
    .replace(/-+$/g, '');
}

export function validateSiteSlug(slug: string): string | null {
  if (!slug) return 'Escolha a marca do endereço do site.';
  if (slug.length < 3 || slug.length > 48) return 'Use de 3 a 48 caracteres.';
  if (!/^[a-z0-9-]+$/.test(slug)) return 'Use apenas letras minúsculas, números e hífen.';
  if (slug.startsWith('-') || slug.endsWith('-')) return 'Não pode começar nem terminar com hífen.';
  if (slug.includes('--')) return 'Não use hífens seguidos.';
  if (!/[a-z]/.test(slug)) return 'Use pelo menos uma letra na marca.';
  if ((RESERVED_SITE_SLUGS as readonly string[]).includes(slug)) {
    return 'Essa marca é reservada.';
  }
  return null;
}
