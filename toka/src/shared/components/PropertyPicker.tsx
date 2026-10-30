import { useMemo, useState } from "react";
import { ChevronsUpDown, Check, Plus } from "lucide-react";
import { clsx } from "clsx";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/popover";
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "../ui/command";

type PropertyOption = {
  id: string;
  title?: string;
  displayId?: string;
  address?: { street?: string; neighborhood?: string };
};

export function PropertyPicker({
  id,
  properties,
  value,
  onChange,
  placeholder = "Selecione um imóvel...",
  noneLabel = "Nenhum imóvel vinculado",
  includeNone = true,
  disabled,
  buttonClassName,
  contentClassName,
  inputPlaceholder = "Buscar imóvel...",
  onCreateNew,
  createNewLabel = "Cadastrar novo imóvel",
}: {
  id?: string;
  properties: PropertyOption[];
  value?: string;
  onChange: (next: string) => void;
  placeholder?: string;
  noneLabel?: string;
  includeNone?: boolean;
  disabled?: boolean;
  buttonClassName?: string;
  contentClassName?: string;
  inputPlaceholder?: string;
  /**
   * Abre o cadastro de imóvel (o mesmo do menu Imóveis). Quando existe, o
   * seletor oferece o caminho tanto no estado vazio ("nada encontrado") quanto
   * no rodapé da lista — sem imóvel cadastrado não há negócio a criar, e o
   * operador não deveria precisar sair da esteira para descobrir isso.
   */
  onCreateNew?: () => void;
  createNewLabel?: string;
}) {
  const [open, setOpen] = useState(false);

  const selected = useMemo(() => {
    const id = value || "";
    if (!id) return undefined;
    return properties.find((p) => p.id === id);
  }, [properties, value]);

  const selectedLabel = useMemo(() => {
    if (!selected) return "";
    const title = selected.title || "Imóvel";
    const code = selected.displayId || selected.id;
    return `${title} (ID: ${code})`;
  }, [selected]);

  return (
    <Popover open={open} onOpenChange={(v) => !disabled && setOpen(v)}>
      <PopoverTrigger asChild>
        <button
          id={id}
          type="button"
          role="combobox"
          aria-expanded={open}
          onClick={(e) => {
            e.stopPropagation();
            if (disabled) return;
            setOpen((p) => !p);
          }}
          className={clsx(
            "w-full flex items-center justify-between px-3 py-2 bg-card border border-border rounded-md text-sm text-foreground hover:bg-muted focus:border-primary-500 focus:ring-1 focus:ring-primary-500 outline-none transition-all",
            disabled ? "cursor-not-allowed opacity-60" : "cursor-pointer",
            buttonClassName,
          )}
        >
          {selected ? (
            <span className="truncate">{selectedLabel}</span>
          ) : (
            <span className="text-muted-foreground truncate">{placeholder}</span>
          )}
          <ChevronsUpDown className="w-4 h-4 text-muted-foreground opacity-50 shrink-0" />
        </button>
      </PopoverTrigger>
      <PopoverContent
        className={clsx("w-[var(--radix-popover-trigger-width)] p-0 bg-card border border-border shadow-xl rounded-lg z-50", contentClassName)}
        align="start"
      >
        <Command>
          <CommandInput placeholder={inputPlaceholder} />
          <CommandList>
            <CommandEmpty>
              <div className="px-3 py-4 text-center">
                <p className="text-sm text-muted-foreground">Nenhum imóvel encontrado.</p>
                {onCreateNew && (
                  <button
                    type="button"
                    onClick={() => {
                      setOpen(false);
                      onCreateNew();
                    }}
                    className="mt-3 inline-flex items-center gap-1.5 rounded-lg border border-primary/30 bg-primary/5 px-3 py-2 text-sm font-semibold text-primary transition-colors hover:bg-primary/10"
                  >
                    <Plus className="w-4 h-4" /> {createNewLabel}
                  </button>
                )}
              </div>
            </CommandEmpty>
            <CommandGroup>
              {includeNone && (
                <CommandItem
                  value="none"
                  onSelect={() => {
                    onChange("");
                    setOpen(false);
                  }}
                  className="cursor-pointer"
                >
                  <Check className={clsx("w-4 h-4 mr-2", !value ? "opacity-100" : "opacity-0")} />
                  <span className="italic text-muted-foreground">{noneLabel}</span>
                </CommandItem>
              )}
              {properties.map((p) => {
                const title = p.title || "Imóvel";
                const code = p.displayId || p.id;
                const subtitle =
                  p.address?.street || p.address?.neighborhood
                    ? `${p.address?.street || ""}${p.address?.street && p.address?.neighborhood ? ", " : ""}${p.address?.neighborhood || ""}`
                    : "";
                return (
                  <CommandItem
                    key={p.id}
                    value={`${title} ${code} ${subtitle}`}
                    onSelect={() => {
                      onChange(p.id);
                      setOpen(false);
                    }}
                    className="cursor-pointer"
                  >
                    <Check className={clsx("w-4 h-4 mr-2", value === p.id ? "opacity-100" : "opacity-0")} />
                    <div className="flex flex-col min-w-0">
                      <span className="font-medium truncate">{title}</span>
                      <span className="text-xs text-muted-foreground truncate">
                        ID: {code}
                        {subtitle ? ` • ${subtitle}` : ""}
                      </span>
                    </div>
                  </CommandItem>
                );
              })}
              {onCreateNew && (
                <CommandItem
                  value="__create_new_property__"
                  onSelect={() => {
                    setOpen(false);
                    onCreateNew();
                  }}
                  className="cursor-pointer border-t border-border text-primary"
                >
                  <Plus className="w-4 h-4 mr-2" />
                  <span className="font-semibold">{createNewLabel}</span>
                  <span className="ml-2 text-xs text-muted-foreground">abre o cadastro do imóvel</span>
                </CommandItem>
              )}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

