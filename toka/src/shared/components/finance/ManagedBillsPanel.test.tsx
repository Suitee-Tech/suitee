import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import api from "../../services/api";
import {
  managedBillService,
  type ManagedBill,
  type ManagedBillMutationResult,
} from "../../services/managedBill.service";
import { PermissionsContext } from "../../context/PermissionsContext";

vi.mock("../../context/AuthContext", () => ({
  useOptionalAuth: () => ({
    user: authState.userId ? { id: authState.userId, email: "operador@example.com" } : null,
  }),
}));
import { getManagedBillActions, ManagedBillsPanel } from "./ManagedBillsPanel";

const approvedBill: ManagedBill = {
  id: "managed-bill-condominio",
  tenant_id: "imobiliaria-estrela",
  source_key: "managed-bill:charge-2026-08:item-condominio",
  source_transfer_id: "transfer-2026-08",
  source_charge_id: "charge-2026-08",
  charge_item_id: "item-condominio",
  contract_id: "ctr-palmeiras",
  property_id: "property-palmeiras",
  owner_id: "owner-marisa",
  component: "Condomínio",
  creditor_name: "Condomínio Palmeiras",
  description: "Condomínio de agosto/2026",
  amount_cents: 70_000,
  due_date: "2026-08-08T00:00:00Z",
  competence_month: "2026-08",
  status: "APPROVED",
  version: 2,
  reserved_by: "financeiro-1",
  reserved_at: "2026-08-01T10:00:00Z",
  approved_by: "financeiro-2",
  approved_at: "2026-08-05T10:00:00Z",
  evidence: [
    {
      kind: "approval_document",
      reference: "javascript:alert('não abrir')",
      actor: "financeiro-2",
      at: "2026-08-05T10:00:00Z",
    },
  ],
  history: [
    {
      action: "reserved",
      actor: "financeiro-1",
      at: "2026-08-01T10:00:00Z",
      version: 1,
    },
    {
      action: "approved",
      actor: "financeiro-2",
      at: "2026-08-05T10:00:00Z",
      version: 2,
      reference: "approval-batch-42",
    },
  ],
  created_at: "2026-08-01T10:00:00Z",
  updated_at: "2026-08-05T10:00:00Z",
};

const paidBill: ManagedBill = {
  ...approvedBill,
  status: "PAID",
  version: 3,
  payment_reference: "PIX-E2E-700",
  paid_by: "financeiro-3",
  paid_at: "2026-08-08T12:00:00Z",
  history: [
    ...approvedBill.history,
    {
      action: "paid",
      actor: "financeiro-3",
      at: "2026-08-08T12:00:00Z",
      version: 3,
      reference: "PIX-E2E-700",
    },
  ],
  updated_at: "2026-08-08T12:00:00Z",
};

const reversedBill: ManagedBill = {
  ...approvedBill,
  version: 4,
  payment_attempts: [
    {
      id: "attempt-pix-700",
      payment_reference: "PIX-E2E-700",
      payment_journal_source_id: "journal-payment-700",
      paid_by: "financeiro-3",
      paid_at: "2026-08-08T12:00:00Z",
      reversal_reference: "ESTORNO-PIX-700",
      reversal_journal_source_id: "journal-reversal-700",
      reversal_proof_reference: "https://proof.example/reversal.pdf",
      reversed_by: "financeiro-4",
      reversed_at: "2026-08-09T10:30:00Z",
      reversal_reason: "Pagamento devolvido pelo banco",
    },
  ],
  history: [
    ...paidBill.history,
    {
      action: "payment_reversed",
      actor: "financeiro-4",
      at: "2026-08-09T10:30:00Z",
      version: 4,
      reference: "ESTORNO-PIX-700",
      reason: "Pagamento devolvido pelo banco",
    },
  ],
  updated_at: "2026-08-09T10:30:00Z",
};

