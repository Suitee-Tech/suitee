import { useConfirmAction } from "../../hooks/useConfirmAction";
import { useEffect, useMemo, useRef, useState } from "react";
import {
  Camera,
  Check,
  ChevronDown,
  ChevronRight,
  CloudUpload,
  Loader2,
  Plus,
  Trash2,
  X,
  AlertTriangle,
  ImageIcon,
} from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "../../ui/dialog";
import { Button } from "../../ui/button";
import { inspectionService, type Inspection, type Finding } from "../../services/inspection.service";
import { compressImageFile } from "../../utils/imageCompression";
import { MediaImage } from "../MediaImage";
import {
  CONDITION_VALUES,
  DEFAULT_RESIDENTIAL_TEMPLATE,
  buildFindingsFromTemplate,
  loadInspectionTemplate,
  makeFindingId,
  type ConditionValue,
  type RoomTemplate,
} from "./inspectionTemplates";

type FindingState = Required<Pick<Finding, "id" | "room" | "description" | "condition">> & {
  notes: string;
  photos: string[];
  thumbnails: string[];
};

interface InspectionChecklistDialogProps {
  inspection: Inspection;
  open: boolean;
  onClose: () => void;
  onSaved?: (updated: Inspection) => void;
}

export function InspectionChecklistDialog({ inspection, open, onClose, onSaved }: InspectionChecklistDialogProps) {
  const { confirmAction, confirmationDialog } = useConfirmAction();
  const [findings, setFindings] = useState<FindingState[]>([]);
  const [notes, setNotes] = useState(inspection.notes || "");
  const [saving, setSaving] = useState(false);
  const [completing, setCompleting] = useState(false);
  const [lightboxUrl, setLightboxUrl] = useState<string | null>(null);
  const [collapsedRooms, setCollapsedRooms] = useState<Record<string, boolean>>({});
  const [template, setTemplate] = useState<RoomTemplate[]>(DEFAULT_RESIDENTIAL_TEMPLATE);

  const [dirty, setDirty] = useState(false);
  const [autosaving, setAutosaving] = useState(false);
  const [autoSavedAt, setAutoSavedAt] = useState<Date | null>(null);
  const [autosaveError, setAutosaveError] = useState<string | null>(null);
  const suppressDirtyRef = useRef(true);

  const isReadOnly = inspection.status === "Concluída" || inspection.status === "Cancelada";

  useEffect(() => {
    if (!open) return;
    suppressDirtyRef.current = true;

    let cancelled = false;
    loadInspectionTemplate().then((tpl) => {
      if (cancelled) return;
      setTemplate(tpl);
      if (!isReadOnly && (!inspection.findings || inspection.findings.length === 0)) {
        suppressDirtyRef.current = true;
        setFindings(buildFindingsFromTemplate(tpl));
      }
    });

    if (inspection.findings && inspection.findings.length > 0) {
      const normalized: FindingState[] = inspection.findings.map((f) => {
        const photos = f.photos || [];
        const raw = f.thumbnails || [];
        const thumbnails = photos.map((_, i) => raw[i] || "");
        return {
          id: f.id || makeFindingId(),
          room: f.room || "",
          description: f.description || "",
          condition: f.condition || "",
          notes: f.notes || "",
          photos,
          thumbnails,
        };
      });
      setFindings(normalized);
    }
    setNotes(inspection.notes || "");
    setCollapsedRooms({});
    setDirty(false);
    setAutosaveError(null);
    setAutoSavedAt(null);

    const t = setTimeout(() => {
      suppressDirtyRef.current = false;
    }, 0);
    return () => {
      cancelled = true;
      clearTimeout(t);
    };
  }, [open, inspection.id, inspection.findings, inspection.notes, isReadOnly]);

  useEffect(() => {
    if (suppressDirtyRef.current) return;
    setDirty(true);
  }, [findings, notes]);

  const groupedByRoom = useMemo(() => {
    const groups: Array<{ room: string; items: FindingState[] }> = [];
    const indexMap = new Map<string, number>();
    for (const f of findings) {
      if (!indexMap.has(f.room)) {
        indexMap.set(f.room, groups.length);
        groups.push({ room: f.room, items: [] });
      }
      groups[indexMap.get(f.room)!].items.push(f);
    }
    return groups;
  }, [findings]);

  const updateFinding = (id: string, patch: Partial<FindingState>) => {
    setFindings((curr) => curr.map((f) => (f.id === id ? { ...f, ...patch } : f)));
  };

  const removeFinding = (id: string) => {
    setFindings((curr) => curr.filter((f) => f.id !== id));
  };

  const addItem = (room: string) => {
    setFindings((curr) => [
      ...curr,
      { id: makeFindingId(), room, description: "", condition: "", notes: "", photos: [], thumbnails: [] },
    ]);
  };

  const addRoom = () => {
    const name = window.prompt("Nome do novo cômodo:")?.trim();
    if (!name) return;
    if (findings.some((f) => f.room.toLowerCase() === name.toLowerCase())) {
      toast.error("Já existe um cômodo com esse nome.");
      return;
    }
    addItem(name);
  };

  const removePhoto = (findingId: string, photoUrl: string) => {
    setFindings((curr) =>
      curr.map((f) => {
        if (f.id !== findingId) return f;
        const idx = f.photos.indexOf(photoUrl);
        if (idx < 0) return f;
        return {
          ...f,
          photos: f.photos.filter((_, i) => i !== idx),
          thumbnails: f.thumbnails.filter((_, i) => i !== idx),
        };
      }),
    );
  };

  const toggleRoom = (room: string) => {
    setCollapsedRooms((curr) => ({ ...curr, [room]: !curr[room] }));
  };

  const persist = async (overrides?: { status?: string; completedDate?: string }): Promise<Inspection | null> => {
    const cleaned: Finding[] = findings
      .filter((f) => f.room.trim() !== "" && f.description.trim() !== "")
      .map((f) => ({
        id: f.id,
        room: f.room.trim(),
        description: f.description.trim(),
        condition: f.condition,
        notes: f.notes,
        photos: f.photos,
      }));

    try {
      const res = await inspectionService.update(inspection.id, {
        findings: cleaned,
        notes,
        ...(overrides?.status ? { status: overrides.status } : {}),
        ...(overrides?.completedDate ? { completed_date: overrides.completedDate } : {}),
      });
      return res?.data || null;
    } catch (err: any) {
      const msg = err?.response?.data?.message || err?.message || "Erro ao salvar.";
      toast.error(msg);
      return null;
    }
  };

  const handleSave = async () => {
    setSaving(true);
    const updated = await persist();
    setSaving(false);
    if (updated) {
      toast.success("Vistoria atualizada.");
      onSaved?.(updated);
      onClose();
    }
  };

  useEffect(() => {
    if (isReadOnly || !dirty || saving || completing || autosaving) return;
    const handle = setTimeout(async () => {
      setAutosaving(true);
      setAutosaveError(null);
      const updated = await persist();
      setAutosaving(false);
      if (updated) {
        setAutoSavedAt(new Date());
        setDirty(false);
      } else {
        setAutosaveError("Falha no autosave. Salve manualmente.");
      }
    }, 3000);
    return () => clearTimeout(handle);
  }, [dirty, saving, completing, autosaving, isReadOnly, findings, notes]);

  const handleComplete = async () => {
    const complete = async () => {
      setCompleting(true);
      try {
        const updated = await persist({ status: "Concluída", completedDate: new Date().toISOString() });
        if (!updated) throw new Error("Não foi possível concluir a vistoria.");
        toast.success("Vistoria concluída.");
        onSaved?.(updated);
        onClose();
      } finally { setCompleting(false); }
    };
    const missing = findings.filter(f => f.room && f.description && !f.condition).length;
    if (missing > 0) {
      confirmAction(`${missing} item(ns) sem condição preenchida. Concluir mesmo assim?`, complete, { title: "Concluir vistoria", confirmLabel: "Concluir" });
    } else { await complete().catch(() => {}); }
  };

  const handleUpload = async (findingId: string, files: FileList | null) => {
    if (!files || files.length === 0) return;
    if (files.length > 5) {
      toast.error("Máximo de 5 fotos por upload.");
      return;
    }

    let compressed: File[];
    try {
      compressed = await Promise.all(
        Array.from(files).map((f) => compressImageFile(f, { maxDimensionPx: 2000, quality: 0.85 })),
      );
    } catch (err: any) {
      toast.error("Falha ao processar imagem: " + (err?.message || "erro desconhecido"));
      return;
    }

    const persisted = await persist();
    if (!persisted) return;

    try {
      const { urls } = await inspectionService.uploadFindingPhotos(inspection.id, findingId, compressed);
      if (urls.length === 0) {
        toast.error("Nenhuma foto foi processada com sucesso.");
        return;
      }
      updateFinding(findingId, {
        photos: [...(findings.find((f) => f.id === findingId)?.photos || []), ...urls],
      });
      toast.success(`${urls.length} foto${urls.length === 1 ? "" : "s"} enviada${urls.length === 1 ? "" : "s"} com sucesso.`);
    } catch (err: any) {
      const status = err?.response?.status;
      const msg = err?.response?.data?.message || err?.message || "Erro no upload.";
      if (status === 404) {
        toast.error("Item ou vistoria não encontrada.");
      } else if (status === 422) {
        toast.error("Nenhuma foto foi processada (formato inválido?).");
      } else {
        toast.error(msg);
      }
    }
  };

  return (
    <>
      {confirmationDialog}
      <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
        <DialogContent className="max-w-4xl max-h-[90vh] flex flex-col overflow-hidden">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              Checklist da Vistoria
              <span className="text-caption font-mono px-2 py-0.5 rounded bg-muted text-muted-foreground border border-border">
                {inspection.id.slice(0, 8)}
              </span>
              <span
                className={clsx(
                  "text-caption font-bold uppercase tracking-wide px-2 py-0.5 rounded",
                  inspection.status === "Concluída"
                    ? "bg-status-ok-soft text-status-ok"
                    : inspection.status === "Cancelada"
                      ? "bg-muted text-muted-foreground"
                      : "bg-status-warning-soft text-status-warning",
                )}
              >
                {inspection.status}
              </span>
            </DialogTitle>
            <p className="text-caption text-muted-foreground flex items-center flex-wrap gap-x-2">
              <span>
                {inspection.type === "ENTRY"
                  ? "Vistoria de Entrada"
                  : inspection.type === "EXIT"
                    ? "Vistoria de Saída"
                    : "Vistoria Periódica"}
                {" · "}
                {inspection.inspector_name}
                {" · "}
                {new Date(inspection.scheduled_date).toLocaleDateString("pt-BR")}
              </span>
              {!isReadOnly && (
                <AutosaveIndicator autosaving={autosaving} dirty={dirty} autoSavedAt={autoSavedAt} error={autosaveError} />
              )}
            </p>
          </DialogHeader>

          <div className="flex-1 overflow-y-auto pr-1 -mr-1 space-y-4 py-2">
            {groupedByRoom.length === 0 ? (
              <div className="text-center py-8 text-sm text-muted-foreground">
                Nenhum item no checklist ainda.{" "}
                {!isReadOnly && (
                  <button onClick={addRoom} className="text-primary underline-offset-2 hover:underline font-bold">
                    Adicionar primeiro cômodo
                  </button>
                )}
              </div>
            ) : (
              groupedByRoom.map(({ room, items }) => (
                <RoomBlock
                  key={room}
                  room={room}
                  items={items}
                  collapsed={!!collapsedRooms[room]}
                  readOnly={isReadOnly}
                  onToggle={() => toggleRoom(room)}
                  onAddItem={() => addItem(room)}
                  onChangeItem={updateFinding}
                  onRemoveItem={removeFinding}
                  onUploadPhotos={handleUpload}
                  onRemovePhoto={removePhoto}
                  onOpenLightbox={setLightboxUrl}
                />
              ))
            )}
          </div>

          <DialogFooter className="gap-2 flex-col sm:flex-row pt-3 border-t border-border">
            <div className="flex-1 min-w-0 mr-auto">
              <label className="block text-micro font-bold text-muted-foreground uppercase tracking-wide mb-1">
                Observações gerais
              </label>
              <input
                type="text"
                value={notes}
                onChange={(e) => setNotes(e.target.value)}
                disabled={isReadOnly}
                placeholder="Observações que aplicam ao laudo todo…"
                className="w-full px-3 py-1.5 text-sm bg-card border border-border rounded-lg disabled:opacity-50"
              />
            </div>
            <div className="flex items-center gap-2">
              {!isReadOnly && (
                <Button variant="outline" onClick={addRoom} className="gap-1.5">
                  <Plus className="w-4 h-4" />
                  Cômodo
                </Button>
              )}
              <Button variant="outline" onClick={onClose}>
                Fechar
              </Button>
              {!isReadOnly && (
                <>
                  <Button variant="outline" onClick={handleSave} disabled={saving || completing}>
                    {saving && <Loader2 className="w-4 h-4 animate-spin mr-2" />}
                    Salvar Rascunho
                  </Button>
                  <Button
                    onClick={handleComplete}
                    disabled={saving || completing}
                    className="bg-status-ok hover:bg-status-ok/90 text-white"
                  >
                    {completing ? <Loader2 className="w-4 h-4 animate-spin mr-2" /> : <Check className="w-4 h-4 mr-2" />}
                    Concluir Vistoria
                  </Button>
                </>
              )}
            </div>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {lightboxUrl && <Lightbox url={lightboxUrl} onClose={() => setLightboxUrl(null)} />}
    </>
  );
}

