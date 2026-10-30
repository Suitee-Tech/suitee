import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { TeamSection } from "./TeamSection";
import { settingsService } from "../../../services/settings.service";
import { PERMISSION_CATALOG } from "../../../../lib/permissions/catalog";

// Refino visual pedido: o PAPEL na Equipe e Empresa mostra a LABEL do perfil de
// acesso personalizado (antes saía o id cru `cr_36339ef5-…`, cortado pelo CSS) e
// sinaliza o vínculo órfão ("Perfil removido").

vi.mock("../../../services/settings.service", () => ({
  settingsService: {
    getPermissions: vi.fn(),
    listAccessProfiles: vi.fn(),
    listInvitations: vi.fn(),
    listCompanyMembers: vi.fn(),
    updateCompanyMember: vi.fn(),
    removeCompanyMember: vi.fn(),
    resendInvitation: vi.fn(),
    cancelInvitation: vi.fn(),
    createInvitation: vi.fn(),
    addGroupMember: vi.fn(),
    removeGroupMember: vi.fn(),
    moveMember: vi.fn(),
    updateGroup: vi.fn(),
  },
}));

vi.mock("../../../context/AgentsContext", () => ({
  useAgentsContext: () => ({ refreshBrokers: vi.fn().mockResolvedValue(undefined) }),
}));

const profile = {
  id: "cr_36339ef5-6dc9-4f33-a733-a2cfa0f63011",
  name: "Gestor Comercial Sênior",
  baseRole: "gestor",
  color: "#355872",
  permissions: ["leads:read"],
};

const matrix = PERMISSION_CATALOG.map((p) => ({
  id: p.id,
  label: p.label,
  module: p.module,
  roles: { ...p.defaultRoles },
}));

function renderSection(members: any[], invitations: any[] = []) {
  render(
    <MemoryRouter>
      <TeamSection
        companyId="company-1"
        user={{ id: "owner-1" }}
        teamMembers={members}
        invitations={invitations}
        groups={[]}
        canManageGroupMembers
        onReload={vi.fn()}
      />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.mocked(settingsService.getPermissions).mockReset().mockResolvedValue({
    defaultRole: "corretor",
    permissions: matrix,
    customRoles: [profile],
  } as never);
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("Equipe — papel com perfil de acesso personalizado", () => {
  it("mostra o NOME do perfil no membro e no convite, não o id cr_…", async () => {
    renderSection(
      [
        {
          account_id: "acc-1",
          name: "Ana Souza",
          email: "ana@example.test",
          profile: "member",
          functional_role: profile.id,
          status: "active",
        },
      ],
      [
        {
          id: "inv-1",
          email: "convidado@example.test",
          role: profile.id,
          status: "pending",
        },
      ],
    );

    const chips = await screen.findAllByText("Gestor Comercial Sênior");
    // Um para o vínculo ativo, outro para o convite pendente.
    expect(chips).toHaveLength(2);
    expect(screen.queryByText(/cr_36339ef5/)).toBeNull();
  });

  it("avisa quando o perfil do vínculo foi removido (fail-closed visível)", async () => {
    renderSection([
      {
        account_id: "acc-2",
        name: "Bruno Lima",
        email: "bruno@example.test",
        profile: "member",
        functional_role: "cr_perfil_que_nao_existe_mais",
        status: "active",
      },
    ]);

    expect(await screen.findByText("Perfil removido")).toBeTruthy();
  });
});