const reservedBill: ManagedBill = {
  ...approvedBill,
  id: "managed-bill-iptu",
  source_key: "managed-bill:charge-2026-08:item-iptu",
  charge_item_id: "item-iptu",
  component: "IPTU",
  creditor_name: "Prefeitura Municipal",
  description: "IPTU de agosto/2026",
  amount_cents: 30_000,
  status: "RESERVED",
  version: 1,
  evidence: [],
  history: approvedBill.history.slice(0, 1),
  approved_by: undefined,
  approved_at: undefined,
};

const allManagedBillPermissions = ["finance:approve_transfer", "finance:write"];

/** Usuário atual visto pelo painel; trocado por teste para exercitar a
 * segregação de funções. */
const authState: { userId: string | null } = { userId: null };

const panelAsUser = (
  userId: string,
  props: ComponentProps<typeof ManagedBillsPanel>,
  permissions: string[] = allManagedBillPermissions,
) => {
  authState.userId = userId;
  return panelWithPermissions(props, permissions);
};

const panelWithPermissions = (
  props: ComponentProps<typeof ManagedBillsPanel>,
  permissions: string[] = allManagedBillPermissions,
) => (
  <PermissionsContext.Provider
    value={{
      functionalRole: null,
      baseRole: null,
      hasTenantWideScope: false,
      profile: null,
      permissions: new Set(permissions),
      can: (permissionId) => permissions.includes(permissionId),
      isLoading: false,
      refresh: async () => {},
    }}
  >
    <ManagedBillsPanel {...props} />
  </PermissionsContext.Provider>
);

const paidResult: ManagedBillMutationResult = {
  bill: paidBill,
  owner_balance_before: {
    total_cents: 560_000,
    reserved_cents: 100_000,
    blocked_cents: 0,
    available_cents: 460_000,
  },
  owner_balance_after: {
    total_cents: 490_000,
    reserved_cents: 30_000,
    blocked_cents: 0,
    available_cents: 460_000,
  },
  coverage_complete: true,
};

const reversedResult: ManagedBillMutationResult = {
  bill: reversedBill,
  owner_balance_before: paidResult.owner_balance_after,
  owner_balance_after: paidResult.owner_balance_before,
  coverage_complete: true,
};

afterEach(() => {
  vi.restoreAllMocks();
});

describe("getManagedBillActions", () => {
  it("expõe somente as ações válidas em cada estado", () => {
    expect(getManagedBillActions("RESERVED")).toEqual(["approve", "cancel"]);
    expect(getManagedBillActions("APPROVED")).toEqual(["pay", "cancel"]);
    expect(getManagedBillActions("PAID")).toEqual(["reconcile", "reverse"]);
    expect(getManagedBillActions("RECONCILED")).toEqual(["reverse"]);
    expect(getManagedBillActions("CANCELLED")).toEqual([]);
  });
});

