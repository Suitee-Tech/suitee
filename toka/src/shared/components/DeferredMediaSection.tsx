import { useEffect, useState } from "react";
import {
  useFieldArray,
  type Control,
  type FieldArrayPath,
  type FieldPath,
  type FieldValues,
  type UseFormRegister,
  type UseFormSetValue,
  type UseFormWatch,
} from "react-hook-form";
import { ArrowDown, ArrowUp, Plus, Trash2, UploadCloud } from "lucide-react";
import { getImgproxyUrl } from "../imgproxy";
import { toGatewayUrl } from "../services/api";

type DeferredImage = {
  url?: string;
  pending_file?: File;
  description?: string;
};

type Props<TFieldValues extends FieldValues> = {
  control: Control<TFieldValues>;
  register: UseFormRegister<TFieldValues>;
  watch: UseFormWatch<TFieldValues>;
  setValue: UseFormSetValue<TFieldValues>;
  name?: FieldArrayPath<TFieldValues>;
};

export function DeferredMediaSection<TFieldValues extends FieldValues>({
  control,
  register,
  watch,
  setValue,
  name,
}: Props<TFieldValues>) {
  const fieldName = name ?? ("images" as FieldArrayPath<TFieldValues>);
  const { fields, append, remove, move } = useFieldArray({
    control: control as Control<FieldValues>,
    name: fieldName as FieldArrayPath<FieldValues>,
  });

  const images = (watch(fieldName as FieldPath<TFieldValues>) || []) as unknown as DeferredImage[];
  const [externalUrl, setExternalUrl] = useState("");

  useEffect(() => {
    return () => {
      for (const img of images) {
        if (img.url?.startsWith("blob:") && img.pending_file) {
          URL.revokeObjectURL(img.url);
        }
      }
    };
  }, []);

  const renderUrl = (raw?: string) => {
    if (!raw) return "";
    if (raw.startsWith("blob:") || raw.startsWith("data:")) return raw;
    return getImgproxyUrl(toGatewayUrl(raw), { width: 900, height: 600 });
  };

  const onSelectFiles = (files: FileList | null) => {
    if (!files || files.length === 0) return;
    for (const f of Array.from(files)) {
      const blob = URL.createObjectURL(f);
      append({ url: blob, pending_file: f, description: "" });
    }
  };

  const onAddExternalUrl = () => {
    const url = externalUrl.trim();
    if (!url) return;
    append({ url, description: "" });
    setExternalUrl("");
  };

  const onRemove = (idx: number) => {
    const current = images[idx];
    if (current?.url?.startsWith("blob:") && current.pending_file) {
      URL.revokeObjectURL(current.url);
    }
    remove(idx);
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <label className="text-sm font-bold text-foreground">Mídia (opcional)</label>
        <label className="inline-flex items-center gap-2 px-3 py-2 rounded-lg border text-sm font-medium cursor-pointer hover:bg-muted bg-card border-border text-foreground">
          <UploadCloud className="w-4 h-4" />
          Upload
          <input
            type="file"
            accept="image/jpeg,image/png,image/webp"
            multiple
            className="hidden"
            onChange={(e) => onSelectFiles(e.target.files)}
          />
        </label>
      </div>

      <p className="text-xs text-muted-foreground">
        Adicione fotos do imóvel agora ou pule e adicione depois em PropertyDetails. Aceitamos arquivos (JPEG/PNG/WebP até 10MB) e URLs externas.
      </p>

      <div className="flex gap-2">
        <input
          type="url"
          value={externalUrl}
          onChange={(e) => setExternalUrl(e.target.value)}
          className="flex-1 px-3 py-2.5 rounded-lg border border-border bg-card text-sm focus:ring-2 focus:ring-primary/30 focus:outline-none"
          placeholder="https://… (URL externa)"
        />
        <button
          type="button"
          onClick={onAddExternalUrl}
          disabled={!externalUrl.trim()}
          className="px-4 py-2.5 bg-card border border-border text-foreground text-sm font-medium rounded-lg hover:bg-muted transition-colors disabled:opacity-50"
          title="Adicionar URL"
        >
          <Plus className="w-4 h-4" />
        </button>
      </div>

      {fields.length > 0 ? (
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          {fields.map((f, idx) => {
            const item = images[idx] || {};
            // Alinha ao campo do fieldArray (f.url) e ignora entradas sem URL
            // (cards vazios/quebrados em re-renderes com arrays fora de sinc).
            const src = renderUrl((f as any)?.url || item.url);
            const isPending = Boolean(item.pending_file);
            if (!src) return null;
            return (
              <div key={f.id} className="border border-border bg-card rounded-xl overflow-hidden">
                <div className="aspect-[16/10] bg-muted relative">
                  <img src={src} alt="" className="w-full h-full object-cover" />
                  {isPending && (
                    <span className="absolute top-2 left-2 text-caption font-bold uppercase tracking-wider bg-primary text-primary-foreground px-2 py-0.5 rounded-full">
                      Pendente
                    </span>
                  )}
                </div>
                <div className="p-3 space-y-2">
                  <input
                    {...register(`${fieldName}.${idx}.description` as FieldPath<TFieldValues>)}
                    type="text"
                    placeholder="Descrição (opcional)"
                    className="w-full px-3 py-2 rounded-lg border border-border bg-card text-xs focus:ring-2 focus:ring-primary/30 focus:outline-none"
                  />
                  <div className="flex items-center justify-between gap-1">
                    <div className="flex gap-1">
                      <button
                        type="button"
                        disabled={idx === 0}
                        onClick={() => move(idx, idx - 1)}
                        className="p-1.5 bg-muted border border-border rounded-md text-foreground hover:bg-muted/80 disabled:opacity-40"
                        title="Mover para cima"
                      >
                        <ArrowUp className="w-3.5 h-3.5" />
                      </button>
                      <button
                        type="button"
                        disabled={idx === fields.length - 1}
                        onClick={() => move(idx, idx + 1)}
                        className="p-1.5 bg-muted border border-border rounded-md text-foreground hover:bg-muted/80 disabled:opacity-40"
                        title="Mover para baixo"
                      >
                        <ArrowDown className="w-3.5 h-3.5" />
                      </button>
                    </div>
                    <button
                      type="button"
                      onClick={() => onRemove(idx)}
                      className="p-1.5 text-status-critical hover:bg-status-critical-soft rounded-md transition-colors"
                      title="Remover"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      ) : (
        <div className="rounded-xl border-2 border-dashed border-border bg-muted/30 p-6 text-center">
          <UploadCloud className="w-6 h-6 text-muted-foreground mx-auto mb-2" />
          <p className="text-sm text-muted-foreground">Nenhuma imagem adicionada. Use Upload ou cole uma URL acima.</p>
        </div>
      )}
    </div>
  );
}
