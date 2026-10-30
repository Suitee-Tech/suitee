import { useMemo, useState } from "react";
import { Check } from "lucide-react";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/popover";
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "../ui/command";
import { Avatar, AvatarFallback, AvatarImage } from "../ui/avatar";
import { clsx } from "clsx";
import type { Agent } from "../types/agent";
import { getAvatarThumbUrl } from "../media";

export function LeadAssigneePicker({
  agents,
  value,
  onChange,
  disabled,
  className,
  size = "sm",
}: {
  agents: Agent[];
  value?: string;
  onChange: (next: string) => void;
  disabled?: boolean;
  className?: string;
  size?: "sm" | "md";
}) {
  const [open, setOpen] = useState(false);

  const selected = useMemo(() => {
    const id = value || "";
    if (!id) return undefined;
    return agents.find((a) => a.id === id);
  }, [agents, value]);
 
  const avatarSize = size === "md" ? "w-8 h-8" : "w-6 h-6";
 
  return (
    <Popover open={open} onOpenChange={(v) => !disabled && setOpen(v)}>
      <PopoverTrigger asChild>
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            if (disabled) return;
            setOpen((p) => !p);
          }}
          className={clsx(
            "relative inline-flex items-center justify-center rounded-full border border-border bg-card hover:bg-muted transition-colors",
            disabled ? "cursor-not-allowed opacity-60" : "cursor-pointer",
            className,
          )}
          aria-label="Selecionar corretor responsável"
        >
          <Avatar className={clsx(avatarSize, "border-0")}>
            {selected?.avatar ? <AvatarImage src={getAvatarThumbUrl(selected.avatar, 64)} alt={selected?.name || "Sem responsável"} /> : null}
            <AvatarFallback className="text-caption font-bold">
              {selected?.name ? selected.name.charAt(0).toUpperCase() : "—"}
            </AvatarFallback>
          </Avatar>
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-72 p-0 bg-card border border-border shadow-xl rounded-lg z-50" align="start">
        <Command>
          <CommandInput placeholder="Buscar corretor..." />
          <CommandList>
            <CommandEmpty>Nenhum corretor encontrado.</CommandEmpty>
            <CommandGroup>
              <CommandItem
                value="none"
                onSelect={() => {
                  onChange("");
                  setOpen(false);
                }}
                className="cursor-pointer"
              >
                <Check className={clsx("w-4 h-4 mr-2", !value ? "opacity-100" : "opacity-0")} />
                <span className="italic text-muted-foreground">Sem responsável</span>
              </CommandItem>
              {agents.map((a) => (
                <CommandItem
                  key={a.id}
                  value={a.name}
                  onSelect={() => {
                    onChange(a.id);
                    setOpen(false);
                  }}
                  className="cursor-pointer"
                >
                  <Check className={clsx("w-4 h-4 mr-2", value === a.id ? "opacity-100" : "opacity-0")} />
                  <div className="flex items-center gap-2">
                    <Avatar className="w-5 h-5 border border-border">
                      {a.avatar ? <AvatarImage src={getAvatarThumbUrl(a.avatar, 40)} alt={a.name} /> : null}
                      <AvatarFallback className="text-caption font-bold">{a.name.charAt(0).toUpperCase()}</AvatarFallback>
                    </Avatar>
                    <span>{a.name}</span>
                  </div>
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
