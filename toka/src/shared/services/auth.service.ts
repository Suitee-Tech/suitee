import api, { setAccessToken } from './api';
import { runSessionRefresh } from './sessionRefresh';
import { clearAgentsCache } from '../lib/agentsCache';

export interface Company {
 id: string;
 name: string;
 legal_name?: string;
 tax_id: string;
 email: string;
 phone?: string;
 address?: {
 street: string;
 number: string;
 complement?: string;
 district: string;
 city: string;
 state: string;
 zip_code: string;
 };
 type: string;
 is_active: boolean;
 profile?: string;
 members?: Array<{ account_id: string, profile: string }>;
}

interface AccountResponse {
 id: string;
 type: string;
 company?: {
 id: string;
 name: string;
 legal_name?: string;
 tax_id: string;
 email: string;
 phone?: string;
 address?: {
 street: string;
 number: string;
 complement?: string;
 district: string;
 city: string;
 state: string;
 zip_code: string;
 };
 type: string;
 is_active: boolean;
 owner_id?: string;
 members?: Array<{ account_id: string, profile: string }>;
 };
}

export interface User {
 id: string;
 email: string;
 firstname: string;
 lastname: string;
 fullname?: string;
 role: string;
 functional_role?: string;
 account_type?: string;
 permission?: string;
 avatar_url?: string;
 companies?: Company[];
 current_company_id?: string;
}

export interface LoginResponse {
 access_token: string;
 refresh_token?: string;
}

export interface RegisterResponse {
 access_token: string;
 refresh_token?: string;
}

const extractData = <T>(response: any): T => {
 return response.data?.data || response.data;
};

const shouldFallbackEndpoint = (error: any) => {
 const status = error?.response?.status;
 return status === 404 || status === 405;
};

const tryGet = async <T>(paths: string[]): Promise<T> => {
  let lastError: unknown;
  for (const path of paths) {
    try {
      const response = await api.get<any>(path);
      return extractData<T>(response);
    } catch (error: any) {
      lastError = error;
      const status = error?.response?.status;
      if (status === 429) {
        throw error;
      }
      if (!shouldFallbackEndpoint(error)) throw error;
    }
  }
  throw lastError;
};

const tryPost = async <T>(paths: string[], body: unknown): Promise<T> => {
 let lastError: unknown;
 for (const path of paths) {
 try {
 const response = await api.post<any>(path, body);
 return extractData<T>(response);
 } catch (error: any) {
 lastError = error;
 if (!shouldFallbackEndpoint(error)) throw error;
 }
 }
 throw lastError;
};

export const login = async (email: string, password: string): Promise<LoginResponse> => {
  const response = await api.post<any>('/auth/login', { email, password }, {
    headers: { 'X-Auth-Version': 'legacy' },
  });
  const data = extractData<LoginResponse>(response);
  setAccessToken(data.access_token);
  return data;
};

export interface RegisterRequest {
 email: string;
 password: string;
 confirm_password: string;
 type: 'individual' | 'company';
 firstname?: string;
 lastname?: string;
 company_name?: string;
 tax_id?: string;
 phone?: string;
 bio?: string;
 role?: string;
}

export const register = async (data: RegisterRequest): Promise<RegisterResponse> => {
  const response = await api.post<any>('/auth/register', data, {
    headers: { 'X-Auth-Version': 'legacy' },
  });
  const tokens = extractData<RegisterResponse>(response);
  setAccessToken(tokens.access_token);
  return tokens;
};

export const logout = async (): Promise<void> => {
  try {
    await api.post('/auth/logout-all', {});
  } catch {}
  setAccessToken(null);
  localStorage.removeItem('current_company_id');
  clearAgentsCache();
};

export const forgotPassword = async (email: string): Promise<void> => {
  await api.post('/auth/forgot-password', { email });
};

/**
 * Aplica a nova senha a partir da sessão de recuperação do link.
 *
 * A troca acontece no NOSSO backend (`POST /auth/reset-password`), que valida o
 * par access/refresh no provedor e usa a API admin para gravar a senha. O
 * navegador deixou de falar com o Supabase aqui — era o único uso do
 * supabase-js que sobrava, e portanto o único motivo para a URL e a anon key do
 * Supabase existirem em /env.js (um valor com cara de segredo visível para
 * qualquer usuário). Ver docs/deployment/hostinger.md § env.js.
 */
