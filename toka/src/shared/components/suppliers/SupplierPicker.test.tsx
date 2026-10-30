import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  getAll: vi.fn(),
  getServiceTypes: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  deactivate: vi.fn(),
  toastError: vi.fn(),
  toastSuccess: vi.fn(),
}));

vi.mock("sonner", () => ({ toast: { error: mocks.toastError, success: mocks.toastSuccess } }));
vi.mock("../../services/supplier.service", async () => {
  const actual = await vi.importActual<typeof import("../../services/supplier.service")>(
    "../../services/supplier.service",
  );
  return {
    ...actual,
    supplierService: {
      getAll: mocks.getAll,
      getServiceTypes: mocks.getServiceTypes,
      create: mocks.create,
      update: mocks.update,
      deactivate: mocks.deactivate,
    },
  };
});

import { SupplierPicker, validateSupplierForm } from "./SupplierPicker";
import type { Supplier } from "../../services/supplier.service";

const electrician: Supplier = {
  id: "supplier-1",
  tenant_id: "company-1",
  name: "Carlos Silva",
  service_type: "Elétrica",
  kind: "externo",
  phone: "(11) 99999-0000",
  notes: "Atende zona sul; aceita emergência",
  active: true,
  created_at: "2026-09-01T10:00:00Z",
  updated_at: "2026-09-01T10:00:00Z",
};

const inspector: Supplier = {
  id: "supplier-2",
  tenant_id: "company-1",
  name: "Roberto Alves",
  service_type: "Vistoria",
  kind: "interno",
  email: "roberto@imob.com",
  active: true,
  created_at: "2026-09-02T10:00:00Z",
  updated_at: "2026-09-02T10:00:00Z",
};

const retired: Supplier = {
  ...electrician,
  id: "supplier-3",
  name: "Fornecedor Antigo",
  active: false,
};

function renderPicker(onChange = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  render(
    <QueryClientProvider client={client}>
      <SupplierPicker value="" onChange={onChange} />
    </QueryClientProvider>,
  );
  return { onChange };
}

beforeAll(() => {
  // cmdk (usado pelo combobox) depende de ResizeObserver e scrollIntoView.
  const proto = Element.prototype as unknown as Record<string, unknown>;
  proto.scrollIntoView ??= vi.fn();
  (globalThis as Record<string, unknown>).ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
});

beforeEach(() => {
  vi.clearAllMocks();
  mocks.getAll.mockResolvedValue({ data: [electrician, inspector, retired], meta: { total: 3 } });
  mocks.getServiceTypes.mockResolvedValue({ data: ["Elétrica", "Vistoria"] });
  mocks.create.mockResolvedValue({ data: electrician });
});

