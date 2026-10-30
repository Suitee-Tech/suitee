import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import {
  blockInvalidationResource,
  isInvalidationResourceBlocked,
  queryRetryDelay,
  QUERY_MAX_RETRIES,
  resetQueriesForTenant,
  shouldRetryQuery,
  unblockInvalidationResource,
} from "./reactQuery";
import { RECOVERY_RETRY_MAX_DELAY_MS } from "../shared/lib/sessionRecovery";

describe("resetQueriesForTenant", () => {
  it("removes the previous tenant snapshot synchronously", async () => {
    const client = new QueryClient();
    client.setQueryData(["documents", "list"], [{ id: "tenant-a-document" }]);

    const reset = resetQueriesForTenant(client);

    expect(client.getQueryData(["documents", "list"])).toBeUndefined();
    await reset;
  });
});

describe("shouldRetryQuery", () => {
  // Erro real: insistir só atrasa a mensagem para o usuário.
  it.each([403, 404, 409, 422])("does not retry HTTP %s", (status) => {
    expect(shouldRetryQuery(0, { response: { status } })).toBe(false);
  });

  /**
   * O caso do usuário: aba parada por horas, volta ao foco e as consultas saem
   * com o access token vencido (401) ou batem no gateway reiniciando
   * (502/503/504). Sem retry a tela mostrava "não foi possível carregar" por um
   * instante, até o refresh da sessão terminar e a consulta ser refeita.
   */
  it.each([401, 408, 429, 500, 502, 503, 504])("retries recoverable HTTP %s", (status) => {
    expect(shouldRetryQuery(0, { response: { status } })).toBe(true);
  });

  it("retries network failures (no response) and gives up after the cap", () => {
    expect(shouldRetryQuery(0, new Error("Network Error"))).toBe(true);
    expect(shouldRetryQuery(QUERY_MAX_RETRIES - 1, { response: { status: 500 } })).toBe(true);
    expect(shouldRetryQuery(QUERY_MAX_RETRIES, { response: { status: 500 } })).toBe(false);
  });

  it("backs off exponentially with a ceiling", () => {
    expect(queryRetryDelay(0)).toBeLessThan(queryRetryDelay(1));
    expect(queryRetryDelay(1)).toBeLessThan(queryRetryDelay(2));
    expect(queryRetryDelay(10)).toBeLessThanOrEqual(RECOVERY_RETRY_MAX_DELAY_MS);
  });
});

describe("invalidation resource locks", () => {
  it("keeps a resource blocked until every concurrent mutation releases it", () => {
    blockInvalidationResource("deals");
    blockInvalidationResource("deals");

    unblockInvalidationResource("deals");
    expect(isInvalidationResourceBlocked("deals")).toBe(true);

    unblockInvalidationResource("deals");
    expect(isInvalidationResourceBlocked("deals")).toBe(false);
  });
});
