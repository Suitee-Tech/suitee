import { describe, expect, it } from "vitest";

import { decidirDesfechoDoStream, type ChatStreamState } from "./chatStreamOutcome";

const estado = (overrides: Partial<ChatStreamState> = {}): ChatStreamState => ({
  conclusaoRecebida: false,
  erroRecebido: false,
  abortadoPorWatchdog: false,
  temConteudo: false,
  ...overrides,
});

describe("decidirDesfechoDoStream", () => {
  it("trata `done` como conclusão, com ou sem conteúdo", () => {
    expect(decidirDesfechoDoStream(estado({ conclusaoRecebida: true }))).toBe("concluido");
    expect(decidirDesfechoDoStream(estado({ conclusaoRecebida: true, temConteudo: true }))).toBe("concluido");
  });

  it("trata `error` como erro declarado, mesmo com conteúdo já renderizado", () => {
    expect(decidirDesfechoDoStream(estado({ erroRecebido: true }))).toBe("erro-declarado");
    expect(decidirDesfechoDoStream(estado({ erroRecebido: true, temConteudo: true }))).toBe("erro-declarado");
  });

  // O defeito que originou este módulo: o modelo manda o preâmbulo, a conexão
  // cai, e o usuário fica com a resposta truncada sem saber que falhou.
  it("reporta resposta incompleta quando o stream morre DEPOIS de conteúdo parcial", () => {
    expect(decidirDesfechoDoStream(estado({ temConteudo: true }))).toBe("resposta-incompleta");
  });

  // Sem conteúdo nenhum, vale reconectar uma vez antes de desistir.
  it("manda reconectar quando o stream morre sem nenhum conteúdo", () => {
    expect(decidirDesfechoDoStream(estado())).toBe("reconectar");
  });

  // O watchdog é interrupção nossa, não queda — não deve virar aviso de falha.
  it("trata abort do watchdog como interrupção, não como queda", () => {
    expect(decidirDesfechoDoStream(estado({ abortadoPorWatchdog: true }))).toBe("interrompido");
    expect(decidirDesfechoDoStream(estado({ abortadoPorWatchdog: true, temConteudo: true }))).toBe("interrompido");
  });

  // Precedência: um `done` que chega depois de um `error` (ou vice-versa) não
  // pode inverter o desfecho já conhecido.
  it("dá precedência ao erro sobre a conclusão quando ambos chegam", () => {
    expect(decidirDesfechoDoStream(estado({ erroRecebido: true, conclusaoRecebida: true }))).toBe("erro-declarado");
  });
});