describe("SupplierPicker", () => {
  it("reutiliza o prestador cadastrado em vez de digitar o nome", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const { onChange } = renderPicker();

    await user.click(await screen.findByRole("combobox", { name: /Profissional \/ Fornecedor/i }));
    await user.click(await screen.findByRole("option", { name: /Carlos Silva \(Elétrica\)/i }));

    expect(onChange).toHaveBeenCalledWith("supplier-1");
  });

  it("mostra contato e características do prestador escolhido", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
    render(
      <QueryClientProvider client={client}>
        <SupplierPicker value="supplier-1" onChange={vi.fn()} />
      </QueryClientProvider>,
    );

    expect(await screen.findByText("Elétrica")).toBeInTheDocument();
    expect(screen.getByText(/Atende zona sul/)).toBeInTheDocument();
    expect(screen.getByText(/99999-0000/)).toBeInTheDocument();
    expect(screen.getByText(/externo/i)).toBeInTheDocument();
  });

  it("esconde prestador desativado da seleção", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    renderPicker();

    await user.click(await screen.findByRole("combobox", { name: /Profissional \/ Fornecedor/i }));

    expect(await screen.findByRole("option", { name: /Roberto Alves/ })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /Fornecedor Antigo/ })).not.toBeInTheDocument();
  });

  it("abre o cadastro em painel lateral, não em diálogo central", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    renderPicker();

    await user.click(await screen.findByRole("button", { name: /Novo/i }));

    // Padrão do projeto: painel lateral (Sheet) para formulários.
    const panel = await screen.findByTestId("supplier-form-panel");
    expect(panel).toBeInTheDocument();
    expect(panel).toHaveAttribute("data-slot", "sheet-content");
    // Ancorado à direita: é painel lateral, não diálogo central.
    expect(panel.className).toContain("right-0");
    // Acima do painel de agendamento (z-50), que continua montado atrás.
    expect(panel.className).toContain("z-[60]");
    expect(screen.getByText("Novo profissional")).toBeInTheDocument();
  });

  it("fecha o painel e devolve ao gatilho quando o cadastro é cancelado", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    renderPicker();

    await user.click(await screen.findByRole("button", { name: /Novo/i }));
    await screen.findByTestId("supplier-form-panel");

    await user.click(screen.getByRole("button", { name: /^Cancelar$/ }));

    await waitFor(() => expect(screen.queryByTestId("supplier-form-panel")).not.toBeInTheDocument());
    expect(mocks.create).not.toHaveBeenCalled();
  });

  it("cadastra novo prestador com tipo de serviço, contato e características", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    renderPicker();

    await user.click(await screen.findByRole("button", { name: /Novo/i }));
    await user.type(screen.getByLabelText(/^Nome/), "João Santos");
    await user.type(screen.getByLabelText(/Tipo de serviço/), "Hidráulica");
    await user.type(screen.getByLabelText(/Telefone/), "(11) 98888-1111");
    await user.type(screen.getByLabelText(/Características/), "Atende fins de semana");
    await user.click(screen.getByRole("button", { name: /Cadastrar profissional/i }));

    await waitFor(() => expect(mocks.create).toHaveBeenCalledTimes(1));
    expect(mocks.create).toHaveBeenCalledWith({
      name: "João Santos",
      service_type: "Hidráulica",
      kind: "externo",
      phone: "11988881111",
      email: "",
      document: "",
      city: "",
      notes: "Atende fins de semana",
    });
  });

  it("recusa cadastro sem contato — prestador sem contato não é acionável", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    renderPicker();

    await user.click(await screen.findByRole("button", { name: /Novo/i }));
    await user.type(screen.getByLabelText(/^Nome/), "João Santos");
    await user.type(screen.getByLabelText(/Tipo de serviço/), "Hidráulica");
    await user.click(screen.getByRole("button", { name: /Cadastrar profissional/i }));

    await waitFor(() => expect(mocks.toastError).toHaveBeenCalledWith("Informe telefone ou e-mail de contato."));
    expect(mocks.create).not.toHaveBeenCalled();
  });

  it("edita o cadastro sem perder os campos não alterados", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
    render(
      <QueryClientProvider client={client}>
        <SupplierPicker value="supplier-1" onChange={vi.fn()} />
      </QueryClientProvider>,
    );

    await screen.findByText("Elétrica");
    await user.click(screen.getByTitle("Editar cadastro"));
    await user.type(screen.getByLabelText(/Cidade/), "São Paulo");
    await user.click(screen.getByRole("button", { name: /Salvar alterações/i }));

    await waitFor(() => expect(mocks.update).toHaveBeenCalledTimes(1));
    expect(mocks.update).toHaveBeenCalledWith(
      "supplier-1",
      expect.objectContaining({ name: "Carlos Silva", service_type: "Elétrica", city: "São Paulo" }),
    );
  });

  it("desativa o prestador e limpa a seleção do formulário", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const onChange = vi.fn();
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
    render(
      <QueryClientProvider client={client}>
        <SupplierPicker value="supplier-1" onChange={onChange} />
      </QueryClientProvider>,
    );

    await screen.findByText("Elétrica");
    await user.click(screen.getByTitle(/Desativar/));

    await waitFor(() => expect(mocks.deactivate).toHaveBeenCalledWith("supplier-1"));
    await waitFor(() => expect(onChange).toHaveBeenCalledWith(""));
  });
});

describe("validateSupplierForm", () => {
  const base = {
    name: "Carlos",
    serviceType: "Elétrica",
    kind: "externo" as const,
    phone: "11999990000",
    email: "",
    document: "",
    city: "",
    notes: "",
  };

  it("exige nome, tipo de serviço e um contato", () => {
    expect(validateSupplierForm({ ...base, name: "" })).toMatch(/nome/i);
    expect(validateSupplierForm({ ...base, serviceType: "" })).toMatch(/tipo de serviço/i);
    expect(validateSupplierForm({ ...base, phone: "", email: "" })).toMatch(/contato/i);
    expect(validateSupplierForm(base)).toBeNull();
    expect(validateSupplierForm({ ...base, phone: "", email: "c@x.com" })).toBeNull();
  });
});

