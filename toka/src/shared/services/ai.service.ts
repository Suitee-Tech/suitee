import api, { API_URL, getAccessToken } from './api';

export type Conversation = {
  id: string;
  tenant_id: string;
  account_id: string;
  title: string;
  created_at: string;
  updated_at: string;
};

export type AIMessage = {
  id: string;
  tenant_id: string;
  conversation_id: string;
  role: 'system' | 'user' | 'assistant' | 'tool';
  content: string;
  tool_call_id?: string;
  tool_name?: string;
  created_at: string;
};

export type ToolEvent = {
  tool_name: string;
  tool_call_id: string;
  status: 'loading' | 'done';
  result_preview?: string;
};

export type MCPTool = {
  type: 'function';
  function: {
    name: string;
    description?: string;
    parameters: any;
  };
};

export type ToolsResponse = {
  mcp_connected: boolean;
  tools: MCPTool[];
  mcp_endpoint?: string;
  mcp_server_url_used?: string;
  error?: string;
};

export type ChatStreamEvent =
  | { type: 'message_start'; data: { conversation_id: string; mcp_connected?: boolean; tools_count?: number; build?: string } }
  | { type: 'message_delta'; data: { conversation_id: string; content: string } }
  | { type: 'tool_start'; data: { tool_name: string; tool_call_id: string } }
  | { type: 'tool_end'; data: { tool_call_id: string; result_preview: string; has_error?: boolean } }
  | { type: 'message'; data: { conversation_id: string; content: string } }
  | { type: 'done'; data: { conversation_id: string } }
  | { type: 'error'; data: { message: string; debug?: string } };

const getTenantId = (): string | null => {
  if (typeof window === 'undefined') return null;
  return localStorage.getItem('current_company_id');
};

const buildApiUrl = (path: string): string => {
  const base = new URL(API_URL, window.location.origin);
  const url = new URL(path.replace(/^\//, ''), base.toString().replace(/\/+$/, '') + '/');
  return url.toString();
};

const shouldDebugAI = (): boolean => {
  if (typeof window === 'undefined') return false;
  try {
    if (localStorage.getItem('suitee.ai.debug') === '1') return true;
  } catch {
  }
  try {
    const params = new URLSearchParams(window.location.search);
    if (params.get('ai_debug') === '1') return true;
  } catch {
  }
  return false;
};

const parseSSEBlock = (block: string, onEvent: (evt: ChatStreamEvent) => void) => {
  const lines = block.split(/\r?\n/);
  let eventType = '';
  let dataLine = '';
  for (const line of lines) {
    if (line.startsWith('event:')) eventType = line.slice('event:'.length).trim();
    if (line.startsWith('data:')) dataLine = line.slice('data:'.length).trim();
  }
  if (!eventType) return;
  let parsed: any = {};
  if (dataLine) {
    try {
      parsed = JSON.parse(dataLine);
    } catch {
      parsed = {};
    }
  }
  onEvent({ type: eventType as any, data: parsed } as ChatStreamEvent);
};

export const chatStream = async (
  conversationId: string | null,
  message: string,
  screenContext: any | undefined,
  onEvent: (e: ChatStreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> => {
  const token = getAccessToken();
  const tenantId = getTenantId();
  const debugAI = shouldDebugAI();

  const url = buildApiUrl(debugAI ? '/ai/chat?debug=1' : '/ai/chat');
  const resp = await fetch(url, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(tenantId ? { 'X-Tenant-Id': tenantId } : {}),
      ...(debugAI ? { 'X-Debug': '1' } : {}),
    },
    body: JSON.stringify({ conversation_id: conversationId ?? undefined, message, screen_context: screenContext ?? undefined }),
    signal,
  });

  if (!resp.ok || !resp.body) {
    const text = await resp.text().catch(() => '');
    console.error('[Suitee Intelligence] /ai/chat non-2xx', { status: resp.status, body: text });
    // O status acompanha o erro porque 429 (teto de uso do assistente) tem
    // tratamento próprio na interface: dizer "não consegui concluir" para um
    // limite de uso manda o usuário tentar de novo na hora e falhar de novo.
    const erro = new Error(text || `HTTP ${resp.status}`) as Error & { status?: number };
    erro.status = resp.status;
    throw erro;
  }

  const reader = resp.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const parts = buffer.split(/\r?\n\r?\n/);
    buffer = parts.pop() || '';
    for (const part of parts) {
      if (part.trim()) parseSSEBlock(part, onEvent);
    }
  }
  if (buffer.trim()) parseSSEBlock(buffer, onEvent);
};

export const confirmAction = async (
  conversationId: string,
  actionId: string,
  inputs: Record<string, any>,
  screenContext: any | undefined,
  onEvent: (e: ChatStreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> => {
  const token = getAccessToken();
  const tenantId = getTenantId();
  const debugAI = shouldDebugAI();

  const url = buildApiUrl(debugAI ? '/ai/chat?debug=1' : '/ai/chat');
  const resp = await fetch(url, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(tenantId ? { 'X-Tenant-Id': tenantId } : {}),
      ...(debugAI ? { 'X-Debug': '1' } : {}),
    },
    body: JSON.stringify({
      conversation_id: conversationId,
      confirm: { action_id: actionId, inputs },
      screen_context: screenContext ?? undefined,
    }),
    signal,
  });

  if (!resp.ok || !resp.body) {
    const text = await resp.text().catch(() => '');
    console.error('[Suitee Intelligence] /ai/chat non-2xx (confirm)', { status: resp.status, body: text });
    throw new Error(text || `HTTP ${resp.status}`);
  }

  const reader = resp.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const parts = buffer.split(/\r?\n\r?\n/);
    buffer = parts.pop() || '';
    for (const part of parts) {
      if (part.trim()) parseSSEBlock(part, onEvent);
    }
  }
  if (buffer.trim()) parseSSEBlock(buffer, onEvent);
};

export const listConversations = async (limit = 20): Promise<Conversation[]> => {
  const r = await api.get(`ai/conversations?limit=${limit}`);
  return r.data?.data ?? r.data;
};

export const listMessages = async (conversationId: string, limit = 200): Promise<AIMessage[]> => {
  const r = await api.get(`ai/conversations/${conversationId}/messages?limit=${limit}`);
  return r.data?.data ?? r.data;
};

export const deleteConversation = async (conversationId: string): Promise<void> => {
  await api.delete(`ai/conversations/${conversationId}`);
};

export const clearConversations = async (): Promise<{ deleted: number }> => {
  const r = await api.delete(`ai/conversations`);
  return r.data?.data ?? r.data;
};

export const checkAIHealth = async (): Promise<boolean> => {
  try {
    await api.get('ai/health');
    return true;
  } catch {
    return false;
  }
};

export const listTools = async (): Promise<ToolsResponse> => {
  const r = await api.get('ai/tools');
  return r.data?.data ?? r.data;
};

export const getSuggestions = async (
  screenLabel: string,
  screenPath?: string,
): Promise<Array<{ label: string; prompt: string }>> => {
  const q = encodeURIComponent(screenLabel || 'Tela atual');
  const p = encodeURIComponent(screenPath || '');
  const qs = p ? `screen_label=${q}&screen_path=${p}` : `screen_label=${q}`;
  const r = await api.get(`ai/suggestions?${qs}`);
  const data = r.data?.data ?? r.data;
  return data?.suggestions ?? [];
};

