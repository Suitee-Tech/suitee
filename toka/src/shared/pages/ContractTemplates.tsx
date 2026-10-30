import { useState, useEffect, useRef, useCallback, useMemo } from "react";
import { Plus, Edit2, Trash2, Copy, FileText, Save, X, Maximize2, Minimize2, Upload, ChevronDown, PanelLeftOpen } from "lucide-react";
import { templateService, ContractTemplate } from "../services/template.service";
import { SuperDocEditor, SuperDocRef } from "@superdoc-dev/react";
import "@superdoc-dev/react/style.css";
import { EmptyState } from "../ui/EmptyState";
import { Button } from "../ui/button";
import { cn } from "../ui/utils";
import { toast } from "sonner";
import { BlankDOCX, getFileObject } from "superdoc";
import { useVariableDetection } from "../hooks/useVariableDetection";
import { useConfirmAction } from "../hooks/useConfirmAction";
import { useDomVariableHighlight } from "../hooks/useDomVariableHighlight";
import { useIsAtLeastMd } from "../hooks/useMediaQuery";
import { VariableSidebar } from "../components/contract-editor/VariableSidebar";
import { VariableMappingDialog } from "../components/contract-editor/VariableMappingDialog";
import { TextToVariablePopover } from "../components/contract-editor/TextToVariablePopover";
import { getVariableByKey, isSystemVariable } from "../contracts/contractVariables";
import {
  type DocxVariableMarker,
  formatDocxTemplateIssue,
  inspectDocxTemplate,
  replaceDocxText,
  replaceDocxVariable,
  fileToBase64,
} from "../utils/docx";

const EDITOR_USER = { name: "Admin", email: "admin@suitee.com" };

const formatUpdatedAt = (value?: string): string => {
  if (!value) return "—";
  const d = new Date(value);
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 1970) return "—";
  return d.toLocaleDateString("pt-BR");
};

const base64ToFile = (base64: string, filename: string): File => {
  try {
    let base64Data = base64.includes(",") ? base64.split(",")[1] : base64;
    base64Data = base64Data.replace(/-/g, "+").replace(/_/g, "/").replace(/\s/g, "");

    while (base64Data.length % 4 !== 0) {
      base64Data += "=";
    }

    const bstr = atob(base64Data);
    let n = bstr.length;
    const u8arr = new Uint8Array(n);
    while (n--) {
      u8arr[n] = bstr.charCodeAt(n);
    }
    return new File([u8arr], filename, {
      type: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    });
  } catch (e) {
    console.error("Failed to convert base64 to file", e);
    return new File([], filename);
  }
};


/**
 * Lê o texto renderizado nas coordenadas (x, y) usando caretRangeFromPoint.
 * Retorna uma string única o suficiente para localizar a posição no documento.
 */
function getTextAtPoint(x: number, y: number): string | null {
  try {
    const doc = document as any;
    let range: Range | null = null;

    if (doc.caretRangeFromPoint) {
      range = doc.caretRangeFromPoint(x, y);
    } else if (doc.caretPositionFromPoint) {
      const pos = doc.caretPositionFromPoint(x, y);
      if (pos) {
        range = document.createRange();
        range.setStart(pos.offsetNode, pos.offset);
        range.collapse(true);
      }
    }

    if (!range) return null;

    // Expande o range para pegar contexto ao redor (até 40 chars)
    const node = range.startContainer;
    if (node.nodeType !== Node.TEXT_NODE) return null;

    const text = node.textContent || "";
    const offset = range.startOffset;

    // Pega até 30 caracteres antes e depois da posição
    const start = Math.max(0, offset - 30);
    const end = Math.min(text.length, offset + 30);
    const context = text.substring(start, end).trim();

    return context || null;
  } catch {
    return null;
  }
}

const injectVariableStyles = () => {
  if (document.getElementById("superdoc-variable-styles")) return;
  const style = document.createElement("style");
  style.id = "superdoc-variable-styles";
  style.innerHTML = `
 .variable-highlight {
 background-color: #f0fdf4 !important;
 color: #15803d !important;
 border: 1px solid #bbf7d0 !important;
 border-radius: 4px !important;
 padding: 2px 4px !important;
 font-family: monospace !important;
 font-size: 0.9em !important;
 font-weight: 600 !important;
 cursor: pointer !important;
 transition: all 0.2s !important;
 display: inline-block !important;
 line-height: 1.2 !important;
 box-shadow: 0 1px 2px rgba(0,0,0,0.05) !important;
 }
 .variable-highlight:hover {
 background-color: #dcfce7 !important;
 border-color: #86efac !important;
 box-shadow: 0 2px 4px rgba(0,0,0,0.1) !important;
 }
 `;
  document.head.appendChild(style);
};

