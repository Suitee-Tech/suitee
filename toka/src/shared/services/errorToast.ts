import { toast } from 'sonner';
import { isAbortedError } from '../lib/retry';

interface NotifyOptions {
  context: string;
  silent?: boolean;
}

// Translate well-known backend error strings into actionable pt-BR copy.
// Keys are matched as case-insensitive substrings against the server's
// `message` field, so a partial match like "limit exceeds maximum of 100"
// fires regardless of the exact number.
const KNOWN_MESSAGES: Array<{ pattern: RegExp; copy: string }> = [
  { pattern: /limit exceeds maximum/i, copy: 'A página solicitada é muito grande. O sistema vai recarregar paginando — tente novamente em instantes.' },
  { pattern: /invalid limit value/i, copy: 'Parâmetro de paginação inválido.' },
  { pattern: /invalid phone/i, copy: 'Telefone em formato inválido. Use DDD + número (ex: (11) 99999-9999).' },
  { pattern: /invalid_cpf|cpf inválido/i, copy: 'CPF inválido.' },
  { pattern: /invalid_cnpj|cnpj inválido/i, copy: 'CNPJ inválido.' },
  { pattern: /search required for tenant scope/i, copy: 'Informe um termo de busca para listar leads de toda a empresa.' },
  { pattern: /lead not found/i, copy: 'Lead não encontrado.' },
  { pattern: /atualizado por outra pessoa/i, copy: 'Este lead foi alterado por outra pessoa enquanto você editava. Recarregue e tente novamente.' },
  { pattern: /invalid signatory role/i, copy: 'Um dos papéis de assinatura não é compatível com este tipo de contrato.' },
  { pattern: /invalid contract type/i, copy: 'O tipo de contrato informado não é válido.' },
  { pattern: /end_date must be >= start_date/i, copy: 'A data de término não pode ser anterior à data de início.' },
];

function statusFallback(status?: number): string | null {
  switch (status) {
    case 400: return 'Requisição inválida.';
    case 403: return 'Você não tem permissão para essa ação.';
    case 404: return 'Recurso não encontrado.';
    case 409: return 'Conflito detectado. Atualize a página e tente novamente.';
    case 412: return 'O registro foi modificado por outra pessoa. Recarregue para continuar.';
    case 429: return 'Muitas requisições em pouco tempo. Aguarde um momento.';
    case 500:
    case 502:
    case 503:
    case 504:
      return 'Serviço temporariamente indisponível. Tente novamente em instantes.';
    default:
      return null;
  }
}

export function notifyApiError(err: unknown, opts: NotifyOptions): void {
  const anyErr = err as any;
  const status = anyErr?.response?.status as number | undefined;

  // 401 is handled by the axios refresh interceptor; avoid duplicate toasts.
  if (status === 401) return;

  // Aborted / canceled requests happen during navigation (first-load
  // auth transition, tab close, etc.). They are not real application
  // errors — the browser cancels the XHR and the caller already has
  // no interest in the result. Logging them only adds console noise.
  if (!status && isAbortedError(anyErr)) return;

  const rawMessage: string | undefined =
    anyErr?.response?.data?.error ??
    anyErr?.response?.data?.message ??
    anyErr?.message;

  // Prefer a translated copy over the raw backend message — the raw form
  // is often technical and not localized.
  const known = rawMessage
    ? KNOWN_MESSAGES.find(k => k.pattern.test(rawMessage))?.copy
    : undefined;
  const fallback = statusFallback(status);
  const message = known ?? rawMessage ?? fallback ?? 'Erro desconhecido';

  if (!opts.silent) {
    toast.error(`${opts.context}: ${message}`);
  }

  // 404s are often expected (e.g. probing for an optional resource); keep
  // them out of the console to reduce noise.
  if (status !== 404) {
    console.error(`[${opts.context}]`, status ?? '', err);
  }
}
