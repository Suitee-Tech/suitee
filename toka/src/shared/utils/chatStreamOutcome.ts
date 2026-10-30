/**
 * Decide o que fazer quando o stream do chat termina.
 *
 * Existe porque o stream pode fechar de três formas diferentes, e só duas delas
 * são conclusões: `done`, `error` e — a que causava o problema — **nada**. Nesse
 * último caso o backend perdeu a conexão no meio (ferramenta MCP falhando, por
 * exemplo) e o leitor do SSE resolve como se tivesse acabado.
 *
 * O sintoma que isso produzia: o modelo já tinha mandado o preâmbulo ("Vou
 * buscar as cobranças em atraso..."), o que marca a resposta como "tem
 * conteúdo", e então a conexão caía. O caminho de erro do componente só
 * disparava quando NENHUM conteúdo chegava, então a tela ficava com a resposta
 * truncada — ou com a bolha "Analisando..." parada — sem nenhuma indicação de
 * que algo tinha falhado.
 */
export type ChatStreamOutcome =
  /** Terminou de verdade: `done` chegou (com ou sem conteúdo). */
  | "concluido"
  /** Terminou com `error`: a mensagem do erro já foi mostrada. */
  | "erro-declarado"
  /** O usuário (ou o watchdog) abortou: interrupção já tratada. */
  | "interrompido"
  /** Fechou sem `done` nem `error` e sem nenhum conteúdo: vale reconectar. */
  | "reconectar"
  /** Fechou sem `done` nem `error`, mas com conteúdo parcial: avisar. */
  | "resposta-incompleta";

export interface ChatStreamState {
  /** Chegou evento `done`. */
  conclusaoRecebida: boolean;
  /** Chegou evento `error`. */
  erroRecebido: boolean;
  /** O abort partiu do watchdog de inatividade (não do usuário). */
  abortadoPorWatchdog: boolean;
  /** Algum conteúdo do assistente foi renderizado. */
  temConteudo: boolean;
}

export function decidirDesfechoDoStream(estado: ChatStreamState): ChatStreamOutcome {
  const { conclusaoRecebida, erroRecebido, abortadoPorWatchdog, temConteudo } = estado;

  if (erroRecebido) return "erro-declarado";
  if (conclusaoRecebida) return "concluido";

  // Sem `done` e sem `error`: a conexão morreu no meio.
  //
  // O watchdog é uma interrupção deliberada nossa, não uma queda: ele já segue
  // para a reconexão pelo caminho normal e não deve ser reportado como falha.
  if (abortadoPorWatchdog) return "interrompido";

  return temConteudo ? "resposta-incompleta" : "reconectar";
}

// ── Recuperação da resposta persistida ───────────────────────────────
//
// O backend continua processando depois que o cliente desconecta: a resposta é
// gerada e persistida mesmo sem ninguém lendo. Quando o stream cai no meio, a
// recuperação certa é BUSCAR o que já foi produzido — não pedir para o usuário
// enviar de novo, o que refaz a pergunta e duplica a bolha.
//
// Cobre o caso comum de trocar de tela com o chat aberto: a conexão cai na
// navegação, a resposta termina de ser gerada no servidor, e ao voltar o texto
// completo já está lá.

export interface MensagemPersistida {
  role: string;
  content?: string | null;
}

export interface RecuperacaoOpts {
  /** O stream fechou sem evento de conclusão. */
  streamCaiu?: boolean;
  /** Texto que já está na tela, para comparação. */
  parcial?: string | null;
}

export function escolherRespostaRecuperada(
  mensagens: MensagemPersistida[] | null | undefined,
  opts: RecuperacaoOpts = {},
): string | null {
  if (!mensagens || mensagens.length === 0) return null;

  // Precisa saber se houve ferramenta DEPOIS da última mensagem do assistente:
  // isso é o que distingue "o modelo só mandou o preâmbulo e ainda vai
  // sintetizar" de "essa mensagem é a resposta final".
  let ultimoAssistant = -1;
  for (let i = mensagens.length - 1; i >= 0; i--) {
    const role = mensagens[i]?.role;
    if (role === "assistant") {
      ultimoAssistant = i;
      break;
    }
    if (role === "user") return null; // pedido atual ainda não produziu resposta
  }
  if (ultimoAssistant < 0) return null;

  const texto = (mensagens[ultimoAssistant].content ?? "").trim();
  if (!texto) return null;

  const houveFerramentaDepois = mensagens
    .slice(ultimoAssistant + 1)
    .some((m) => m?.role === "tool");

  // Ferramenta depois do preâmbulo => o modelo ainda vai escrever a resposta
  // com os dados. Devolver o preâmbulo aqui marcaria como pronta uma resposta
  // que ainda não existe.
  if (houveFerramentaDepois) return null;

  return texto;
}