interface RoomBlockProps {
  room: string;
  items: FindingState[];
  collapsed: boolean;
  readOnly: boolean;
  onToggle: () => void;
  onAddItem: () => void;
  onChangeItem: (id: string, patch: Partial<FindingState>) => void;
  onRemoveItem: (id: string) => void;
  onUploadPhotos: (findingId: string, files: FileList | null) => void | Promise<void>;
  onRemovePhoto: (findingId: string, url: string) => void;
  onOpenLightbox: (url: string) => void;
}

function RoomBlock({
  room,
  items,
  collapsed,
  readOnly,
  onToggle,
  onAddItem,
  onChangeItem,
  onRemoveItem,
  onUploadPhotos,
  onRemovePhoto,
  onOpenLightbox,
}: RoomBlockProps) {
  const completed = items.filter((i) => i.condition !== "").length;
  const total = items.length;
  const ok = total > 0 && completed === total;

  return (
    <section className="rounded-xl border border-border overflow-hidden">
      <header className="flex items-center justify-between px-4 py-2.5 bg-muted/40 border-b border-border">
        <button onClick={onToggle} className="flex items-center gap-2 flex-1 text-left">
          {collapsed ? (
            <ChevronRight className="w-4 h-4 text-muted-foreground" />
          ) : (
            <ChevronDown className="w-4 h-4 text-muted-foreground" />
          )}
          <h4 className="text-sm font-bold text-foreground">{room || "(sem nome)"}</h4>
          <span
            className={clsx(
              "text-micro font-bold px-1.5 py-0.5 rounded-full tabular-nums",
              ok ? "bg-status-ok-soft text-status-ok" : "bg-muted text-muted-foreground",
            )}
          >
            {completed}/{total}
          </span>
        </button>
        {!readOnly && (
          <button
            onClick={onAddItem}
            className="text-caption font-bold text-primary hover:underline underline-offset-2 inline-flex items-center gap-1"
          >
            <Plus className="w-3.5 h-3.5" /> Item
          </button>
        )}
      </header>

      {!collapsed && (
        <div className="divide-y divide-border bg-card">
          {items.map((item) => (
            <ItemRow
              key={item.id}
              item={item}
              readOnly={readOnly}
              onChange={(patch) => onChangeItem(item.id, patch)}
              onRemove={() => onRemoveItem(item.id)}
              onUploadPhotos={(files) => onUploadPhotos(item.id, files)}
              onRemovePhoto={(url) => onRemovePhoto(item.id, url)}
              onOpenLightbox={onOpenLightbox}
            />
          ))}
        </div>
      )}
    </section>
  );
}

