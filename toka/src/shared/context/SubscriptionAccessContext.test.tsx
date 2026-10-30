import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, render, screen } from "@testing-library/react";
import { SubscriptionAccessProvider, useSubscriptionAccess } from "./SubscriptionAccessContext";

const getAccess = vi.hoisted(() => vi.fn());
vi.mock("../services/platformSubscription.service", () => ({
  platformSubscriptionService: { getAccess },
}));

const access = (tier: string, features: string[]) => ({
  access_control_enabled: true,
  subscription_enabled: true,
  entitled: true,
  status: "ACTIVE",
  plan: { tier, name: tier, amount_cents: 98700, cycle: "MONTHLY", description: "", limits: { leads: 1000, properties: 3000, users: 20 }, contact_only: false, trial_days: 15 },
  features,
});

function Probe() {
  const { access: current, isLoading, error } = useSubscriptionAccess();
  return (
    <div>
      <span data-testid="tier">{current?.plan?.tier ?? "-"}</span>
      <span data-testid="features">{(current?.features ?? []).join(",") || "-"}</span>
      <span data-testid="loading">{isLoading ? "carregando" : "pronto"}</span>
      <span data-testid="error">{error ?? "-"}</span>
    </div>
  );
}

const renderProvider = (props: { active?: boolean; companyId?: string } = {}) =>
  render(
    <SubscriptionAccessProvider active={props.active ?? true} companyId={props.companyId ?? "tenant-1"}>
      <Probe />
    </SubscriptionAccessProvider>,
  );

describe("SubscriptionAccessProvider", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    getAccess.mockReset();
    Object.defineProperty(document, "visibilityState", { value: "visible", configurable: true });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("carrega o acesso ao montar", async () => {
    getAccess.mockResolvedValue(access("starter", ["crm"]));
    renderProvider();

    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(screen.getByTestId("tier")).toHaveTextContent("starter");
    expect(getAccess).toHaveBeenCalledTimes(1);
  });

  it("atualiza o acesso a cada 30 segundos com o app aberto", async () => {
    getAccess.mockResolvedValueOnce(access("starter", ["crm"]));
    renderProvider();
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(screen.getByTestId("tier")).toHaveTextContent("starter");

    // O provedor confirma o upgrade enquanto a aba está aberta.
    getAccess.mockResolvedValue(access("pro", ["crm", "client_portal"]));
    await act(async () => { await vi.advanceTimersByTimeAsync(30_000); });

    expect(screen.getByTestId("tier")).toHaveTextContent("pro");
    expect(screen.getByTestId("features")).toHaveTextContent("crm,client_portal");
    expect(getAccess).toHaveBeenCalledTimes(2);
  });

  it("atualiza ao voltar para a aba (foco e visibilidade)", async () => {
    getAccess.mockResolvedValue(access("starter", ["crm"]));
    renderProvider();
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });

    getAccess.mockResolvedValue(access("pro", ["crm", "client_portal"]));
    await act(async () => {
      window.dispatchEvent(new Event("focus"));
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByTestId("tier")).toHaveTextContent("pro");

    getAccess.mockResolvedValue(access("pro", ["crm", "client_portal", "reports"]));
    await act(async () => {
      document.dispatchEvent(new Event("visibilitychange"));
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByTestId("features")).toHaveTextContent("crm,client_portal,reports");
  });

  it("não consulta com a aba escondida", async () => {
    getAccess.mockResolvedValue(access("starter", ["crm"]));
    renderProvider();
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(getAccess).toHaveBeenCalledTimes(1);

    Object.defineProperty(document, "visibilityState", { value: "hidden", configurable: true });
    await act(async () => {
      document.dispatchEvent(new Event("visibilitychange"));
      window.dispatchEvent(new Event("focus"));
    });
    expect(getAccess).toHaveBeenCalledTimes(1);

    Object.defineProperty(document, "visibilityState", { value: "visible", configurable: true });
    await act(async () => {
      document.dispatchEvent(new Event("visibilitychange"));
    });
    expect(getAccess).toHaveBeenCalledTimes(2);
  });

  it("mantém o acesso anterior quando a consulta falha (sem piscar o plano)", async () => {
    getAccess.mockResolvedValueOnce(access("pro", ["crm", "client_portal"]));
    renderProvider();
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });

    getAccess.mockRejectedValueOnce(new Error("offline"));
    await act(async () => { await vi.advanceTimersByTimeAsync(30_000); });

    expect(screen.getByTestId("tier")).toHaveTextContent("pro");
    expect(screen.getByTestId("error")).toHaveTextContent("Não foi possível consultar o acesso do plano.");
  });

  it("não consulta sem empresa ativa", async () => {
    renderProvider({ active: false });
    await act(async () => { await vi.advanceTimersByTimeAsync(60_000); });
    expect(getAccess).not.toHaveBeenCalled();
    expect(screen.getByTestId("tier")).toHaveTextContent("-");
  });
});
