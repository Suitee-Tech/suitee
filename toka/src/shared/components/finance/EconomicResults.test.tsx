import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import type { CashFlowReport, DREReport } from "../../services/financialReport.service";
import { CashflowChart } from "./CashflowChart";
import { DRETable } from "./DRETable";

vi.mock("recharts", () => ({
  ResponsiveContainer: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  BarChart: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  Bar: () => null,
  XAxis: () => null,
  YAxis: () => null,
  Tooltip: () => null,
  CartesianGrid: () => null,
  Legend: () => null,
}));

const dre: DREReport = {
  tenant_id: "tenant-1",
  from: "2026-08-01T00:00:00Z",
  to: "2026-08-31T23:59:59Z",
  generated_at: "2026-08-31T12:00:00Z",
  revenue: [{ category: "SALE_AGENCY_RETAINED", count: 1, total_cents: 2_352_000 }],
  expense: [],
  net_cents: 2_352_000,
};

const cashflow: CashFlowReport = {
  tenant_id: "tenant-1",
  from: "2026-08-01T00:00:00Z",
  to: "2026-08-31T23:59:59Z",
  granularity: "day",
  generated_at: "2026-08-31T12:00:00Z",
  entries: [{
    period: "2026-08-29",
    inflow_cents: 98_000_000,
    outflow_cents: 95_648_000,
    net_cents: 2_352_000,
  }],
};

describe("resultado econômico e caixa", () => {
  it("separa recebimentos a classificar da receita e mostra a comissão como despesa", () => {
    render(<DRETable data={{
      ...dre,
      revenue: [{ category: 'SALE_INTERMEDIATION', count: 1, total_cents: 5_880_000 }],
      expense: [{ category: 'Comissão', count: 2, total_cents: 3_528_000 }],
      unclassified_receipts_cents: 500_000,
    }} />);
    expect(screen.getByRole('status')).toHaveTextContent('Recebimentos a classificar: R$ 5.000,00');
    expect(screen.getByRole('button', { name: /Receita própria R\$\s*58.800,00/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Despesas próprias -R\$\s*35.280,00/ })).toBeInTheDocument();
    expect(screen.getByText('Resultado da imobiliária').nextElementSibling).toHaveTextContent('R$ 23.520,00');
  });
  it("apresenta somente a parcela própria como receita e explica a titularidade", () => {
    render(<DRETable data={dre} />);

    expect(screen.getByRole("heading", { name: "Resultado econômico da imobiliária" })).toBeInTheDocument();
    expect(screen.getByText("Corretagem retida na venda")).toBeInTheDocument();
    expect(screen.getAllByText("R$ 23.520,00")).toHaveLength(3);
    expect(screen.getByRole("button", { name: /Despesas próprias R\$\s*0,00/ })).toBeInTheDocument();
    expect(screen.queryByText(/-R\$\s*0,00/)).not.toBeInTheDocument();
    expect(screen.queryByText("R$ 980.000,00")).not.toBeInTheDocument();
    expect(screen.getByText(/Repasses a proprietários e parceiros liquidam obrigações de terceiros/)).toBeInTheDocument();
  });

  it("mantém o recebimento bruto no caixa e o distingue do resultado", () => {
    render(<CashflowChart data={cashflow} />);

    expect(screen.getByRole("heading", { name: "Movimentação de caixa" })).toBeInTheDocument();
    expect(screen.getByText("R$ 980.000,00")).toBeInTheDocument();
    expect(screen.getByText("R$ 956.480,00")).toBeInTheDocument();
    expect(screen.getByText("+R$ 23.520,00")).toBeInTheDocument();
    expect(screen.getByText(/inclui dinheiro próprio e valores administrados de terceiros/)).toBeInTheDocument();
  });
});