interface ItemRowProps {
  item: FindingState;
  readOnly: boolean;
  onChange: (patch: Partial<FindingState>) => void;
  onRemove: () => void;
  onUploadPhotos: (files: FileList | null) => void | Promise<void>;
  onRemovePhoto: (url: string) => void;
  onOpenLightbox: (url: string) => void;
}

function ItemRow({ item, readOnly, onChange, onRemove, onUploadPhotos, onRemovePhoto, onOpenLightbox }: ItemRowProps) {
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const [uploading, setUploading] = useState(false);

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    if (!e.target.files || e.target.files.length === 0) return;
    setUploading(true);
    try {
      await onUploadPhotos(e.target.files);
    } finally {
      setUploading(false);
      if (fileInputRef.current) fileInputRef.current.value = "";
    }
  };

  return (
    <div className="px-4 py-3 flex flex-col gap-2">
      <div className="flex items-start gap-3">
        <input
          type="text"
          value={item.description}
          onChange={(e) => onChange({ description: e.target.value })}
          disabled={readOnly}
          placeholder="Descrição do item"
          className="flex-1 px-2 py-1 text-sm font-medium bg-transparent border-0 border-b border-transparent focus:border-primary focus:outline-none disabled:opacity-70"
        />

        <select
          value={item.condition}
          onChange={(e) => onChange({ condition: e.target.value as ConditionValue })}
          disabled={readOnly}
          className={clsx("text-caption font-bold px-2 py-1 rounded border tabular-nums shrink-0", conditionStyle(item.condition))}
        >
          <option value="">— Condição —</option>
          {CONDITION_VALUES.map((c) => (
            <option key={c} value={c}>
              {c}
            </option>
          ))}
        </select>

        {!readOnly && (
          <button
            onClick={onRemove}
            title="Remover item"
            className="p-1 text-muted-foreground hover:text-status-critical shrink-0"
          >
            <Trash2 className="w-4 h-4" />
          </button>
        )}
      </div>

      <input
        type="text"
        value={item.notes}
        onChange={(e) => onChange({ notes: e.target.value })}
        disabled={readOnly}
        placeholder="Observação (opcional)…"
        className="w-full px-2 py-1 text-caption bg-muted/30 border border-border rounded disabled:opacity-70"
      />

      <div className="flex flex-wrap items-center gap-2">
        {item.photos.length === 0 && readOnly && (
          <span className="text-caption text-muted-foreground inline-flex items-center gap-1">
            <ImageIcon className="w-3.5 h-3.5" /> Sem fotos
          </span>
        )}
        {item.photos.map((url, idx) => {
          const thumbUrl = item.thumbnails?.[idx];
          const displayUrl = thumbUrl && thumbUrl !== "" ? thumbUrl : url;
          return (
            <PhotoThumb
              key={url}
              url={displayUrl}
              readOnly={readOnly}
              onOpen={() => onOpenLightbox(url)}
              onRemove={() => onRemovePhoto(url)}
            />
          );
        })}
        {!readOnly && (
          <>
            <button
              type="button"
              onClick={() => fileInputRef.current?.click()}
              disabled={uploading}
              className="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-md border border-dashed border-border hover:border-primary hover:bg-primary/5 text-caption font-bold text-foreground disabled:opacity-50"
            >
              {uploading ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Camera className="w-3.5 h-3.5" />}
              {uploading ? "Enviando…" : "+ Foto"}
            </button>
            <input
              ref={fileInputRef}
              type="file"
              accept="image/jpeg,image/png,image/webp"
              multiple
              onChange={handleFileChange}
              className="hidden"
            />
          </>
        )}
      </div>
    </div>
  );
}

