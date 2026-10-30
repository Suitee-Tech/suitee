import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Contract } from "../contracts/types";
import type { RentalChargeConfig } from "../rentals/types";

const mutation = vi.hoisted(() => ({
  mutateAsync: vi.fn(),
}));

vi.mock("../hooks/useContracts", () => ({
  useCreateUtilityAccountMutation: () => ({
    mutateAsync: mutation.mutateAsync,
    isPending: false,
  }),
}));

vi.mock("./ExpandableSidePanel", () => ({
  ExpandableSidePanel: ({ open, children }: { open: boolean; children: ReactNode }) => open ? <div>{children}</div> : null,
}));

vi.mock("../ui/searchable-select", () => ({
  SearchableSelect: ({ value, onValueChange, options }: any) => (
    <select aria-label="Item da composição mensal" value={value} onChange={(event) => onValueChange(event.target.value)}>
      {options.map((option: any) => <option key={option.value} value={option.value}>{option.label}</option>)}
    </select>
  ),
}));

import { UtilityAccountModal } from "./UtilityAccountModal";

const contract = {
  id: "contract-1",
  property_id: "property-1",
  client: "Ana Inquilina",
  property_title: "Apartamento Centro",
} as Contract;

const rentalConfig = {
  id: "config-1",
  bundled_items: [
    { key: "condominio", description: "Condomínio", amount_cents: 45_000, enabled: true },
  ],
} as RentalChargeConfig;

describe("UtilityAccountModal", () => {
  beforeEach(() => {
    mutation.mutateAsync.mockReset();
    mutation.mutateAsync.mockResolvedValue({ id: "utility-1" });
  });

  afterEach(() => cleanup());

  it("associa label, obrigatoriedade e estado dos grupos", () => {
    render(<UtilityAccountModal contract={contract} rentalConfig={rentalConfig} onClose={vi.fn()} />);

    expect(screen.getByLabelText(/Concessionária \/ fornecedor/i)).toHaveAttribute("aria-required", "true");
    expect(screen.getByRole("group", { name: "Responsável pela conta" })).toBeInTheDocument();
    expect(screen.getByRole("group", { name: "Modo de cobrança" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Energia/ })).toHaveAttribute("aria-pressed", "true");
  });

  it("envia por padrão uma conta variável, com responsabilidade financeira explícita", async () => {
    render(<UtilityAccountModal contract={contract} rentalConfig={rentalConfig} onClose={vi.fn()} />);

    fireEvent.change(screen.getByPlaceholderText(/Enel, Sabesp/i), { target: { value: "Enel" } });
    fireEvent.click(screen.getByRole("button", { name: /^Vincular conta$/i }));

    await waitFor(() => expect(mutation.mutateAsync).toHaveBeenCalledTimes(1));
    expect(mutation.mutateAsync).toHaveBeenCalledWith(expect.objectContaining({
      contract_id: "contract-1",
      property_id: "property-1",
      provider: "Enel",
      responsible_party: "TENANT",
      billing_mode: "VARIABLE_SEPARATE",
      bundled_item_key: undefined,
    }));
  });

  it("vincula uma conta inclusa ao item pesquisável da composição mensal", async () => {
    render(<UtilityAccountModal contract={contract} rentalConfig={rentalConfig} onClose={vi.fn()} />);

    fireEvent.change(screen.getByPlaceholderText(/Enel, Sabesp/i), { target: { value: "Condomínio Sol" } });
    fireEvent.click(screen.getByRole("button", { name: /Inclusa na cobrança mensal/i }));
    fireEvent.change(screen.getByLabelText("Item da composição mensal"), { target: { value: "condominio" } });
    fireEvent.click(screen.getByRole("button", { name: /^Vincular conta$/i }));

    await waitFor(() => expect(mutation.mutateAsync).toHaveBeenCalledTimes(1));
    expect(mutation.mutateAsync).toHaveBeenCalledWith(expect.objectContaining({
      billing_mode: "BUNDLED",
      bundled_item_key: "condominio",
    }));
  });
});
