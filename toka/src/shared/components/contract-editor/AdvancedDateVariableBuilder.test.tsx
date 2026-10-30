import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { AdvancedDateVariableBuilder } from "./AdvancedDateVariableBuilder";

describe("AdvancedDateVariableBuilder", () => {
  it("shows and sends the current date plus an arbitrary number of days", () => {
    const onSelect = vi.fn();
    render(<AdvancedDateVariableBuilder onSelect={onSelect} />);

    expect(screen.getByText("{{current_date_plus_30_days}}")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Quantidade"), { target: { value: "45" } });
    expect(screen.getByText("{{current_date_plus_45_days}}")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Copiar variável de data" }));
    expect(onSelect).toHaveBeenCalledWith("current_date_plus_45_days");
  });

  it("supports another reference, subtraction, business days and an ISO result", () => {
    const onSelect = vi.fn();
    render(<AdvancedDateVariableBuilder onSelect={onSelect} actionLabel="Usar data" />);

    fireEvent.change(screen.getByLabelText("Data de referência"), { target: { value: "contract_end_date" } });
    fireEvent.change(screen.getByLabelText("Operação"), { target: { value: "minus" } });
    fireEvent.change(screen.getByLabelText("Quantidade"), { target: { value: "5" } });
    fireEvent.change(screen.getByLabelText("Unidade do prazo"), { target: { value: "business_days" } });
    fireEvent.change(screen.getByLabelText("Formato da data"), { target: { value: "iso" } });

    expect(screen.getByText(/feriados são contados/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Usar data" }));
    expect(onSelect).toHaveBeenCalledWith("contract_end_date_minus_5_business_days_iso");
  });

  it("prevents invalid offsets and applies the limit of the chosen unit", () => {
    const onSelect = vi.fn();
    render(<AdvancedDateVariableBuilder onSelect={onSelect} />);
    const amount = screen.getByLabelText("Quantidade");
    const submit = screen.getByRole("button", { name: "Copiar variável de data" });

    for (const value of ["", "-1", "1.5", "36601"]) {
      fireEvent.change(amount, { target: { value } });
      expect(submit).toBeDisabled();
      fireEvent.click(submit);
    }
    expect(onSelect).not.toHaveBeenCalled();

    fireEvent.change(amount, { target: { value: "0" } });
    expect(submit).toBeEnabled();
    fireEvent.change(screen.getByLabelText("Unidade do prazo"), { target: { value: "years" } });
    expect(amount).toHaveAttribute("max", "100");
    fireEvent.change(amount, { target: { value: "101" } });
    expect(submit).toBeDisabled();
    expect(screen.getByRole("alert")).toHaveTextContent("0 a 100");
  });
});
