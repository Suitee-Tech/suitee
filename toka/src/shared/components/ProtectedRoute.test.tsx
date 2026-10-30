import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ProtectedRoute } from "./ProtectedRoute";

const authMocks = vi.hoisted(() => ({ isAuthenticated: true, isLoading: false }));
const recoveryMocks = vi.hoisted(() => ({ isRecovering: false, recover: vi.fn(async () => true), touch: vi.fn(), lastRecoveredAt: 0 }));

vi.mock("../context/AuthContext", () => ({ useAuth: () => authMocks }));
vi.mock("../context/SessionRecoveryContext", () => ({ useSessionRecovery: () => recoveryMocks }));

function renderRoute() {
  return render(
    <MemoryRouter initialEntries={["/painel"]}>
      <Routes>
        <Route element={<ProtectedRoute />}>
          <Route path="/painel" element={<p>conteúdo da página</p>} />
        </Route>
        <Route path="/login" element={<p>tela de login</p>} />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  authMocks.isAuthenticated = true;
  authMocks.isLoading = false;
  recoveryMocks.isRecovering = false;
});

/**
 * Depois de muito tempo parada, a aba volta com a sessão vencida. Enquanto a
 * renovação acontece a página NÃO pode montar: era isso que fazia o erro
 * ("não foi possível carregar") piscar antes de os dados entrarem.
 */
describe("ProtectedRoute — sessão em recuperação", () => {
  it("mostra 'Restaurando sua sessão' e não monta a página", () => {
    authMocks.isAuthenticated = false;
    recoveryMocks.isRecovering = true;
    renderRoute();

    expect(screen.getByRole("status")).toHaveTextContent("Restaurando sua sessão...");
    expect(screen.queryByText("conteúdo da página")).not.toBeInTheDocument();
  });

  it("libera a página quando não há recuperação em andamento", () => {
    renderRoute();

    expect(screen.getByText("conteúdo da página")).toBeInTheDocument();
  });

  it("durante a recuperação não redireciona para o login", () => {
    authMocks.isAuthenticated = false;
    recoveryMocks.isRecovering = true;
    renderRoute();

    expect(screen.queryByText("tela de login")).not.toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("Restaurando sua sessão...");
  });

  /**
   * Com o usuário já dentro do app, uma renovação em segundo plano (401 em uma
   * consulta qualquer) NÃO pode desmontar a página: isso apagaria estado local
   * (formulário pela metade) e a navegação em curso.
   */
  it("mantém a página montada quando a renovação acontece com sessão ativa", () => {
    authMocks.isAuthenticated = true;
    recoveryMocks.isRecovering = true;
    renderRoute();

    expect(screen.getByText("conteúdo da página")).toBeInTheDocument();
    expect(screen.queryByText("Restaurando sua sessão...")).not.toBeInTheDocument();
  });
});
