import { useState, useEffect } from "react";
import { CheckCircle, ArrowRight, Search, FileCode2, ChevronLeft } from "lucide-react";
import { toast } from "sonner";

import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "../../ui/sheet";
import { CONTRACT_VARIABLE_CATEGORIES, CONTRACT_VARIABLES } from "../../contracts/contractVariables";
import { ScrollArea } from "../../ui/scroll-area";
import { Button } from "../../ui/button";
import type { DocxVariableMarker } from "../../utils/docx";
import { AdvancedDateVariableBuilder } from "./AdvancedDateVariableBuilder";

interface VariableMappingDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  variableKey: string;
  mode: "map" | "remap";
  mappingTarget?: {
    key: string;
    mode: "map" | "remap";
    isSingleReplace?: boolean;
    occurrenceIndex?: number;
    marker?: DocxVariableMarker;
    sourceTag?: string;
  };
  onSelect: (
    oldKey: string,
    newKey: string,
    isSingleReplace?: boolean,
    occurrenceIndex?: number,
    marker?: DocxVariableMarker,
    sourceTag?: string,
  ) => void;
}

export function VariableMappingDialog({
  open,
  onOpenChange,
  variableKey,
  mode,
  mappingTarget,
  onSelect,
}: VariableMappingDialogProps) {
  const [search, setSearch] = useState("");
  const [view, setView] = useState<"choose" | "existing" | "custom" | "date">("choose");
  const [customKey, setCustomKey] = useState("");

  const searchLower = search.toLowerCase();

  useEffect(() => {
    if (open) {
      setView("choose");
      setSearch("");
      setCustomKey(variableKey.replace(/[^a-zA-Z0-9_]/g, "_").toLowerCase());
    }
  }, [open, variableKey]);

  const handleSelect = (newKey: string) => {
    onSelect(
      variableKey,
      newKey,
      mappingTarget?.isSingleReplace,
      mappingTarget?.occurrenceIndex,
      mappingTarget?.marker,
      mappingTarget?.sourceTag,
    );
    onOpenChange(false);
    setSearch("");
  };

  const handleCreateCustom = () => {
    if (!customKey.trim()) {
      toast.error("Nome da variável é obrigatório");
      return;
    }
    onSelect(
      variableKey,
      customKey.trim(),
      mappingTarget?.isSingleReplace,
      mappingTarget?.occurrenceIndex,
      mappingTarget?.marker,
      mappingTarget?.sourceTag,
    );
    onOpenChange(false);
  };

  return (
    <Sheet
      open={open}
      onOpenChange={(v) => {
        onOpenChange(v);
        if (!v) setSearch("");
      }}
    >
      <SheetContent
        side="right"
        className="p-0 gap-0 overflow-hidden !bg-card sm:max-w-[400px] w-full flex flex-col"
      >
        <SheetHeader className="px-5 pt-5 pb-3 border-b border-border">
          <div className="flex items-center gap-2">
            {view !== "choose" && (
              <Button
                variant="ghost"
                size="icon"
                aria-label="Voltar às opções de variável"
                onClick={() => setView("choose")}
                className="-ml-1.5 size-8 text-muted-foreground"
              >
                <ChevronLeft className="w-4 h-4" />
              </Button>
            )}
            <SheetTitle className="text-base font-semibold text-foreground pr-6">
              {view === "choose"
                ? mode === "map"
                  ? "Variável Pendente"
                  : "Alterar variável"
                : view === "existing"
                  ? "Mapear para Existente"
                  : view === "date"
                    ? "Variável de Data"
                    : "Criar Customizada"}
            </SheetTitle>
          </div>
          {view === "choose" && (
            <SheetDescription asChild>
              <div className="flex flex-wrap items-center gap-2 mt-2">
                <span className="text-xs text-muted-foreground">Variável no documento:</span>
                <code className="break-all px-2 py-1 bg-status-warning-soft border border-status-warning/30 rounded-md text-status-warning font-mono text-xs font-semibold">
                  {`{{${variableKey}}}`}
                </code>
              </div>
            </SheetDescription>
          )}
          {view !== "choose" && <SheetDescription className="sr-only">Escolha a variável que substituirá {`{{${variableKey}}}`} no documento.</SheetDescription>}
        </SheetHeader>

        {view === "choose" && (
          <div className="flex flex-col flex-1 p-5 space-y-6">
            <div className="space-y-3">
              <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
                O que deseja fazer?
              </p>

              <button
                type="button"
                onClick={() => setView("existing")}
                className="w-full flex items-center justify-between p-4 text-sm font-medium rounded-xl border border-border bg-card hover:border-primary-300 hover:shadow-md transition-all group"
              >
                <div className="flex flex-col items-start gap-1">
                  <span className="text-foreground group-hover:text-primary-700">
                    {mode === "map"
                      ? "Mapear para variável do sistema"
                      : "Trocar por variável do sistema"}
                  </span>
                  <span className="text-xs text-muted-foreground font-normal">
                    {mode === "map"
                      ? "Vincular a uma variável existente"
                      : "Substituir por outra existente"}
                  </span>
                </div>
                <ArrowRight className="w-4 h-4 text-muted-foreground group-hover:text-primary-500 transition-colors" />
              </button>

              <button
                type="button"
                onClick={() => setView("date")}
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
                onClick={() => setView("custom")}
                className="w-full flex items-center justify-between p-4 text-sm font-medium rounded-xl border border-border bg-card hover:border-primary-300 hover:shadow-md transition-all group"
              >
                <div className="flex flex-col items-start gap-1">
                  <span className="text-foreground group-hover:text-primary-700">
                    Manter como customizada
                  </span>
                  <span className="text-xs text-muted-foreground font-normal">
                    Registrar esta variável no sistema
                  </span>
                </div>
                <ArrowRight className="w-4 h-4 text-muted-foreground group-hover:text-primary-500 transition-colors" />
              </button>
            </div>
          </div>
        )}

        {view === "existing" && (
          <div className="border-t border-border flex-1 min-h-0 flex flex-col bg-muted/50">
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
                            onClick={() => handleSelect(v.key)}
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
                            {v.key === variableKey ? (
                              <CheckCircle className="w-4 h-4 text-status-ok shrink-0 ml-2" />
                            ) : (
                              <div className="opacity-0 group-hover:opacity-100 px-2 py-1 bg-card border border-border rounded text-caption font-semibold text-muted-foreground shadow-sm transition-opacity ml-2 shrink-0">
                                Selecionar
                              </div>
                            )}
                          </button>
                        ))}
                      </div>
                    </div>
                  );
                })}
                {search &&
                  CONTRACT_VARIABLES.filter(
                    (v) =>
                      v.key.toLowerCase().includes(searchLower) ||
                      v.label.toLowerCase().includes(searchLower) ||
                      v.description?.toLowerCase().includes(searchLower),
                  ).length === 0 && (
                    <div className="flex flex-col items-center justify-center py-12 text-center">
                      <div className="w-12 h-12 rounded-full bg-muted flex items-center justify-center mb-4">
                        <FileCode2 className="w-6 h-6 text-muted-foreground" />
                      </div>
                      <p className="text-sm font-medium text-muted-foreground">
                        Nenhuma variável encontrada
                      </p>
                      <p className="text-xs text-muted-foreground mt-1 max-w-[200px] leading-relaxed">
                        Tente buscar por outros termos ou verifique a ortografia.
                      </p>
                    </div>
                  )}
              </div>
            </ScrollArea>
          </div>
        )}

        {view === "date" && (
          <ScrollArea className="flex-1 min-h-0">
            <div className="p-5">
              <AdvancedDateVariableBuilder onSelect={handleSelect} actionLabel="Usar variável de data" />
            </div>
          </ScrollArea>
        )}

        {view === "custom" && (
          <div className="p-5 space-y-6 bg-card flex-1">
            <div>
              <label className="text-sm font-semibold text-foreground mb-2 block">
                Nome da variável
              </label>
              <p className="text-xs text-muted-foreground mb-3">
                Você pode manter o nome original ou usar um nome em inglês para registrar no sistema.
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
              <Button variant="outline" onClick={() => setView("choose")} className="flex-1">
                Voltar
              </Button>
              <Button onClick={handleCreateCustom} className="flex-1">
                Confirmar
              </Button>
            </div>
          </div>
        )}
      </SheetContent>
    </Sheet>
  );
}
