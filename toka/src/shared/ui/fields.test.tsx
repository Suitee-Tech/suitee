import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import { Form } from "./form";
import { TextField, DocumentField, MoneyField } from "./fields";
import { zDocument } from "../validation/fields";

const schema = z.object({
  name: z.string().min(2, "Mínimo 2 caracteres"),
  tax_id: zDocument({ mode: "cnpj", required: true }),
  price: z.number().nonnegative(),
});
type Values = z.infer<typeof schema>;

function Harness({ onSubmit }: { onSubmit: (v: Values) => void }) {
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { name: "", tax_id: "", price: undefined },
  });
  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)}>
        <TextField control={form.control} name="name" label="Nome" required />
        <DocumentField control={form.control} name="tax_id" label="CNPJ" mode="cnpj" required />
        <MoneyField control={form.control} name="price" label="Preço" />
        <button type="submit">Salvar</button>
      </form>
    </Form>
  );
}

describe("field wrappers", () => {
  it("associa o label ao input", () => {
    render(<Harness onSubmit={vi.fn()} />);
    expect(screen.getByLabelText(/nome/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/cnpj/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/nome/i)).toHaveAttribute("aria-required", "true");
    expect(screen.getByLabelText(/preço/i)).not.toHaveAttribute("aria-required");
  });

  it("aplica a estilização base do Input também nos campos mascarados", () => {
    render(<Harness onSubmit={vi.fn()} />);
    // DocumentField/MoneyField renderizam um <input> próprio; devem herdar a
    // classe base do primitivo Input (regressão de estilo).
    expect(screen.getByLabelText(/cnpj/i)).toHaveClass("border-input");
    expect(screen.getByLabelText(/preço/i)).toHaveClass("border-input");
  });

  it("mostra mensagens de erro e marca aria-invalid em submit inválido", async () => {
    render(<Harness onSubmit={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: /salvar/i }));
    expect(await screen.findByText("Mínimo 2 caracteres")).toBeInTheDocument();
    const error = screen.getByText("Mínimo 2 caracteres");
    expect(screen.getByText("CNPJ inválido")).toBeInTheDocument();
    const input = screen.getByLabelText(/nome/i);
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(input).toHaveAttribute("aria-describedby", expect.stringContaining(error.id));
    expect(error).toHaveAttribute("role", "alert");
  });

  it("submete os valores parseados quando válido", async () => {
    const onSubmit = vi.fn();
    render(<Harness onSubmit={onSubmit} />);
    fireEvent.change(screen.getByLabelText(/nome/i), { target: { value: "Imobiliaria X" } });
    fireEvent.change(screen.getByLabelText(/cnpj/i), { target: { value: "11222333000181" } });
    fireEvent.change(screen.getByLabelText(/preço/i), { target: { value: "150000" } });
    fireEvent.click(screen.getByRole("button", { name: /salvar/i }));
    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ name: "Imobiliaria X", tax_id: "11222333000181", price: 1500 }),
      expect.anything(),
    );
  });
});
