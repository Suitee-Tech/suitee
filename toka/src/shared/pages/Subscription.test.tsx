import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { Subscription, textoDoTrial } from "./Subscription";
import { ChangePlanDialog } from "../components/subscription/ChangePlanDialog";
import { safeSubscriptionURL, type SubscriptionSnapshot } from "../services/platformSubscription.service";

const mocks = vi.hoisted(() => ({
  get: vi.fn(), select: vi.fn(), cancel: vi.fn(), cancelChange: vi.fn(), changePaymentMethod: vi.fn(), receipt: vi.fn(),
  canManage: true, tenant: "tenant-a", refreshAccess: vi.fn(),
}));
vi.mock("../context/SubscriptionAccessContext", () => ({
  useSubscriptionAccess: () => ({ access: null, isLoading: false, error: null, refresh: mocks.refreshAccess, hasFeature: () => true }),
}));
vi.mock("../context/AuthContext", () => ({ useAuth: () => ({ user: { current_company_id: mocks.tenant } }) }));
vi.mock("../permissions/usePermission", () => ({ usePermission: () => mocks.canManage }));
vi.mock("../services/platformSubscription.service", async importOriginal => ({ ...await importOriginal<object>(), platformSubscriptionService: mocks }));

const highlights = ["Leads e imóveis sem planilha paralela", "Contratos, locação e financeiro no mesmo lugar"];
const starter = {
  tier: "starter", name: "Starter", amount_cents: 49700, cycle: "MONTHLY" as const,
  description: "Para organizar a operação.", limits: { leads: 200, properties: 500, users: 5 }, contact_only: false, trial_days: 15, highlights,
};
const pro = { ...starter, tier: "pro", name: "Pro", amount_cents: 98700, limits: { leads: 1000, properties: 3000, users: 20 }, highlights: ["Tudo do Starter, com limites 5x maiores"] };
const enterprise = { ...starter, tier: "enterprise", name: "Enterprise", cycle: "MONTHLY" as const, contact_only: true, amount_cents: 0, highlights: undefined, limits: { leads: 0, properties: 0, users: 0 } };
const catalog = [starter, { ...starter, cycle: "ANNUALLY" as const, amount_cents: 477120 }, pro, { ...pro, cycle: "ANNUALLY" as const, amount_cents: 947520 }, enterprise];

const snapshot = (over: Partial<SubscriptionSnapshot> = {}): SubscriptionSnapshot => ({
  subscription: { status: "NONE", plan: null, dev_mode: true },
  plans: catalog, payments: [], usage: { leads: 0, properties: 0, users: 0 }, enabled: true, dev_mode: true, ...over,
});
const active = (plan = starter, usage = { leads: 10, properties: 3, users: 2 }) => snapshot({
  subscription: { status: "ACTIVE", plan, subscription_id: "subs_a", paid_through: "2027-01-01T12:00:00Z", dev_mode: true }, usage,
});

