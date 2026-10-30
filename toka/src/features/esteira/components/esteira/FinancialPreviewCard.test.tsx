import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  financialPreviewService,
  type FinancialPreview,
} from "../../../../shared/services/financialPreview.service";
import { FinancialPreviewCard } from "./FinancialPreviewCard";
import { rentalPlanCacheKey } from "../../../../shared/lib/finance/preContractPlanCache";

/** Projeção de locação usada nos testes de datas do plano. */
const rentPreview: FinancialPreview = {
  deal_id: "deal-rent",
  deal_type: "RENT",
  charges: [],
  commissions: [],
  transfers: [],
  total_charges_cents: 0,
  total_commission_cents: 0,
  total_transfers_cents: 0,
  gateway_disabled: true,
};

function renderPreview(preview: FinancialPreview) {
  vi.spyOn(financialPreviewService, "getByDeal").mockResolvedValue(preview);
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <FinancialPreviewCard dealId={preview.deal_id} />
    </QueryClientProvider>,
  );
}

function renderPreviewError(error: unknown) {
  const request = vi.spyOn(financialPreviewService, "getByDeal").mockRejectedValue(error);
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  render(
    <QueryClientProvider client={queryClient}>
      <FinancialPreviewCard dealId="deal-error" />
    </QueryClientProvider>,
  );

  return request;
}

const salePreview: FinancialPreview = {
  deal_id: "deal-sale",
  deal_type: "SALE",
  charges: [],
  commissions: [
    { broker_id: "broker-1", role: "captador", percentage: 20, amount_cents: 4_740_000 },
    { broker_id: "broker-2", role: "fechamento", percentage: 40, amount_cents: 9_480_000 },
    { broker_id: "", role: "imobiliaria", percentage: 40, amount_cents: 9_480_000, retained: true },
  ],
  transfers: [],
  composition: {
    flow: "sale",
    gross_cents: 395_000_000,
    commission_pool_cents: 23_700_000,
    commission_payable_cents: 14_220_000,
    partner_payable_cents: 0,
    agency_revenue_cents: 9_480_000,
    agency_retained_cents: 9_480_000,
    owner_payable_cents: 371_300_000,
    reserved_cents: 0,
    available_cents: 371_300_000,
    deductions_cents: 0,
  },
  total_charges_cents: 0,
  total_commission_cents: 23_700_000,
  total_transfers_cents: 0,
  gateway_disabled: true,
  notes: ["Nenhum plano de pagamento SALE ativo; a assinatura não criará cobrança automaticamente."],
};

afterEach(() => {
  vi.restoreAllMocks();
});

describe("FinancialPreviewCard", () => {
  it("trata a negativa do servidor como acesso restrito sem expor o erro HTTP", async () => {
    const request = renderPreviewError({
      message: "Request failed with status code 403",
      response: { status: 403 },
    });

    expect(await screen.findByText("Área financeira restrita")).toBeInTheDocument();
    expect(screen.getByText(/Seu perfil não possui acesso/)).toBeInTheDocument();
    expect(screen.queryByText(/status code 403/i)).not.toBeInTheDocument();
    expect(request).toHaveBeenCalledTimes(1);
  });

  it("oferece nova tentativa para falhas operacionais sem mostrar detalhes técnicos", async () => {
    const request = renderPreviewError({
      message: "Request failed with status code 503",
      response: { status: 503 },
    });

    expect(await screen.findByText("Não foi possível calcular a projeção")).toBeInTheDocument();
    expect(screen.getByText(/Houve uma falha ao carregar/)).toBeInTheDocument();
    expect(screen.queryByText(/status code 503/i)).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Tentar novamente" }));
    await waitFor(() => expect(request).toHaveBeenCalledTimes(2));
  });

  it("mantém os totais econômicos da venda visíveis sem inventar cobranças", async () => {
    renderPreview(salePreview);

    const saleValue = (await screen.findByText("Valor da venda")).closest("div.bg-muted\\/40");
    const sellerValue = screen.getByText("Ao vendedor").closest("div.bg-muted\\/40");

    expect(saleValue).not.toBeNull();
    expect(sellerValue).not.toBeNull();
    expect(within(saleValue as HTMLElement).getByText("R$ 3.950.000,00")).toBeInTheDocument();
    expect(within(saleValue as HTMLElement).getByText("Cronograma pendente")).toBeInTheDocument();
    expect(within(sellerValue as HTMLElement).getByText("R$ 3.713.000,00")).toBeInTheDocument();
    expect(within(sellerValue as HTMLElement).getByText("Obrigação prevista")).toBeInTheDocument();
    expect(screen.queryByText("Recebíveis previstos (0)")).not.toBeInTheDocument();
  });

  it("preserva os totais operacionais e as contagens da locação", async () => {
    renderPreview({
      ...salePreview,
      deal_id: "deal-rent",
      deal_type: "RENT",
      charges: [
        {
          source_event_id: "charge-1",
          type: "RENT",
          description: "Aluguel",
          amount_cents: 320_000,
          due_date: "2026-09-10T00:00:00Z",
          reference_month: "2026-09",
        },
      ],
      transfers: [
        {
          reference_month: "2026-09",
          gross_cents: 320_000,
          admin_fee_pct: 10,
          admin_fee_cents: 32_000,
          intermediation_cents: 0,
          commission_funded_cents: 0,
          agency_intermediation_cents: 0,
          intermediation_carry_cents: 0,
          net_cents: 288_000,
        },
      ],
      composition: {
        ...salePreview.composition!,
        flow: "rent",
        gross_cents: 320_000,
        owner_payable_cents: 288_000,
        available_cents: 288_000,
      },
      total_charges_cents: 320_000,
      total_transfers_cents: 288_000,
    });

    const receivable = (await screen.findByText("A receber")).closest("div.bg-muted\\/40");
    const owner = screen.getByText("Ao proprietário").closest("div.bg-muted\\/40");

    expect(within(receivable as HTMLElement).getByText("R$ 3.200,00")).toBeInTheDocument();
    expect(within(receivable as HTMLElement).getByText("1 item")).toBeInTheDocument();
    expect(within(owner as HTMLElement).getByText("R$ 2.880,00")).toBeInTheDocument();
    expect(within(owner as HTMLElement).getByText("1 item")).toBeInTheDocument();
  });
});


