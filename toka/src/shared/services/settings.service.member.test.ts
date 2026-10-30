import { beforeEach, describe, expect, it, vi } from "vitest";

import api from "./api";
import { settingsService } from "./settings.service";

// P2/M6+M7 — o PATCH de membro é a contraparte do endpoint que faltava:
// alterar papel/perfil e suspender/reativar vínculo sem remover o membro.
// Mantém o módulo real (unwrapApiData etc.) e troca só a instância HTTP.
vi.mock("./api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./api")>();
  return {
    ...actual,
    default: {
      get: vi.fn(),
      post: vi.fn(),
      put: vi.fn(),
      patch: vi.fn(),
      delete: vi.fn(),
    },
  };
});

const mockedApi = vi.mocked(api, true);

describe("settingsService.updateCompanyMember", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("altera o papel funcional com PATCH no membro certo", async () => {
    mockedApi.patch.mockResolvedValue({ data: { success: true, data: { account_id: "acc-2", functional_role: "financeiro" } } } as never);

    const member = await settingsService.updateCompanyMember("company-1", "acc-2", { functional_role: "financeiro" });

    expect(mockedApi.patch).toHaveBeenCalledWith("/companies/company-1/members/acc-2", { functional_role: "financeiro" });
    expect(member?.functional_role).toBe("financeiro");
  });

  it("suspende o vínculo com is_active=false", async () => {
    mockedApi.patch.mockResolvedValue({ data: { success: true, data: { account_id: "acc-2", is_active: false } } } as never);

    const member = await settingsService.updateCompanyMember("company-1", "acc-2", { is_active: false });

    expect(mockedApi.patch).toHaveBeenCalledWith("/companies/company-1/members/acc-2", { is_active: false });
    expect(member?.is_active).toBe(false);
  });

  it("reativa o vínculo com is_active=true", async () => {
    mockedApi.patch.mockResolvedValue({ data: { success: true, data: { account_id: "acc-2", is_active: true } } } as never);

    await settingsService.updateCompanyMember("company-1", "acc-2", { is_active: true });

    expect(mockedApi.patch).toHaveBeenCalledWith("/companies/company-1/members/acc-2", { is_active: true });
  });
});