export const resetPasswordWithRecoverySession = async (
  accessToken: string,
  refreshToken: string,
  password: string,
  tokenHash?: string,
  expiresAt?: number,
  signature?: string,
): Promise<void> => {
  await api.post('/auth/reset-password', tokenHash
    ? { token_hash: tokenHash, exp: expiresAt, sig: signature, new_password: password }
    : { access_token: accessToken, refresh_token: refreshToken, new_password: password });
};

/**
 * O fragmento do link de recuperação (#access_token=...&type=recovery) é apagado
 * da URL pelo próprio cliente Supabase assim que o módulo carrega
 * (`detectSessionInUrl`). Lendo `window.location.hash` depois disso, a tela de
 * redefinição encontrava vazio e concluía "link inválido ou expirado" — mesmo
 * com o link correto, com a sessão já disponível.
 *
 * Por isso o fragmento é capturado UMA vez na carga do módulo, antes do cliente
 * processá-lo, e usado como fonte confiável.
 */
const recoveryHashSnapshot: string = (() => {
  if (typeof window === 'undefined') return '';
  const hash = window.location.hash || '';
  return hash.includes('type=recovery') &&
    (hash.includes('access_token') || hash.includes('token_hash'))
    ? hash
    : '';
})();

export type RecoverySession = {
  accessToken: string;
  refreshToken: string;
  /** Link próprio: o backend troca o hash por uma sessão no GoTrue. */
  tokenHash?: string;
  /** Validade assinada pelo backend (epoch seconds). */
  expiresAt?: number;
  signature?: string;
};

export const getRecoverySessionFromUrl = async (): Promise<RecoverySession | null> => {
  // Leitura pura do fragmento: o cliente Supabase saiu do navegador, então
  // ninguém mais limpa o hash antes desta leitura (era o motivo do fallback).
  // Preferência pelo instantâneo: o hash ao vivo já pode ter sido limpo.
  const hash = recoveryHashSnapshot || window.location.hash || '';
  const params = new URLSearchParams(hash.startsWith('#') ? hash.slice(1) : hash);
  const accessToken = params.get('access_token') || '';
  const refreshToken = params.get('refresh_token') || '';
  const tokenHash = params.get('token_hash') || '';
  const expiresAt = Number(params.get('exp') || 0);
  const signature = params.get('sig') || '';
  const type = params.get('type') || '';
  if (type === 'recovery' && tokenHash) {
    // Sem exp/sig o link foi adulterado ou é de um formato antigo: não
    // entregamos o formulário. O backend também rejeita.
    if (!expiresAt || !signature || expiresAt <= Math.floor(Date.now() / 1000)) return null;
    return { accessToken: '', refreshToken: '', tokenHash, expiresAt, signature };
  }
  if (type === 'recovery' && accessToken && refreshToken) {
    return { accessToken, refreshToken };
  }
  return null;
};

export const getInvitation = async (token: string): Promise<any> => {
  const response = await api.get(`/auth/invitations/${token}`);
  return extractData(response);
};

export const acceptPublicInvitation = async (token: string, data?: Partial<RegisterRequest>): Promise<LoginResponse> => {
  const response = await api.post(`/auth/invitations/${token}/accept`, data || {}, {
    headers: { 'X-Auth-Version': 'legacy' },
  });
  return extractData<LoginResponse>(response);
};

export const getCurrentUser = async (): Promise<User> => {
 const response = await api.get<any>('/auth/me');
 return extractData<User>(response);
};