function PhotoThumb({
  url,
  readOnly,
  onOpen,
  onRemove,
}: {
  url: string;
  readOnly: boolean;
  onOpen: () => void;
  onRemove: () => void;
}) {
  return (
    <div className="relative group">
      <button
        type="button"
        onClick={onOpen}
        className="w-14 h-14 rounded-md overflow-hidden border border-border bg-muted block hover:opacity-90"
      >
        <MediaImage src={url} alt="" className="w-full h-full object-cover" loading="lazy" />
      </button>
      {!readOnly && (
        <button
          type="button"
          onClick={onRemove}
          title="Remover foto"
          className="absolute -top-1 -right-1 w-5 h-5 rounded-full bg-status-critical text-white opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center"
        >
          <X className="w-3 h-3" />
        </button>
      )}
    </div>
  );
}

function Lightbox({ url, onClose }: { url: string; onClose: () => void }) {
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-5xl p-0 overflow-hidden bg-black border-0">
        <MediaImage src={url} alt="" className="w-full h-auto max-h-[85vh] object-contain bg-black" />
      </DialogContent>
    </Dialog>
  );
}

function AutosaveIndicator({
  autosaving,
  dirty,
  autoSavedAt,
  error,
}: {
  autosaving: boolean;
  dirty: boolean;
  autoSavedAt: Date | null;
  error: string | null;
}) {
  if (autosaving) {
    return (
      <span className="inline-flex items-center gap-1 text-status-info">
        <Loader2 className="w-3 h-3 animate-spin" /> Salvando…
      </span>
    );
  }
  if (error) {
    return (
      <span className="inline-flex items-center gap-1 text-status-critical font-bold">
        <AlertTriangle className="w-3 h-3" /> {error}
      </span>
    );
  }
  if (autoSavedAt && !dirty) {
    return (
      <span className="inline-flex items-center gap-1 text-status-ok">
        <CloudUpload className="w-3 h-3" /> Salvo às{" "}
        {autoSavedAt.toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit" })}
      </span>
    );
  }
  if (dirty) {
    return (
      <span className="inline-flex items-center gap-1 text-status-warning">
        <span className="w-2 h-2 rounded-full bg-status-warning" /> Mudanças não salvas
      </span>
    );
  }
  return null;
}

function conditionStyle(condition: string): string {
  switch (condition) {
    case "Ótimo":
      return "bg-status-ok-soft text-status-ok border-status-ok/30";
    case "Bom":
      return "bg-status-info-soft text-status-info border-status-info/30";
    case "Regular":
      return "bg-status-warning-soft text-status-warning border-status-warning/30";
    case "Ruim":
      return "bg-status-critical-soft text-status-critical border-status-critical/30";
    case "Danificado":
      return "bg-status-critical text-white border-status-critical";
    default:
      return "bg-muted text-muted-foreground border-border";
  }
}

const _unused = AlertTriangle;
void _unused;
