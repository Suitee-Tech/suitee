import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeAll, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";

import { EsteiraDealModal } from "./EsteiraDealModal";

// `PropertyModal` é só o CONTEÚDO do formulário de imóvel (ver
// shared/components/PropertyModal.tsx): fundo, posicionamento e botão de fechar
// vêm do `ExpandableSidePanel`. Este teste fixa essa fiação — o CTA "Cadastrar
// novo imóvel" da esteira precisa abrir o formulário em um painel PRÓPRIO (um
// `role="dialog"` nomeado), e não solto dentro do painel do negócio, onde ele
// nascia no fim do formulário, sem sobreposição e fora da área visível.
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

vi.mock("../../../shared/components/PropertyModal", () => ({
  PropertyModal: ({
    isOpen,
    onClose,
    onCreated,
  }: {
    isOpen: boolean;
    onClose: () => void;
    onCreated?: (property: { id: string; title: string }) => void;
  }) =>
    isOpen ? (
      <div data-testid="property-form-stub">
        Formulário de imóvel
        <button type="button" onClick={onClose}>
          Cancelar
        </button>
        <button
          type="button"
          onClick={() => {
            onCreated?.({ id: "prop-9", title: "Casa Nova" });
            onClose();
          }}
        >
          Simular criação
        </button>
      </div>
    ) : null,
}));

vi.mock("../../../shared/context/CurrentUserContext", () => ({
  useCurrentUser: () => ({ userId: "user-1" }),
}));

vi.mock("../../../shared/context/AuthContext", () => ({
  useAuth: () => ({ user: { id: "user-1", current_company_id: "tenant-1" } }),
}));

vi.mock("../../../shared/context/PropertiesContext", () => ({
  usePropertiesContext: () => ({ properties: [], addProperty: vi.fn(), updateProperty: vi.fn() }),
}));

vi.mock("../../../shared/context/LeadsContext", () => ({
  useLeadsContext: () => ({ leads: [] }),
}));

vi.mock("../../../shared/context/DealsContext", () => ({
  useDealsContext: () => ({
    deals: [],
    dealDTOs: [],
    dealsLoading: false,
    dealsError: null,
    refreshDeals: vi.fn(),
    createDeal: vi.fn(),
    updateDealStatus: vi.fn(),
  }),
}));

vi.mock("../../../shared/hooks/useRequestScopeGuard", () => ({
  useRequestScopeGuard: () => () => () => true,
}));

vi.mock("../../../shared/services/partnership.service", () => ({
  partnershipService: { getById: vi.fn().mockResolvedValue(null) },
}));

vi.mock("../../../shared/services/commissionRules.service", () => ({
  commissionRulesService: { get: vi.fn().mockResolvedValue([]) },
}));

// cmdk (usado pelo combobox de imóvel) depende de ResizeObserver e de
// scrollIntoView, que o jsdom não implementa.
beforeAll(() => {
  (globalThis as Record<string, unknown>).ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
  Element.prototype.scrollIntoView ??= () => {};
});

function renderPanel() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <EsteiraDealModal open onClose={() => {}} />
    </QueryClientProvider>,
  );
}

/** Abre o seletor de imóvel e aciona o CTA de cadastro. */
async function openPropertyForm() {
  fireEvent.click(screen.getByText("Selecione um imóvel"));
  fireEvent.click(await screen.findByText("Cadastrar novo imóvel"));
  return screen.findByRole("dialog", { name: "Novo imóvel" });
}

describe("EsteiraDealModal — cadastro de imóvel pelo seletor", () => {
  it("abre o formulário de imóvel em um painel próprio, por cima do negócio", async () => {
    renderPanel();

    // Sem imóvel cadastrado o seletor abre no estado vazio e já oferece o
    // cadastro — o caminho que evita sair da esteira para a tela de Imóveis.
    const propertyPanel = await openPropertyForm();
    expect(within(propertyPanel).getByTestId("property-form-stub")).toBeInTheDocument();

    // O painel do negócio continua montado atrás (partes/valor preenchidos).
    // Comparação por texto, não por role: enquanto o cadastro está aberto o
    // Radix marca o painel de trás como `aria-hidden`.
    expect(screen.getByText("Nova proposta")).toBeInTheDocument();
  });

  it("fecha o painel de cadastro sem fechar o negócio", async () => {
    renderPanel();

    const propertyPanel = await openPropertyForm();

    // O fechamento do cadastro é o "Cancelar" do próprio formulário de imóvel.
    fireEvent.click(within(propertyPanel).getByRole("button", { name: "Cancelar" }));

    expect(screen.queryByRole("dialog", { name: "Novo imóvel" })).not.toBeInTheDocument();
    // Sem o cadastro aberto o painel do negócio volta a ser exposto para
    // leitores de tela: a asserção por role volta a valer.
    expect(screen.getByRole("dialog", { name: "Detalhes" })).toBeInTheDocument();
  });

  it("vincula o imóvel recém-criado ao negócio e avisa o operador", async () => {
    renderPanel();

    const propertyPanel = await openPropertyForm();
    fireEvent.click(within(propertyPanel).getByRole("button", { name: "Simular criação" }));

    await waitFor(() => expect(toast.success).toHaveBeenCalledWith(expect.stringContaining("Casa Nova")));
    expect(toast.success).toHaveBeenCalledWith(expect.stringContaining("vinculado ao negócio"));
    expect(screen.queryByRole("dialog", { name: "Novo imóvel" })).not.toBeInTheDocument();
  });
});