it("vistorias só oferecem profissionais ativos do tipo correspondente e preenchem o novo cadastro", async () => {
  const user = userEvent.setup({ pointerEventsCheck: 0 });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><SupplierPicker value="" onChange={vi.fn()} inspectorsOnly label="Vistoriador" /></QueryClientProvider>);
  await user.click(await screen.findByRole("combobox", { name: "Vistoriador" }));
  expect(await screen.findByRole("option", { name: /Roberto Alves/ })).toBeInTheDocument();
  expect(screen.queryByRole("option", { name: /Carlos Silva/ })).not.toBeInTheDocument();
  expect(screen.queryByRole("option", { name: /Fornecedor Antigo/ })).not.toBeInTheDocument();
  await user.keyboard("{Escape}");
  await user.click(screen.getByRole("button", { name: /Novo/ }));
  expect(screen.getByLabelText("Tipo de serviço *")).toHaveValue("Vistoria");
});

it("aplica máscaras e impede envio de e-mail/documento inválidos mesmo com telefone válido", async () => {
  const user = userEvent.setup({ pointerEventsCheck: 0 });
  renderPicker();
  await user.click(await screen.findByRole("button", { name: /Novo/i }));
  await user.type(screen.getByLabelText(/^Nome/), "Ana Parceira");
  await user.type(screen.getByLabelText(/Tipo de serviço/), "Vistoria");
  const phone = screen.getByLabelText("Telefone");
  const document = screen.getByLabelText("CPF/CNPJ");
  const email = screen.getByLabelText("E-mail");
  await user.type(phone, "11988887777");
  expect(phone).toHaveValue("(11) 98888-7777");
  await user.type(document, "11111111111");
  expect(document).toHaveValue("111.111.111-11");
  await user.type(email, "ana@invalido");
  await user.click(screen.getByRole("button", { name: /Cadastrar profissional/ }));
  expect(mocks.create).not.toHaveBeenCalled();
  expect(email).toHaveFocus();
  expect(email).toHaveAttribute("aria-invalid", "true");
  expect(document).toHaveAccessibleDescription("Informe um CPF ou CNPJ válido.");
  await user.clear(email);
  await user.type(email, "Ana@Example.com");
  await user.clear(document);
  await user.type(document, "52998224725");
  expect(document).toHaveValue("529.982.247-25");
  expect(document).toHaveAttribute("aria-invalid", "false");
  await user.click(screen.getByRole("button", { name: /Cadastrar profissional/ }));
  await waitFor(() => expect(mocks.create).toHaveBeenCalledWith(expect.objectContaining({
    phone: "11988887777", document: "52998224725", email: "ana@example.com",
  })));
});

it("valida o contato ao sair do campo e aceita telefone fixo com CNPJ", async () => {
  const user = userEvent.setup({ pointerEventsCheck: 0 });
  renderPicker();
  await user.click(await screen.findByRole("button", { name: /Novo/i }));
  await user.type(screen.getByLabelText(/^Nome/), "Empresa 123");
  await user.type(screen.getByLabelText(/Tipo de serviço/), "Hidráulica");
  const phone = screen.getByLabelText("Telefone");
  await user.type(phone, "00999998888");
  await user.tab();
  expect(phone).toHaveAccessibleDescription("Informe um telefone válido com DDD.");
  await user.clear(phone);
  await user.type(phone, "1133334444");
  expect(phone).toHaveValue("(11) 3333-4444");
  await user.type(screen.getByLabelText("CPF/CNPJ"), "04252011000110");
  expect(screen.getByLabelText("CPF/CNPJ")).toHaveValue("04.252.011/0001-10");
  await user.click(screen.getByRole("button", { name: /Cadastrar profissional/ }));
  await waitFor(() => expect(mocks.create).toHaveBeenCalledWith(expect.objectContaining({ phone: "1133334444", document: "04252011000110", email: "" })));
});
