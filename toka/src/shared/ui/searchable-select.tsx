"use client";

import * as React from "react";
import { Check, ChevronsUpDown } from "lucide-react";

import { Button } from "./button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "./command";
import { Dialog, DialogContent, DialogTitle, DialogTrigger } from "./dialog";
import { Popover, PopoverContent, PopoverTrigger } from "./popover";
import { useIsSmallScreen, useVisibleViewportHeight } from "../hooks/useVisualViewportInset";
import { cn } from "./utils";

export type SearchableSelectOption = {
  value: string;
  label: React.ReactNode;
  /** Texto adicional usado pela busca sem alterar o valor exibido. */
  searchText?: string;
  disabled?: boolean;
};

function getSearchValue(option: SearchableSelectOption): string {
  if (option.searchText) return option.searchText;
  if (typeof option.label === "string" || typeof option.label === "number") {
    return `${option.label} ${option.value}`;
  }
  return option.value;
}

type SearchableSelectProps = Omit<React.ComponentProps<typeof Button>, "value" | "onChange"> & {
  value?: string;
  onValueChange: (value: string) => void;
  options: SearchableSelectOption[];
  placeholder?: string;
  searchPlaceholder?: string;
  emptyMessage?: string;
};

/**
 * Combobox para relações extensas, com pesquisa no próprio dropdown.
 *
 * No celular a lista NÃO é um dropdown flutuante ancorado ao campo: com o
 * teclado aberto a área abaixo do campo fica coberta (e no iOS a viewport de
 * layout nem encolhe, então o posicionamento flutuante erra o alvo), o que
 * deixava as opções inalcançáveis — "o teclado cobre a área impedindo a
 * seleção". Em tela pequena a lista vira um painel fixo no TOPO da área
 * visível: o teclado cobre só a parte de baixo, a lista rola dentro do painel e
 * qualquer opção continua alcançável.
 */
export function SearchableSelect({
  value = "",
  onValueChange,
  options,
  placeholder = "Selecione…",
  searchPlaceholder = "Buscar…",
  emptyMessage = "Nenhuma opção encontrada.",
  className,
  disabled,
  ...buttonProps
}: SearchableSelectProps) {
  const [open, setOpen] = React.useState(false);
  const selectedOption = options.find((option) => option.value === value);
  const isSmallScreen = useIsSmallScreen();
  const visibleHeight = useVisibleViewportHeight();

  const trigger = (
    <Button
      type="button"
      variant="outline"
      role="combobox"
      aria-expanded={open}
      disabled={disabled}
      className={cn(
        "h-10 w-full justify-between gap-2 rounded-xl px-3 text-left font-normal",
        !selectedOption && "text-muted-foreground",
        className,
      )}
      {...buttonProps}
    >
      <span className="min-w-0 flex-1 truncate">
        {selectedOption?.label ?? placeholder}
      </span>
      <ChevronsUpDown className="size-4 shrink-0 opacity-50" aria-hidden="true" />
    </Button>
  );

  const optionsContent = (
    <>
      <CommandEmpty>{emptyMessage}</CommandEmpty>
      <CommandGroup>
        {options.map((option) => (
          <CommandItem
            key={option.value}
            value={getSearchValue(option)}
            disabled={option.disabled}
            onSelect={() => {
              onValueChange(option.value);
              setOpen(false);
            }}
          >
            <Check
              className={cn(
                "size-4",
                option.value === value ? "opacity-100" : "opacity-0",
              )}
              aria-hidden="true"
            />
            <span className="min-w-0 flex-1 truncate">{option.label}</span>
          </CommandItem>
        ))}
      </CommandGroup>
    </>
  );

  if (isSmallScreen) {
    return (
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogTrigger asChild>{trigger}</DialogTrigger>
        <DialogContent
          aria-describedby={undefined}
          overlayClassName="z-[410]"
          className="fixed inset-x-2 top-2 z-[420] flex max-w-none translate-x-0 translate-y-0 flex-col gap-0 overflow-hidden p-0 sm:max-w-none"
          // A altura máxima é a área VISÍVEL (tela menos teclado): o painel fica
          // no topo e a lista rola dentro dele, então nenhuma opção fica sob o
          // teclado.
          style={{ maxHeight: `${Math.max(200, visibleHeight - 16)}px` }}
        >
          <DialogTitle className="sr-only">{placeholder}</DialogTitle>
          <Command className="min-h-0 flex-1">
            {/* Sem `autoFocus` (a11y): o foco vai programático para o campo de
                busca quando o diálogo de telas pequenas abre. */}
            <CommandInput
              ref={(node: HTMLInputElement | null) => {
                if (node && open) node.focus();
              }}
              placeholder={searchPlaceholder}
            />
            <CommandList className="max-h-none min-h-0 flex-1">
              {optionsContent}
            </CommandList>
          </Command>
        </DialogContent>
      </Dialog>
    );
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent
        align="start"
        className="w-[var(--radix-popover-trigger-width)] max-w-[calc(100vw-2rem)] overflow-hidden p-0"
      >
        <Command>
          <CommandInput placeholder={searchPlaceholder} />
          <CommandList className="max-h-72">{optionsContent}</CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