export const getUserCompanies = async (currentUserParam?: User): Promise<Company[]> => {
  const currentUser = currentUserParam || await api.get<any>('/auth/me').then(res => extractData<User>(res));
  const accounts = await tryGet<AccountResponse[] | any>(['/auth/companies']);

  if (!Array.isArray(accounts)) return [];

 return accounts
 .filter(acc => acc.type === 'company' && acc.company)
 .map(acc => {
 const member = acc.company?.members?.find((m: any) => m.account_id === currentUser?.id);
 return {
 id: acc.id,
 name: acc.company!.name,
 legal_name: acc.company!.legal_name,
 tax_id: acc.company!.tax_id,
 email: acc.company!.email,
 phone: acc.company!.phone,
 address: acc.company!.address,
 type: acc.type,
 is_active: acc.company!.is_active,
      profile: member?.profile || (acc.company!.owner_id === currentUser?.id ? 'owner' : 'member'),
      members: acc.company!.members,
    };
  });
};

export interface CreateCompanyRequest {
 name: string;
 legal_name: string;
 tax_id: string;
 email: string;
 phone: string;
 address: {
 street: string;
 number: string;
 complement?: string;
 district: string;
 city: string;
 state: string;
 zip_code: string;
 };
 type: string;
}

export const createCompany = async (data: CreateCompanyRequest): Promise<Company> => {
 const response = await api.post<any>('/companies', data);
 const acc = extractData<AccountResponse>(response);

 if (acc.type === 'company' && acc.company) {
 return {
 id: acc.id,
 name: acc.company.name,
 legal_name: acc.company.legal_name,
 tax_id: acc.company.tax_id,
 email: acc.company.email,
 phone: acc.company.phone,
 address: acc.company.address,
 type: acc.type,
      is_active: acc.company.is_active,
      profile: 'owner',
      members: acc.company.members,
    };
  }
 throw new Error("Invalid company response");
};

export const getCompany = async (id: string): Promise<Company> => {
 const [response, currentUser] = await Promise.all([
 api.get<any>(`/companies/${id}`),
 getCurrentUser(),
 ]);
 const acc = extractData<AccountResponse>(response);

 if (acc.type === 'company' && acc.company) {
 const member = acc.company.members?.find((m: any) => m.account_id === currentUser.id);
 return {
 id: acc.id,
 name: acc.company.name,
 legal_name: acc.company.legal_name,
 tax_id: acc.company.tax_id,
 email: acc.company.email,
 phone: acc.company.phone,
 address: acc.company.address,
 type: acc.type,
 is_active: acc.company.is_active,
    profile: member?.profile || (acc.company.owner_id === currentUser.id ? 'owner' : 'member'),
    members: acc.company.members,
  };
}
 throw new Error("Invalid company response");
};

export const switchContext = async (companyId: string): Promise<LoginResponse> => {
  const body = { company_id: companyId };
  const paths = ['/auth/switch-context', '/auth/switch-company'];
  let lastError: any;

  for (const path of paths) {
    try {
      const response = await api.post<any>(path, body, {
        headers: { 'X-Auth-Version': 'legacy' },
      });
      const tokens = extractData<LoginResponse>(response);
      setAccessToken(tokens.access_token);
      return tokens;
    } catch (error: any) {
      lastError = error;
      if (!shouldFallbackEndpoint(error)) throw error;
    }
  }
  throw lastError;
};

/**
 * Renova a sessão pelo cookie de refresh.
 *
 * Passa pelo single-flight de `sessionRefresh`: o boot do AuthProvider, o
 * interceptor do httpClient e o SessionRecoveryProvider podem chamar isto ao
 * mesmo tempo (volta do foco com token vencido, várias consultas em 401) — e
 * duas trocas concorrentes do mesmo refresh token fariam o provedor tratar
 * reuso como vazamento e derrubar a sessão.
 */
export const refreshTokenRequest = async (): Promise<LoginResponse> =>
  runSessionRefresh(async () => {
    const response = await api.post<any>('/auth/refresh', {});
    const data = extractData<LoginResponse>(response);
    setAccessToken(data.access_token);
    return data;
  });

export const requestOtp = async (contact: string, method: 'email' | 'whatsapp' = 'email'): Promise<void> => {
  await api.post('/auth/otp/request', { contact, method });
};

export const verifyOtp = async (contact: string, code: string): Promise<LoginResponse> => {
  const response = await api.post<any>('/auth/otp/verify', { contact, code });
  return extractData<LoginResponse>(response);
};

