import { act, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import {
  PermissionsProvider,
  usePermissionsContext,
  type PermissionsPayload,
} from "./PermissionsContext";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function Probe() {
  const { can, isLoading, profile } = usePermissionsContext();
  return (
    <output>
      {JSON.stringify({
        isLoading,
        profile,
        canA: can("tenant-a:read"),
        canC: can("deals:read"),
        canB: can("tenant-b:read"),
      })}
    </output>
  );
}

describe("PermissionsProvider", () => {
  it("never exposes or reapplies permissions from the previous tenant", async () => {
    const tenantA = deferred<PermissionsPayload>();
    const tenantB = deferred<PermissionsPayload>();
    const getMyPermissions = vi
      .fn<() => Promise<PermissionsPayload>>()
      .mockImplementationOnce(() => tenantA.promise)
      .mockImplementationOnce(() => tenantB.promise);

    const { rerender } = render(
      <PermissionsProvider
        user={{ current_company_id: "tenant-a" }}
        getMyPermissions={getMyPermissions}
      >
        <Probe />
      </PermissionsProvider>,
    );

    await waitFor(() => expect(getMyPermissions).toHaveBeenCalledTimes(1));

    rerender(
      <PermissionsProvider
        user={{ current_company_id: "tenant-b" }}
        getMyPermissions={getMyPermissions}
      >
        <Probe />
      </PermissionsProvider>,
    );

    expect(screen.getByText(/"isLoading":true/)).toHaveTextContent('"canA":false');
    expect(screen.getByText(/"isLoading":true/)).toHaveTextContent('"canB":false');
    await waitFor(() => expect(getMyPermissions).toHaveBeenCalledTimes(2));

    await act(async () => {
      tenantB.resolve({
        functional_role: "agent",
        profile: "member",
        permissions: ["tenant-b:read"],
      });
      await tenantB.promise;
    });

    await waitFor(() => {
      expect(screen.getByText(/"profile":"member"/)).toHaveTextContent('"canB":true');
    });

    await act(async () => {
      tenantA.resolve({
        functional_role: "admin",
        profile: "admin",
        permissions: ["tenant-a:read"],
      });
      await tenantA.promise;
    });

    expect(screen.getByText(/"profile":"member"/)).toHaveTextContent('"canA":false');
    expect(screen.getByText(/"profile":"member"/)).toHaveTextContent('"canB":true');
  });
});

/**
 * Sessão vencida não pode virar "usuário sem permissão".
 *
 * Antes, a primeira falha do fetch de permissões (401 com o token vencido, ou
 * gateway reiniciando) caía direto no estado vazio: a tela mostrava "acesso
 * restrito" por um instante, até algum outro evento recarregar o contexto.
 */
describe("PermissionsProvider — falha transitória", () => {
  it("repete a busca e mantém as permissões quando a falha é recuperável", async () => {
    const payload: PermissionsPayload = {
      functional_role: "corretor",
      profile: "member",
      permissions: ["deals:read"],
    };
    const getMyPermissions = vi
      .fn<() => Promise<PermissionsPayload>>()
      .mockRejectedValueOnce({ response: { status: 401 } })
      .mockResolvedValueOnce(payload);

    render(
      <PermissionsProvider user={{ current_company_id: "tenant-a" }} getMyPermissions={getMyPermissions}>
        <Probe />
      </PermissionsProvider>,
    );

    await waitFor(() => expect(getMyPermissions).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(screen.getByText(/"isLoading":false/)).toBeInTheDocument());
    expect(screen.getByText(/"canC":true/)).toBeInTheDocument();
  });

  it("não insiste em erro real (403) e assume o estado vazio", async () => {
    const getMyPermissions = vi.fn<() => Promise<PermissionsPayload>>().mockRejectedValue({ response: { status: 403 } });

    render(
      <PermissionsProvider user={{ current_company_id: "tenant-a" }} getMyPermissions={getMyPermissions}>
        <Probe />
      </PermissionsProvider>,
    );

    await waitFor(() => expect(screen.getByText(/"isLoading":false/)).toBeInTheDocument());
    expect(getMyPermissions).toHaveBeenCalledTimes(1);
    expect(screen.getByText(/"canA":false/)).toBeInTheDocument();
  });
});
