import { useState, useCallback } from "react";
import { Plus, Type, ArrowRight, Search, ChevronLeft } from "lucide-react";
import { toast } from "sonner";
import { SuperDocRef } from "@superdoc-dev/react";

import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
  SheetTrigger,
} from "../../ui/sheet";
import {
  CONTRACT_VARIABLE_CATEGORIES,
  CONTRACT_VARIABLES,
  slugifyVariableName,
} from "../../contracts/contractVariables";
import { ScrollArea } from "../../ui/scroll-area";
import { Button } from "../../ui/button";
import { AdvancedDateVariableBuilder } from "./AdvancedDateVariableBuilder";

interface TextToVariablePopoverProps {
  onCreateVariable: (
    selectedText: string,
    variableKey: string,
    selectionTarget?: unknown,
  ) => boolean | Promise<boolean>;
  editorRef: React.RefObject<SuperDocRef | null>;
}

export function TextToVariablePopover({ onCreateVariable, editorRef }: TextToVariablePopoverProps) {
  const [open, setOpen] = useState(false);
  const [selectedText, setSelectedText] = useState("");
  const [mode, setMode] = useState<"choose" | "existing" | "custom" | "date">("choose");
  const [customKey, setCustomKey] = useState("");
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(false);
  const [selectionTarget, setSelectionTarget] = useState<unknown>();

  const searchLower = search.toLowerCase();

  const handleOpen = useCallback(async (e: React.MouseEvent) => {
    e.preventDefault();

    setLoading(true);

    try {
      // v2: usa Document API (editor.doc.selection.current) para obter texto selecionado
      const superdoc = editorRef.current?.getInstance();
      const editor = superdoc?.activeEditor;

      let text = "";
      let target: unknown;

      if (editor?.doc?.selection?.current) {
        const sel = await editor.doc.selection.current({ includeText: true });
        text = (sel as any)?.text?.trim() || "";
        target = (sel as any)?.selectionTarget ?? (sel as any)?.target;
      }

      // Fallback: tenta window.getSelection() (funciona em modo view/edit de alguns editores)
      if (!text) {
        const sel = window.getSelection();
        text = sel?.toString().trim() || "";
      }

      if (!text) {
        toast.info("Selecione um trecho de texto no documento primeiro");
        return;
      }

      setSelectedText(text);
      setSelectionTarget(target);
      setCustomKey(slugifyVariableName(text));
      setMode("choose");
      setSearch("");
      setOpen(true);
    } catch (err) {
      console.error("Erro ao obter seleção do editor", err);
      toast.info("Selecione um trecho de texto no documento primeiro");
    } finally {
      setLoading(false);
    }
  }, [editorRef]);

  const handleSelectExisting = async (key: string) => {
    const replaced = await onCreateVariable(selectedText, key, selectionTarget);
    setOpen(false);
    if (replaced) toast.success(`Texto substituído por {{${key}}}`);
  };

  const handleCreateCustom = async () => {
    if (!customKey.trim()) {
      toast.error("Nome da variável é obrigatório");
      return;
    }
    const replaced = await onCreateVariable(selectedText, customKey.trim(), selectionTarget);
    setOpen(false);
    if (replaced) toast.success(`Variável {{${customKey.trim()}}} criada`);
  };

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild>
        <Button
          variant="outline"
          onClick={handleOpen}
          disabled={loading}
          className="border-primary/30 text-primary hover:bg-primary/10 hover:text-primary"
          title="Criar variável a partir da seleção de texto"
        >
          <Plus className="w-4 h-4" />
          {loading ? "Lendo..." : "Criar Variável"}
        </Button>
      </SheetTrigger>
      <SheetContent
        side="right"
        className="p-0 gap-0 overflow-hidden !bg-card sm:max-w-[400px] w-full flex flex-col"
      >
        <SheetHeader className="px-5 pt-5 pb-3 border-b border-border">
          <div className="flex items-center gap-2">
            {mode !== "choose" && (
              <Button
                variant="ghost"
                size="icon"
                aria-label="Voltar às opções de variável"
                onClick={() => setMode("choose")}
                className="-ml-1.5 size-8 text-muted-foreground"
              >
                <ChevronLeft className="w-4 h-4" />
              </Button>
            )}
            <SheetTitle className="text-base font-semibold text-foreground">
              {mode === "choose"
                ? "Nova Variável"
                : mode === "existing"
                  ? "Mapear para Existente"
                  : mode === "date"
                    ? "Variável de Data"
                    : "Variável Customizada"}
            </SheetTitle>
          </div>
          <SheetDescription className="sr-only">Substitua o texto selecionado por uma variável existente, uma data calculada ou uma variável customizada.</SheetDescription>
        </SheetHeader>

        {mode === "choose" && (
          <div className="flex flex-col flex-1 p-5 space-y-6">
            <div className="bg-muted border border-border rounded-xl p-4">
              <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
                Texto selecionado
              </p>
              <div className="flex items-start gap-3">
                <div className="w-8 h-8 rounded-lg bg-primary-100 flex items-center justify-center shrink-0">
                  <Type className="w-4 h-4 text-primary-600" />
                </div>
                <p className="text-sm text-foreground font-medium leading-relaxed italic">
                  "{selectedText}"
                </p>
              </div>
            </div>

            <div className="space-y-3">
              <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
                O que deseja fazer?
              </p>
              <button
                type="button"
                onClick={() => setMode("existing")}
                className="w-full flex items-center justify-between p-4 text-sm font-medium rounded-xl border border-border bg-card hover:border-primary-300 hover:shadow-md transition-all group"
              >
                <div className="flex flex-col items-start gap-1">
                  <span className="text-foreground group-hover:text-primary-700">
                    Mapear para variável existente
                  </span>
                  <span className="text-xs text-muted-foreground font-normal">
                    Substituir por uma variável do sistema
                  </span>
                </div>
                <ArrowRight className="w-4 h-4 text-muted-foreground group-hover:text-primary-500 transition-colors" />
              </button>

              <button
                type="button"
                onClick={() => setMode("date")}
                className="w-full flex items-center justify-between p-4 text-sm font-medium rounded-xl border border-border bg-card hover:border-primary-300 hover:shadow-md transition-all group"
              >
                <div className="flex flex-col items-start gap-1">
                  <span className="text-foreground group-hover:text-primary-700">Calcular data ou vencimento</span>
                  <span className="text-xs text-muted-foreground font-normal">Data de referência mais ou menos um prazo</span>
                </div>
                <ArrowRight className="w-4 h-4 text-muted-foreground group-hover:text-primary-500" />
              </button>

              <button
                type="button"
                onClick={() => setMode("custom")}
                className="w-full flex items-center justify-between p-4 text-sm font-medium rounded-xl border border-border bg-card hover:border-primary-300 hover:shadow-md transition-all group"
              >
                <div className="flex flex-col items-start gap-1">
                  <span className="text-foreground group-hover:text-primary-700">
                    Criar variável customizada
                  </span>
                  <span className="text-xs text-muted-foreground font-normal">
                    Criar uma nova variável para este texto
                  </span>
                </div>
                <ArrowRight className="w-4 h-4 text-muted-foreground group-hover:text-primary-500 transition-colors" />
              </button>
            </div>
          </div>
        )}

        {mode === "existing" && (
          <div className="flex flex-col flex-1 min-h-0 bg-muted/50">
            <div className="px-5 py-3 bg-card border-b border-border">
              <div className="relative">
                <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground pointer-events-none" />
                <input
                  type="text"
                  placeholder="Buscar variável..."
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  className="w-full pl-9 pr-3 py-2 bg-muted border border-border rounded-lg text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-primary-100 focus:border-primary-300 transition-colors"
                  autoFocus
                />
              </div>
            </div>
            <ScrollArea className="flex-1 min-h-0">
              <div className="py-3">
                {CONTRACT_VARIABLE_CATEGORIES.map((cat) => {
                  const vars = CONTRACT_VARIABLES.filter((v) => {
                    if (!search) return v.category === cat;
                    return (
                      v.category === cat &&
                      (v.key.toLowerCase().includes(searchLower) ||
                        v.label.toLowerCase().includes(searchLower) ||
                        v.description?.toLowerCase().includes(searchLower))
                    );
                  });
                  if (vars.length === 0) return null;
                  return (
                    <div key={cat} className="mb-4 last:mb-0">
                      <div className="px-5 pb-2">
                        <span className="text-[11px] font-bold text-muted-foreground uppercase tracking-wider">
                          {cat}
                        </span>
                      </div>
                      <div className="px-3 space-y-1">
                        {vars.map((v) => (
                          <button
                            key={v.key}
                            type="button"
                            onClick={() => handleSelectExisting(v.key)}
                            className="w-full flex items-center justify-between px-3 py-2.5 bg-card hover:bg-primary-50 shadow-sm hover:shadow-md border border-border hover:border-primary-200 transition-all text-left rounded-xl group"
                          >
                            <div className="flex flex-col gap-1 min-w-0">
                              <span className="break-all font-mono text-xs text-primary-700 font-semibold group-hover:text-primary-800">
                                {`{{${v.key}}}`}
                              </span>
                              <span className="text-xs text-muted-foreground group-hover:text-muted-foreground">
                                {v.label}
                              </span>
                              {v.description && <span className="text-[11px] text-muted-foreground leading-relaxed">{v.description}</span>}
                            </div>
                            <div className="opacity-0 group-hover:opacity-100 px-2 py-1 bg-card border border-border rounded text-caption font-semibold text-muted-foreground shadow-sm transition-opacity ml-2 shrink-0">
                              Selecionar
                            </div>
                          </button>
                        ))}
                      </div>
                    </div>
                  );
                })}
              </div>
            </ScrollArea>
          </div>
        )}

        {mode === "date" && (
          <ScrollArea className="flex-1 min-h-0">
            <div className="p-5">
              <AdvancedDateVariableBuilder onSelect={handleSelectExisting} actionLabel="Substituir seleção pela data" />
            </div>
          </ScrollArea>
        )}

        {mode === "custom" && (
          <div className="p-5 space-y-6 bg-card flex-1">
            <div>
              <label className="text-sm font-semibold text-foreground mb-2 block">
                Nome da variável
              </label>
              <p className="text-xs text-muted-foreground mb-3">
                Escolha um nome em inglês, único, sem espaços ou caracteres especiais.
              </p>
              <div className="flex items-center gap-2 bg-muted border border-border rounded-xl px-3 py-2.5 focus-within:ring-2 focus-within:ring-primary-100 focus-within:border-primary-300 transition-colors shadow-inner">
                <span className="text-sm text-muted-foreground font-mono font-bold">
                  {"{{"}
                </span>
                <input
                  type="text"
                  value={customKey}
                  onChange={(e) =>
                    setCustomKey(
                      e.target.value.replace(/[^a-zA-Z0-9_]/g, "_").toLowerCase(),
                    )
                  }
                  className="flex-1 bg-transparent text-sm font-mono text-foreground focus:outline-none placeholder:text-muted-foreground/60"
                  placeholder="variable_name"
                  autoFocus
                />
                <span className="text-sm text-muted-foreground font-mono font-bold">
                  {"}}"}
                </span>
              </div>
            </div>
            <div className="flex gap-3 pt-4 border-t border-border mt-auto">
              <Button variant="outline" onClick={() => setMode("choose")} className="flex-1">
                Cancelar
              </Button>
              <Button onClick={handleCreateCustom} className="flex-1">
                Criar Variável
              </Button>
            </div>
          </div>
        )}
      </SheetContent>
    </Sheet>
  );
}
