import { useEffect, useMemo, useRef, useState } from "react";
import { useFieldArray } from "react-hook-form";
import { ArrowDown, ArrowUp, Loader2, Plus, Trash2, UploadCloud, XCircle } from "lucide-react";
import { getImgproxyUrl } from "../services/imgproxy";
import { toGatewayUrl } from "../services/api";
import { detectVideoPlatform } from "../services/video-platform";

type Props = {
  control: any;
  register: any;
  watch: any;
  setValue: any;
  getValues: any;
  propertyId?: string;
  onAddPendingFile: (pendingId: string, file: File) => void;
  onRemovePendingFile: (pendingId: string) => void;
  onMarkDeleteFilename: (filename: string) => void;
  /** Notifica o form sobre quantas imagens estão em validação/carregamento. */
  onImagesLoadingChange?: (count: number) => void;
  /** Notifica o form sobre imagens que falharam no carregamento (bloqueio de criação). */
  onImagesErrorChange?: (hasErrors: boolean) => void;
};

// Valida se o arquivo realmente renderiza como imagem (onload) com timeout.
// O preview (blob) aparece imediatamente; o card fica "carregando" até o
// probe terminar. Falha/timeout marca a entrada como inválida.
const IMAGE_PROBE_TIMEOUT_MS = 20_000;

const probeImage = (url: string, timeoutMs = IMAGE_PROBE_TIMEOUT_MS): Promise<void> =>
  new Promise((resolve, reject) => {
    const img = new Image();
    const timer = window.setTimeout(() => {
      img.src = "";
      reject(new Error("timeout"));
    }, timeoutMs);
    img.onload = () => {
      window.clearTimeout(timer);
      resolve();
    };
    img.onerror = () => {
      window.clearTimeout(timer);
      reject(new Error("invalid image"));
    };
    img.src = url;
  });

const newPendingId = () => {
  const id =
    typeof crypto !== "undefined" && "randomUUID" in crypto
      ? (crypto as any).randomUUID()
      : Math.random().toString(36).slice(2);
  return `pending:${id}`;
};

