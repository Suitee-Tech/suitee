/**
 * Ponto único da lógica de comprovante dos fluxos de baixa manual.
 *
 * Antes desta centralização existiam quatro cópias de `isLikelyURL` (duas com
 * comentário divergente), três cópias da mensagem de indisponibilidade de
 * armazenamento e três blocos de input de arquivo byte-idênticos. A duplicação
 * não era só estética: a mensagem de erro era escolhida por código de status
 * HTTP, sem distinguir o estágio que falhou, então um 5xx da mutação financeira
 * era apresentado ao operador como falha do armazenamento de comprovantes.
 */

/** Limite aceito pelo seletor de arquivo. O mesmo valor é comunicado na UI. */
export const MAX_PROOF_SIZE_BYTES = 20 * 1024 * 1024;

/**
 * Valida URL de comprovante. Vazio é válido: o comprovante pode vir por arquivo.
 * A obrigatoriedade é decidida por quem chama, não aqui — a regra de negócio
 * vive no backend e é refletida na interface.
 */
export const isLikelyProofURL = (value: string): boolean => {
  if (!value) return true;
  try {
    const url = new URL(value);
    return url.protocol === "http:" || url.protocol === "https:";
  } catch {
    return false;
  }
};

/**
 * Falha ocorrida ao enviar o arquivo ao armazenamento. Distinta da falha de
 * gravação financeira: as duas exigem mensagens e recuperações diferentes, e
 * confundi-las faz o operador concluir que o comprovante está indisponível
 * quando o problema foi o registro da baixa (ou o contrário).
 */
export class ProofUploadError extends Error {
  /** Causa original, para log. O tipo do target não expõe `Error.cause`. */
  readonly reason: unknown;

  constructor(cause: unknown) {
    super("Falha ao enviar o comprovante");
    this.name = "ProofUploadError";
    this.reason = cause;
  }
}

/** Mensagem de erro de armazenamento, única para todos os fluxos. */
export const PROOF_STORAGE_UNAVAILABLE_MESSAGE =
  "O armazenamento de comprovantes está temporariamente indisponível. O arquivo selecionado foi mantido; tente novamente.";

/** Mensagem de falha da gravação financeira, que não é falha de armazenamento. */
export const FINANCIAL_WRITE_FAILED_MESSAGE =
  "O comprovante foi enviado, mas o registro financeiro não pôde ser concluído. Verifique se a operação foi registrada antes de tentar novamente.";

/** Status HTTP de uma falha de axios, quando houver. */
export const httpStatusOf = (error: unknown): number | undefined =>
  (error as { response?: { status?: number } })?.response?.status;

/**
 * Extrai o erro original de um `ProofUploadError`, para que o status HTTP não
 * se perca no embrulho — do contrário um 503 do armazenamento deixaria de ser
 * reconhecido como indisponibilidade.
 */
export const proofErrorCause = (error: unknown): unknown =>
  error instanceof ProofUploadError ? error.reason : error;

/** Verdadeiro para 5xx — inclui falha do armazenamento e do backend financeiro. */
export const isServerError = (error: unknown): boolean => {
  const status = httpStatusOf(proofErrorCause(error));
  return typeof status === "number" && status >= 500;
};

/**
 * Mensagem para falha de envio/gravação do comprovante.
 *
 * `stage` identifica onde a falha ocorreu; sem ele um 5xx da mutação seria
 * atribuído ao armazenamento, que é exatamente o defeito FIN-08.
 */
export const proofErrorStageMessage = (
  error: unknown,
  stage: "upload" | "mutation",
  fallback: string,
): string => {
  if (stage === "upload") {
    return isServerError(error) ? PROOF_STORAGE_UNAVAILABLE_MESSAGE : fallback;
  }
  // Sem código de status (erro de rede) o resultado da mutação é incerto:
  // não afirmar sucesso nem falha, e orientar a verificar antes de repetir.
  return FINANCIAL_WRITE_FAILED_MESSAGE;
};
