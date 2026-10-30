import { describe, expect, it } from "vitest";

import { escolherRespostaRecuperada, type MensagemPersistida } from "./chatStreamOutcome";

const msg = (role: string, content: string): MensagemPersistida => ({ role, content });

describe("escolherRespostaRecuperada", () => {
  // O caso que motivou a recuperação: o stream cai no meio da resposta, mas o
  // backend CONTINUA gerando e persiste a resposta completa. Em vez de pedir
  // para o usuário enviar de novo (o que refaz a pergunta e duplica a bolha), o
  // cliente busca o que já foi produzido.
  it("devolve a última resposta do assistente quando o backend concluiu", () => {
    const msgs = [
      msg("user", "Quantas cobranças em atraso?"),
      msg("assistant", "Vou buscar os dados."),
      msg("tool", "{}"),
      msg("assistant", "# Cobranças em atraso\n\nNão há cobranças em atraso."),
    ];
    expect(escolherRespostaRecuperada(msgs)).toBe("# Cobranças em atraso\n\nNão há cobranças em atraso.");
  });

  // O caso real da queda no meio: o modelo mandou o preâmbulo, as ferramentas
  // rodaram, e a síntese com os dados ainda não foi escrita. Devolver o
  // preâmbulo aqui marcaria como pronta uma resposta que ainda não existe.
  it("não devolve o preâmbulo quando ainda há ferramenta depois dele", () => {
    const msgs = [
      msg("user", "Quantas cobranças em atraso?"),
      msg("assistant", "Vou buscar as cobranças em atraso."),
      msg("tool", '{"success": true}'),
    ];
    expect(escolherRespostaRecuperada(msgs)).toBeNull();
  });

  // Sem ferramenta depois, o preâmbulo É a resposta final de um pedido simples
  // (o modelo respondeu direto, sem consultar nada) — e a recuperação precisa
  // entregá-lo, senão a resposta some da tela.
  it("devolve o texto quando o modelo respondeu direto, sem ferramenta", () => {
    const msgs = [
      msg("user", "Oi, tudo bem?"),
      msg("assistant", "Tudo bem! Como posso ajudar?"),
    ];
    expect(escolherRespostaRecuperada(msgs)).toBe("Tudo bem! Como posso ajudar?");
  });

  // Defesa contra perder trabalho: se o servidor não concluiu, o texto parcial
  // que está na tela é o melhor que temos e não pode ser descartado.
  it("mantém o parcial quando a recuperação não encontra resposta nova", () => {
    const parcial = "# Cobranças em atraso\n\nNão há cobran";
    expect(escolherRespostaRecuperada([], { streamCaiu: true, parcial })).toBeNull();
  });

  it("ignora mensagens de ferramenta e de usuário ao escolher", () => {
    const msgs = [msg("user", "pergunta"), msg("tool", "{}"), msg("user", "outra")];
    expect(escolherRespostaRecuperada(msgs)).toBeNull();
  });

  it("devolve null para lista vazia ou nula", () => {
    expect(escolherRespostaRecuperada([])).toBeNull();
    expect(escolherRespostaRecuperada(null)).toBeNull();
  });

  // A recuperação não pode devolver a resposta ANTERIOR do assistente quando a
  // nova pergunta ainda não produziu nada: isso mostraria uma resposta velha
  // como se fosse a atual.
  it("não devolve resposta anterior à última pergunta", () => {
    const msgs = [
      msg("user", "pergunta antiga"),
      msg("assistant", "resposta antiga"),
      msg("user", "pergunta nova"),
    ];
    expect(escolherRespostaRecuperada(msgs)).toBeNull();
  });
});
