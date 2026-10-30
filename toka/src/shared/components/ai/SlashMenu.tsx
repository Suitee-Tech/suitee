import { Command, CommandEmpty, CommandGroup, CommandItem, CommandList } from "../../ui/command";
import type { SlashCommand } from "./slashCommands";

interface SlashMenuProps {
  open: boolean;
  query: string;
  commands: SlashCommand[];
  activeIndex: number;
  onSelect: (cmd: SlashCommand) => void;
}

export function SlashMenu({ open, query, commands, activeIndex, onSelect }: SlashMenuProps) {
  if (!open) return null;
  return (
    <div className="absolute bottom-full left-0 right-0 mb-2 z-50 rounded-lg border border-border/50 bg-popover shadow-lg overflow-hidden">
      <Command shouldFilter={false}>
        <CommandList className="max-h-72">
          <CommandEmpty>Nenhum comando para “{query}”.</CommandEmpty>
          <CommandGroup heading="Comandos">
            {commands.map((c, idx) => (
              <CommandItem
                key={c.command}
                value={c.command}
                onSelect={() => onSelect(c)}
                className={`flex items-center justify-between gap-3 ${idx === activeIndex ? "bg-accent text-accent-foreground aria-selected:bg-accent" : ""}`}
              >
                <span className="font-mono text-label">{c.label}</span>
                <span className="text-xs text-muted-foreground truncate">{c.description}</span>
              </CommandItem>
            ))}
          </CommandGroup>
        </CommandList>
      </Command>
    </div>
  );
}

