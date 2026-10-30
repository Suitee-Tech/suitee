/**
 * Extrai a mensagem que o SERVIDOR devolveu para uma falha de API.
 *
 * Regra da tela de inadimplência: quando a resposta é 4xx/5xx, o usuário vê a
 * causa real (ex.: "invalid collection status transition: ACTIVE -> LEGAL_ACTION",
 * "número CNJ malformado") — nunca um toast de sucesso nem um texto genérico que
 * esconda o motivo. Mesmo padrão de `selectErroServidor` em `Subscription.tsx`.
 */
export function serverMessage(err: unknown, fallback: string): string {
  const data = (err as { response?: { data?: { message?: unknown; error?: unknown } } })?.response?.data;
  const fromServer =
    (typeof data?.message === 'string' && data.message.trim()) ||
    (typeof data?.error === 'string' && data.error.trim()) ||
    '';
  if (fromServer) return fromServer;

  const status = (err as { response?: { status?: number } })?.response?.status;
  if (typeof status === 'number' && status >= 400) {
    return `${fallback} (HTTP ${status})`;
  }
  return fallback;
}
