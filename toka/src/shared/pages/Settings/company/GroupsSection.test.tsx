import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { GroupsSection } from "./GroupsSection";
import { settingsService, type Group } from "../../../services/settings.service";
import { PERMISSION_CATALOG } from "../../../../lib/permissions/catalog";

// Unificação "Grupos de Usuários" ↔ "Perfis Personalizados".
//
// O grupo é quem ATRIBUI o perfil: ele aponta para um papel do catálogo ou para
// um perfil de acesso (`cr_…`) e é isso que os membros recebem. Antes o diálogo
// criava grupo só com nome/descrição, a exclusão só aparecia em grupos com
// `role_id` começando por `custom_` (um grupo criado pela tela não podia ser
// editado nem excluído) e os perfis da outra aba eram decorativos.

vi.mock("../../../services/settings.service", () => ({
  settingsService: {
    listAccessProfiles: vi.fn(),
    getPermissions: vi.fn(),
    createGroup: vi.fn(),
    updateGroup: vi.fn(),
    deleteGroup: vi.fn(),
  },
}));

const profile = {
  id: "cr_9f1c2b3a-0000-4000-8000-000000000001",
  name: "Corretor Sênior",
  baseRole: "corretor",
  color: "#355872",
  permissions: ["leads:read"],
};

const baseMatrix = PERMISSION_CATALOG.map((p) => ({
  id: p.id,
  label: p.label,
  module: p.module,
  roles: { ...p.defaultRoles },
}));

const customGroup: Group = {
  id: "group-vendas",
  company_id: "company-1",
  name: "Vendas Centro",
  description: "time do centro",
  role_id: profile.id,
  is_default: false,
  members: [],
  created_by: "owner-1",
  created_at: "",
  updated_at: "",
};

const defaultGroup: Group = {
  id: "group-corretor",
  company_id: "company-1",
  name: "Corretor",
  description: "Acesso padrão para corretores",
  role_id: "corretor",
  is_default: true,
  members: [],
  created_by: "system",
  created_at: "",
  updated_at: "",
};

function renderSection(groups: Group[]) {
  const onReload = vi.fn().mockResolvedValue(undefined);
  render(<GroupsSection companyId="company-1" groups={groups} canManageGroups onReload={onReload} />);
  return { onReload };
}

beforeEach(() => {
  vi.mocked(settingsService.listAccessProfiles).mockReset().mockResolvedValue([profile] as never);
  vi.mocked(settingsService.getPermissions).mockReset().mockResolvedValue({
    defaultRole: "corretor",
    permissions: baseMatrix,
    customRoles: [profile],
  } as never);
  vi.mocked(settingsService.createGroup).mockReset().mockResolvedValue(undefined as never);
  vi.mocked(settingsService.updateGroup).mockReset().mockResolvedValue(undefined as never);
  vi.mocked(settingsService.deleteGroup).mockReset().mockResolvedValue(undefined as never);
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("Grupos de Usuários", () => {
  it("mostra o perfil que cada grupo concede", async () => {
    renderSection([customGroup, defaultGroup]);

    expect(await screen.findByText("Vendas Centro")).toBeTruthy();
    expect(await screen.findByText("Corretor Sênior")).toBeTruthy();
    // Grupo padrão continua identificado como tal.
    expect(screen.getByText("padrão")).toBeTruthy();
  });

  it("cria grupo já com o perfil escolhido (catálogo ou personalizado)", async () => {
    renderSection([]);

    await userEvent.click(await screen.findByRole("button", { name: /novo grupo/i }));
    const dialog = await screen.findByRole("dialog");

    await userEvent.type(within(dialog).getByPlaceholderText("Ex: Corretores Centro"), "Corretores Sênior");
    await userEvent.selectOptions(within(dialog).getByLabelText("Perfil de acesso do grupo"), profile.id);
    await userEvent.click(within(dialog).getByRole("button", { name: /salvar grupo/i }));

    await waitFor(() => expect(settingsService.createGroup).toHaveBeenCalledTimes(1));
    expect(settingsService.createGroup).toHaveBeenCalledWith("company-1", {
      name: "Corretores Sênior",
      description: "",
      functional_role: profile.id,
    });
  });

  // Mesma premissa do editor de perfil: o formulário do grupo é painel lateral;
  // o alert fica para a exclusão (ação crítica).
  it("usa painel lateral para o formulário e alerta para a exclusão", async () => {
    renderSection([customGroup]);
    await screen.findByText("Vendas Centro");

    await userEvent.click(screen.getByRole("button", { name: /novo grupo/i }));
    expect(document.querySelector('[data-slot="sheet-content"]')).not.toBeNull();
    expect(await screen.findByLabelText("Perfil de acesso do grupo")).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: /cancelar/i }));

    await userEvent.click(screen.getByRole("button", { name: /excluir grupo vendas centro/i }));
    const alert = await screen.findByRole("alertdialog");
    // O aviso é o de impacto (quantos membros perdem o perfil), não um formulário.
    expect(within(alert).getByText(/perdem o perfil/i)).toBeTruthy();
  });

  it("permite excluir e editar grupos personalizados, e protege os padrão", async () => {
    const { onReload } = renderSection([customGroup, defaultGroup]);
    await screen.findByText("Vendas Centro");

    // Grupo custom: as duas ações existem.
    expect(screen.getByRole("button", { name: /excluir grupo vendas centro/i })).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: /editar grupo vendas centro/i }));
    const dialog = await screen.findByRole("dialog");
    await userEvent.selectOptions(within(dialog).getByLabelText("Perfil de acesso do grupo"), "gestor");
    await userEvent.click(within(dialog).getByRole("button", { name: /salvar grupo/i }));

    await waitFor(() =>
      expect(settingsService.updateGroup).toHaveBeenCalledWith("company-1", "group-vendas", {
        name: "Vendas Centro",
        description: "time do centro",
        functional_role: "gestor",
      }),
    );
    expect(onReload).toHaveBeenCalled();

    // Grupo padrão do sistema: sem exclusão e sem troca de perfil.
    expect(screen.queryByRole("button", { name: /excluir grupo corretor/i })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: /editar grupo corretor/i }));
    const defaultDialog = await screen.findByRole("dialog");
    expect(within(defaultDialog).getByLabelText("Perfil de acesso do grupo")).toBeDisabled();
  });

  it("avisa quando o perfil do grupo foi removido (ninguém fica sem saber)", async () => {
    renderSection([{ ...customGroup, role_id: "cr_perfil_apagado" }]);

    expect(await screen.findByText("Perfil removido")).toBeTruthy();
  });
});
