import { describe, it, expect, vi, beforeAll } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { LeadPicker, type LeadOption } from "./LeadPicker";

const options: LeadOption[] = [
  { id: "a", name: "Amanda Botelho", email: "amanda@example.com", phone: "11999990000" },
  { id: "b", name: "Djavan Alagoano" }, // no email
];

describe("LeadPicker trigger label", () => {
  it("shows the placeholder when nothing is selected", () => {
    render(<LeadPicker leads={options} value="" onChange={vi.fn()} placeholder="Selecione um proprietário…" />);
    expect(screen.getByRole("combobox")).toHaveTextContent("Selecione um proprietário…");
  });

  it("shows 'name · email' for a selected lead that has an email", () => {
    render(<LeadPicker leads={options} value="a" onChange={vi.fn()} />);
    expect(screen.getByRole("combobox")).toHaveTextContent("Amanda Botelho · amanda@example.com");
  });

  it("shows just the name for a selected lead without an email", () => {
    render(<LeadPicker leads={options} value="b" onChange={vi.fn()} />);
    const trigger = screen.getByRole("combobox");
    expect(trigger).toHaveTextContent("Djavan Alagoano");
    expect(trigger).not.toHaveTextContent("·");
  });

  it("falls back to the placeholder when the selected id is not in the list", () => {
    render(<LeadPicker leads={options} value="missing" onChange={vi.fn()} placeholder="Selecione um proprietário…" />);
    expect(screen.getByRole("combobox")).toHaveTextContent("Selecione um proprietário…");
  });
});

describe("LeadPicker search + select", () => {
  beforeAll(() => {
    // jsdom lacks a few DOM APIs that Radix Popover + cmdk touch when opening.
    const proto = Element.prototype as unknown as Record<string, unknown>;
    proto.scrollIntoView ??= vi.fn();
    proto.hasPointerCapture ??= vi.fn(() => false);
    proto.setPointerCapture ??= vi.fn();
    proto.releasePointerCapture ??= vi.fn();
    (globalThis as Record<string, unknown>).ResizeObserver ??= class {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
  });

  it("opens, filters by typed text (name/email), and reports the picked id", async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    render(<LeadPicker leads={options} value="" onChange={onChange} />);

    await user.click(screen.getByRole("combobox"));
    const input = await screen.findByPlaceholderText(/buscar/i);

    // Filter to a single match by name; the other option is filtered out.
    await user.type(input, "djavan");
    expect(screen.queryByText("Amanda Botelho")).toBeNull();

    await user.click(screen.getByText("Djavan Alagoano"));
    expect(onChange).toHaveBeenCalledWith("b");
  });

  it("matches on email fragments too", async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    render(<LeadPicker leads={options} value="" onChange={onChange} />);

    await user.click(screen.getByRole("combobox"));
    const input = await screen.findByPlaceholderText(/buscar/i);
    await user.type(input, "amanda@example");

    expect(screen.queryByText("Djavan Alagoano")).toBeNull();
    await user.click(screen.getByText("Amanda Botelho"));
    expect(onChange).toHaveBeenCalledWith("a");
  });
});
