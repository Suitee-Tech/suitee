import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createRef } from "react";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { PermissionsTab } from "./PermissionsTab";
import type { TabRef } from "./_shared";
import { settingsService } from "../../services/settings.service";
import { PERMISSION_CATALOG } from "../../../lib/permissions/catalog";

// P0.2 / A6 — a tela de Permissões não pode salvar quando a carga falhou.
//
// Antes: o estado iniciava em `DEFAULT_MATRIX` e a carga era
// `.catch(console.error)`. Uma falha transitória no GET + "Salvar" persistia a
// matriz inteira de defaults do frontend — que, com os catálogos divergentes
// (46 de 63 permissões), ESCALAVA privilégios: `cliente` ganhava
// `documents:write`/`contracts:read`, `operacoes` ganhava
// `settings:manage_general`, e o toast dizia "salvo com sucesso".
//
// Agora o save é recusado até a carga concluir com sucesso e o motivo fica
// visível.
//
// A segunda parte do arquivo cobre a UNIFICAÇÃO com "Grupos de Usuários": o
// perfil de acesso deixou de ser um rótulo decorativo — ele tem conjunto de
// permissões, é gravado por endpoint próprio (auditado) e a exclusão reatribui
// quem o usava (fail-closed sem lockout silencioso).

vi.mock("../../services/settings.service", () => ({
  settingsService: {
    getPermissions: vi.fn(),
    updatePermissions: vi.fn(),
    listAccessProfiles: vi.fn(),
    createAccessProfile: vi.fn(),
    updateAccessProfile: vi.fn(),
    deleteAccessProfile: vi.fn(),
    listCompanyMembers: vi.fn(),
    listGroups: vi.fn(),
    updateCompanyMember: vi.fn(),
    updateGroup: vi.fn(),
  },
}));

vi.mock("../../context/AgentsContext", () => ({
  useAgentsContext: () => ({ refreshBrokers: vi.fn().mockResolvedValue(undefined) }),
}));

vi.mock("../../context/AuthContext", () => ({
  useAuth: () => ({ user: { current_company_id: "company-1" } }),
}));

const serverMatrix = PERMISSION_CATALOG.map((p) => ({
  id: p.id,
  label: p.label,
  module: p.module,
  roles: { ...p.defaultRoles },
}));

function renderTab() {
  const ref = createRef<TabRef>();
  render(<PermissionsTab ref={ref} onChange={() => {}} />);
  return ref;
}

const profile = {
  id: "cr_9f1c2b3a-0000-4000-8000-000000000001",
  name: "Corretor Sênior",
  description: "Corretor com financeiro de leitura",
  baseRole: "corretor",
  color: "#355872",
  permissions: ["leads:read", "properties:read"],
};

