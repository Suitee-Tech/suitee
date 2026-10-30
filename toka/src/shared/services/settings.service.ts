import api, { unwrapApiData } from './api';

// --- User & Profile Types ---

// Per-scope view state set by the UI (e.g. selected viewMode, filter values,
// sort key). Free-form because each scope owns its own keys.
export type ViewPreference = Record<string, unknown>;

/**
 * LGPD-Art.7 consents. Issue #478. Cada timestamp marca quando o
 * consent foi dado OU revogado pela última vez — auditoria distingue
 * via before/after no log.
 */
export interface UserConsent {
  marketing_emails: boolean;
  marketing_emails_at?: string;
  marketing_sms: boolean;
  marketing_sms_at?: string;
  marketing_whatsapp: boolean;
  marketing_whatsapp_at?: string;
  /** Versão dos Termos/Política aceita. Empty = nunca aceitou explicitamente. */
  terms_version?: string;
  terms_accepted_at?: string;
}

export interface UserPreferences {
  id?: string;
  account_id: string;
  language: string;
  timezone: string;
  date_format: string;
  session_timeout: number;
  focus_mode: boolean;
  last_company_id?: string;
  view_preferences?: Record<string, ViewPreference>;
  /** LGPD consents (#478). */
  consent?: UserConsent;
}

export interface UpdateProfileDTO {
  firstname: string;
  lastname: string;
  phone?: string;
  bio?: string;
  avatar_url?: string;
}

export interface ChangePasswordDTO {
  current_password?: string; // made optional for standard matching if empty
  new_password?: string;
}

// --- Company Settings Types ---

export interface CompanyBusinessHours {
  timezone?: string;
  windows?: Record<string, { start: string; end: string }>;
}

export interface CompanySettings {
  id?: string;
  tenant_id: string;
  /**
   * Fuso da imobiliária (Configurações → "Fuso horário"). Manda nos dias civis
   * dos fluxos — lançamentos, relatórios, agendamentos — via rota interna do
   * core. Vazio significa o padrão do produto (Brasília, GMT-3).
   */
  timezone?: string;
  creci: string;
  website: string;
  horarioAbertura: string;
  horarioFechamento: string;
  diasOperacao: string[];
  businessHours?: CompanyBusinessHours;
  logoPreview?: string;
  currency: string;
  autoSave: boolean;
  compactTables: boolean;
}

export interface AppearanceSettings {
  id?: string;
  tenant_id: string;
  primaryColor: string;
  accentColor: string;
  sidebarStyle: string;
  theme: string;
  fontFamily: string;
  iconStyle: string;
  showLogoLogin: boolean;
  darkModeEnabled: boolean;
  customDomain: string;
  domainVerified: boolean;
  loginTagline: string;
  /** Segunda linha do painel de login (white label). */
  loginSubtitle: string;
  /** Id de um preset de `shared/loginBanners.ts`. */
  loginBannerPreset: string;
  /** Imagem própria da imobiliária, guardada mesmo quando o preset está ativo. */
  loginBannerUrl: string;
  /**
   * Qual das duas imagens está ativa: `custom` (a enviada) ou `preset`. As duas
   * ficam gravadas, então alternar entre elas não reenvia nem apaga arquivo.
   */
  loginBannerSource: string;
  emailFooter: string;
}

export interface Permission {
  id: string;
  label: string;
  module: string;
  roles: Record<string, boolean>; // Keys will be FunctionalRole
}

/**
 * Perfil de acesso personalizado do tenant (Configurações → Permissões →
 * Perfis Personalizados). É a unidade ATRIBUÍVEL por um Grupo de Usuários: o
 * grupo aponta para o perfil e o membro passa a ter o id dele como
 * `functional_role`, decidido no servidor por `settings.RolePermissionsFor`.
 *
 * `permissions` é o conjunto EFETIVO concedido — não é rótulo decorativo.
 */
export interface CustomRole {
  id: string;
  name: string;
  description?: string;
  baseRole: string;
  color: string;
  permissions?: string[];
}

export interface AccessProfileInput {
  name: string;
  description?: string;
  baseRole?: string;
  color?: string;
  /** Ausente na criação = copiar do papel base; ausente na edição = preservar. */
  permissions?: string[];
}

