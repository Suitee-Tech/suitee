import { afterEach, describe, expect, it, vi } from "vitest";
import { reportClientError, supportErrorMessage } from "./errorDiagnostics";

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });
describe("safe frontend diagnostics", () => {
  it("correlates the visible support code without leaking request or error content", () => {
    const send = vi.fn().mockResolvedValue({});
    vi.stubGlobal("fetch", send);
    vi.spyOn(console, "error").mockImplementation(() => {});
    const id = "11111111-1111-4111-8111-111111111111";
    const error = { message: "private client name", response: { status: 500, headers: { "x-request-id": id }, data: { email: "private@example.invalid" } }, config: { method: "put", url: "/appointments/private@example.invalid?token=secret", data: "private form", headers: { Authorization: "Bearer secret" } } };
    expect(reportClientError(error)).toBe(id);
    expect(supportErrorMessage(error, "Falha ao salvar.")).toContain(id);
    const payload = send.mock.calls[0][1].body;
    expect(JSON.parse(payload)).toMatchObject({ request_id: id, operation: "PUT appointments", status: 500 });
    for (const secret of ["private", "secret", "Bearer", "token=", "email"]) expect(payload).not.toContain(secret);
  });
  it("generates a support code for render failures without reporting exception messages", () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({}));
    vi.spyOn(console, "error").mockImplementation(() => {});
    const error = new Error("confidential form contents");
    const code = reportClientError(error, "render");
    expect(supportErrorMessage(error, "Tente novamente")).toContain(code);
    expect(code).toMatch(/^[a-f0-9-]{36}$/);
  });
});
