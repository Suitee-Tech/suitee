import { useCallback, useId, useRef, useState } from "react";
import { X } from "lucide-react";
import { clsx } from "clsx";
import { MAX_TAG_ITEMS, normalizeTag, normalizeTags, splitTagDraft, tagListError } from "../utils/tags";

/**
 * Campo de lista (etiquetas/comodidades).
 *
 * O campo anterior escrevia direto no array derivando o texto exibido do
 * `join(", ")`: digitar a vírgula era engolido na hora (o separador virava item
 * vazio e sumia), então só dava para cadastrar UMA comodidade por vez — colar
 * funcionava porque a vírgula vinha junto de outro item no mesmo evento. Aqui o
 * texto em digitação vive em estado próprio (`draft`) e vira item quando o
 * operador confirma com Enter, vírgula, vírgula digitada no meio do texto, blur
 * ou colagem de uma lista.
 */
export interface TagInputProps {
  value: string[];
  onChange: (next: string[]) => void;
  /** Nome do campo nas mensagens ("Comodidades"). */
  label: string;
  id?: string;
  placeholder?: string;
  /** Lista curada exibida no autocomplete nativo do navegador. */
  suggestions?: string[];
  maxItems?: number;
  disabled?: boolean;
  className?: string;
  /** Texto de apoio abaixo do campo (também lido por leitor de tela). */
  hint?: string;
  "aria-describedby"?: string;
}

export function TagInput({
  value,
  onChange,
  label,
  id,
  placeholder,
  suggestions,
  maxItems = MAX_TAG_ITEMS,
  disabled,
  className,
  hint,
  "aria-describedby": describedBy,
}: TagInputProps) {
  const [draft, setDraft] = useState("");
  const [error, setError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const reactId = useId();
  const inputId = id ?? `tag-input-${reactId}`;
  const hintId = `${inputId}-hint`;
  const errorId = `${inputId}-error`;
  const datalistId = `${inputId}-options`;

  const commit = useCallback(
    (raw: string) => {
      const parts = splitTagDraft(raw);
      setDraft("");
      if (parts.length === 0) return;

      const existing = normalizeTags(value);
      const known = new Set(existing.map((tag) => tag.toLowerCase()));
      const next = [...existing];
      let hitLimit = false;
      for (const part of parts) {
        const key = part.toLowerCase();
        if (known.has(key)) continue; // já está na lista: ignorar sem erro
        if (next.length >= maxItems) {
          hitLimit = true;
          break;
        }
        known.add(key);
        next.push(part);
      }

      setError(hitLimit ? `${label}: máximo de ${maxItems} itens.` : tagListError(next, label));
      if (next.length !== existing.length) onChange(next);
    },
    [label, maxItems, onChange, value],
  );

  const removeAt = useCallback(
    (index: number) => {
      const next = value.filter((_, i) => i !== index);
      setError(tagListError(next, label));
      onChange(next);
      inputRef.current?.focus();
    },
    [label, onChange, value],
  );

  const tags = normalizeTags(value ?? []);
  const describedByIds = [hint ? hintId : null, describedBy, error ? errorId : null].filter(Boolean).join(" ");

  return (
    <div className={className}>
      {/* Sem handler de clique no container (elemento não interativo não deve
          ter interação — regra de acessibilidade do lint): o próprio input
          ocupa a área livre e recebe o foco por clique, Tab ou toque. */}
      <div
        className={clsx(
          "flex flex-wrap items-center gap-2 w-full bg-muted border border-border rounded-xl px-3 py-2 transition-all",
          "focus-within:ring-2 focus-within:ring-primary-500/20 focus-within:border-primary-500",
          disabled && "opacity-60",
        )}
      >
        {tags.map((tag, index) => (
          <span
            key={`${tag}-${index}`}
            className="inline-flex items-center gap-1 rounded-lg border border-primary/20 bg-primary/10 px-2 py-1 text-xs font-medium text-foreground"
          >
            {tag}
            <button
              type="button"
              disabled={disabled}
              onClick={(event) => {
                event.stopPropagation();
                removeAt(index);
              }}
              aria-label={`Remover ${tag}`}
              className="rounded p-0.5 text-muted-foreground hover:text-destructive focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/40 disabled:cursor-not-allowed"
            >
              <X className="size-3" aria-hidden="true" />
            </button>
          </span>
        ))}
        <input
          id={inputId}
          ref={inputRef}
          value={draft}
          disabled={disabled}
          list={suggestions?.length ? datalistId : undefined}
          aria-describedby={describedByIds || undefined}
          onChange={(event) => {
            const raw = event.target.value;
            // Vírgula (ou ponto e vírgula) no texto confirma o item sem esperar
            // o Enter — é o separador combinado no rótulo do campo.
            if (/[,\n;]/.test(raw)) {
              commit(raw);
              return;
            }
            setDraft(raw);
          }}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              // Enter aqui não pode enviar o formulário: ele confirma o item.
              event.preventDefault();
              commit(draft);
              return;
            }
            if (event.key === "Backspace" && draft === "" && tags.length > 0) {
              event.preventDefault();
              removeAt(tags.length - 1);
              return;
            }
            if (event.key === "Escape") {
              setDraft("");
            }
          }}
          onBlur={() => commit(draft)}
          placeholder={tags.length === 0 ? placeholder : undefined}
          className={clsx(
            "flex-1 bg-transparent text-sm font-medium text-foreground outline-none placeholder:text-muted-foreground",
            disabled ? "cursor-not-allowed" : "min-w-[10rem] cursor-text",
          )}
        />
      </div>

      {suggestions?.length ? (
        <datalist id={datalistId}>
          {suggestions.map((suggestion) => (
            <option key={suggestion} value={suggestion} />
          ))}
        </datalist>
      ) : null}

      <div className="mt-1 flex flex-wrap items-center justify-between gap-2">
        {hint ? (
          <p id={hintId} className="text-xs text-muted-foreground">
            {hint}
          </p>
        ) : (
          <span />
        )}
        {tags.length > 0 && (
          <p className="text-xs text-muted-foreground" aria-live="polite">
            {tags.length} de {maxItems}
          </p>
        )}
      </div>
      {error && (
        <p id={errorId} role="alert" className="mt-1 text-xs text-destructive">
          {error}
        </p>
      )}
    </div>
  );
}