export interface RolePermissions {
  id?: string;
  tenant_id: string;
  defaultRole: string;
  permissions: Permission[];
  customRoles: CustomRole[];
}

export interface NotificationChannel {
  id: string;
  label: string;
  email: boolean;
  whatsapp: boolean;
  inApp: boolean;
  category: string;
}

export interface NotificationSettings {
  id?: string;
  tenant_id: string;
  channels: NotificationChannel[];
  deliveryFreq: string;
  quietHoursStart: string;
  quietHoursEnd: string;
}

export interface WebhookEndpoint {
  id?: string;
  tenant_id?: string;
  url: string;
  events: string[];
  active: boolean;
  secret?: string;
  lastTriggered?: string;
}

export interface APIKey {
  id?: string;
  tenant_id?: string;
  name: string;
  keyPrefix?: string;
  scopes: string[];
  isActive?: boolean;
  lastUsed?: string;
  expiresAt?: string;
}

export interface CreateAPIKeyResponse {
  api_key: APIKey;
  raw_key: string;
}

export interface CompanyMember {
  account_id: string;
  name: string;
  email: string;
  profile: string;
  functional_role?: string;
  status: string;
  avatar_url?: string;
  /**
   * P2/M7 — vínculo suspenso. O backend devolve `false` para membro desativado
   * (campo ausente em vínculos antigos = ativo).
   */
  is_active?: boolean;
}

/** Campos aceitos pelo PATCH de membro (P2/M6+M7). */
export interface UpdateCompanyMemberPayload {
  functional_role?: string;
  profile?: string;
  is_active?: boolean;
}

export interface GroupMember {
  account_id: string;
  added_at: string;
  added_by: string;
  name: string;
  email: string;
  avatar_url: string;
}

export interface Group {
  id: string;
  company_id: string;
  name: string;
  description: string;
  /**
   * Papel do catálogo OU id de um perfil de acesso (`cr_…`) — é o que o grupo
   * concede aos membros. `is_default` marca os nove grupos de papel criados pelo
   * sistema, que não podem ser renomeados nem excluídos.
   */
  role_id: string;
  is_default?: boolean;
  members: GroupMember[];
  created_by: string;
  created_at: string;
  updated_at: string;
}