export function PropertyMediaSection({
  control,
  register,
  watch,
  setValue,
  getValues,
  propertyId,
  onAddPendingFile,
  onRemovePendingFile,
  onMarkDeleteFilename,
  onImagesLoadingChange,
  onImagesErrorChange,
}: Props) {
  const {
    fields: imageFields,
    append: appendImage,
    remove: removeImage,
    move: moveImage,
  } = useFieldArray({
    control,
    name: "images",
  });

  const {
    fields: videoFields,
    append: appendVideo,
    remove: removeVideo,
    move: moveVideo,
  } = useFieldArray({
    control,
    name: "videos",
  });

  const images = watch("images") || [];
  const videos = watch("videos") || [];
  const [externalImageUrl, setExternalImageUrl] = useState("");

  // Imagens em validação (spinner) e que falharam na validação.
  const [processingUrls, setProcessingUrls] = useState<Set<string>>(new Set());
  const [failedUrls, setFailedUrls] = useState<Set<string>>(new Set());

  useEffect(() => {
    onImagesLoadingChange?.(processingUrls.size);
  }, [processingUrls.size, onImagesLoadingChange]);

  useEffect(() => {
    onImagesErrorChange?.(failedUrls.size > 0);
  }, [failedUrls.size, onImagesErrorChange]);

  const dropFrom = (set: Set<string>, url: string): Set<string> => {
    if (!set.has(url)) return set;
    const next = new Set(set);
    next.delete(url);
    return next;
  };

  // Ciclo de vida dos blob URLs de preview: criados aqui, revogados apenas
  // quando o item é removido DE VERDADE (ou no desmonte do form). Um blob
  // revogado antes do item sair da tela — ou nunca revogado até um reload —
  // gera ERR_FILE_NOT_FOUND intermitente no <img>.
  const liveBlobsRef = useRef<Set<string>>(new Set());

  useEffect(() => {
    const live = liveBlobsRef.current;
    return () => {
      live.forEach((url) => {
        try {
          URL.revokeObjectURL(url);
        } catch {
          /* noop */
        }
      });
      live.clear();
    };
  }, []);

  // Sincroniza a capa ("image") de forma EVENT-DRIVEN (append/remove/move),
  // nunca via efeito por render — zero risco de escrever outros campos.
  const syncCoverFromFirst = () => {
    const first = watch("images")?.[0]?.url ?? "";
    setValue("image", first, { shouldDirty: true });
  };

  const isNew = !propertyId;

  const renderUrl = (raw: string) => {
    if (!raw) return raw;
    if (raw.startsWith("blob:")) {
      // Blob revogado/inexistente não pode virar src (ERR_FILE_NOT_FOUND).
      return liveBlobsRef.current.has(raw) ? raw : "";
    }
    if (raw.startsWith("data:")) return raw;
    return getImgproxyUrl(toGatewayUrl(raw), { width: 900, height: 600 });
  };

  const onSelectFiles = async (files: FileList | null) => {
    if (!files || files.length === 0) return;
    for (const f of Array.from(files)) {
      const pendingId = newPendingId();
      onAddPendingFile(pendingId, f);
      const url = URL.createObjectURL(f);
      liveBlobsRef.current.add(url);
      appendImage({ url, order: images.length, filename: pendingId });
      if (images.length === 0) syncCoverFromFirst();
      // Validação de render com timeout: enquanto valida, o card mostra
      // spinner e o form bloqueia a criação; falha/timeout marca como
      // inválida (nunca vira imagem quebrada no imóvel).
      setProcessingUrls((prev) => new Set(prev).add(url));
      probeImage(url)
        .then(() => {
          setProcessingUrls((prev) => dropFrom(prev, url));
        })
        .catch(() => {
          setProcessingUrls((prev) => dropFrom(prev, url));
          setFailedUrls((prev) => new Set(prev).add(url));
        });
    }
  };

  const onAddExternalUrl = () => {
    const url = externalImageUrl.trim();
    if (!url) return;
    appendImage({ url, order: images.length });
    if (images.length === 0) syncCoverFromFirst();
    setExternalImageUrl("");
  };

  const onRemoveImage = (idx: number) => {
    const current = images[idx] || (imageFields[idx] as any) || {};
    if (current?.filename && typeof current.filename === "string") {
      if (current.filename.startsWith("pending:")) onRemovePendingFile(current.filename);
      else onMarkDeleteFilename(current.filename);
    }
    // Revoga o blob DEPOIS de remover o campo — o <img> some no mesmo
    // render, sem janela em que a URL já revogada ainda é referenciada.
    removeImage(idx);
    syncCoverFromFirst();
    if (current?.url?.startsWith("blob:")) {
      liveBlobsRef.current.delete(current.url);
      setProcessingUrls((prev) => dropFrom(prev, current.url!));
      setFailedUrls((prev) => dropFrom(prev, current.url!));
      try {
        URL.revokeObjectURL(current.url);
      } catch {
        /* noop */
      }
    }
  };

  const videoPlatforms = useMemo(() => ["youtube", "vimeo", "other"], []);

  return (
    <div className="space-y-6">
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <label className="text-sm font-bold text-foreground">Mídia</label>
          <div className="flex items-center gap-2">
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
        </div>

        {isNew && (
          <div className="text-xs text-muted-foreground">
            As imagens novas serão enviadas quando você criar o imóvel.
          </div>
        )}

        <div className="flex gap-2">
          <input
            value={externalImageUrl}
            onChange={(e) => setExternalImageUrl(e.target.value)}
            className="flex-1 bg-muted border border-border text-foreground text-sm rounded-xl px-4 py-3 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all font-medium"
            placeholder="Adicionar URL externa de imagem"
          />
          <button
            type="button"
            onClick={onAddExternalUrl}
            className="px-4 py-3 bg-card border border-border text-foreground text-sm font-medium rounded-xl hover:bg-muted transition-colors"
          >
            <Plus className="w-4 h-4" />
          </button>
        </div>

        {imageFields.length > 0 && (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {imageFields.map((f, idx) => {
              const item = images[idx] || {};
              const fieldUrl = (f as any)?.url;
              // Alinha a renderização ao campo do fieldArray (f.url) — a
              // dupla fonte (watch vs fields) pode divergir por um render e
              // criar card vazio/quebrado.
              const src = renderUrl(fieldUrl || item.url);
              if (!src) return null;
              return (
                <div key={f.id} className="border border-border bg-card rounded-2xl overflow-hidden">
                  <div className="aspect-[16/10] bg-muted relative">
                    <img src={src} className="w-full h-full object-cover" />
                    {(processingUrls.has(src) || failedUrls.has(src)) && (
                      <div className="absolute inset-0 bg-foreground/60 backdrop-blur-sm flex flex-col items-center justify-center gap-2 text-white">
                        {processingUrls.has(src) ? (
                          <>
                            <Loader2 className="h-6 w-6 animate-spin" />
                            <span className="text-xs font-bold">Carregando imagem…</span>
                            <span className="text-caption opacity-80">
                              validando arquivo (timeout {Math.round(IMAGE_PROBE_TIMEOUT_MS / 1000)}s)
                            </span>
                          </>
                        ) : (
                          <>
                            <XCircle className="h-6 w-6 text-status-critical" />
                            <span className="text-xs font-bold max-w-[220px] text-center">
                              Não foi possível carregar a imagem — remova e tente novamente.
                            </span>
                          </>
                        )}
                      </div>
                    )}
                  </div>
                  <div className="p-3 space-y-2">
                    <input
                      {...register(`images.${idx}.description`)}
                      className="w-full bg-muted border border-border text-foreground text-sm rounded-xl px-3 py-2 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all font-medium"
                      placeholder="Descrição (opcional)"
                    />
                    <div className="flex items-center justify-between gap-2">
                      <div className="flex gap-1">
                        <button
                          type="button"
                          disabled={idx === 0}
                          onClick={() => { moveImage(idx, idx - 1); syncCoverFromFirst(); }}
                          className="p-2 bg-card border border-border text-foreground rounded-lg disabled:opacity-50"
                        >
                          <ArrowUp className="w-4 h-4" />
                        </button>
                        <button
                          type="button"
                          disabled={idx === imageFields.length - 1}
                          onClick={() => { moveImage(idx, idx + 1); syncCoverFromFirst(); }}
                          className="p-2 bg-card border border-border text-foreground rounded-lg disabled:opacity-50"
                        >
                          <ArrowDown className="w-4 h-4" />
                        </button>
                      </div>
                      <button
                        type="button"
                        onClick={() => onRemoveImage(idx)}
                        className="p-2 bg-card border border-border text-status-critical rounded-lg hover:bg-status-critical-soft"
                      >
                        <Trash2 className="w-4 h-4" />
                      </button>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <label className="text-sm font-bold text-foreground">Vídeos</label>
          <button
            type="button"
            onClick={() => appendVideo({ url: "", platform: "youtube" })}
            className="text-xs flex items-center gap-1 text-primary-600 dark:text-primary-400 hover:text-primary-700 dark:hover:text-primary-300 font-medium"
          >
            <Plus className="w-3 h-3" /> Adicionar
          </button>
        </div>

        {videoFields.length > 0 && (
          <div className="space-y-3">
            {videoFields.map((f, idx) => (
              <div key={f.id} className="flex gap-2 items-start">
                <div className="flex-1 space-y-2">
                  <input
                    {...register(`videos.${idx}.url`, {
                      onChange: (e: any) => {
                        const platform = detectVideoPlatform(e.target.value || "");
                        setValue(`videos.${idx}.platform`, platform, { shouldDirty: true });
                      },
                    })}
                    className="w-full bg-muted border border-border text-foreground text-sm rounded-xl px-4 py-3 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all font-medium"
                    placeholder="URL do vídeo (YouTube, Vimeo...)"
                  />
                  <div className="flex gap-2">
                    <select
                      {...register(`videos.${idx}.platform`)}
                      className="bg-muted border border-border text-foreground text-sm rounded-xl px-3 py-2 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all font-medium"
                    >
                      {videoPlatforms.map((p) => (
                        <option key={p} value={p}>
                          {p}
                        </option>
                      ))}
                    </select>
                    <div className="flex gap-1 ml-auto">
                      <button
                        type="button"
                        disabled={idx === 0}
                        onClick={() => moveVideo(idx, idx - 1)}
                        className="p-2 bg-card border border-border text-foreground rounded-lg disabled:opacity-50"
                      >
                        <ArrowUp className="w-4 h-4" />
                      </button>
                      <button
                        type="button"
                        disabled={idx === videoFields.length - 1}
                        onClick={() => moveVideo(idx, idx + 1)}
                        className="p-2 bg-card border border-border text-foreground rounded-lg disabled:opacity-50"
                      >
                        <ArrowDown className="w-4 h-4" />
                      </button>
                    </div>
                    <button
                      type="button"
                      onClick={() => removeVideo(idx)}
                      className="p-2 bg-card border border-border text-status-critical rounded-lg hover:bg-status-critical-soft"
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}

        {videos.length === 0 ? (
          <div className="text-xs text-muted-foreground">Nenhum vídeo adicionado.</div>
        ) : null}
      </div>
    </div>
  );
}