// O componente usa `useIsMobile` (matchMedia), que o jsdom não implementa.
beforeEach(() => {
  vi.stubGlobal(
    "matchMedia",
    vi.fn((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  );
  vi.mocked(settingsService.getPermissions).mockReset();
  vi.mocked(settingsService.updatePermissions).mockReset().mockResolvedValue(undefined as never);
  vi.mocked(settingsService.listAccessProfiles).mockReset().mockResolvedValue([] as never);
  vi.mocked(settingsService.createAccessProfile).mockReset();
  vi.mocked(settingsService.updateAccessProfile).mockReset();
  vi.mocked(settingsService.deleteAccessProfile).mockReset().mockResolvedValue(undefined as never);
  vi.mocked(settingsService.listCompanyMembers).mockReset().mockResolvedValue([] as never);
  vi.mocked(settingsService.listGroups).mockReset().mockResolvedValue([] as never);
  vi.mocked(settingsService.updateCompanyMember).mockReset().mockResolvedValue(undefined as never);
  vi.mocked(settingsService.updateGroup).mockReset().mockResolvedValue(undefined as never);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("PermissionsTab — A6 (salvar só com a matriz carregada)", () => {
  it("recusa salvar quando o GET da matriz falha, e não chama a API", async () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    vi.mocked(settingsService.getPermissions).mockRejectedValue(new Error("503 do core"));

    const ref = renderTab();

    await waitFor(() => {
      expect(screen.getByRole("alert").textContent).toContain("Salvar está bloqueado");
    });

    await expect(ref.current!.save()).rejects.toThrow(/bloqueado|sobrescreveria/i);
    expect(settingsService.updatePermissions).not.toHaveBeenCalled();
  });

  it("salva normalmente quando a matriz carrega, enviando o catálogo completo", async () => {
    vi.mocked(settingsService.getPermissions).mockResolvedValue({
      defaultRole: "corretor",
      permissions: serverMatrix,
      customRoles: [],
    } as never);

    const ref = renderTab();

    await waitFor(() => {
      expect(screen.queryByRole("alert")).toBeNull();
    });

    await ref.current!.save();

    expect(settingsService.updatePermissions).toHaveBeenCalledTimes(1);
    const payload = vi.mocked(settingsService.updatePermissions).mock.calls[0][0] as {
      permissions: Array<{ id: string }>;
      customRoles?: unknown;
    };
    // O catálogo do frontend é GERADO do backend, então o payload cobre
    // exatamente os ids que o servidor conhece — nada é descartado.
    expect(payload.permissions.map((p) => p.id)).toEqual(PERMISSION_CATALOG.map((p) => p.id));
    // Perfis NÃO viajam no PUT da matriz: um documento replace que carregasse os
    // perfis apagaria o acesso de quem os usa quando alguém salvasse só a matriz.
    expect(payload.customRoles).toBeUndefined();
  });
});

// A suíte completa roda em paralelo e o editor renderiza o catálogo inteiro:
// `delay: null` tira a latência artificial do userEvent e o timeout maior evita
// flake por carga (não por lentidão real do fluxo).
const fastUser = () => userEvent.setup({ delay: null });

describe("Perfis de acesso personalizados", () => {
  it("mostra o conjunto efetivo de permissões e quanto o perfil é usado", { timeout: 20000 }, async () => {
    vi.mocked(settingsService.getPermissions).mockResolvedValue({
      defaultRole: "corretor",
      permissions: serverMatrix,
      customRoles: [profile],
    } as never);
    vi.mocked(settingsService.listCompanyMembers).mockResolvedValue([
      { account_id: "acc-1", name: "Ana", functional_role: profile.id, profile: "member", status: "active", email: "a@b.c" },
    ] as never);

    renderTab();

    const section = await screen.findByText("Perfis de Acesso Personalizados");
    expect(section).toBeTruthy();
    const row = (await screen.findByText("Corretor Sênior")).closest("div")!;
    expect(within(row.parentElement as HTMLElement).getByText(/2 de \d+ permissões/)).toBeTruthy();
    expect(await screen.findByText(/1 usuário\(s\)/)).toBeTruthy();
  });

  // Premissa da casa: FORMULÁRIO abre em painel lateral (Sheet). Alert/modal
  // fica reservado a confirmação de ação crítica — por isso o editor de perfil
  // não pode voltar a ser um diálogo centralizado.
  it("abre o editor de perfil no painel lateral direito (não em alert)", { timeout: 20000 }, async () => {
    vi.mocked(settingsService.getPermissions).mockResolvedValue({
      defaultRole: "corretor",
      permissions: serverMatrix,
      customRoles: [],
    } as never);

    renderTab();
    await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());

    const user = fastUser();
    await user.click(screen.getByRole("button", { name: /novo perfil/i }));

    const sheet = document.querySelector('[data-slot="sheet-content"]');
    expect(sheet).not.toBeNull();
    // O painel traz o formulário do perfil e o rodapé de ação.
    expect(within(sheet as HTMLElement).getByLabelText("Nome do perfil")).toBeTruthy();
    expect(within(sheet as HTMLElement).getByRole("button", { name: /criar perfil/i })).toBeTruthy();
  });

  // Regressão: comentário de código NÃO pode aparecer na tela. Um `//` em
  // posição de filho de JSX vira texto literal — foi o que aconteceu no painel
  // do editor ("// Contraste do estado NÃO marcado..." impresso na lista de
  // permissões). O teste olha o texto renderizado, não o fonte.
  it("não imprime comentário de código no painel do editor", { timeout: 20000 }, async () => {
    vi.mocked(settingsService.getPermissions).mockResolvedValue({
      defaultRole: "corretor",
      permissions: serverMatrix,
      customRoles: [],
    } as never);

    renderTab();
    await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());

    const user = fastUser();
    await user.click(screen.getByRole("button", { name: /novo perfil/i }));
    const sheet = document.querySelector('[data-slot="sheet-content"]') as HTMLElement;
    expect(sheet).not.toBeNull();

    const rendered = sheet.textContent || "";
    expect(rendered).not.toContain("//");
    expect(rendered).not.toContain("bg-muted");
    expect(rendered).not.toContain("{/*");
  });

  // Contraste: a permissão NÃO marcada precisa ser visível (borda em token) nos
  // dois temas. Antes era `bg-muted` sobre linha `hover:bg-muted` — sumia.
  it("mantém o toggle de permissão visível quando não marcado", { timeout: 20000 }, async () => {
    vi.mocked(settingsService.getPermissions).mockResolvedValue({
      defaultRole: "corretor",
      permissions: serverMatrix.map((p) => ({ ...p, roles: { ...p.roles, financeiro: false, corretor: true } })),
      customRoles: [profile],
    } as never);

    renderTab();
    await screen.findByText("Corretor Sênior");

    const toggles = document.querySelectorAll('button[aria-pressed] > div');
    expect(toggles.length).toBeGreaterThan(0);
    const unchecked = Array.from(toggles).filter((el) => !el.className.includes("bg-status-ok"));
    expect(unchecked.length).toBeGreaterThan(0);
    unchecked.forEach((el) => {
      expect(el.className).toContain("border-muted-foreground/45");
      expect(el.className).not.toContain("bg-muted ");
    });
  });

  it("cria o perfil no endpoint próprio, com as permissões marcadas", { timeout: 20000 }, async () => {
    vi.mocked(settingsService.getPermissions).mockResolvedValue({
      defaultRole: "corretor",
      permissions: serverMatrix,
      customRoles: [],
    } as never);
    vi.mocked(settingsService.createAccessProfile).mockResolvedValue({ ...profile, permissions: ["leads:read"] } as never);

    renderTab();
    await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());

    const user = fastUser();
    await user.click(screen.getByRole("button", { name: /novo perfil/i }));

    const dialog = await screen.findByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("Nome do perfil"), { target: { value: "Corretor Sênior" } });

    // O editor parte do papel base (corretor) e permite desmarcar.
    const catalogSwitch = within(dialog).getByText("Leads").closest("div")!;
    expect(catalogSwitch).toBeTruthy();
    await user.click(within(dialog).getByRole("button", { name: /criar perfil/i }));

    await waitFor(() => expect(settingsService.createAccessProfile).toHaveBeenCalledTimes(1));
    const payload = vi.mocked(settingsService.createAccessProfile).mock.calls[0][0];
    expect(payload.name).toBe("Corretor Sênior");
    expect(payload.baseRole).toBe("corretor");
    // O conjunto vai explícito: é ele que o servidor aplica.
    expect(Array.isArray(payload.permissions)).toBe(true);
    expect((payload.permissions || []).length).toBeGreaterThan(0);
  });

  it("excluir um perfil em uso reatribui usuários e grupos antes de removê-lo", { timeout: 20000 }, async () => {
    vi.mocked(settingsService.getPermissions).mockResolvedValue({
      defaultRole: "corretor",
      permissions: serverMatrix,
      customRoles: [profile],
    } as never);
    vi.mocked(settingsService.listCompanyMembers).mockResolvedValue([
      { account_id: "acc-1", name: "Ana", functional_role: profile.id, profile: "member", status: "active", email: "a@b.c" },
    ] as never);
    vi.mocked(settingsService.listGroups).mockResolvedValue([
      { id: "g-1", name: "Vendas Centro", description: "time", role_id: profile.id, members: [], company_id: "company-1", created_by: "u", created_at: "", updated_at: "" },
    ] as never);

    renderTab();
    await screen.findByText("Corretor Sênior");

    const user = fastUser();
    await user.click(screen.getByRole("button", { name: /excluir perfil corretor sênior/i }));

    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText(/1 usuário\(s\) e 1 grupo\(s\)/)).toBeTruthy();

    await user.click(within(dialog).getByRole("button", { name: /excluir e reatribuir/i }));

    await waitFor(() => expect(settingsService.deleteAccessProfile).toHaveBeenCalledWith(profile.id));
    expect(settingsService.updateCompanyMember).toHaveBeenCalledWith("company-1", "acc-1", { functional_role: "corretor" });
    expect(settingsService.updateGroup).toHaveBeenCalledWith("company-1", "g-1", {
      name: "Vendas Centro",
      description: "time",
      functional_role: "corretor",
    });
  });
});
