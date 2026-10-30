import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { PortalLogin } from "./PortalLogin";

const auth = vi.hoisted(() => ({ requestOtp: vi.fn(), verifyOtp: vi.fn() }));
vi.mock("../context/PortalAuthContext", () => ({ usePortalAuth: () => ({ isAuthenticated: false, ...auth }) }));
vi.mock("../../../shared/components/CompanyLogo", () => ({ CompanyLogo: () => null }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
beforeEach(() => {
  vi.resetAllMocks();
  auth.requestOtp.mockResolvedValue(undefined);
  auth.verifyOtp.mockResolvedValue(undefined);
});
async function requestCode() {
  render(<MemoryRouter initialEntries={["/portal/login"]}><PortalLogin /></MemoryRouter>);
  fireEvent.change(screen.getByPlaceholderText("Ex: carlos@email.com"), { target: { value: "cliente@example.test" } });
  fireEvent.click(screen.getByRole("button", { name: "Enviar Código" }));
  return await screen.findByRole("textbox", { name: "Código de verificação" });
}
describe("login do portal com código", () => {
  it.each(["012345", "01234567"])("envia o código %s completo sem alterar zeros iniciais", async (code) => {
    const input = await requestCode();
    expect(input).toHaveAttribute("inputmode", "numeric");
    expect(input).toHaveAttribute("autocomplete", "one-time-code");
    fireEvent.change(input, { target: { value: code } });
    expect(auth.verifyOtp).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Verificar" }));
    await waitFor(() => expect(auth.verifyOtp).toHaveBeenCalledExactlyOnceWith("cliente@example.test", code));
  });
  it("não envia os primeiros seis dígitos de um código de oito automaticamente", async () => {
    const input = await requestCode();
    fireEvent.change(input, { target: { value: "123456" } });
    expect(auth.verifyOtp).not.toHaveBeenCalled();
    fireEvent.change(input, { target: { value: "1234567" } });
    expect(screen.getByRole("button", { name: "Verificar" })).toBeDisabled();
    fireEvent.keyDown(input, { key: "Enter" });
    expect(auth.verifyOtp).not.toHaveBeenCalled();
    fireEvent.change(input, { target: { value: "12345678" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(auth.verifyOtp).toHaveBeenCalledExactlyOnceWith("cliente@example.test", "12345678"));
  });
  it("permite colar oito dígitos com separadores sem truncar o código", async () => {
    const input = await requestCode();
    fireEvent.paste(input, { clipboardData: { getData: () => "0123 4567" } });
    expect(input).toHaveValue("01234567");
    fireEvent.click(screen.getByRole("button", { name: "Verificar" }));
    await waitFor(() => expect(auth.verifyOtp).toHaveBeenCalledExactlyOnceWith("cliente@example.test", "01234567"));
  });
  it("bloqueia envio incompleto e permite nova tentativa após código inválido", async () => {
    auth.verifyOtp.mockRejectedValueOnce(new Error("Código inválido"));
    const input = await requestCode();
    fireEvent.change(input, { target: { value: "12345" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(screen.getByRole("button", { name: "Verificar" })).toBeDisabled();
    expect(auth.verifyOtp).not.toHaveBeenCalled();
    fireEvent.change(input, { target: { value: "123456" } });
    fireEvent.click(screen.getByRole("button", { name: "Verificar" }));
    await waitFor(() => expect(input).toHaveValue(""));
    fireEvent.change(input, { target: { value: "012345" } });
    fireEvent.click(screen.getByRole("button", { name: "Verificar" }));
    await waitFor(() => expect(auth.verifyOtp).toHaveBeenLastCalledWith("cliente@example.test", "012345"));
  });
});
