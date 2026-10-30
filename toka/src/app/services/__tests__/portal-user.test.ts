import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";

vi.mock("../../../apps/portal/services/portal-api", () => ({
  default: { get: vi.fn() },
  unwrapPortalData: <T,>(res: any): T => res?.data?.data ?? res?.data ?? res,
}));

import portalApi from "../../../apps/portal/services/portal-api";
import { portalService, type PortalUser } from "../portal.service";

describe("portalService.getCurrentUser normalization", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  const callMe = async (raw: any): Promise<PortalUser> => {
    (portalApi.get as any).mockResolvedValueOnce({ data: { data: raw } });
    return portalService.getCurrentUser();
  };

  it("derives name from fullname when present", async () => {
    const u = await callMe({
      id: "u1",
      firstname: "Lucas",
      lastname: "Ferreira",
      fullname: "Lucas Ferreira",
      role: "user",
      functional_role: "cliente",
    });
    expect(u.name).toBe("Lucas Ferreira");
    expect(u.firstname).toBe("Lucas");
    expect(u.lastname).toBe("Ferreira");
  });

  it("falls back to firstname + lastname when fullname missing", async () => {
    const u = await callMe({
      id: "u2",
      firstname: "Ana",
      lastname: "Souza",
      fullname: "",
      role: "user",
      functional_role: "cliente",
    });
    expect(u.name).toBe("Ana Souza");
  });

  it("splits legacy client accounts with the full name in firstname", async () => {
    const u = await callMe({
      id: "legacy",
      firstname: "  Lucas   Mendes Barros  ",
      lastname: "",
      fullname: "Lucas Mendes Barros",
    });
    expect(u.firstname).toBe("Lucas");
    expect(u.lastname).toBe("Mendes Barros");
    expect(u.name).toBe("Lucas Mendes Barros");
  });

  it("removes an overlapping surname from legacy firstname", async () => {
    const u = await callMe({
      id: "overlap",
      firstname: "Lucas Mendes Barros",
      lastname: "Mendes Barros",
    });
    expect(u.firstname).toBe("Lucas");
    expect(u.lastname).toBe("Mendes Barros");
    expect(u.name).toBe("Lucas Mendes Barros");
  });

  it("preserves explicitly separated compound names", async () => {
    const u = await callMe({ id: "compound", firstname: "Maria Clara", lastname: "de Souza" });
    expect(u.firstname).toBe("Maria Clara");
    expect(u.lastname).toBe("de Souza");
  });

  it("derives editable fields from fullname when separate fields are missing", async () => {
    const u = await callMe({ id: "full", fullname: "Lucas Mendes Barros" });
    expect(u.firstname).toBe("Lucas");
    expect(u.lastname).toBe("Mendes Barros");
  });

  it("does not invent a surname for a single name", async () => {
    const u = await callMe({ id: "single", firstname: "Lucas" });
    expect(u.firstname).toBe("Lucas");
    expect(u.lastname).toBeUndefined();
  });

  it("maps functional_role to a Portuguese label for display", async () => {
    const u = await callMe({
      id: "u3",
      firstname: "Carla",
      fullname: "Carla M",
      role: "user",
      functional_role: "cliente",
    });
    expect(u.role).toBe("Cliente");
    expect(u.functional_role).toBe("cliente");
    expect(u.account_role).toBe("user");
  });

  it("preserves account role when functional_role is absent", async () => {
    const u = await callMe({
      id: "u4",
      firstname: "Joao",
      fullname: "Joao P",
      role: "admin",
    });
    expect(u.role).toBe("admin");
    expect(u.functional_role).toBeUndefined();
  });

  it("returns empty name when no name fields are populated (no hardcoded fallback)", async () => {
    const u = await callMe({
      id: "u5",
      firstname: "",
      lastname: "",
      fullname: "",
      role: "user",
      functional_role: "cliente",
    });
    expect(u.name).toBe("");
    expect(u.role).toBe("Cliente");
  });
});