it("sinaliza acordo a revisar sem repetir a consulta nem sugerir erro operacional", async () => {
 const request = renderPreviewError({ response: { status: 409 } });
 expect(await screen.findByText("Condições da parceria a revisar")).toBeInTheDocument();
 expect(screen.queryByRole("button", {name: "Tentar novamente"})).not.toBeInTheDocument();
 expect(request).toHaveBeenCalledTimes(1);
});

it.each(["payable", "receivable"] as const)("mostra taxa %s sem descontar do proprietário", async direction => {
 renderPreview({...salePreview, partnership_fees: [{direction, kind: "admin_fee", label: "Taxa administrativa da parceria", amount_cents: 12345, payer_tenant: "payer", payee_tenant: "payee"}]});
 expect(await screen.findByText("Taxa administrativa da parceria")).toBeInTheDocument();
 expect(screen.getByText("R$ 123,45")).toBeInTheDocument();
 expect(screen.getByText(direction === "payable" ? "A pagar à parceira de origem" : "A receber da parceira de origem")).toBeInTheDocument();
 expect(screen.getAllByText("R$ 3.713.000,00").length).toBeGreaterThan(0);
});

/**
 * Antes de existir contrato, as datas vêm do rascunho do plano pré-contrato.
 * Sem isso o backend projeta o prazo padrão (30 meses) e o residual do último
 * aluguel sai numa data que o operador não escolheu — divergindo do calendário
 * mostrado na aba Assinatura.
 */
describe("FinancialPreviewCard — datas do plano pré-contrato", () => {
  it("projeta com início e término configurados na locação", async () => {
    const request = vi.spyOn(financialPreviewService, "getByDeal").mockResolvedValue(rentPreview);
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    queryClient.setQueryData(rentalPlanCacheKey("deal-rent"), {
      startDate: "2026-10-02",
      endDate: "2027-10-02",
      termMonths: 12,
      firstChargeType: "same_month_pro_rata",
      firstChargeDate: "2026-11-07",
      defaultDueDay: 7,
    });

    render(
      <QueryClientProvider client={queryClient}>
        <FinancialPreviewCard dealId="deal-rent" />
      </QueryClientProvider>,
    );

    await waitFor(() => expect(request).toHaveBeenCalled());
    expect(request).toHaveBeenCalledWith("deal-rent", { startDate: "2026-10-02", endDate: "2027-10-02" });
  });

  it("sem rascunho nem contrato, mantém o comportamento anterior (datas do negócio)", async () => {
    const request = vi.spyOn(financialPreviewService, "getByDeal").mockResolvedValue(rentPreview);
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });

    render(
      <QueryClientProvider client={queryClient}>
        <FinancialPreviewCard dealId="deal-sem-plano" />
      </QueryClientProvider>,
    );

    await waitFor(() => expect(request).toHaveBeenCalled());
    expect(request).toHaveBeenCalledWith("deal-sem-plano", undefined);
  });
});
