import { describe, expect, it } from "vitest";

import type { SignatureEnvelope, SignatureEnvelopeSigner } from "../../contracts/types";
import {
  isEnvelopeActive,
  isEnvelopeTerminal,
  isSignatureInFlight,
  signatureProgress,
  signatureProgressLabel,
  signerTurnState,
  waitingTurnLabel,
} from "./signingProgress";

const signer = (overrides: Partial<SignatureEnvelopeSigner>): SignatureEnvelopeSigner =>
  ({
    signer_id: "s1",
    name: "Fulano",
    email: "fulano@example.com",
    role: "sign",
    auth: [],
    status: "PENDING",
    ...overrides,
  }) as SignatureEnvelopeSigner;

const envelope = (signers: SignatureEnvelopeSigner[], status = "RUNNING"): SignatureEnvelope =>
  ({
    id: "e1",
    tenant_id: "t1",
    contract_id: "c1",
    environment: "sandbox",
    status,
    signers,
    created_at: "2026-10-05T13:00:00Z",
    updated_at: "2026-10-05T13:00:00Z",
  }) as SignatureEnvelope;

describe("estado parcial da assinatura", () => {
  // Este é o estado que a tela não cobria: o contrato sai de "aguardando
  // assinatura" no primeiro signatário e permanece parcial até o último.
  it("reconhece o contrato em assinatura parcial como assinatura em andamento", () => {
    expect(isSignatureInFlight("PARTIALLY_SIGNED")).toBe(true);
    expect(isSignatureInFlight("PENDING_SIGNATURE")).toBe(true);
    expect(isSignatureInFlight("DRAFT")).toBe(true);
    expect(isSignatureInFlight("SIGNED")).toBe(false);
    expect(isSignatureInFlight(undefined)).toBe(false);
  });

  it("separa os estados do envelope em ativos e terminais", () => {
    expect(isEnvelopeActive("PARTIALLY_SIGNED")).toBe(true);
    expect(isEnvelopeActive("RUNNING")).toBe(true);
    expect(isEnvelopeActive("FINISHED")).toBe(false);
    expect(isEnvelopeTerminal("FINISHED")).toBe(true);
    expect(isEnvelopeTerminal("CANCELED")).toBe(true);
    expect(isEnvelopeTerminal("PARTIALLY_SIGNED")).toBe(false);
  });

  it("conta o progresso com grupos de ordem", () => {
    const progress = signatureProgress(
      envelope([
        signer({ signer_id: "s1", group: 1, status: "SIGNED" }),
        signer({ signer_id: "s2", group: 2, status: "PENDING" }),
        signer({ signer_id: "s3", group: 2, status: "PENDING" }),
      ], "PARTIALLY_SIGNED"),
    );

    expect(progress.total).toBe(3);
    expect(progress.signed).toBe(1);
    expect(progress.pending).toBe(2);
    expect(progress.hasSigningOrder).toBe(true);
    // Grupo 2 é o próximo com alguém pendente: é ele que está com a vez.
    expect(progress.activeGroup).toBe(2);
    expect(progress.waitingGroups).toEqual([]);
    expect(progress.allGroupsDone).toBe(false);
    expect(signatureProgressLabel(progress)).toBe("1 de 3 assinou");
  });

  it("marca quem espera a vez quando o grupo da frente ainda tem pendente", () => {
    const env = envelope(
      [
        signer({ signer_id: "s1", group: 1, status: "SIGNED" }),
        signer({ signer_id: "s2", group: 1, status: "PENDING" }),
        signer({ signer_id: "s3", group: 2, status: "PENDING" }),
      ],
      "PARTIALLY_SIGNED",
    );
    const progress = signatureProgress(env);

    expect(progress.activeGroup).toBe(1);
    expect(progress.waitingGroups).toEqual([2]);
    // Quem está no grupo da vez está assinando agora.
    expect(signerTurnState(env, { status: "PENDING", group: 1 })).toBe("SIGNING");
    // Quem está atrás NÃO foi convidado — não pode aparecer igual a quem já
    // recebeu o pedido e não assinou.
    expect(signerTurnState(env, { status: "PENDING", group: 2 })).toBe("WAITING_TURN");
    expect(signerTurnState(env, { status: "SIGNED", group: 1 })).toBe("SIGNED");
    expect(signerTurnState(env, { status: "REFUSED", group: 2 })).toBe("REFUSED");
    expect(waitingTurnLabel(2, progress.activeGroup)).toBe("Aguardando o Grupo 1");
  });

  it("sem ordem de assinatura ninguém fica esperando a vez", () => {
    const env = envelope([
      signer({ signer_id: "s1", status: "SIGNED" }),
      signer({ signer_id: "s2", status: "PENDING" }),
    ]);
    const progress = signatureProgress(env);

    expect(progress.hasSigningOrder).toBe(false);
    expect(progress.activeGroup).toBe(0);
    expect(signerTurnState(env, { status: "PENDING", group: 0 })).toBe("PENDING");
    expect(waitingTurnLabel(0, 0)).toBe("Aguardando assinatura");
  });

  it("detecta todos os grupos concluídos quando só falta quem não tem grupo", () => {
    const progress = signatureProgress(
      envelope([
        signer({ signer_id: "s1", group: 1, status: "SIGNED" }),
        signer({ signer_id: "s2", group: 2, status: "SIGNED" }),
        signer({ signer_id: "s3", status: "PENDING" }),
      ], "PARTIALLY_SIGNED"),
    );

    expect(progress.activeGroup).toBe(0);
    expect(progress.allGroupsDone).toBe(true);
    expect(progress.pending).toBe(1);
  });

  it("envelope vazio não inventa progresso", () => {
    const progress = signatureProgress(null);
    expect(progress.total).toBe(0);
    expect(signatureProgressLabel(progress)).toBe("Nenhum signatário no envelope");
    expect(signerTurnState(null, undefined)).toBe("PENDING");
  });
});
