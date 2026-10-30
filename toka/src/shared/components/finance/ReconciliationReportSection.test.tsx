import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

import { ReconciliationReportSection } from "./ReconciliationReportSection";
import {
  financialReportService,
  type ReconciliationFinding,
  type ReconciliationReport,
} from "../../services/financialReport.service";

vi.mock("../../services/financialReport.service", () => ({
  financialReportService: { reconciliation: vi.fn() },
}));

function report(findings: ReconciliationFinding[], overrides: Partial<ReconciliationReport["summary"]> = {}): ReconciliationReport {
  return {
    tenant_id: "tenant-1",
    generated_at: "2026-09-10T12:00:00Z",
    dry_run: true,
    summary: {
      total_findings: findings.length,
      errors: findings.filter((f) => f.severity === "ERROR").length,
      warnings: findings.filter((f) => f.severity === "WARN").length,
      duplicates: 0,
      orphans: 0,
      excess_alloc: 0,
      sub_alloc: 0,
      consistency: 0,
      ...overrides,
    },
    findings,
  };
}

function renderSection() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return render(<ReconciliationReportSection />, { wrapper });
}

const duplicateFinding: ReconciliationFinding = {
  severity: "ERROR",
  category: "duplicate",
  entity_type: "Commission",
  entity_id: "evt-123",
  reference: "2 occurrences",
  message: "Comissão duplicada para o mesmo evento de origem",
};

beforeEach(() => {
  vi.mocked(financialReportService.reconciliation).mockReset().mockResolvedValue(report([]));
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("ReconciliationReportSection · FIN-03", () => {
  it("lista as inconsistências com gravidade, entidade e contexto", async () => {
    vi.mocked(financialReportService.reconciliation).mockResolvedValue(report([duplicateFinding]));
    renderSection();

    expect(await screen.findByText(/Comissão duplicada para o mesmo evento/i)).toBeInTheDocument();
    expect(screen.getByText("Erro")).toBeInTheDocument();
    expect(screen.getByText("Duplicatas")).toBeInTheDocument();
    // A entidade é nomeada em português, não pelo nome cru do tipo no backend.
    expect(screen.getByText("Comissão")).toBeInTheDocument();
    expect(screen.getByText("evt-123")).toBeInTheDocument();
    expect(screen.getByText("2 occurrences")).toBeInTheDocument();
  });

  it("declara o relatório como somente leitura", async () => {
    vi.mocked(financialReportService.reconciliation).mockResolvedValue(report([duplicateFinding]));
    renderSection();
    expect(await screen.findByText(/somente leitura e não altera registros/i)).toBeInTheDocument();
  });

  it("falha do diagnóstico não aparece como ausência de pendências", async () => {
    vi.mocked(financialReportService.reconciliation).mockRejectedValue(new Error("offline"));
    renderSection();

    // O backend é fail-closed: erro significa que a verificação não rodou.
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/não significa que não há pendências/i);
    expect(screen.queryByText(/Nenhuma inconsistência encontrada/i)).not.toBeInTheDocument();
  });

  it("permite tentar novamente após falha", async () => {
    const user = userEvent.setup();
    vi.mocked(financialReportService.reconciliation).mockRejectedValueOnce(new Error("offline"));
    renderSection();

    await screen.findByRole("alert");
    vi.mocked(financialReportService.reconciliation).mockResolvedValue(report([duplicateFinding]));
    await user.click(screen.getByRole("button", { name: /Tentar novamente/i }));

    expect(await screen.findByText(/Comissão duplicada/i)).toBeInTheDocument();
  });

  it("aponta possível truncamento quando a listagem alcança o limite por verificação", async () => {
    // Cada verificação é limitada a 100 achados e o payload não sinaliza
    // truncamento: sem este aviso, 100 apareceria como inventário completo.
    const many = Array.from({ length: 100 }, (_, i) => ({ ...duplicateFinding, entity_id: `evt-${i}` }));
    vi.mocked(financialReportService.reconciliation).mockResolvedValue(report(many));
    renderSection();

    expect(await screen.findByText(/A listagem pode estar incompleta/i)).toBeInTheDocument();
  });

  it("não alarma quando a listagem está abaixo do limite", async () => {
    vi.mocked(financialReportService.reconciliation).mockResolvedValue(report([duplicateFinding]));
    renderSection();

    await screen.findByText(/Comissão duplicada/i);
    expect(screen.queryByText(/A listagem pode estar incompleta/i)).not.toBeInTheDocument();
  });

  it("não promete deep-link para o registro, pois o id é a origem compartilhada", async () => {
    vi.mocked(financialReportService.reconciliation).mockResolvedValue(report([duplicateFinding]));
    renderSection();

    await screen.findByText(/Comissão duplicada/i);
    // Nenhum link ou botão de navegação para a entidade: em duplicatas o
    // entity_id é o source_event_id, não o id da linha.
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /abrir|ver registro|detalhe/i })).not.toBeInTheDocument();
  });

  it("mostra estado de verificação em andamento", async () => {
    vi.mocked(financialReportService.reconciliation).mockReturnValue(new Promise(() => {}));
    renderSection();
    expect(await screen.findByRole("status")).toHaveTextContent(/Verificando duplicatas/i);
  });

  it("exibe ausência de pendências quando total é zero", async () => {
    vi.mocked(financialReportService.reconciliation).mockResolvedValue(report([]));
    renderSection();
    await waitFor(() => expect(screen.getByText(/Nenhuma inconsistência encontrada/i)).toBeInTheDocument());
  });
});
