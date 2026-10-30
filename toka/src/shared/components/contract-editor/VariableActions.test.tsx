import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { VariableChip } from "./VariableChip";
import { VariableMappingDialog } from "./VariableMappingDialog";
import { TextToVariablePopover } from "./TextToVariablePopover";
import { VariableSidebar } from "./VariableSidebar";
import { VariableStructureHelp } from "./VariableStructureHelp";
import type { SuperDocRef } from "@superdoc-dev/react";
import type { VariableDetectionState } from "../../hooks/useVariableDetection";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }));

describe("contract variable actions", () => {
  it("keeps full English tags visible and preserves copy, keyboard and drag actions", () => {
    const onInsert = vi.fn();
    const key = "current_date_plus_120_business_days_written";
    render(<VariableChip variableKey={key} label="Vencimento por extenso" status="system" onInsert={onInsert} />);

    expect(screen.getByText(`{{${key}}}`)).not.toHaveClass("truncate");
    expect(screen.getByText("Vencimento por extenso")).toBeInTheDocument();
    const chip = screen.getByRole("button", { name: `Copiar {{${key}}}` });
    fireEvent.click(chip);
    fireEvent.keyDown(chip, { key: "Enter" });
    expect(onInsert).toHaveBeenNthCalledWith(1, key);
    expect(onInsert).toHaveBeenNthCalledWith(2, key);

    const dataTransfer = { setData: vi.fn(), effectAllowed: "" };
    fireEvent.dragStart(chip, { dataTransfer });
    expect(dataTransfer.setData).toHaveBeenCalledWith("text/plain", `{{${key}}}`);
    expect(dataTransfer.setData).toHaveBeenCalledWith("text/html", `<span>{{${key}}}</span>`);
    expect(dataTransfer.effectAllowed).toBe("copy");
  });

  it("keeps pending and identified variables actionable", () => {
    const onMap = vi.fn();
    const onRemap = vi.fn();
    render(<><VariableChip variableKey="old_due" label="old_due" status="unidentified" onMap={onMap} /><VariableChip variableKey="buyer_name" label="Comprador" status="identified" onRemap={onRemap} /></>);

    fireEvent.click(screen.getByRole("button", { name: "Mapear variável old_due" }));
    fireEvent.click(screen.getByRole("button", { name: "Alterar {{buyer_name}}" }));
    expect(onMap).toHaveBeenCalledOnce();
    expect(onMap).toHaveBeenCalledWith("old_due");
    expect(onRemap).toHaveBeenCalledWith("buyer_name");
  });

  it("maps a computed date while preserving the replacement occurrence target", () => {
    const onSelect = vi.fn();
    const onOpenChange = vi.fn();
    render(<VariableMappingDialog open onOpenChange={onOpenChange} variableKey="due_date" mode="remap" mappingTarget={{ key: "due_date", mode: "remap", isSingleReplace: true, occurrenceIndex: 2, sourceTag: "{{due_date}}" }} onSelect={onSelect} />);

    fireEvent.click(screen.getByRole("button", { name: /Calcular data ou vencimento/ }));
    fireEvent.change(screen.getByLabelText("Quantidade"), { target: { value: "60" } });
    fireEvent.click(screen.getByRole("button", { name: "Usar variável de data" }));

    expect(onSelect).toHaveBeenCalledWith("due_date", "current_date_plus_60_days", true, 2, undefined, "{{due_date}}");
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("replaces selected text with a computed date and retains its selection target", async () => {
    const onCreateVariable = vi.fn().mockResolvedValue(true);
    const selectionTarget = { from: 8, to: 18 };
    const editorRef = { current: { getInstance: () => ({ activeEditor: { doc: { selection: { current: vi.fn().mockResolvedValue({ text: "22/09/2026", selectionTarget }) } } } }) } } as unknown as React.RefObject<SuperDocRef | null>;
    render(<TextToVariablePopover onCreateVariable={onCreateVariable} editorRef={editorRef} />);

    fireEvent.click(screen.getByRole("button", { name: "Criar Variável" }));
    fireEvent.click(await screen.findByRole("button", { name: /Calcular data ou vencimento/ }));
    fireEvent.change(screen.getByLabelText("Formato da data"), { target: { value: "written" } });
    fireEvent.click(screen.getByRole("button", { name: "Substituir seleção pela data" }));

    await waitFor(() => expect(onCreateVariable).toHaveBeenCalledWith("22/09/2026", "current_date_plus_30_days_written", selectionTarget));
  });

  // INFRA (não é mudança de produto): este caso falhou de forma intermitente
  // por timeout de 5s quando a suíte completa roda em paralelo (jsdom + 177
  // arquivos). O corpo é síncrono; o tempo é do render sob carga. Timeout
  // explícito para a suíte não produzir falha falsa.
  it("exposes the date builder in the sidebar using the existing copy callback", { timeout: 40000 }, () => {
    const onInsertVariable = vi.fn();
    const detection = { identifiedVars: [], unidentifiedKeys: [], allFoundKeys: [], isScanning: false } as unknown as VariableDetectionState;
    render(<VariableSidebar detection={detection} onTriggerScan={vi.fn()} onInsertVariable={onInsertVariable} onMapVariable={vi.fn()} onRemapVariable={vi.fn()} />);

    fireEvent.click(screen.getByText('Calcular data ou vencimento'));
    fireEvent.change(screen.getByLabelText("Quantidade"), { target: { value: "90" } });
    fireEvent.click(screen.getByRole("button", { name: "Copiar variável de data" }));
    expect(onInsertVariable).toHaveBeenCalledWith("current_date_plus_90_days");
  });

  it("shows object field paths and keeps a complete list repetition example draggable", () => {
    const fields = [{ key: "name", label: "Nome", kind: "text" as const }];
    const example = "{{#tenants}}{{name}}{{/tenants}}";
    render(<><VariableStructureHelp variable={{ key: "buyer", label: "Comprador", category: "Pessoas", kind: "object", fields }} /><VariableStructureHelp variable={{ key: "tenants", label: "Inquilinos", category: "Pessoas", kind: "list", fields, example }} /></>);

    expect(screen.getByText("{{buyer.name}}")).toBeInTheDocument();
    expect(screen.getByText("{{name}}")).toBeInTheDocument();
    const dataTransfer = { setData: vi.fn(), effectAllowed: "" };
    fireEvent.dragStart(screen.getByText(example), { dataTransfer });
    expect(dataTransfer.setData).toHaveBeenCalledWith("text/plain", example);
    expect(dataTransfer.effectAllowed).toBe("copy");
  });
});
