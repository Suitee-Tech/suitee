import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { PropertyModal } from "./PropertyModal";

const propertyContext = vi.hoisted(() => ({
  addProperty: vi.fn(),
  updateProperty: vi.fn(),
  setProperties: vi.fn(),
}));

vi.mock("../context/PropertiesContext", () => ({
  usePropertiesContext: () => propertyContext,
}));

vi.mock("../context/LeadsContext", () => ({
  useLeadsContext: () => ({ leads: [] }),
}));

vi.mock("../context/AgentsContext", () => ({
  useAgentsContext: () => ({ agents: [] }),
}));

vi.mock("../context/AuthContext", () => ({
  useAuth: () => ({ user: null }),
}));

vi.mock("../hooks/useRequestScopeGuard", () => ({
  useRequestScopeGuard: () => () => () => true,
}));

vi.mock("./MapPinPicker", () => ({
  MapPinPicker: () => null,
}));

class SuccessfulImageProbe {
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;

  set src(_value: string) {
    queueMicrotask(() => this.onload?.());
  }
}

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}

const moneyFields = [
  { name: "price", digits: "850000", formatted: /^R\$\s850\.000$/ },
  { name: "condominiumFee", digits: "1250", formatted: "1.250" },
  { name: "propertyTax", digits: "4800", formatted: "4.800" },
] as const;

beforeEach(() => {
  propertyContext.addProperty.mockReset();
  propertyContext.updateProperty.mockReset();
  propertyContext.setProperties.mockReset();
  vi.stubGlobal("Image", SuccessfulImageProbe);
  vi.stubGlobal("ResizeObserver", ResizeObserverStub);
  vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:imovel-preview");
  vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => undefined);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function renderPropertyModal() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <PropertyModal isOpen onClose={vi.fn()} />
    </QueryClientProvider>,
  );
}

function getMoneyInput(container: HTMLElement, name: (typeof moneyFields)[number]["name"]) {
  const input = container.querySelector<HTMLInputElement>(`input[name="${name}"]`);
  expect(input).not.toBeNull();
  return input!;
}

async function fillMoneyFields(user: ReturnType<typeof userEvent.setup>, container: HTMLElement) {
  for (const field of moneyFields) {
    const input = getMoneyInput(container, field.name);
    await user.click(input);
    fireEvent.change(input, { target: { value: field.digits } });
    expect(input.value).toEqual(expect.stringMatching(field.formatted));
  }
}

function expectMoneyFieldsPreserved(container: HTMLElement) {
  for (const field of moneyFields) {
    expect(getMoneyInput(container, field.name).value).toEqual(
      expect.stringMatching(field.formatted),
    );
  }
}

function fillRequiredFields(container: HTMLElement) {
  const values = {
    title: "Apartamento teste",
    "address.street": "Avenida Paulista",
    "address.number": "1000",
    "address.neighborhood": "Bela Vista",
    "address.city": "São Paulo",
    "address.stateCode": "SP",
    "address.postalCode": "01310100",
  };

  for (const [name, value] of Object.entries(values)) {
    const input = container.querySelector<HTMLInputElement>(`input[name="${name}"]`);
    expect(input).not.toBeNull();
    fireEvent.change(input!, { target: { value } });
  }
}

describe("PropertyModal — preservação dos campos de valor", () => {
  it("mantém valor, condomínio e IPTU ao carregar uma imagem", async () => {
    const user = userEvent.setup();
    const { container } = renderPropertyModal();
    await fillMoneyFields(user, container);

    const upload = container.querySelector<HTMLInputElement>('input[type="file"]');
    expect(upload).not.toBeNull();
    await user.upload(
      upload!,
      new File(["image"], "imovel.webp", { type: "image/webp" }),
    );

    await waitFor(() => {
      expect(container.querySelector('img[src="blob:imovel-preview"]')).not.toBeNull();
    });
    expectMoneyFieldsPreserved(container);
  });

  it("mantém valor, condomínio e IPTU ao alternar venda e locação", async () => {
    const user = userEvent.setup();
    const { container } = renderPropertyModal();
    await fillMoneyFields(user, container);

    const sale = screen.getByRole("checkbox", { name: "Venda" });
    const rent = screen.getByRole("checkbox", { name: "Locação" });
    expect(sale).toBeChecked();
    expect(rent).not.toBeChecked();

    await user.click(rent);

    expect(sale).toBeChecked();
    expect(rent).toBeChecked();
    expectMoneyFieldsPreserved(container);

    // Além do display, comprova que o react-hook-form preservou números no
    // payload depois do blur e dos re-renders provocados pelos outros campos.
    fillRequiredFields(container);
    expectMoneyFieldsPreserved(container);
    await user.click(screen.getByRole("button", { name: "Criar Imóvel" }));

    await waitFor(() => expect(propertyContext.addProperty).toHaveBeenCalledOnce());
    expect(propertyContext.addProperty).toHaveBeenCalledWith(
      expect.objectContaining({
        price: 850000,
        condominiumFee: 1250,
        propertyTax: 4800,
        availableFor: ["sale", "rent"],
      }),
    );
  });
});
