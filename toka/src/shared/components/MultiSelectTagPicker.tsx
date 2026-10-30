import { useMemo, useState } from "react";
import { Check, ChevronsUpDown, Plus, X } from "lucide-react";
import { clsx } from "clsx";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/popover";
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "../ui/command";

export function MultiSelectTagPicker(props: {
  label?: string;
  placeholder?: string;
  value: string[];
  options: string[];
  allowCreate?: boolean;
  onChange: (next: string[]) => void;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");

  const normalizedValue = useMemo(
    () => Array.from(new Set((props.value ?? []).map((v) => String(v).trim()).filter(Boolean))),
    [props.value],
  );

  const normalizedOptions = useMemo(() => {
    const set = new Set<string>();
    (props.options ?? []).forEach((v) => {
      const n = String(v).trim();
      if (n) set.add(n);
    });
    normalizedValue.forEach((v) => set.add(v));
    return Array.from(set).sort((a, b) => a.localeCompare(b));
  }, [props.options, normalizedValue]);

  const q = query.trim().toLowerCase();
  const filtered = useMemo(() => {
    if (!q) return normalizedOptions;
    return normalizedOptions.filter((o) => o.toLowerCase().includes(q));
  }, [normalizedOptions, q]);

  const canCreate = (props.allowCreate ?? true) && !!q && !normalizedOptions.some((o) => o.toLowerCase() === q);

  const toggle = (raw: string) => {
    const v = String(raw).trim();
    if (!v) return;
    if (normalizedValue.includes(v)) {
      props.onChange(normalizedValue.filter((x) => x !== v));
    } else {
      props.onChange([...normalizedValue, v]);
    }
  };

  const create = (raw: string) => {
    const v = String(raw).trim();
    if (!v) return;
    if (normalizedValue.includes(v)) return;
    props.onChange([...normalizedValue, v]);
    setQuery("");
  };

  return (
    <div className="space-y-2">
      {props.label && <div className="text-xs font-bold text-muted-foreground">{props.label}</div>}
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <button
            type="button"
            className="w-full flex items-center justify-between px-3 py-2 bg-card border border-border rounded-xl text-sm hover:bg-muted focus:outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 transition-all"
          >
            <span className={clsx("truncate", normalizedValue.length === 0 && "text-muted-foreground")}>
              {normalizedValue.length === 0 ? props.placeholder ?? "Selecionar..." : `${normalizedValue.length} selecionado(s)`}
            </span>
            <ChevronsUpDown className="w-4 h-4 text-muted-foreground opacity-70" />
          </button>
        </PopoverTrigger>
        <PopoverContent className="w-[var(--radix-popover-trigger-width)] p-0 bg-card border border-border shadow-xl rounded-lg z-50" align="start">
          <Command shouldFilter={false}>
            <CommandInput placeholder="Buscar..." value={query} onValueChange={setQuery} />
            <CommandList>
              <CommandEmpty>Nenhum item.</CommandEmpty>
              <CommandGroup>
                {canCreate && (
                  <CommandItem
                    value={`create:${q}`}
                    onSelect={() => create(query)}
                    className="cursor-pointer"
                  >
                    <Plus className="w-4 h-4 mr-2 text-muted-foreground" />
                    Adicionar “{query.trim()}”
                  </CommandItem>
                )}
                {filtered.map((o) => (
                  <CommandItem key={o} value={o} onSelect={() => toggle(o)} className="cursor-pointer">
                    <Check className={clsx("w-4 h-4 mr-2", normalizedValue.includes(o) ? "opacity-100" : "opacity-0")} />
                    <span className="truncate">{o}</span>
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>

      {normalizedValue.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {normalizedValue.map((t) => (
            <button
              key={t}
              type="button"
              onClick={() => props.onChange(normalizedValue.filter((x) => x !== t))}
              className="inline-flex items-center gap-2 rounded-lg bg-muted px-2 py-1 text-xs font-bold text-foreground"
            >
              {t}
              <X className="w-3.5 h-3.5 text-muted-foreground" />
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