// API methods
export const settingsService = {
  // User Profile
  updateProfile: async (data: UpdateProfileDTO): Promise<void> => {
    await api.put('/settings/profile', data);
  },

  uploadAvatar: async (file: File): Promise<{ avatar_url: string }> => {
    const formData = new FormData();
    formData.append('avatar', file);
    const res = await api.post('/settings/profile/avatar', formData);
    return unwrapApiData(res);
  },

  changePassword: async (data: ChangePasswordDTO): Promise<void> => {
    await api.post('/settings/change-password', data);
  },

  // User Preferences
  getPreferences: async (): Promise<UserPreferences> => {
    const response = await api.get('/settings/preferences');
    return unwrapApiData<UserPreferences>(response);
  },

  updatePreferences: async (data: Partial<UserPreferences>): Promise<void> => {
    await api.put('/settings/preferences', data);
  },

  /**
   * Patch parcial (merge). Use quando você quer mudar 1-2 campos sem
   * sobrescrever os outros. O backend faz merge por chave; consent é
   * merge granular (por canal). LGPD #478.
   */
  patchPreferences: async (data: Partial<UserPreferences>): Promise<void> => {
    await api.patch('/settings/preferences', data);
  },

  /**
   * LGPD Art. 9 — baixa os dados de conta e preferências mantidos por
   * toca-auth e referências de acesso a dados em outros serviços. Não
   * representa uma exportação integral cross-service. Issue #475.
   * Side-effect: registra "lgpd.data_export_requested" no backend.
   */
  getMyLGPDData: async (): Promise<unknown> => {
    const res = await api.get('/lgpd/me/data');
    return unwrapApiData(res);
  },

  /**
   * LGPD Art. 18 — solicita anonimização nos domínios registrados.
   * O backend pode retornar falhas parciais, que precisam ser
   * informadas ao usuário. Issue #476. Exige confirmação literal.
   *
   * Cabe à UI informar o resultado e encerrar a sessão após a
   * solicitação, inclusive quando outro domínio precisar de suporte.
   */
  deleteMyAccount: async (confirm: string, reason?: string): Promise<unknown> => {
    const res = await api.delete('/lgpd/me/account', { data: { confirm, reason } });
    return unwrapApiData(res);
  },

  // Company Information (Basic)
  updateCompanyAccount: async (companyId: string, data: any): Promise<void> => {
    await api.put(`/companies/${companyId}`, data);
  },

  createInvitation: async (companyId: string, email: string, role: string): Promise<void> => {
    await api.post(`/companies/${companyId}/invite`, { email, role });
  },

  listInvitations: async (companyId: string): Promise<any[]> => {
    const response = await api.get(`/companies/${companyId}/invitations`);
    return unwrapApiData<any[]>(response);
  },

  listCompanyMembers: async (companyId: string): Promise<CompanyMember[]> => {
    const response = await api.get(`/companies/${companyId}/members`);
    return unwrapApiData<CompanyMember[]>(response);
  },

  removeCompanyMember: async (companyId: string, memberId: string): Promise<void> => {
    await api.delete(`/companies/${companyId}/members/${memberId}`);
  },

  /**
   * P2/M6+M7 — altera papel/perfil do membro e suspende/reativa o vínculo sem
   * removê-lo da empresa. Antes só existia remover: mudar papel exigia passar
   * por grupo e não havia como bloquear alguém.
   */
  updateCompanyMember: async (
    companyId: string,
    memberId: string,
    data: UpdateCompanyMemberPayload,
  ): Promise<CompanyMember | undefined> => {
    const response = await api.patch(`/companies/${companyId}/members/${memberId}`, data);
    return unwrapApiData<CompanyMember>(response);
  },

  resendInvitation: async (companyId: string, invitationId: string): Promise<void> => {
    await api.post(`/companies/${companyId}/invitations/${invitationId}/resend`, {});
  },

  cancelInvitation: async (companyId: string, invitationId: string): Promise<void> => {
    await api.post(`/companies/${companyId}/invitations/${invitationId}/cancel`, {});
  },

  // Groups
  createGroup: async (
    companyId: string,
    data: { name: string; description?: string; functional_role?: string },
  ): Promise<any> => {
    const res = await api.post(`/companies/${companyId}/groups`, data);
    return unwrapApiData(res);
  },

  listGroups: async (companyId: string): Promise<Group[]> => {
    const res = await api.get(`/companies/${companyId}/groups`);
    return unwrapApiData(res);
  },

  getGroup: async (companyId: string, groupId: string): Promise<Group> => {
    const res = await api.get(`/companies/${companyId}/groups/${groupId}`);
    return unwrapApiData(res);
  },

  updateGroup: async (
    companyId: string,
    groupId: string,
    data: { name: string; description?: string; functional_role?: string },
  ): Promise<void> => {
    await api.put(`/companies/${companyId}/groups/${groupId}`, data);
  },

  deleteGroup: async (companyId: string, groupId: string): Promise<void> => {
    await api.delete(`/companies/${companyId}/groups/${groupId}`);
  },

  addGroupMember: async (companyId: string, groupId: string, accountId: string): Promise<void> => {
    await api.post(`/companies/${companyId}/groups/${groupId}/members`, { account_id: accountId });
  },

  removeGroupMember: async (companyId: string, groupId: string, accountId: string): Promise<void> => {
    await api.delete(`/companies/${companyId}/groups/${groupId}/members/${accountId}`);
  },

  moveMember: async (companyId: string, groupId: string, accountId: string, targetGroupId: string): Promise<void> => {
    await api.post(`/companies/${companyId}/groups/${groupId}/members/${accountId}/move`, { target_group_id: targetGroupId });
  },

  // Company Settings
  getCompanySettings: async (): Promise<CompanySettings> => {
    const response = await api.get('/company-settings');
    return unwrapApiData<CompanySettings>(response);
  },

  updateCompanySettings: async (data: Partial<CompanySettings>): Promise<void> => {
    await api.put('/company-settings', data);
  },

  uploadCompanyLogo: async (file: File): Promise<{ logo_url: string }> => {
    const formData = new FormData();
    formData.append('logo', file);
    const res = await api.post('/company-settings/logo', formData);
    return unwrapApiData(res);
  },

  uploadLoginBanner: async (file: File): Promise<{ banner_url: string }> => {
    const formData = new FormData();
    formData.append('banner', file);
    const res = await api.post('/company-settings/login-banner', formData);
    return unwrapApiData(res);
  },

  // Appearance
  getAppearance: async (): Promise<AppearanceSettings> => {
    const response = await api.get('/company-settings/appearance');
    return unwrapApiData<AppearanceSettings>(response);
  },

  updateAppearance: async (data: Partial<AppearanceSettings>): Promise<void> => {
    await api.put('/company-settings/appearance', data);
  },

  // Permissions
  getPermissions: async (): Promise<RolePermissions> => {
    const response = await api.get('/company-settings/permissions');
    return unwrapApiData<RolePermissions>(response);
  },

  updatePermissions: async (data: Partial<RolePermissions>): Promise<void> => {
    await api.put('/company-settings/permissions', data);
  },

  // Perfis de acesso personalizados (Configurações → Permissões).
  //
  // Operação própria e auditada — o PUT da matriz NÃO grava perfis (um
  // documento replace apagaria o acesso de quem usa o perfil). Por isso o
  // editor da tela salva por aqui, não junto do "Salvar" da matriz.
  listAccessProfiles: async (): Promise<CustomRole[]> => {
    const response = await api.get('/company-settings/access-profiles');
    return unwrapApiData<CustomRole[]>(response) || [];
  },

  createAccessProfile: async (data: AccessProfileInput): Promise<CustomRole> => {
    const response = await api.post('/company-settings/access-profiles', data);
    return unwrapApiData<CustomRole>(response);
  },

  updateAccessProfile: async (profileId: string, data: AccessProfileInput): Promise<CustomRole> => {
    const response = await api.put(`/company-settings/access-profiles/${profileId}`, data);
    return unwrapApiData<CustomRole>(response);
  },

  deleteAccessProfile: async (profileId: string): Promise<void> => {
    await api.delete(`/company-settings/access-profiles/${profileId}`);
  },

  getMyPermissions: async (): Promise<{ functional_role: string; profile: string; permissions: string[] }> => {
    const response = await api.get('/me/permissions');
    return unwrapApiData(response);
  },

  // Notifications
  getNotifications: async (): Promise<NotificationSettings> => {
    const response = await api.get('/company-settings/notifications');
    return unwrapApiData<NotificationSettings>(response);
  },

  updateNotifications: async (data: Partial<NotificationSettings>): Promise<void> => {
    await api.put('/company-settings/notifications', data);
  },

  // Webhooks
  listWebhooks: async (): Promise<WebhookEndpoint[]> => {
    const response = await api.get('/company-settings/webhooks');
    return unwrapApiData<WebhookEndpoint[]>(response) || [];
  },

  createWebhook: async (data: WebhookEndpoint): Promise<WebhookEndpoint> => {
    const response = await api.post('/company-settings/webhooks', data);
    return unwrapApiData<WebhookEndpoint>(response);
  },

  updateWebhook: async (id: string, data: Partial<WebhookEndpoint>): Promise<void> => {
    await api.put(`/company-settings/webhooks/${id}`, data);
  },

  deleteWebhook: async (id: string): Promise<void> => {
    await api.delete(`/company-settings/webhooks/${id}`);
  },

  // API Keys
  listAPIKeys: async (): Promise<APIKey[]> => {
    const response = await api.get('/company-settings/api-keys');
    return unwrapApiData<APIKey[]>(response) || [];
  },

  createAPIKey: async (data: APIKey): Promise<CreateAPIKeyResponse> => {
    const response = await api.post('/company-settings/api-keys', data);
    return unwrapApiData<CreateAPIKeyResponse>(response);
  },

  revokeAPIKey: async (id: string): Promise<void> => {
    await api.delete(`/company-settings/api-keys/${id}`);
  },
};

export default settingsService;