describe("Assinatura e pagamento", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.canManage = true;
    mocks.tenant = "tenant-a";
    window.history.replaceState({}, "", "/assinatura");
    mocks.get.mockResolvedValue(snapshot());
  });

  it("carrega o estado real e mostra a cobrança anual completa", async () => {
    render(<Subscription />);
    expect(await screen.findByText("Plano Atual: Nenhum")).toBeInTheDocument();
    expect(screen.getByText("Nenhum pagamento confirmado.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Anual/ }));
    expect(screen.getByText("R$ 4.771,20")).toBeInTheDocument();
    expect(screen.queryByText(/4092/)).not.toBeInTheDocument();
  });

  it("oferece teste grátis sem citar o provedor no convite para assinar", async () => {
    render(<Subscription />);
    await screen.findByText("Plano Atual: Nenhum");
    const card = screen.getByLabelText("Plano Starter");
    expect(within(card).getByRole("button", { name: "Começar 15 dias grátis" })).toBeInTheDocument();
    expect(within(card).getByText("15 dias grátis")).toBeInTheDocument();
  });

  it("exige confirmação e agenda alterações sem ativá-las", async () => {
    mocks.get.mockResolvedValue(active());
    mocks.select.mockResolvedValue({ status: "ACTIVE", plan: starter, pending_change: { id: "subu_a", plan: { ...starter, cycle: "ANNUALLY" }, requested_at: "2026-10-03T00:00:00Z" } });
    render(<Subscription />);
    await screen.findByText("Plano Atual: Starter");
    fireEvent.click(screen.getByRole("button", { name: /Anual/ }));
    fireEvent.click(screen.getByRole("button", { name: "Mudar para Starter" }));
    expect(mocks.select).not.toHaveBeenCalled();
    const dialog = screen.getByRole("alertdialog");
    expect(dialog).toHaveTextContent("começa na próxima renovação");
    fireEvent.click(within(dialog).getByRole("button", { name: "Agendar mudança" }));
    await waitFor(() => expect(mocks.select).toHaveBeenCalledWith("starter", "ANNUALLY"));
    expect(await screen.findByText(/Mudança para Starter agendada para depois do período atual/)).toBeInTheDocument();
    expect(mocks.refreshAccess).toHaveBeenCalled();
  });

  it("não promete teste grátis para quem já usou o período", async () => {
    mocks.get.mockResolvedValue(snapshot({
      subscription: { status: "NONE", plan: null, trial_granted_at: "2026-09-01T00:00:00Z", dev_mode: true },
    }));
    render(<Subscription />);
    await screen.findByText("Plano Atual: Nenhum");
    const card = screen.getByLabelText("Plano Starter");
    expect(within(card).getByRole("button", { name: "Assinar agora" })).toBeInTheDocument();
    expect(within(card).queryByText(/dias grátis/)).not.toBeInTheDocument();
    expect(within(card).getByText(/O período de teste já foi usado nesta conta/)).toBeInTheDocument();
    expect(screen.getByText(/A partir de agora a assinatura é cobrada na contratação/)).toBeInTheDocument();
  });

  it("preserva os dias que restam do teste em vez de reiniciar", async () => {
    const end = new Date();
    end.setDate(end.getDate() + 9);
    end.setHours(23, 59, 59, 999);
    mocks.get.mockResolvedValue(snapshot({
      subscription: { status: "NONE", plan: null, trial_granted_at: "2026-10-03T00:00:00Z", trial_ends_at: end.toISOString(), dev_mode: true },
    }));
    render(<Subscription />);
    await screen.findByText("Plano Atual: Nenhum");
    const card = screen.getByLabelText("Plano Starter");
    // O teste já começou: o botão convida a assinar (não a "começar" de novo) e
    // os 9 dias restantes continuam anunciados na própria tela.
    expect(within(card).getByRole("button", { name: "Assinar Starter" })).toBeInTheDocument();
    expect(within(card).getByText("Dias que restam do seu período de teste.")).toBeInTheDocument();
    expect(screen.getByText(/Restam 9 dias do seu período de teste/)).toBeInTheDocument();
  });

  it("avisa o que é cobrado ao trocar o cartão sem teste restante", async () => {
    mocks.get.mockResolvedValue(snapshot({
      subscription: { status: "TRIALING", plan: starter, subscription_id: "subs_t", trial_granted_at: "2026-09-01T00:00:00Z", dev_mode: true },
      usage: { leads: 10, properties: 3, users: 2 },
    }));
    render(<Subscription />);
    const panel = await screen.findByRole("region", { name: "Forma de pagamento" });
    fireEvent.click(within(panel).getByRole("button", { name: "Trocar cartão" }));
    expect(screen.getByRole("dialog")).toHaveTextContent("é cobrada no cartão novo já na contratação");
  });

  it("destaca o plano testado e a primeira cobrança durante o período de teste", async () => {
    const trialEnd = new Date(Date.now() + 12 * 86_400_000).toISOString();
    mocks.get.mockResolvedValue(snapshot({
      subscription: { status: "TRIALING", plan: starter, subscription_id: "subs_t", trial_ends_at: trialEnd, dev_mode: true },
      usage: { leads: 10, properties: 3, users: 2 },
    }));
    render(<Subscription />);
    expect(await screen.findByText(/Período de teste: 12 dias restantes/)).toBeInTheDocument();
    expect(screen.getByText(/Primeira cobrança em/)).toBeInTheDocument();
    expect(screen.getAllByText("Período de teste").length).toBeGreaterThan(0);
    expect(screen.getByText(/seu período de teste está em andamento/)).toBeInTheDocument();
  });

  it("conta os dias de teste em dias de calendário", async () => {
    const end = new Date();
    end.setDate(end.getDate() + 15);
    end.setHours(23, 59, 59, 999);
    mocks.get.mockResolvedValue(snapshot({
      subscription: { status: "TRIALING", plan: starter, subscription_id: "subs_t", trial_ends_at: end.toISOString(), dev_mode: true },
    }));
    render(<Subscription />);
    // Fifteen days from today is 15, never 16: the trial ends at the last day's end.
    expect(await screen.findByText(/Período de teste: 15 dias restantes/)).toBeInTheDocument();
  });

  it("mostra que o cancelamento está em andamento", async () => {
    mocks.get.mockResolvedValue(active(pro, { leads: 10, properties: 3, users: 2 }));
    let finish!: (value: unknown) => void;
    mocks.cancel.mockImplementation(() => new Promise(resolve => { finish = resolve; }));
    render(<Subscription />);
    await screen.findByText("Plano Atual: Pro");
    fireEvent.click(screen.getByRole("button", { name: "Cancelar renovação" }));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Confirmar cancelamento" }));
    expect(await within(screen.getByRole("dialog")).findByRole("button", { name: "Cancelando…" })).toBeDisabled();
    finish({ status: "CANCELLED", plan: pro, dev_mode: true });
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("destaca o upgrade agendado com os recursos que passam a valer", async () => {
    mocks.get.mockResolvedValue(snapshot({
      subscription: { status: "TRIALING", plan: starter, subscription_id: "subs_t", trial_ends_at: new Date(Date.now() + 12 * 86_400_000).toISOString(), pending_change: { id: "subu_a", plan: pro, requested_at: "2026-10-03T00:00:00Z" }, dev_mode: true },
    }));
    render(<Subscription />);
    const panel = await screen.findByRole("region", { name: "Mudança agendada" });
    expect(panel).toHaveTextContent("Upgrade para Pro já liberado");
    expect(panel).toHaveTextContent("Todos os membros da empresa já têm os benefícios e limites do Pro");
    expect(panel).toHaveTextContent("R$ 987,00");
    expect(panel).toHaveTextContent("começa na próxima renovação");
    expect(panel).toHaveTextContent(pro.highlights[0]);
  });

  it("conduz o downgrade mostrando o que encolhe", async () => {
    mocks.get.mockResolvedValue(active(pro, { leads: 10, properties: 3, users: 2 }));
    mocks.select.mockResolvedValue({ status: "ACTIVE", plan: pro, pending_change: { id: "subu_d", plan: starter, requested_at: "2026-10-03T00:00:00Z" } });
    render(<Subscription />);
    await screen.findByText("Plano Atual: Pro");
    fireEvent.click(screen.getByRole("button", { name: "Mudar para Starter" }));
    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveTextContent("usuários: 20 → 5");
    expect(dialog).toHaveTextContent("você usa 2");
    expect(dialog).toHaveTextContent("Deixa de estar incluído");
    expect(dialog).toHaveTextContent("Seu uso atual cabe neste plano");
    expect(dialog).toHaveTextContent("A redução entra em vigor após o fim do período atual");
    expect(mocks.select).not.toHaveBeenCalled();
    fireEvent.click(within(dialog).getByRole("button", { name: "Agendar redução para Starter" }));
    await waitFor(() => expect(mocks.select).toHaveBeenCalledWith("starter", "MONTHLY"));
  });

  it("não deixa escolher um plano menor que o uso atual", async () => {
    mocks.get.mockResolvedValue(active(pro, { leads: 10, properties: 3, users: 12 }));
    render(<Subscription />);
    await screen.findByText("Plano Atual: Pro");
    const card = screen.getByLabelText("Plano Starter");
    // Blocked on the card itself: the reason is visible without a click.
    expect(within(card).getByRole("button", { name: "Mudar para Starter" })).toBeDisabled();
    expect(within(card).getByText("Seu uso atual passa deste plano. Reduza antes de mudar.")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("mantém a trava de uso no diálogo de downgrade", async () => {
    // Defense in depth: usage can change between rendering the cards and the
    // confirmation, and the dialog must not let the change through.
    render(
      <ChangePlanDialog
        open
        target={starter}
        current={{ status: "ACTIVE", plan: pro, subscription_id: "subs_a", dev_mode: true }}
        usage={{ leads: 10, properties: 3, users: 12 }}
        busy={false}
        onCancel={() => undefined}
        onConfirm={() => undefined}
      />,
    );
    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveTextContent("Seu uso atual passa do que o Starter cobre em usuários");
    expect(within(dialog).getByRole("button", { name: "Agendar redução para Starter" })).toBeDisabled();
  });

  it("libera o upgrade para toda a equipe e atualiza o acesso na hora", async () => {
    mocks.get.mockResolvedValue(active(starter, { leads: 10, properties: 3, users: 2 }));
    mocks.select.mockResolvedValue({
      status: "ACTIVE", plan: starter,
      pending_change: { id: "subu_up", plan: pro, requested_at: "2026-10-06T00:00:00Z" },
    });
    render(<Subscription />);
    await screen.findByText("Plano Atual: Starter");

    fireEvent.click(screen.getByRole("button", { name: "Fazer upgrade para Pro" }));
    const dialog = screen.getByRole("alertdialog");
    expect(dialog).toHaveTextContent("liberados imediatamente para todos os membros");
    fireEvent.click(within(dialog).getByRole("button", { name: "Liberar benefícios agora" }));

    await waitFor(() => expect(mocks.select).toHaveBeenCalledWith("pro", "MONTHLY"));
    // Quem confirma vê o novo acesso sem esperar o próximo ciclo de atualização.
    await waitFor(() => expect(mocks.refreshAccess).toHaveBeenCalled());
    expect(await screen.findByText(/Upgrade para Pro liberado para todos os membros/)).toBeInTheDocument();
  });

  it("mostra os limites já liberados pelo upgrade confirmado, não os do plano cobrado", async () => {
    mocks.get.mockResolvedValue(snapshot({
      subscription: {
        status: "ACTIVE", plan: starter, subscription_id: "subs_a", paid_through: "2027-01-01T12:00:00Z",
        pending_change: { id: "subu_up", plan: pro, requested_at: "2026-10-06T00:00:00Z" }, dev_mode: true,
      },
      // O acesso já responde Pro: os limites maiores valem para a equipe agora.
      access: {
        access_control_enabled: true, subscription_enabled: true, entitled: true, status: "ACTIVE",
        plan: pro, features: [...(pro.highlights ? ["client_portal"] : [])],
      },
      usage: { leads: 11, properties: 5, users: 5 },
    }));
    render(<Subscription />);
    await screen.findByText("Plano Atual: Starter");

    const row = (label: string) => screen.getByText(label).closest("div")!.parentElement!;
    expect(within(row("Leads cadastrados")).getByText("11 / 1000")).toBeInTheDocument();
    expect(within(row("Usuários da equipe")).getByText("5 / 20")).toBeInTheDocument();
    // Sem "limite atingido": 5 de 20 usuários não é o teto do plano em vigor.
    expect(screen.queryByText("Limite do plano atingido.")).not.toBeInTheDocument();
  });

  it("desfaz uma alteração agendada", async () => {
    mocks.get.mockResolvedValue(snapshot({
      subscription: { status: "ACTIVE", plan: starter, subscription_id: "subs_a", paid_through: "2027-01-01T12:00:00Z", pending_change: { id: "subu_a", plan: pro, requested_at: "2026-10-03T00:00:00Z" }, dev_mode: true },
      usage: { leads: 10, properties: 3, users: 2 },
    }));
    mocks.cancelChange = vi.fn().mockResolvedValue({ status: "ACTIVE", plan: starter, dev_mode: true });
    render(<Subscription />);
    const panel = await screen.findByRole("region", { name: "Mudança agendada" });
    fireEvent.click(within(panel).getByRole("button", { name: "Desfazer alteração" }));
    fireEvent.click(within(screen.getByRole("alertdialog")).getByRole("button", { name: "Desfazer alteração" }));
    await waitFor(() => expect(mocks.cancelChange).toHaveBeenCalled());
    expect(await screen.findByText(/Alteração cancelada/)).toBeInTheDocument();
  });

  it("mostra o cartão em uso e explica a troca durante o teste", async () => {
    mocks.get.mockResolvedValue(snapshot({
      subscription: { status: "TRIALING", plan: starter, subscription_id: "subs_t", trial_ends_at: new Date(Date.now() + 12 * 86_400_000).toISOString(), payment_method: { kind: "CARD", brand: "VISA", last4: "4242" }, dev_mode: true },
      usage: { leads: 10, properties: 3, users: 2 },
    }));
    render(<Subscription />);
    const panel = await screen.findByRole("region", { name: "Forma de pagamento" });
    expect(panel).toHaveTextContent("Cartão de crédito");
    expect(panel).toHaveTextContent("VISA");
    expect(panel).toHaveTextContent("final 4242");
    fireEvent.click(within(panel).getByRole("button", { name: "Trocar cartão" }));
    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveTextContent("não permite substituí-lo em uma assinatura existente");
    expect(dialog).toHaveTextContent("Nada é cobrado agora");
    expect(mocks.changePaymentMethod).not.toHaveBeenCalled();
  });

  it("permite trocar o cartão durante o período pago, adiando a cobrança", async () => {
    mocks.get.mockResolvedValue(active(starter, { leads: 10, properties: 3, users: 2 }));
    render(<Subscription />);
    const panel = await screen.findByRole("region", { name: "Forma de pagamento" });
    expect(within(panel).getByRole("button", { name: "Trocar cartão" })).toBeEnabled();
    expect(panel).toHaveTextContent("Você já pagou até 01/01/2027");
    expect(panel).toHaveTextContent("a nova assinatura só passa a cobrar quando esse período terminar");
  });

  it("só bloqueia a troca quando o período pago é longo demais para adiar", async () => {
    const far = new Date(Date.now() + 200 * 86_400_000).toISOString();
    mocks.get.mockResolvedValue(snapshot({
      subscription: { status: "ACTIVE", plan: starter, subscription_id: "subs_a", paid_through: far, payment_method: { kind: "CARD", brand: "VISA", last4: "4242" }, dev_mode: true },
      usage: { leads: 10, properties: 3, users: 2 },
    }));
    render(<Subscription />);
    const panel = await screen.findByRole("region", { name: "Forma de pagamento" });
    expect(within(panel).getByRole("button", { name: "Trocar cartão" })).toBeDisabled();
    expect(panel).toHaveTextContent("Período pago longo: a troca fica disponível em");
  });

  it("oferece a troca durante o teste em andamento", async () => {
    const end = new Date();
    end.setDate(end.getDate() + 12);
    mocks.get.mockResolvedValue(snapshot({
      subscription: { status: "TRIALING", plan: starter, subscription_id: "subs_t", trial_ends_at: end.toISOString(), trial_granted_at: "2026-10-03T00:00:00Z", payment_method: { kind: "CARD", brand: "VISA", last4: "4242" }, dev_mode: true },
      usage: { leads: 10, properties: 3, users: 2 },
    }));
    render(<Subscription />);
    const panel = await screen.findByRole("region", { name: "Forma de pagamento" });
    const button = within(panel).getByRole("button", { name: "Trocar cartão" });
    expect(button).toBeEnabled();
    expect(panel).toHaveTextContent("Nada foi cobrado ainda: o cartão vale para a primeira cobrança");
    fireEvent.click(button);
    expect(screen.getByRole("dialog")).toHaveTextContent("Nada é cobrado agora: 12 dias continuam valendo");
  });

  it("retoma a assinatura cancelada ao trocar o cartão", async () => {
    mocks.get.mockResolvedValue(snapshot({
      subscription: { status: "CANCELLED", plan: starter, subscription_id: "subs_a", paid_through: new Date(Date.now() + 10 * 86_400_000).toISOString(), payment_method: { kind: "CARD", brand: "VISA", last4: "4242" }, dev_mode: true },
      usage: { leads: 10, properties: 3, users: 2 },
    }));
    render(<Subscription />);
    const panel = await screen.findByRole("region", { name: "Forma de pagamento" });
    expect(within(panel).getByRole("button", { name: "Trocar cartão" })).toBeEnabled();
    fireEvent.click(within(panel).getByRole("button", { name: "Trocar cartão" }));
    expect(screen.getByRole("dialog")).toHaveTextContent("esta troca também reativa o plano no cartão novo");
  });

  it("explica onde o cartão é informado antes de existir assinatura", async () => {
    render(<Subscription />);
    const panel = await screen.findByRole("region", { name: "Forma de pagamento" });
    expect(within(panel).queryByRole("button", { name: "Trocar cartão" })).not.toBeInTheDocument();
    expect(panel).toHaveTextContent("O cartão é informado no checkout, ao contratar um plano.");
  });

  it("libera a troca de cartão quando a cobrança falha", async () => {
    mocks.get.mockResolvedValue(snapshot({
      subscription: { status: "PAST_DUE", plan: starter, subscription_id: "subs_a", paid_through: "2027-01-01T12:00:00Z", payment_method: { kind: "CARD", brand: "MASTERCARD", last4: "4092" }, dev_mode: true },
      usage: { leads: 10, properties: 3, users: 2 },
    }));
    mocks.changePaymentMethod.mockResolvedValue({ status: "PENDING", plan: starter, checkout: { id: "bill_n", url: "https://app.abacatepay.com/pay/bill_n", plan: starter }, dev_mode: true });
    render(<Subscription />);
    const panel = await screen.findByRole("region", { name: "Forma de pagamento" });
    expect(panel).toHaveTextContent("A última cobrança falhou");
    fireEvent.click(within(panel).getByRole("button", { name: "Trocar cartão" }));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Informar novo cartão" }));
    await waitFor(() => expect(mocks.changePaymentMethod).toHaveBeenCalled());
  });

  it("permite voltar a assinar depois de encerrar o teste", async () => {
    mocks.get.mockResolvedValue(snapshot({
      subscription: { status: "CANCELLED", plan: starter, subscription_id: "subs_t", cancellation_reason: "missing_features", dev_mode: true },
      usage: { leads: 10, properties: 3, users: 2 },
    }));
    render(<Subscription />);
    await screen.findByText("Plano Atual: Starter");
    expect(screen.getByText("Renovação cancelada")).toBeInTheDocument();
    expect(screen.getByText(/Assinatura encerrada e nenhuma cobrança foi feita/)).toBeInTheDocument();
    expect(within(screen.getByLabelText("Plano Starter")).getByRole("button", { name: "Começar 15 dias grátis" })).toBeEnabled();
  });

  it("permite voltar antes do fim do período pago, sem cobrar duas vezes", async () => {
    mocks.get.mockResolvedValue(snapshot({
      subscription: { status: "CANCELLED", plan: starter, subscription_id: "subs_t", paid_through: new Date(Date.now() + 10 * 86_400_000).toISOString(), dev_mode: true },
      usage: { leads: 10, properties: 3, users: 2 },
    }));
    render(<Subscription />);
    await screen.findByText("Plano Atual: Starter");
    expect(screen.getByText(/pode voltar a assinar a qualquer momento/)).toBeInTheDocument();
    const card = screen.getByLabelText("Plano Starter");
    expect(within(card).getByRole("button", { name: "Começar 15 dias grátis" })).toBeEnabled();
    expect(within(card).getByText(/a cobrança da nova assinatura só começa quando ele terminar/)).toBeInTheDocument();
  });

  it("bloqueia a volta só quando o período pago é longo demais", async () => {
    const far = new Date(Date.now() + 200 * 86_400_000).toISOString();
    mocks.get.mockResolvedValue(snapshot({
      subscription: { status: "CANCELLED", plan: starter, subscription_id: "subs_t", paid_through: far, dev_mode: true },
      usage: { leads: 10, properties: 3, users: 2 },
    }));
    render(<Subscription />);
    await screen.findByText("Plano Atual: Starter");
    const card = screen.getByLabelText("Plano Starter");
    expect(within(card).getByRole("button", { name: "Começar 15 dias grátis" })).toBeDisabled();
    expect(within(card).getByText(/Período pago longo: uma nova contratação fica disponível em/)).toBeInTheDocument();
  });

  it("celebra a ativação destacando o que o plano libera", async () => {
    window.history.replaceState({}, "", "/assinatura?payment=returned");
    mocks.get.mockResolvedValue(active());
    render(<Subscription />);
    const panel = await screen.findByRole("region", { name: "Assinatura liberada" });
    expect(panel).toHaveTextContent("Tudo pronto: Starter liberado");
    expect(panel).toHaveTextContent(highlights[0]);
    expect(window.location.search).toBe("");
    fireEvent.click(within(panel).getByRole("button", { name: "Começar a usar" }));
    await waitFor(() => expect(screen.queryByRole("region", { name: "Assinatura liberada" })).not.toBeInTheDocument());
  });

  it("incentiva o upgrade quando o uso se aproxima do limite", async () => {
    mocks.get.mockResolvedValue(active(starter, { leads: 190, properties: 3, users: 2 }));
    render(<Subscription />);
    await screen.findByText("Plano Atual: Starter");
    const nudge = screen.getByText(/Você já usa 95% de leads cadastrados do Starter/).parentElement!;
    expect(within(nudge).getByText(/O Pro amplia para 1.000 leads/)).toBeInTheDocument();
    expect(within(nudge).getByRole("button", { name: /Fazer upgrade/ })).toBeInTheDocument();
  });

  it("conduz o cancelamento com consequências, motivo e oferta de redução", async () => {
    mocks.get.mockResolvedValue(active(pro, { leads: 10, properties: 3, users: 2 }));
    mocks.cancel.mockResolvedValue({ status: "CANCELLED", plan: pro, dev_mode: true });
    // Midday keeps the formatted date stable regardless of the test timezone.
    render(<Subscription />);
    await screen.findByText("Plano Atual: Pro");
    fireEvent.click(screen.getByRole("button", { name: "Cancelar renovação" }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveTextContent("Você continua com acesso até 01/01/2027");
    expect(dialog).toHaveTextContent("nada é apagado");
    expect(within(dialog).getByRole("button", { name: "Mudar para Starter e continuar" })).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole("radio", { name: "Falta um recurso que eu preciso" }));
    fireEvent.click(within(dialog).getByRole("button", { name: "Confirmar cancelamento" }));
    await waitFor(() => expect(mocks.cancel).toHaveBeenCalledWith("missing_features"));
    expect(await screen.findByText(/Cancelamento confirmado/)).toBeInTheDocument();
  });

  it("mantém checkout e cancelamento fora de perfis somente leitura", async () => {
    mocks.canManage = false;
    mocks.get.mockResolvedValue(snapshot({
      subscription: { status: "PENDING", plan: null, checkout: { id: "bill_a", url: "https://app.abacatepay.com/pay/bill_a", plan: starter }, dev_mode: true },
    }));
    render(<Subscription />);
    await screen.findByText(/Plano solicitado/);
    expect(screen.queryByRole("button", { name: /Começar|Assinar|Cancelar|Encerrar|upgrade/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /Continuar pagamento/ })).not.toBeInTheDocument();
    expect(screen.getAllByText("O administrador da conta pode mudar o plano.").length).toBeGreaterThan(0);
  });

  it("emite o comprovante do pagamento em vez de dizer indisponível", async () => {
    mocks.get.mockResolvedValue(snapshot({
      subscription: { status: "ACTIVE", plan: starter, subscription_id: "subs_a", paid_through: "2027-01-01T12:00:00Z", dev_mode: true },
      payments: [{ id: "card_1", amount_cents: 49700, paid_at: "2026-10-03T12:00:00Z" }],
    }));
    const open = vi.fn();
    vi.stubGlobal("open", open);
    // Keep the real URL constructor: the page validates links with `new URL`.
    const createUrl = vi.fn(() => "blob:comprovante");
    Object.defineProperty(URL, "createObjectURL", { value: createUrl, configurable: true, writable: true });
    Object.defineProperty(URL, "revokeObjectURL", { value: vi.fn(), configurable: true, writable: true });
    mocks.receipt.mockResolvedValue(new Blob(["%PDF"], { type: "application/pdf" }));
    render(<Subscription />);
    fireEvent.click(await screen.findByRole("button", { name: "Ver comprovante" }));
    await waitFor(() => expect(mocks.receipt).toHaveBeenCalledWith("card_1"));
    await waitFor(() => expect(open).toHaveBeenCalledWith("blob:comprovante", "_blank", "noopener,noreferrer"));
    expect(screen.queryByText("Indisponível")).not.toBeInTheDocument();
  });

  it("mostra o recibo do provedor quando ele existe e avisa quando o comprovante falha", async () => {
    mocks.get.mockResolvedValue(snapshot({
      subscription: { status: "ACTIVE", plan: starter, subscription_id: "subs_a", paid_through: "2027-01-01T12:00:00Z", dev_mode: true },
      payments: [{ id: "card_1", amount_cents: 49700, paid_at: "2026-10-03T12:00:00Z", receipt_url: "https://app.abacatepay.com/receipt/card_1" }],
    }));
    mocks.receipt.mockRejectedValue(new Error("falhou"));
    render(<Subscription />);
    expect(await screen.findByRole("link", { name: "Recibo do provedor" })).toHaveAttribute("href", "https://app.abacatepay.com/receipt/card_1");
    fireEvent.click(screen.getByRole("button", { name: "Ver comprovante" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Não foi possível emitir o comprovante");
  });

  it("mostra erro visível e tenta de novo em vez de inventar faturas", async () => {
    mocks.get.mockRejectedValueOnce(new Error("offline"));
    render(<Subscription />);
    expect(await screen.findByRole("alert")).toHaveTextContent("Não foi possível consultar");
    fireEvent.click(screen.getByRole("button", { name: "Atualizar" }));
    expect(await screen.findByText("Nenhum pagamento confirmado.")).toBeInTheDocument();
  });

  it("ignora respostas da imobiliária anterior", async () => {
    let finish!: (value: SubscriptionSnapshot) => void;
    mocks.get.mockImplementationOnce(() => new Promise(resolve => { finish = resolve; }));
    const view = render(<Subscription />);
    mocks.tenant = "tenant-b";
    view.rerender(<Subscription />);
    await screen.findByText("Plano Atual: Nenhum");
    finish(active({ ...starter, name: "Previous company" }));
    await waitFor(() => expect(screen.queryByText(/Previous company/)).not.toBeInTheDocument());
  });

  it("recusa URLs de checkout e comprovante fora do provedor", () => {
    for (const value of ["javascript:alert(1)", "https://evil.test", "https://app.abacatepay.com.evil.test/pay", "https://user:pass@app.abacatepay.com/pay"]) expect(safeSubscriptionURL(value)).toBeNull();
    expect(safeSubscriptionURL("https://app.abacatepay.com/pay/test")).toBe("https://app.abacatepay.com/pay/test");
  });
});

// A mensagem genérica antiga ("atualize a assinatura para recuperar um checkout
// pendente") aparecia mesmo quando o servidor explicava a causa — caso real: a
// conta do AbacatePay sem cobrança recorrente habilitada, que repetir não
// resolve. O erro do servidor tem precedência.
describe("mensagem de erro do checkout", () => {
  it("mostra a causa enviada pelo servidor quando existe", async () => {
    const { selectErroServidor } = await import("./Subscription");
    expect(selectErroServidor({ response: { data: { message: "A conta do AbacatePay ainda não tem cobrança recorrente habilitada." } } }))
      .toBe("A conta do AbacatePay ainda não tem cobrança recorrente habilitada.");
  });

  it("cai na mensagem genérica quando o servidor não manda nada", async () => {
    const { selectErroServidor } = await import("./Subscription");
    expect(selectErroServidor(new Error("network"))).toContain("Atualize a assinatura para recuperar um checkout pendente");
  });
});

// Quem está em trial precisa de ação, não só de aviso: o banner ganhou o CTA
// que leva aos planos e o texto que explica que os dias restantes continuam
// valendo. E o checkout em aberto passou a ter painel com "Continuar pagamento"
// e "Trocar de plano" — antes o beco sem saída era o botão desabilitado.
describe("trial e checkout pendente", () => {
  it("explica que assinar durante o teste não perde os dias restantes", () => {
    // Data relativa: o fixture fixo vencia no próprio dia da execução e o
    // teste passava a falhar quando "hoje" alcançava 21/10/2026.
    const trialEnds = new Date();
    trialEnds.setDate(trialEnds.getDate() + 10);
    trialEnds.setHours(12, 0, 0, 0);
    const texto = textoDoTrial({ trial_ends_at: trialEnds.toISOString(), trial_granted_at: new Date().toISOString() });
    expect(texto).toContain(trialEnds.toLocaleDateString("pt-BR"));
    expect(texto).toMatch(/continuam valendo|continua valendo/);
  });

  it("consulta o backend ao continuar um checkout pendente em vez de reabrir a URL salva", async () => {
    mocks.get.mockResolvedValue(snapshot({
      subscription: {
        status: "PENDING", plan: null,
        checkout: { id: "bill_old", url: "https://app.abacatepay.com/pay/bill_old", plan: starter },
        dev_mode: true,
      },
    }));
    mocks.select.mockResolvedValue({
      status: "PENDING", plan: null,
      checkout: { id: "bill_new", url: "https://app.abacatepay.com/pay/bill_new", plan: starter },
      dev_mode: true,
    });
    render(<Subscription />);
    const heading = await screen.findByRole("heading", { name: /Falta concluir o pagamento/i });
    const panel = heading.closest("div");
    expect(panel).not.toBeNull();
    fireEvent.click(within(panel as HTMLElement).getByRole("button", { name: /Continuar pagamento/i }));
    await waitFor(() => expect(mocks.select).toHaveBeenCalledWith("starter", "MONTHLY"));
  });
});
