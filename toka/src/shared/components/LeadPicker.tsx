import { useMemo, useState } from "react";
import { ChevronsUpDown, Check } from "lucide-react";
import { clsx } from "clsx";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/popover";
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "../ui/command";

export type LeadOption = {
  id: string;
  name: string;
  email?: string;
  phone?: string;
};

export function LeadPicker({
  leads,
  value,
  onChange,
  placeholder = "Selecione um proprietário…",
  inputPlaceholder = "Buscar por nome, e-mail ou telefone…",
  emptyLabel = "Nenhum lead encontrado.",
  disabled,
  buttonClassName,
  contentClassName,
}: {
  leads: LeadOption[];
  value?: string;
  onChange: (next: string) => void;
  placeholder?: string;
  inputPlaceholder?: string;
  emptyLabel?: string;
  disabled?: boolean;
  buttonClassName?: string;
  contentClassName?: string;
}) {
  const [open, setOpen] = useState(false);

  const selected = useMemo(() => {
    const id = value || "";
    if (!id) return undefined;
    return leads.find((l) => l.id === id);
  }, [leads, value]);

  const selectedLabel = useMemo(() => {
    if (!selected) return "";
    return selected.email ? `${selected.name} · ${selected.email}` : selected.name;
  }, [selected]);

  return (
    <Popover open={open} onOpenChange={(v) => !disabled && setOpen(v)}>
      <PopoverTrigger asChild>
        <button
          type="button"
          role="combobox"
          aria-expanded={open}
          onClick={(e) => {
            e.stopPropagation();
            if (disabled) return;
            setOpen((p) => !p);
          }}
          className={clsx(
            "w-full flex items-center justify-between px-3 py-2.5 bg-card border border-border rounded-lg text-sm text-foreground hover:bg-muted focus:border-primary-500 focus:ring-1 focus:ring-primary-500 outline-none transition-all",
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
        className={clsx("w-[var(--radix-popover-trigger-width)] p-0 bg-card border border-border shadow-xl rounded-lg", contentClassName)}
        align="start"
      >
        <Command>
          <CommandInput placeholder={inputPlaceholder} />
          <CommandList>
            <CommandEmpty>{emptyLabel}</CommandEmpty>
            <CommandGroup>
              {leads.map((l) => {
                const subtitle = [l.email, l.phone].filter(Boolean).join(" · ");
                return (
                  <CommandItem
                    key={l.id}
                    value={`${l.name} ${l.email ?? ""} ${l.phone ?? ""}`}
                    onSelect={() => {
                      onChange(l.id);
                      setOpen(false);
                    }}
                    className="cursor-pointer"
                  >
                    <Check className={clsx("w-4 h-4 mr-2 shrink-0", value === l.id ? "opacity-100" : "opacity-0")} />
                    <div className="flex flex-col min-w-0">
                      <span className="font-medium truncate">{l.name}</span>
                      {subtitle && <span className="text-xs text-muted-foreground truncate">{subtitle}</span>}
                    </div>
                  </CommandItem>
                );
              })}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