describe("ManagedBillsPanel", () => {
  it("abre o detalhe lateral com credor, vencimento esperado e liquidação", async () => {
    vi.spyOn(managedBillService, "list").mockResolvedValue({
      data: [paidBill],
      meta: { total: 1, limit: 25, page: 1, totalPages: 1 },
    });
    const user = userEvent.setup();

    render(panelWithPermissions({
      contractId: "ctr-palmeiras",
      propertyLabels: { "property-palmeiras": "Rua das Palmeiras, 245" },
    }));

    expect(await screen.findByText("Condomínio Palmeiras")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Ver detalhes da conta de Condomínio Palmeiras" }));

    const dialog = await screen.findByRole("dialog", { name: "Detalhes da conta" });
    expect(within(dialog).getByText("Beneficiário")).toBeInTheDocument();
    expect(within(dialog).getByText("Condomínio Palmeiras")).toBeInTheDocument();
    expect(within(dialog).getByText("Credor · Condomínio")).toBeInTheDocument();
    expect(within(dialog).getByText("Vencimento esperado")).toBeInTheDocument();
    expect(within(dialog).getByText("08/08/2026")).toBeInTheDocument();
    expect(within(dialog).getByText("Liquidação")).toBeInTheDocument();
    expect(within(dialog).getByText("Pagamento registrado")).toBeInTheDocument();
    expect(within(dialog).getByText("PIX-E2E-700")).toBeInTheDocument();
  });

  it("paga R$ 700, mostra o impacto autoritativo e mantém a conta PAID, não RECONCILED", async () => {
    vi.spyOn(managedBillService, "list").mockResolvedValue({
      data: [approvedBill],
      meta: { total: 1, limit: 25, page: 1, totalPages: 1 },
    });
    const pay = vi.spyOn(managedBillService, "pay").mockResolvedValue(paidResult);
    const user = userEvent.setup();

    render(
      panelWithPermissions({
        contractId: "ctr-palmeiras",
        propertyLabels: { "property-palmeiras": "Rua das Palmeiras, 245" },
      }),
    );

    expect(await screen.findByText("Condomínio Palmeiras")).toBeInTheDocument();
    await user.click(screen.getByText("Trilha (2) · tentativas (0) · evidências (1)"));
    const opaqueReference = screen.getByText("Referência: javascript:alert('não abrir')");
    expect(opaqueReference).toBeInTheDocument();
    expect(opaqueReference.closest("a")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Registrar pagamento" }));

    const dialog = screen.getByRole("dialog");
    await user.type(within(dialog).getByLabelText(/Referência do pagamento/i), "PIX-E2E-700");
    await user.click(within(dialog).getByRole("button", { name: "Registrar pagamento" }));

    await waitFor(() => {
      expect(pay).toHaveBeenCalledWith("managed-bill-condominio", {
        expected_version: 2,
        payment_reference: "PIX-E2E-700",
        proof_url: undefined,
      });
    });

    expect(await screen.findByText("Paga — aguardando conciliação")).toBeInTheDocument();
    expect(screen.getByText("PAID: pagamento registrado; ainda não conciliado.")).toBeInTheDocument();
    expect(screen.queryByText("Conciliada")).not.toBeInTheDocument();

    const before = screen.getByTestId("managed-bill-balance-before");
    const after = screen.getByTestId("managed-bill-balance-after");
    expect(before).toHaveTextContent(/Total\s*R\$ 5\.600,00/);
    expect(before).toHaveTextContent(/Reservado\s*R\$ 1\.000,00/);
    expect(before).toHaveTextContent(/Disponível\s*R\$ 4\.600,00/);
    expect(after).toHaveTextContent(/Total\s*R\$ 4\.900,00/);
    expect(after).toHaveTextContent(/Reservado\s*R\$ 300,00/);
    expect(after).toHaveTextContent(/Disponível\s*R\$ 4\.600,00/);
    expect(screen.getAllByText(/não representa saldo bancário/i).length).toBeGreaterThan(0);
  });

  it("mantém usuário somente leitura sem ações mutáveis", async () => {
    vi.spyOn(managedBillService, "list").mockResolvedValue({
      data: [approvedBill],
      meta: { total: 1, limit: 25, page: 1, totalPages: 1 },
    });

    render(panelWithPermissions({ contractId: "ctr-palmeiras" }, ["finance:read"]));

    expect(await screen.findByText("Condomínio Palmeiras")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Registrar pagamento" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Cancelar" })).not.toBeInTheDocument();
    expect(screen.getByText("Sem permissão para agir")).toBeInTheDocument();
  });

  it("mapeia aprovação e escrita para as mesmas permissões das rotas", async () => {
    vi.spyOn(managedBillService, "list").mockResolvedValue({
      data: [reservedBill],
      meta: { total: 1, limit: 25, page: 1, totalPages: 1 },
    });

    const { rerender } = render(
      panelWithPermissions({ contractId: "ctr-palmeiras" }, ["finance:approve_transfer"]),
    );

    expect(await screen.findByText("Prefeitura Municipal")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Aprovar" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Cancelar" })).not.toBeInTheDocument();

    rerender(panelWithPermissions({ contractId: "ctr-palmeiras" }, ["finance:write"]));
    expect(screen.queryByRole("button", { name: "Aprovar" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cancelar" })).toBeInTheDocument();
  });

  it("reserva o estorno para alta confiança e mantém a conciliação em finance:write", async () => {
    vi.spyOn(managedBillService, "list").mockResolvedValue({
      data: [paidBill],
      meta: { total: 1, limit: 25, page: 1, totalPages: 1 },
    });

    const { rerender } = render(
      panelWithPermissions({ contractId: "ctr-palmeiras" }, ["finance:write"]),
    );

    expect(await screen.findByText("Condomínio Palmeiras")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Conciliar" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Estornar pagamento" })).not.toBeInTheDocument();

    rerender(panelWithPermissions({ contractId: "ctr-palmeiras" }, ["finance:approve_transfer"]));
    expect(screen.queryByRole("button", { name: "Conciliar" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Estornar pagamento" })).toBeInTheDocument();
  });

  it("estorna uma conta paga, arquiva a tentativa e permite registrar novo pagamento", async () => {
    vi.spyOn(managedBillService, "list").mockResolvedValue({
      data: [paidBill],
      meta: { total: 1, limit: 25, page: 1, totalPages: 1 },
    });
    const reversePayment = vi.spyOn(managedBillService, "reversePayment").mockResolvedValue(reversedResult);
    const user = userEvent.setup();

    render(panelWithPermissions({ contractId: "ctr-palmeiras" }));

    expect(await screen.findByText("Paga — aguardando conciliação")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Estornar pagamento" }));

    const dialog = screen.getByRole("dialog");
    await user.type(within(dialog).getByLabelText(/Referência do estorno/i), "ESTORNO-PIX-700");
    await user.type(within(dialog).getByLabelText(/Motivo do estorno/i), "Pagamento devolvido pelo banco");
    await user.type(within(dialog).getByLabelText(/comprovante do estorno/i), "https://proof.example/reversal.pdf");
    await user.click(within(dialog).getByRole("button", { name: "Confirmar estorno" }));

    await waitFor(() => {
      expect(reversePayment).toHaveBeenCalledWith("managed-bill-condominio", {
        expected_version: 3,
        reversal_reference: "ESTORNO-PIX-700",
        reason: "Pagamento devolvido pelo banco",
        proof_url: "https://proof.example/reversal.pdf",
      });
    });

    expect(await screen.findByText("Aprovada")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Registrar pagamento" })).toBeInTheDocument();
    await user.click(screen.getByText("Trilha (4) · tentativas (1) · evidências (1)"));
    expect(screen.getByText(/Pagamento estornado · v4/)).toBeInTheDocument();
    expect(screen.getByText("Tentativas anteriores de pagamento")).toBeInTheDocument();
    expect(screen.getByText(/PIX-E2E-700 · estornada/)).toBeInTheDocument();
    expect(screen.getByText(/Estorno: ESTORNO-PIX-700/)).toBeInTheDocument();
    expect(screen.getAllByText(/Pagamento devolvido pelo banco/).length).toBeGreaterThan(0);
  });

  it("pagina no servidor, mostra o total e volta à primeira página ao trocar escopo ou busca", async () => {
    const list = vi.spyOn(managedBillService, "list").mockImplementation(async (params) => {
      const query = params ?? {};
      if (query.search) {
        return {
          data: [reservedBill],
          meta: { total: 1, limit: 25, page: 1, totalPages: 1 },
        };
      }
      if (query.offset === 25) {
        return {
          data: [reservedBill],
          meta: { total: 26, limit: 25, page: 2, totalPages: 2 },
        };
      }
      return {
        data: [approvedBill],
        meta: { total: 26, limit: 25, page: 1, totalPages: 2 },
      };
    });
    const user = userEvent.setup();

    const { rerender } = render(
      panelWithPermissions({ contractId: "ctr-palmeiras", search: "" }),
    );

    expect(await screen.findByText("26 contas administradas")).toBeInTheDocument();
    expect(list).toHaveBeenLastCalledWith({
      contract_id: "ctr-palmeiras",
      search: undefined,
      limit: 25,
      offset: 0,
    });
    expect(screen.getByText("Página 1 de 2")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Próxima" }));
    expect(await screen.findByText("Prefeitura Municipal")).toBeInTheDocument();
    expect(list).toHaveBeenLastCalledWith({
      contract_id: "ctr-palmeiras",
      search: undefined,
      limit: 25,
      offset: 25,
    });

    rerender(panelWithPermissions({ contractId: "ctr-novo", search: "" }));
    await waitFor(() => {
      expect(list).toHaveBeenLastCalledWith({
        contract_id: "ctr-novo",
        search: undefined,
        limit: 25,
        offset: 0,
      });
    });

    expect(await screen.findByText("Página 1 de 2")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Próxima" }));
    expect(await screen.findByText("Página 2 de 2")).toBeInTheDocument();
    rerender(panelWithPermissions({ contractId: "ctr-novo", search: "  IPTU  " }));
    await waitFor(() => {
      expect(list).toHaveBeenLastCalledWith({
        contract_id: "ctr-novo",
        search: "IPTU",
        limit: 25,
        offset: 0,
      });
    });
    expect(await screen.findByText("1 conta administrada")).toBeInTheDocument();
  });
});

describe("managedBillService", () => {
  it("envia somente o DTO versionado, sem header de idempotência não suportado", async () => {
    const post = vi.spyOn(api, "post").mockResolvedValue({ data: { data: paidResult } });

    await managedBillService.approve(approvedBill.id, { expected_version: 2 });
    await managedBillService.pay(approvedBill.id, {
      expected_version: 2,
      payment_reference: "PIX-E2E-700",
      proof_url: "https://files.example/proof.pdf",
    });
    await managedBillService.reconcile(approvedBill.id, {
      expected_version: 3,
      reconciliation_reference: "bank-transaction-700",
    });
    await managedBillService.reversePayment(approvedBill.id, {
      expected_version: 4,
      reversal_reference: "bank-reversal-700",
      reason: "Pagamento devolvido",
      proof_url: "https://files.example/reversal.pdf",
    });
    await managedBillService.cancel(approvedBill.id, {
      expected_version: 2,
      reason: "Documento emitido em duplicidade",
    });

    expect(post.mock.calls.map((call) => call[0])).toEqual([
      "/managed-bills/managed-bill-condominio/approve",
      "/managed-bills/managed-bill-condominio/pay",
      "/managed-bills/managed-bill-condominio/reconcile",
      "/managed-bills/managed-bill-condominio/reverse-payment",
      "/managed-bills/managed-bill-condominio/cancel",
    ]);
    expect(post.mock.calls.map((call) => call[1])).toEqual([
      { expected_version: 2 },
      {
        expected_version: 2,
        payment_reference: "PIX-E2E-700",
        proof_url: "https://files.example/proof.pdf",
      },
      { expected_version: 3, reconciliation_reference: "bank-transaction-700" },
      {
        expected_version: 4,
        reversal_reference: "bank-reversal-700",
        reason: "Pagamento devolvido",
        proof_url: "https://files.example/reversal.pdf",
      },
      { expected_version: 2, reason: "Documento emitido em duplicidade" },
    ]);

    expect(post.mock.calls.every((call) => call.length === 2)).toBe(true);
  });
});

describe("ManagedBillsPanel · conflito ao registrar pagamento", () => {
  it("recarrega a lista e explica o conflito de versão, em vez de só exibir o 409", async () => {
    const list = vi.spyOn(managedBillService, "list").mockResolvedValue({
      data: [approvedBill],
      meta: { total: 1, limit: 25, page: 1, totalPages: 1 },
    });
    // 409 com o motivo discriminado, como o backend passou a responder.
    vi.spyOn(managedBillService, "pay").mockRejectedValue({
      response: {
        status: 409,
        data: {
          message:
            "esta conta foi alterada por outra operação (versão enviada 2, versão atual 4, situação APPROVED): recarregue a lista antes de registrar o pagamento",
          data: { reason: "stale_version", current_version: 4, current_status: "APPROVED" },
        },
      },
    });
    const user = userEvent.setup();

    render(panelWithPermissions({ contractId: "ctr-palmeiras" }));

    expect(await screen.findByText("Condomínio Palmeiras")).toBeInTheDocument();
    const listCallsBefore = list.mock.calls.length;
    await user.click(screen.getByRole("button", { name: "Registrar pagamento" }));
    const dialog = screen.getByRole("dialog");
    await user.type(within(dialog).getByLabelText(/Referência do pagamento/i), "PIX-DEFASADA");
    await user.click(within(dialog).getByRole("button", { name: "Registrar pagamento" }));

    // A mensagem diz o que fazer...
    expect(await screen.findByText(/recarregue a lista antes de registrar o pagamento/i)).toBeInTheDocument();
    // ...e a lista foi efetivamente recarregada, para a próxima tentativa usar o
    // estado atual em vez de repetir a mesma versão defasada.
    expect(list.mock.calls.length).toBeGreaterThan(listCallsBefore);
  });

  it("conta já paga com outra referência não sugere nova tentativa de pagamento", async () => {
    vi.spyOn(managedBillService, "list").mockResolvedValue({
      data: [approvedBill],
      meta: { total: 1, limit: 25, page: 1, totalPages: 1 },
    });
    vi.spyOn(managedBillService, "pay").mockRejectedValue({
      response: {
        status: 409,
        data: {
          message:
            "esta conta já está paga com outra referência de pagamento (situação PAID): confira a referência registrada ou estorne o pagamento para corrigir",
          data: { reason: "already_paid", current_status: "PAID", payment_reference: "PIX-ORIGINAL" },
        },
      },
    });
    const user = userEvent.setup();

    render(panelWithPermissions({ contractId: "ctr-palmeiras" }));

    expect(await screen.findByText("Condomínio Palmeiras")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Registrar pagamento" }));
    const dialog = screen.getByRole("dialog");
    await user.type(within(dialog).getByLabelText(/Referência do pagamento/i), "PIX-OUTRA");
    await user.click(within(dialog).getByRole("button", { name: "Registrar pagamento" }));

    expect(await screen.findByText(/já está paga com outra referência/i)).toBeInTheDocument();
    expect(screen.getByText(/estorne o pagamento para corrigir/i)).toBeInTheDocument();
  });
});

describe("ManagedBillsPanel · administrador aprova e paga", () => {
  // Política nova: o administrador da imobiliária conclui os dois passos
  // sozinho quando é o único operador. O hook de permissões é o mesmo que
  // governa as rotas, então quem aprova é quem tem autoridade financeira.
  const billApprovedByBruno: ManagedBill = { ...approvedBill, approved_by: "bruno-id" };

  it("oferece o pagamento ao administrador que aprovou a conta", async () => {
    vi.spyOn(managedBillService, "list").mockResolvedValue({
      data: [billApprovedByBruno],
      meta: { total: 1, limit: 25, page: 1, totalPages: 1 },
    });

    // allManagedBillPermissions inclui finance:approve_transfer.
    render(panelAsUser("bruno-id", { contractId: "ctr-palmeiras" }));

    expect(await screen.findByText("Condomínio Palmeiras")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Registrar pagamento" })).toBeInTheDocument();
    expect(screen.queryByText(/pagamento por outro operador/i)).not.toBeInTheDocument();
  });

  it("mantém a regra de dois operadores para quem não tem autoridade de aprovação", async () => {
    vi.spyOn(managedBillService, "list").mockResolvedValue({
      data: [billApprovedByBruno],
      meta: { total: 1, limit: 25, page: 1, totalPages: 1 },
    });

    // Sem finance:approve_transfer o usuário não tem autoridade financeira; se
    // ele aprovou (por override ou mudança de papel), não paga.
    render(panelAsUser("bruno-id", { contractId: "ctr-palmeiras" }, ["finance:write", "finance:read"]));

    expect(await screen.findByText("Condomínio Palmeiras")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Registrar pagamento" })).not.toBeInTheDocument();
    expect(screen.getByText(/pagamento por outro operador/i)).toBeInTheDocument();
  });
});
