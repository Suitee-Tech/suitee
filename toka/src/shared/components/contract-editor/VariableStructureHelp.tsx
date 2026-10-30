import { Copy } from "lucide-react";
import { toast } from "sonner";
import type { ContractVariable } from "../../contracts/contractVariables";

/** Object fields and loop tags remain visible without relying on hover. */
export function VariableStructureHelp({ variable }: { variable: ContractVariable }) {
  if (variable.kind !== "object" && variable.kind !== "list") return null;

  const copyValue = async (value: string, message: string) => {
    try {
      await navigator.clipboard.writeText(value);
      toast.success(message);
    } catch {
      toast.error("Não foi possível copiar. Selecione o texto e copie manualmente.");
    }
  };

  const copyExample = () => variable.example && copyValue(variable.example, "Exemplo copiado");

  return (
    <details className="rounded-lg border border-border bg-muted/40 text-xs">
      <summary className="cursor-pointer px-2.5 py-2 font-medium text-muted-foreground">
        {variable.kind === "list" ? "Lista: campos e repetição" : "Objeto: campos disponíveis"}
      </summary>
      <div className="space-y-2 px-2.5 pb-3">
        {variable.kind === "list" && <p className="leading-relaxed text-muted-foreground">Use o bloco de repetição para todas as partes. Dentro dele, os campos se referem a cada pessoa.</p>}
        {variable.fields && <div className="space-y-1">
          {variable.fields.map((field) => {
            const tag = `{{${variable.kind === "object" ? `${variable.key}.` : ""}${field.key}}}`;
            return (
              <div key={field.key} className="flex items-start gap-1.5 break-words text-muted-foreground">
                <code className="break-all font-mono text-primary-700">{tag}</code>
                <button
                  type="button"
                  onClick={() => copyValue(tag, `Copiado: ${tag}`)}
                  aria-label={`Copiar ${tag}`}
                  className="shrink-0 rounded p-0.5 text-muted-foreground/60 hover:bg-muted hover:text-primary-700"
                  title="Copiar campo"
                >
                  <Copy className="size-3" />
                </button>
                <span className="min-w-0 flex-1">— {field.label}</span>
              </div>
            );
          })}
        </div>}
        {variable.example && <div className="space-y-2">
          <code draggable onDragStart={(event) => {
            event.dataTransfer.setData("text/plain", variable.example!);
            event.dataTransfer.effectAllowed = "copy";
          }} className="block select-text break-all rounded border border-border bg-card p-2 font-mono leading-relaxed" title="Arraste o exemplo para o documento">{variable.example}</code>
          <button type="button" onClick={copyExample} className="inline-flex items-center gap-1.5 rounded px-1 py-1 font-medium text-primary-700 hover:bg-primary-50">
            <Copy className="size-3" /> Copiar exemplo de {variable.key}
          </button>
        </div>}
      </div>
    </details>
  );
}