/**
 * Tela de modelos de contrato. Com `embedded` ela vira só o EDITOR, para ser
 * montada dentro de um painel de outra tela — hoje o painel de contrato da
 * esteira, que sem modelo cadastrado não tem minuta para gerar.
 *
 * Nesse modo: abre direto no cadastro de um modelo novo, o "Voltar" fecha o
 * painel (`onExit`) em vez de cair na lista de modelos, o botão de tela cheia
 * fica oculto (ele é `fixed` em z-50 e nasceria atrás do painel) e o modelo
 * salvo é devolvido por `onSaved` para a esteira já selecioná-lo.
 */
export function ContractTemplates({
  embedded = false,
  onExit,
  onSaved,
}: {
  embedded?: boolean;
  onExit?: () => void;
  onSaved?: (template: ContractTemplate) => void;
} = {}) {
  const { confirmAction, confirmationDialog } = useConfirmAction();
  const [templates, setTemplates] = useState<ContractTemplate[]>([]);
  const [loading, setLoading] = useState(true);
  const [editingTemplate, setEditingTemplate] = useState<ContractTemplate | null>(null);
  const [isCreating, setIsCreating] = useState(false);
  const [templateName, setTemplateName] = useState("");
  const [templateDesc, setTemplateDesc] = useState("");
  const [templateFile, setTemplateFile] = useState<File | null>(null);
  const [docxVariableKeys, setDocxVariableKeys] = useState<string[]>([]);
  const [isFullScreen, setIsFullScreen] = useState(false);
  const isDesktop = useIsAtLeastMd();
  const [sidebarWidth, setSidebarWidth] = useState(320);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [mobilePanelOpen, setMobilePanelOpen] = useState(true);
  const sidebarRef = useRef<HTMLDivElement>(null);

  const [mappingDialogOpen, setMappingDialogOpen] = useState(false);
  const [mappingTarget, setMappingTarget] = useState<{
    key: string;
    mode: "map" | "remap";
    oldKey?: string;
    isSingleReplace?: boolean;
    occurrenceIndex?: number;
    marker?: DocxVariableMarker;
    sourceTag?: string;
  }>({ key: "", mode: "map" });

  const [isDraggingOverEditor, setIsDraggingOverEditor] = useState(false);

  useEffect(() => {
    const stored = Number(window.localStorage.getItem("contract-editor-sidebar-width"));
    if (Number.isFinite(stored) && stored >= 280 && stored <= 440) setSidebarWidth(stored);
  }, []);

  const beginSidebarResize = (event: React.PointerEvent<HTMLDivElement>) => {
    event.preventDefault();
    const startX = event.clientX;
    const startWidth = sidebarRef.current?.getBoundingClientRect().width ?? sidebarWidth;
    const onMove = (moveEvent: PointerEvent) => {
      const next = Math.min(440, Math.max(280, startWidth + moveEvent.clientX - startX));
      setSidebarWidth(next);
    };
    const onUp = () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
      setSidebarWidth((current) => {
        window.localStorage.setItem("contract-editor-sidebar-width", String(current));
        return current;
      });
    };
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp, { once: true });
  };

  useEffect(() => {
    let cancelled = false;
    if (!templateFile) {
      setDocxVariableKeys([]);
      return;
    }
    fileToBase64(templateFile)
      .then((base64) => {
        if (cancelled) return;
        const inspection = inspectDocxTemplate(base64);
        setDocxVariableKeys(inspection.valid ? inspection.variables : []);
      })
      .catch(() => {
        if (!cancelled) setDocxVariableKeys([]);
      });
    return () => {
      cancelled = true;
    };
  }, [templateFile]);

  const editorRef = useRef<SuperDocRef>(null);
  const editorCanvasRef = useRef<HTMLDivElement>(null);
  const detection = useVariableDetection(editorRef, {
    getDomText: () => editorCanvasRef.current?.textContent || "",
  });

  // O scan do SuperDoc cobre o corpo visível. O DOCX é a fonte de verdade e
  // também inclui cabeçalho/rodapé; unir os dois evita "Nenhuma variável
  // detectada" quando a tag existe apenas no header ou ainda não foi renderizada.
  const mergedDetection = useMemo(() => {
    const keys = Array.from(new Set([...(detection.allFoundKeys ?? []), ...docxVariableKeys]));
    const identifiedVars = keys
      .map((key) => getVariableByKey(key))
      .filter((variable): variable is NonNullable<typeof variable> => Boolean(variable));
    const unidentifiedKeys = keys.filter((key) => !getVariableByKey(key));
    return { ...detection, allFoundKeys: keys, identifiedVars, unidentifiedKeys };
  }, [detection, docxVariableKeys]);

  // Highlight DOM de variáveis e click delegation (v2 native fallback)
  useDomVariableHighlight(editorCanvasRef, !!templateFile);

  useEffect(() => {
    const canvas = editorCanvasRef.current;
    if (!canvas) return;

    let dragCounter = 0;

    const onDragOver = (e: DragEvent) => {
      if (!e.dataTransfer?.types.includes("text/plain")) return;
      e.preventDefault();
      e.dataTransfer.dropEffect = "copy";
    };

    const onDragEnter = (e: DragEvent) => {
      if (!e.dataTransfer?.types.includes("text/plain")) return;
      dragCounter++;
      if (dragCounter === 1) setIsDraggingOverEditor(true);
    };

    const onDragLeave = () => {
      dragCounter--;
      if (dragCounter <= 0) {
        dragCounter = 0;
        setIsDraggingOverEditor(false);
      }
    };

    const onDropFallback = async (e: DragEvent) => {
      dragCounter = 0;
      setIsDraggingOverEditor(false);

      const text = e.dataTransfer?.getData("text/plain");
      if (!text || !/^\{\{.+\}\}$/.test(text)) return;

      e.preventDefault();

      // 1. Lê o texto renderizado nas coordenadas do drop
      const contextText = getTextAtPoint(e.clientX, e.clientY);

      // 2. Usa Document API para localizar e inserir
      try {
        const superdoc = editorRef.current?.getInstance();
        const editor = superdoc?.activeEditor;

        if (contextText && editor?.doc?.query?.match && editor?.doc?.replace) {
          // Escapa caracteres especiais de regex
          const escaped = contextText.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
          const result = await editor.doc.query.match({
            select: { type: "text", pattern: escaped },
          });

          if ((result as any)?.items?.length > 0) {
            const target = (result as any).items[0].target;
            // Substitui o texto encontrado por ele mesmo + variável
            await editor.doc.replace({
              target,
              text: contextText + text,
            });
            setTimeout(() => detection.triggerScan(), 500);
            toast.success(`Variável ${text} inserida no documento`);
            return;
          }
        }

        // Fallback: tenta selectionTarget (insere no cursor atual)
        if (editor?.doc?.selection?.current && editor?.doc?.replace) {
          const sel = await editor.doc.selection.current({ includeText: false });
          const writeTarget = (sel as any)?.selectionTarget ?? (sel as any)?.target;
          if (writeTarget) {
            await editor.doc.replace({ target: writeTarget, text });
            setTimeout(() => detection.triggerScan(), 500);
            toast.success(`Variável ${text} inserida no documento`);
            return;
          }
        }
      } catch (err) {
        console.error("Erro ao inserir via Document API", err);
      }

      // Último fallback: clipboard
      navigator.clipboard.writeText(text);
      toast.info(`Variável ${text} copiada — cole no documento com Ctrl+V`);
    };

    const onDragEnd = () => {
      dragCounter = 0;
      setIsDraggingOverEditor(false);
      setTimeout(() => detection.triggerScan(), 800);
    };

    canvas.addEventListener("dragover", onDragOver, true);
    canvas.addEventListener("dragenter", onDragEnter, true);
    canvas.addEventListener("dragleave", onDragLeave, true);
    canvas.addEventListener("drop", onDropFallback, false);
    window.addEventListener("dragend", onDragEnd, false);

    return () => {
      canvas.removeEventListener("dragover", onDragOver, true);
      canvas.removeEventListener("dragenter", onDragEnter, true);
      canvas.removeEventListener("dragleave", onDragLeave, true);
      canvas.removeEventListener("drop", onDropFallback, false);
      window.removeEventListener("dragend", onDragEnd, false);
    };
  }, [detection, templateFile]);

  useEffect(() => {
    loadTemplates();
  }, []);

  useEffect(() => {
    const handleVariableClicked = (e: Event) => {
      const customEvent = e as CustomEvent<{
        key: string;
        marker?: DocxVariableMarker;
        pos: number;
        sourceTag?: string;
      }>;
      const { key, marker, sourceTag } = customEvent.detail;

      const isIdentified = detection.identifiedVars.some((v) => v.key === key);
      setMappingTarget({
        key,
        mode: isIdentified ? "remap" : "map",
        // Uma tag de lista não pode ser alterada isoladamente sem quebrar seu
        // par de abertura/fechamento; nesse caso remapeamos a variável inteira.
        isSingleReplace: !marker,
        occurrenceIndex: customEvent.detail.pos,
        marker,
        sourceTag,
      });
      setMappingDialogOpen(true);
    };

    window.addEventListener("variable-clicked", handleVariableClicked);
    return () => window.removeEventListener("variable-clicked", handleVariableClicked);
  }, [detection]);

  const loadTemplates = async () => {
    try {
      setLoading(true);
      const data = await templateService.list();
      setTemplates(data || []);
    } catch (error) {
      console.error("Failed to load templates", error);
      toast.error("Erro ao carregar templates");
    } finally {
      setLoading(false);
    }
  };

  const loadBlankDocx = async () => {
    try {
      const file = await getFileObject(
        BlankDOCX,
        "blank.docx",
        "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
      );
      return file;
    } catch (e) {
      console.error("Failed to load blank docx", e);
      return new File([], "blank.docx");
    }
  };

  const handleCreateNew = async () => {
    setIsCreating(true);
    setEditingTemplate(null);
    setTemplateName("");
    setTemplateDesc("");
    const file = await loadBlankDocx();
    setTemplateFile(file);
  };

  // Quem chega pelo painel de contrato da esteira já chega para criar: não há
  // modelo cadastrado e é exatamente isso que o bloqueia. Abrir o editor
  // sozinho evita repetir o beco sem saída ("crie um em Configurações").
  const autoStarted = useRef(false);
  useEffect(() => {
    if (!embedded || autoStarted.current) return;
    autoStarted.current = true;
    void handleCreateNew();
    // eslint-disable-next-line react-hooks/exhaustive-deps -- dispara uma única vez, na abertura do painel
  }, [embedded]);

  const handleEdit = (tpl: ContractTemplate) => {
    setIsCreating(false);
    setEditingTemplate(tpl);
    setTemplateName(tpl.name);
    setTemplateDesc(tpl.description || "");
    const file = base64ToFile(tpl.content, `${tpl.name}.docx`);
    setTemplateFile(file);
  };

  const handleImportDocx = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    if (
      !file.name.endsWith(".docx") &&
      file.type !== "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
    ) {
      toast.error("Por favor, selecione um arquivo .docx válido");
      return;
    }

    setTemplateFile(file);
    toast.success("Documento importado com sucesso!");
    e.target.value = "";
  };

  const handleDelete = (id: string) => {
    confirmAction(
      "Tem certeza que deseja excluir este modelo? Esta ação não pode ser desfeita.",
      async () => {
        try {
          await templateService.delete(id);
          toast.success("Modelo excluído com sucesso");
          loadTemplates();
        } catch (e) {
          console.error(e);
          toast.error("Erro ao excluir modelo");
          throw e;
        }
      },
      { title: "Excluir modelo", confirmLabel: "Excluir", variant: "danger" },
    );
  };

  const handleDuplicate = async (tpl: ContractTemplate) => {
    try {
      await templateService.create({
        name: `${tpl.name} (Cópia)`,
        description: tpl.description,
        content: tpl.content,
        variables: tpl.variables,
      });
      toast.success("Modelo duplicado");
      loadTemplates();
    } catch (e) {
      toast.error("Erro ao duplicar modelo");
    }
  };

  const handleInsertVariable = useCallback((key: string) => {
    navigator.clipboard.writeText(`{{${key}}}`);
    toast.success(`Copiado: {{${key}}}. Cole no documento.`);
    if (typeof window !== "undefined" && window.matchMedia?.("(max-width: 767px)").matches) {
      setMobilePanelOpen(false);
    }
  }, []);

  const handleLocateVariable = useCallback((key: string) => {
    const root = editorCanvasRef.current;
    if (!root) return;
    const target = `{{${key}}}`;
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    let node: Node | null = walker.nextNode();
    while (node) {
      const text = node.nodeValue || "";
      const offset = text.indexOf(target);
      if (offset >= 0) {
        const range = document.createRange();
        range.setStart(node, offset);
        range.setEnd(node, offset + target.length);
        const selection = window.getSelection();
        selection?.removeAllRanges();
        selection?.addRange(range);
        const element = node.parentElement;
        element?.scrollIntoView({ behavior: "smooth", block: "center" });
        return;
      }
      node = walker.nextNode();
    }
    toast.info(`Variável {{${key}}} não encontrada no texto do documento.`);
  }, []);

  const handleMapVariable = useCallback((key: string) => {
    setMappingTarget({ key, mode: "map" });
    setMappingDialogOpen(true);
  }, []);

  const handleRemapVariable = useCallback((key: string) => {
    setMappingTarget({ key, mode: "remap" });
    setMappingDialogOpen(true);
  }, []);

  const handleReplaceVariable = useCallback(
    async (
      oldKey: string,
      newKey: string,
      isSingleReplace?: boolean,
      occurrenceIndex?: number,
      marker?: DocxVariableMarker,
      sourceTag?: string,
    ) => {
      if (oldKey === newKey) return;

      const superdoc = editorRef.current?.getInstance();
      if (!superdoc) return;

      try {
        const blobs = await superdoc.exportEditorsToDOCX();
        if (!blobs || blobs.length === 0) return;

        const arrayBuffer = await blobs[0].arrayBuffer();
        const result = replaceDocxVariable(
          arrayBuffer,
          oldKey,
          newKey,
          {
            replaceAll: !isSingleReplace,
            occurrenceIndex,
            marker,
            sourceTag,
          },
        );
        if (result.replacements === 0) {
          toast.error(`A variável {{${oldKey}}} não foi encontrada no DOCX exportado`);
          return;
        }

        const newFile = new File([result.content], templateFile?.name || "template.docx", {
          type: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
        });

        setTemplateFile(newFile);
        toast.success(
          `${result.replacements} ocorrência(s) de {{${oldKey}}} substituída(s) por {{${newKey}}}`,
        );

        setTimeout(() => detection.triggerScan(), 1000);
      } catch (e) {
        console.error("Error replacing variable", e);
        toast.error("Erro ao substituir variável no documento");
      }
    },
    [templateFile, detection],
  );

  const handleCreateVariableFromText = useCallback(
    async (selectedText: string, variableKey: string, selectionTarget?: unknown) => {
      const superdoc = editorRef.current?.getInstance();
      if (!superdoc) return false;

      try {
        const editor = superdoc.activeEditor;
        if (selectionTarget && editor?.doc?.replace) {
          const receipt = await editor.doc.replace({
            target: selectionTarget as any,
            text: `{{${variableKey}}}`,
          });
          const failed = (receipt as any)?.success === false || (receipt as any)?.ok === false;
          if (!failed) {
            setTimeout(() => detection.triggerScan(), 500);
            return true;
          }
        }

        const blobs = await superdoc.exportEditorsToDOCX();
        if (!blobs || blobs.length === 0) return false;

        const arrayBuffer = await blobs[0].arrayBuffer();
        const matchCount = replaceDocxText(
          arrayBuffer,
          selectedText,
          selectedText,
        ).replacements;
        if (matchCount !== 1) {
          await navigator.clipboard.writeText(`{{${variableKey}}}`);
          toast.info(
            matchCount > 1
              ? `O texto aparece ${matchCount} vezes. A variável foi copiada para evitar substituir o trecho errado.`
              : `Variável {{${variableKey}}} copiada. Substitua o texto selecionado manualmente.`,
          );
          return false;
        }

        const result = replaceDocxText(
          arrayBuffer,
          selectedText,
          `{{${variableKey}}}`,
          false,
        );

        if (result.replacements === 0) {
          await navigator.clipboard.writeText(`{{${variableKey}}}`);
          toast.info(
            `Variável {{${variableKey}}} copiada. Substitua o texto selecionado manualmente.`,
          );
          return false;
        }

        const newFile = new File([result.content], templateFile?.name || "template.docx", {
          type: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
        });

        setTemplateFile(newFile);
        setTimeout(() => detection.triggerScan(), 1000);
        return true;
      } catch (e) {
        console.error("Error creating variable from text", e);
        await navigator.clipboard.writeText(`{{${variableKey}}}`);
        toast.info(`Variável {{${variableKey}}} copiada. Substitua o texto manualmente.`);
        return false;
      }
    },
    [templateFile, detection],
  );

  const persistTemplate = async (base64Content: string, exportedVariables: string[]) => {
    try {
      let saved: ContractTemplate | null = null;

      // Embutido, quem avisa é a esteira (o aviso dela diz que o modelo já
      // entrou selecionado): dois toasts seguidos só fazem ruído.
      if (isCreating) {
        saved = await templateService.create({
          name: templateName,
          description: templateDesc,
          content: base64Content,
          variables: exportedVariables,
        });
        if (!embedded) toast.success("Modelo criado com sucesso");
      } else if (editingTemplate) {
        saved = await templateService.update(editingTemplate.id, {
          name: templateName,
          description: templateDesc,
          content: base64Content,
          variables: exportedVariables,
        });
        if (!embedded) toast.success("Modelo atualizado com sucesso");
      }

      // Embutido, quem fecha o painel e recarrega a lista é a esteira — o
      // modelo salvo volta por `onSaved` para já entrar selecionado.
      if (embedded) {
        if (saved) onSaved?.(saved);
        return;
      }

      setIsCreating(false);
      setEditingTemplate(null);
      setTemplateFile(null);
      loadTemplates();
    } catch (e) {
      console.error(e);
      toast.error("Erro ao salvar modelo");
      // Mantém a confirmação aberta para permitir nova tentativa.
      throw e;
    }
  };

  const handleSaveTemplate = async () => {
    if (!templateName.trim()) {
      toast.error("Nome do modelo é obrigatório");
      return;
    }

    try {
      const superdoc = editorRef.current?.getInstance();
      if (!superdoc) {
        toast.error("Editor não inicializado");
        return;
      }

      const blobs = await superdoc.exportEditorsToDOCX();
      if (!blobs || blobs.length === 0) {
        toast.error("Erro ao gerar arquivo do modelo");
        return;
      }

      const docxBlob = blobs[0];
      const base64Content = await fileToBase64(docxBlob);
      const inspection = inspectDocxTemplate(base64Content);

      if (!inspection.valid) {
        const details = inspection.issues
          .slice(0, 3)
          .map(formatDocxTemplateIssue)
          .join("; ");
        toast.error(`Corrija a formatação do modelo antes de salvar: ${details}`, {
          duration: 9000,
        });
        return;
      }

      // A fonte de verdade é o DOCX exportado. A detecção visual pode ficar
      // defasada ou perder tags cujas chaves estejam divididas entre runs.
      const exportedVariables = inspection.variables;
      const unidentifiedKeys = exportedVariables.filter((key) => {
        if (isSystemVariable(key)) return false;
        const root = key.split(".")[0];
        return !isSystemVariable(root);
      });

      if (unidentifiedKeys.length > 0) {
        confirmAction(
          `Existem ${unidentifiedKeys.length} variáveis não identificadas no modelo (${unidentifiedKeys.join(
            ", ",
          )}). Deseja salvar mesmo assim?`,
          () => persistTemplate(base64Content, exportedVariables),
          { title: "Salvar modelo", confirmLabel: "Salvar mesmo assim", variant: "warning" },
        );
        return;
      }

      await persistTemplate(base64Content, exportedVariables).catch(() => {});
    } catch (e) {
      console.error(e);
      toast.error("Erro ao salvar modelo");
    }
  };

  if (isCreating || editingTemplate) {
    return (
      <div
        className={
          isFullScreen
            ? "fixed inset-0 z-50 bg-card flex flex-col"
            : "flex flex-col h-full bg-muted "
        }
      >
        <div className="bg-card border-b border-border shadow-sm relative z-10">
          <div className="flex flex-col md:flex-row md:items-center justify-between px-4 md:px-5 py-3 md:py-2.5 gap-3 md:gap-0">
            <div className="flex min-w-0 flex-1 items-start gap-2 md:gap-3">
              <Button
                variant="ghost"
                size="icon"
                onClick={() => {
                  if (embedded) {
                    onExit?.();
                    return;
                  }
                  setIsCreating(false);
                  setEditingTemplate(null);
                  setTemplateFile(null);
                  setIsFullScreen(false);
                }}
                className="size-8 shrink-0"
                title={embedded ? "Fechar" : "Voltar"}
                aria-label={embedded ? "Fechar cadastro do modelo" : "Voltar para a lista de modelos"}
              >
                <X className="w-4 h-4" />
              </Button>
              <div className="min-w-0 flex-1 space-y-0.5">
                <input
                  type="text"
                  placeholder="Nome do modelo..."
                  aria-label="Nome do modelo"
                  title={templateName}
                  value={templateName}
                  onChange={(e) => setTemplateName(e.target.value)}
                  className="w-full min-w-0 rounded-sm border-none bg-transparent px-0 py-1 text-base font-semibold text-foreground placeholder:font-normal placeholder:text-muted-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-300"
                />
                <input
                  type="text"
                  placeholder="Descrição (opcional)"
                  aria-label="Descrição do modelo"
                  value={templateDesc}
                  onChange={(e) => setTemplateDesc(e.target.value)}
                  className="w-full rounded-sm border-none bg-transparent px-0 py-0.5 text-xs text-muted-foreground placeholder:text-muted-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-300"
                />
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-2 shrink-0 w-full md:w-auto justify-between md:justify-end border-t md:border-0 border-border pt-3 md:pt-0">
              <div className="flex items-center gap-2">
                <TextToVariablePopover onCreateVariable={handleCreateVariableFromText} editorRef={editorRef} />
                <div className="h-5 w-px bg-muted hidden sm:block" />
                <div className="flex items-center">
                  <input
                    type="file"
                    accept=".docx,application/vnd.openxmlformats-officedocument.wordprocessingml.document"
                    className="hidden"
                    id="import-docx"
                    onChange={handleImportDocx}
                  />
                  <label
                    htmlFor="import-docx"
                    className="px-2 sm:px-3 py-1.5 text-muted-foreground hover:bg-muted rounded-lg text-xs sm:text-sm font-medium transition-colors flex items-center gap-1.5 cursor-pointer"
                  >
                    <Upload className="w-4 h-4" />{" "}
                    <span className="hidden sm:inline">Importar</span>
                  </label>
                </div>
                {/* Tela cheia é `fixed inset-0 z-50`: dentro do painel da esteira
                    (camada 200) o editor nasceria atrás do próprio painel. */}
                {!embedded && (
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => setIsFullScreen(!isFullScreen)}
                    className="hidden sm:flex"
                    title={isFullScreen ? "Sair da Tela Cheia" : "Tela Cheia"}
                  >
                    {isFullScreen ? (
                      <Minimize2 className="w-4 h-4" />
                    ) : (
                      <Maximize2 className="w-4 h-4" />
                    )}
                  </Button>
                )}
              </div>
              <div className="flex items-center gap-2">
                <div className="h-5 w-px bg-muted hidden md:block" />
                <Button size="sm" onClick={handleSaveTemplate}>
                  <Save className="w-4 h-4" /> Salvar
                </Button>
              </div>
            </div>
          </div>
        </div>

        <div className="flex flex-1 flex-col-reverse overflow-hidden md:flex-row">
          <div
            ref={sidebarRef}
            style={isDesktop ? { width: sidebarCollapsed ? 0 : sidebarWidth } : undefined}
            className={cn(
              "flex w-full shrink-0 flex-col overflow-hidden border-t border-border bg-card md:border-r md:border-t-0",
              mobilePanelOpen ? "h-[55vh]" : "h-12",
              "md:h-full",
              sidebarCollapsed && "md:border-r-0",
            )}
          >
            <button
              type="button"
              className="flex w-full shrink-0 items-center justify-between px-4 py-3 text-left md:hidden"
              aria-expanded={mobilePanelOpen}
              aria-controls="mobile-variable-panel"
              onClick={() => setMobilePanelOpen((open) => !open)}
            >
              <span className="flex items-center gap-2 text-sm font-bold text-foreground">
                Variáveis
                {(mergedDetection.allFoundKeys.length) > 0 && (
                  <span className="rounded-full bg-primary-100 px-1.5 py-0.5 text-caption font-bold text-primary-700">
                    {(mergedDetection.allFoundKeys.length)}
                  </span>
                )}
              </span>
              <ChevronDown className={cn("size-4 text-muted-foreground transition-transform", mobilePanelOpen && "rotate-180")} />
            </button>
            <div id="mobile-variable-panel" className={cn("flex min-h-0 flex-1", !mobilePanelOpen && "hidden md:flex")}>
              <VariableSidebar
                detection={mergedDetection}
                onTriggerScan={detection.triggerScan}
                onInsertVariable={handleInsertVariable}
                onMapVariable={handleMapVariable}
                onRemapVariable={handleRemapVariable}
                onLocateVariable={handleLocateVariable}
                onCollapse={() => setSidebarCollapsed(true)}
              />
            </div>
          </div>

          {!sidebarCollapsed && (
            <div
              role="separator"
              aria-orientation="vertical"
              aria-label="Redimensionar painel de variáveis"
              onPointerDown={beginSidebarResize}
              className="hidden w-1.5 shrink-0 cursor-col-resize bg-border/60 transition-colors hover:bg-primary-300 md:block"
            />
          )}

          {templateFile && (
            <div ref={editorCanvasRef} className="relative h-full flex-1 overflow-auto bg-muted">
              {sidebarCollapsed && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setSidebarCollapsed(false)}
                  className="absolute left-3 top-3 z-20 hidden gap-1.5 md:flex"
                  aria-label="Abrir painel de variáveis"
                >
                  <PanelLeftOpen className="size-4" /> Variáveis
                </Button>
              )}
              <div className="flex justify-center min-h-full py-6 px-4">
                <div
                  className={`w-full max-w-[850px] bg-card rounded-lg shadow-md min-h-full transition-all duration-150 ${
                    isDraggingOverEditor
                      ? "ring-2 ring-primary-400 ring-offset-2 border border-primary-300 shadow-lg shadow-primary-100"
                      : "border border-border"
                  }`}
                >
                  <SuperDocEditor
                    key={templateFile.name + templateFile.size}
                    ref={editorRef}
                    document={templateFile}
                    user={EDITOR_USER}
                    isInternal={true}
                    hideToolbar={false}
                    onEditorCreate={() => {
                      injectVariableStyles();
                    }}
                  />
                </div>
              </div>
            </div>
          )}
        </div>

        <VariableMappingDialog
          open={mappingDialogOpen}
          onOpenChange={setMappingDialogOpen}
          variableKey={mappingTarget.key}
          mode={mappingTarget.mode}
          mappingTarget={mappingTarget}
          onSelect={handleReplaceVariable}
        />
        {confirmationDialog}
      </div>
    );
  }

  // Embutido, a lista não existe: o painel da esteira hospeda só o editor (o
  // cadastro abre sozinho no efeito de montagem) e o `null` cobre o instante
  // entre abrir o painel e o modelo em branco ficar pronto.
  if (embedded) return null;

  return (
    <div className="flex flex-col h-full bg-muted p-4 sm:p-6 lg:p-8">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between mb-6 sm:mb-8 gap-4 sm:gap-0">
        <div>
          <h1 className="text-2xl font-bold text-foreground tracking-tight">
            Modelos de Contrato
          </h1>
          <p className="text-sm text-muted-foreground mt-1">
            Gerencie os templates de contratos para a esteira digital.
          </p>
        </div>
        <Button onClick={handleCreateNew} className="w-full sm:w-auto">
          <Plus className="w-4 h-4" /> Novo Modelo
        </Button>
      </div>

      {loading ? (
        <div className="flex justify-center py-12 text-muted-foreground">
          Carregando modelos...
        </div>
      ) : templates.length === 0 ? (
        <EmptyState
          icon={FileText}
          title="Nenhum modelo encontrado"
          message="Crie seu primeiro modelo de contrato para usar na esteira digital."
          action={
            <Button variant="outline" onClick={handleCreateNew}>
              Criar Modelo
            </Button>
          }
          className="bg-card shadow-sm mt-8"
        />
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {(templates || []).map((tpl) => (
            <div
              key={tpl.id}
              className="bg-card rounded-2xl border border-border shadow-sm p-6 flex flex-col hover:shadow-md transition-shadow"
            >
              <div className="flex items-start justify-between mb-4">
                <div className="p-3 bg-primary-50 text-primary-600 rounded-xl">
                  <FileText className="w-6 h-6" />
                </div>
                <div className="flex gap-1">
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => handleDuplicate(tpl)}
                    className="size-9 text-muted-foreground hover:text-primary-600 hover:bg-primary-50"
                    title="Duplicar"
                  >
                    <Copy className="w-4 h-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => handleEdit(tpl)}
                    className="size-9 text-muted-foreground hover:text-status-warning hover:bg-status-warning-soft"
                    title="Editar"
                  >
                    <Edit2 className="w-4 h-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => handleDelete(tpl.id)}
                    className="size-9 text-muted-foreground hover:text-status-critical hover:bg-status-critical-soft"
                    title="Excluir"
                  >
                    <Trash2 className="w-4 h-4" />
                  </Button>
                </div>
              </div>

              <h3 className="text-lg font-bold text-foreground mb-1">{tpl.name}</h3>
              <p className="text-sm text-muted-foreground mb-4 flex-1">
                {tpl.description || "Sem descrição"}
              </p>

              <div className="flex flex-wrap gap-2 mb-4">
                {tpl.variables?.slice(0, 3).map((v) => (
                  <span
                    key={v}
                    className="px-2 py-1 bg-muted text-muted-foreground text-xs rounded font-mono"
                  >
                    {`{{${v}}}`}
                  </span>
                ))}
                {(tpl.variables?.length || 0) > 3 && (
                  <span className="px-2 py-1 bg-muted text-muted-foreground text-xs rounded">
                    +{(tpl.variables?.length || 0) - 3}
                  </span>
                )}
              </div>

              <div className="text-xs text-muted-foreground mt-auto pt-4 border-t border-border">
                Atualizado em {formatUpdatedAt(tpl.updated_at)}
              </div>
            </div>
          ))}
        </div>
      )}
      {confirmationDialog}
    </div>
  );
}
