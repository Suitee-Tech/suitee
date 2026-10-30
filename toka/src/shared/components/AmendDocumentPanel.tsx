import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router";
import {
  Check,
  FileSignature,
  FileText,
  Loader2,
  Maximize2,
  Minimize2,
  Pencil,
  RefreshCw,
  Upload,
} from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";
import { SuperDocEditor, type SuperDocRef } from "@superdoc-dev/react";
import "@superdoc-dev/react/style.css";

import { ContractStatus, type Contract } from "../contracts/types";
import { contractService } from "../services/contract.service";
import { templateService, type ContractTemplate } from "../services/template.service";
import {
  base64ToFile,
  extractDocxTemplateVariables,
  fileToBase64,
  fillDocxTemplate,
} from "../utils/docx";
import { useContractVariableData } from "../hooks/useContractVariableData";
import { PreviewVariableForm } from "./contract-editor/PreviewVariableForm";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "../ui/sheet";
import { Button } from "../ui/button";

/**
 * Painel lateral do aditivo.
 *
 * O fluxo anterior pedia apenas um texto de motivo e criava um aditivo que
 * herdava o documento do contrato original — ou seja, o aditivo não tinha
 * documento próprio, e o que ia para assinatura era o contrato antigo.
 *
 * Aqui o aditivo nasce COM documento, por um de dois caminhos:
 *  - upload de um arquivo .docx
 *  - novo documento criado no editor a partir de um modelo, com as MESMAS
 *    variáveis do editor de contrato da esteira (`fillDocxTemplate` +
 *    `PreviewVariableForm`), para os dois não divergirem.
 *
 * Em ambos o documento é enviado no próprio `amend` (o backend já aceita
 * `content` e o usa como corpo do aditivo) e segue para o fluxo de assinatura
 * da tela do contrato, onde fica o acompanhamento.
 */
interface AmendDocumentPanelProps {
  open: boolean;
  onClose: () => void;
  contract: Contract;
  mode?: "create" | "edit";
  onCompleted?: (amendment: Contract, reviewSignature: boolean) => void;
}

type Source = "upload" | "template" | null;

const MAX_DOCX_BYTES = 10 * 1024 * 1024;

export function AmendDocumentPanel({ open, onClose, contract, mode = "create", onCompleted }: AmendDocumentPanelProps) {
  const editing = mode === "edit";
  const navigate = useNavigate();
  const editorRef = useRef<SuperDocRef>(null);

  // Só carrega o contexto quando o painel está aberto: não faz sentido gastar
  // requisições de negócio, imóvel, partes e corretores com o painel fechado.
  const { data: variableData } = useContractVariableData(open ? contract : null);

  const [source, setSource] = useState<Source>(null);
  const [reason, setReason] = useState("");

  // Upload
  const [uploadName, setUploadName] = useState<string | null>(null);
  const [uploadBase64, setUploadBase64] = useState<string | null>(null);

  // Modelo
  const [templates, setTemplates] = useState<ContractTemplate[]>([]);
  const [loadingTemplates, setLoadingTemplates] = useState(false);
  const [selectedTemplate, setSelectedTemplate] = useState<ContractTemplate | null>(null);
  const [editorFile, setEditorFile] = useState<File | null>(null);
  const [variableFormOpen, setVariableFormOpen] = useState(false);
  const [variableKeys, setVariableKeys] = useState<string[]>([]);
  const [autoFilledData, setAutoFilledData] = useState<Record<string, unknown>>({});
  const [appliedVariables, setAppliedVariables] = useState(false);
  const [isFullScreen, setIsFullScreen] = useState(false);
  const pageRef = useRef<HTMLDivElement>(null);

  const [submitting, setSubmitting] = useState(false);

  const reset = useCallback(() => {
    setSource(null);
    setReason("");
    setUploadName(null);
    setUploadBase64(null);
    setSelectedTemplate(null);
    setEditorFile(null);
    setVariableFormOpen(false);
    setVariableKeys([]);
    setAppliedVariables(false);
    setIsFullScreen(false);
  }, []);

  useEffect(() => {
    if (open && editing) {
      setSource("upload");
      setUploadName("aditivo.docx");
      setUploadBase64(contract.content);
      setEditorFile(base64ToFile(contract.content, "aditivo.docx"));
    }
  }, [open, editing, contract.id, contract.content]);

  const loadTemplates = useCallback(async () => {
    setLoadingTemplates(true);
    try {
      setTemplates(await templateService.list());
    } catch {
      toast.error("Não foi possível carregar os modelos de contrato.");
      setTemplates([]);
    } finally {
      setLoadingTemplates(false);
    }
  }, []);

  const handleFile = async (file: File) => {
    if (!file.name.toLowerCase().endsWith(".docx")) {
      toast.error("Envie um arquivo .docx.");
      return;
    }
    if (file.size > MAX_DOCX_BYTES) {
      toast.error("O arquivo passa de 10 MB.");
      return;
    }
    try {
      const base64 = await fileToBase64(file);
      setUploadName(file.name);
      setUploadBase64(base64);
      setEditorFile(file);
    } catch {
      toast.error("Não foi possível ler o documento. Tente novamente.");
    }
  };

  const handlePickTemplate = (template: ContractTemplate) => {
    setSelectedTemplate(template);
    setAppliedVariables(false);
    setEditorFile(base64ToFile(template.content, `${template.name}.docx`));

    // Mesmas variáveis do editor da esteira: as declaradas no modelo mais as
    // que o próprio documento usa.
    const keys = Array.from(
      new Set([...(template.variables || []), ...extractDocxTemplateVariables(template.content, variableData)]),
    );
    setVariableKeys(keys);
    setAutoFilledData(variableData);
    // Sem variáveis não há o que preencher: vai direto para o documento.
    setVariableFormOpen(keys.length > 0);
    setAppliedVariables(keys.length === 0);
  };

  const handleVariableConfirm = (finalData: Record<string, unknown>) => {
    if (!selectedTemplate) return;
    try {
      const filled = fillDocxTemplate(selectedTemplate.content, finalData);
      setEditorFile(base64ToFile(filled, `Aditivo_${selectedTemplate.name}.docx`));
      setAppliedVariables(true);
      setVariableFormOpen(false);
      toast.success("Variáveis aplicadas ao documento.");
    } catch (error) {
      const detail = (error as { properties?: { errors?: { properties?: { explanation?: string }; message?: string }[] } })
        ?.properties?.errors?.map((item) => item.properties?.explanation || item.message).join(", ");
      toast.error(detail ? `Erro no modelo: ${detail}` : "Erro ao aplicar as variáveis do modelo.");
    }
  };

  const contentFromEditor = async (): Promise<string | null> => {
    const instance = editorRef.current?.getInstance?.();
    if (!instance?.exportEditorsToDOCX) return null;
    const blobs = await instance.exportEditorsToDOCX();
    if (!blobs?.length) return null;
    // Preserva o data URL inteiro: o prefixo `data:...` é o que diz ao renderizador
    // do backend que o conteúdo é um DOCX. Removê-lo fazia o contrato chegar ao
    // signatário como uma página de base64 cru, em vez do contrato.
    return await fileToBase64(blobs[0]);
  };

  const temDocumento =
    (source === "upload" && Boolean(uploadBase64)) || (source === "template" && Boolean(editorFile));

  // Salvar e enviar têm pré-condições DIFERENTES.
  //
  // Salvar corrige o documento e vale em qualquer situação em que o contrato
  // ainda não foi assinado. Enviar exige rascunho sem envio anterior: o backend
  // recusa com 409 quando já existe `signature_locked_at`. Antes as duas usavam
  // a mesma condição, e a tela oferecia "Salvar e revisar assinatura" num
  // contrato que já estava em assinatura — o operador tomava um 409 sem
  // explicação.
  const podeEditar = editing ? contract.status !== ContractStatus.SIGNED && !contract.termination : true;
  const podeEnviar =
    editing && contract.status === ContractStatus.DRAFT && !contract.signature_locked_at;

  const canSubmit =
    podeEditar &&
    (editing || Boolean(reason.trim())) &&
    !variableFormOpen &&
    !submitting &&
    temDocumento;

  const handleSubmit = async (reviewSignature = false) => {
    if (!canSubmit) return;
    setSubmitting(true);
    try {
      const content = await contentFromEditor();
      if (!content) {
        toast.error("Não foi possível obter o documento do aditivo.");
        return;
      }

      const created = editing
        ? await contractService.update(contract.id, { content })
        : await contractService.amend(contract.id, {
            reason: reason.trim(),
            content,
          });
      toast.success(editing ? "Documento salvo." : "Aditivo criado.");
      reset();
      onClose();
      if (!editing) navigate(`/contratos/${created.id}`);
      onCompleted?.(created, reviewSignature);
    } catch (err: unknown) {
      const message = (err as { response?: { data?: { message?: string } } })?.response?.data?.message;
      toast.error(message || (editing ? "Erro ao salvar documento." : "Erro ao criar aditivo."));
    } finally {
      setSubmitting(false);
    }
  };


  /**
   * Impõe ao SuperDoc uma caixa em pixels, medida do espaço disponível.
   *
   * O componente monta o documento inteiro na DOM: o container de edição mede
   * ~40.000px e ignora altura de flex (`flex-1`, `h-full`, `min-h-0` foram
   * testados em cada nível e o conteúdo sempre estourou). Uma altura explícita
   * em pixels, no `.superdoc-layout` — que é o único ancestral com
   * `overflow: auto` — contém o documento, e ele rola dentro da caixa.
   *
   * Verificado isoladamente antes de entrar aqui: com 320px no layout, o editor
   * passou de 40.752px para 320px, manteve o texto renderizado e rolou
   * internamente.
   */
  const [alturaDocumento, setAlturaDocumento] = useState<number | null>(null);

  const medirEspacoDoDocumento = useCallback(() => {
    const raiz = pageRef.current;
    if (!raiz) return;
    // Espaço útil: do topo da área do documento até o fim do que está visível
    // na janela. Descontado o rodapé do painel, que é fixo.
    // Mede do FIM do toolbar do SuperDoc até o fim da janela: é o espaço que
    // sobra de fato para o documento, sem depender de um desconto estimado.
    const toolbar = raiz.querySelector<HTMLElement>(".superdoc-toolbar-container");
    const inicioDocumento = toolbar
      ? toolbar.getBoundingClientRect().bottom
      : raiz.getBoundingClientRect().top;
    const fimDisponivel = window.innerHeight - 16;
    const disponivel = fimDisponivel - inicioDocumento;
    if (disponivel <= 0) return;
    setAlturaDocumento(Math.max(200, Math.round(disponivel)));
  }, []);

  useEffect(() => {
    if (!editorFile || variableFormOpen) return;
    const raiz = pageRef.current;
    if (!raiz) return;

    const timers = [200, 800, 1600].map((ms) => window.setTimeout(medirEspacoDoDocumento, ms));
    const observador =
      typeof ResizeObserver === "undefined" ? null : new ResizeObserver(() => medirEspacoDoDocumento());
    observador?.observe(raiz);
    window.addEventListener("resize", medirEspacoDoDocumento);
    return () => {
      timers.forEach(window.clearTimeout);
      observador?.disconnect();
      window.removeEventListener("resize", medirEspacoDoDocumento);
    };
  }, [editorFile, variableFormOpen, isFullScreen, medirEspacoDoDocumento]);

  // Aplica a caixa no container de rolagem real do SuperDoc. Ele é criado pela
  // biblioteca, fora do nosso JSX, então não há prop do React que o alcance.
  useEffect(() => {
    if (!alturaDocumento) return;
    const layout = pageRef.current?.querySelector<HTMLElement>(".superdoc-layout");
    if (!layout) return;
    layout.style.height = `${alturaDocumento}px`;
    layout.style.maxHeight = `${alturaDocumento}px`;
  }, [alturaDocumento, editorFile, variableFormOpen, isFullScreen]);

  const escolheuDocumento = source === "upload" ? Boolean(uploadBase64) : Boolean(selectedTemplate);
  // Documento aberto no editor: layout de altura cheia e sem scroll externo.
  const hasDocumentOpen = Boolean(editorFile) && !variableFormOpen;
  const steps: { id: string; label: string; done: boolean }[] = [
    { id: "doc", label: "Documento", done: escolheuDocumento },
    { id: "reason", label: "Motivo", done: Boolean(reason.trim()) },
  ];

  return (
    <Sheet
      open={open}
      onOpenChange={(next: boolean) => {
        if (!next && !submitting) {
          reset();
          onClose();
        }
      }}
    >
      <SheetContent
        side="right"
        expandable
        expandedWidth="sm:max-w-5xl"
        className={clsx("w-full gap-0 overflow-hidden p-0 sm:max-w-2xl", isFullScreen && "sm:max-w-none")}
      >
        {/*
          Cabeçalho, conteúdo e rodapé como três faixas do painel. O scroll fica
          só no conteúdo, então o rodapé não desliza por cima dele. `pr-20` abre
          espaço para os controles do Sheet (expandir e fechar), que são
          absolutos e ficariam sobre o título.
        */}
        <div className="flex items-start gap-3 border-b border-border px-6 py-4 pr-20">
          <div className="min-w-0 flex-1">
            <SheetTitle className="flex items-center gap-2 text-xl font-bold text-foreground">
              <FileSignature className="h-5 w-5 shrink-0 text-primary-600" />
              {editing ? "Editar documento" : "Criar aditivo"}
            </SheetTitle>
            <SheetDescription className="mt-1">
              {editing
                ? "Revise o documento e salve as alterações antes de conferir os signatários e enviar para assinatura."
                : "Carregue um documento ou escolha um modelo, revise o texto e salve o aditivo para assinatura."}
            </SheetDescription>
          </div>
        </div>

        <div
          className={clsx(
            "flex min-h-0 flex-1 flex-col gap-4 px-6 py-4",
            // Com o documento aberto o painel vira coluna flex de altura cheia:
            // com `overflow-y-auto` aqui o filho com `flex-1` não cresce (o
            // scroller se dimensiona pelo conteúdo) e o editor ficava preso.
            hasDocumentOpen ? "overflow-hidden" : "overflow-y-auto",
          )}
        >
          {!editing && <ol className="flex items-center gap-3 text-xs">
            {steps.map((step, index) => (
              <li key={step.id} className="flex items-center gap-2">
                <span
                  className={clsx(
                    "flex h-5 w-5 items-center justify-center rounded-full border text-micro font-bold",
                    step.done
                      ? "border-status-ok/30 bg-status-ok-soft text-status-ok"
                      : "border-border bg-muted text-muted-foreground",
                  )}
                >
                  {step.done ? <Check className="h-3 w-3" /> : index + 1}
                </span>
                <span className={clsx(step.done ? "text-foreground" : "text-muted-foreground")}>{step.label}</span>
                {index < steps.length - 1 && <span className="text-muted-foreground/40">›</span>}
              </li>
            ))}
          </ol>}

          {!source && (
            <div className="grid gap-3">
              <button
                type="button"
                onClick={() => setSource("upload")}
                className="flex items-center gap-3 rounded-xl border border-border bg-card p-4 text-left transition-colors hover:border-primary-300 hover:bg-muted"
              >
                <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary-50">
                  <Upload className="h-4 w-4 text-primary-600" />
                </span>
                <span className="min-w-0">
                  <span className="block text-sm font-bold text-foreground">Carregar documento</span>
                  <span className="block text-xs leading-relaxed text-muted-foreground">
                    Envie o aditivo já pronto em .docx.
                  </span>
                </span>
              </button>
              <button
                type="button"
                onClick={() => {
                  setSource("template");
                  if (templates.length === 0) void loadTemplates();
                }}
                className="flex items-center gap-3 rounded-xl border border-border bg-card p-4 text-left transition-colors hover:border-primary-300 hover:bg-muted"
              >
                <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary-50">
                  <Pencil className="h-4 w-4 text-primary-600" />
                </span>
                <span className="min-w-0">
                  <span className="block text-sm font-bold text-foreground">Criar a partir de modelo</span>
                  <span className="block text-xs leading-relaxed text-muted-foreground">
                    Escolha um modelo, preencha as variáveis e edite o documento.
                  </span>
                </span>
              </button>
            </div>
          )}

          {source === "upload" && !editing && (
            <section className="space-y-3">
              <div className="flex items-center justify-between">
                <h3 className="text-xs font-bold uppercase tracking-wider text-foreground">Documento do aditivo *</h3>
                <button
                  type="button"
                  onClick={() => {
                    setSource(null);
                    setUploadName(null);
                    setUploadBase64(null);
                    setEditorFile(null);
                  }}
                  className="text-xs font-medium text-primary-600 hover:underline"
                >
                  Trocar forma
                </button>
              </div>
              <label className="flex cursor-pointer flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-border bg-muted p-10 text-center transition-colors hover:border-primary-300 hover:bg-card">
                <Upload className="h-6 w-6 text-muted-foreground" />
                <span className="text-sm font-medium text-foreground">
                  {uploadName ?? "Clique para escolher um arquivo .docx"}
                </span>
                <span className="text-xs text-muted-foreground">Até 10 MB</span>
                <input
                  type="file"
                  className="hidden"
                  onChange={(event) => {
                    const file = event.target.files?.[0];
                    if (file) void handleFile(file);
                  }}
                />
              </label>
              {uploadName && (
                <p className="flex items-center gap-2 text-xs font-medium text-status-ok">
                  <FileText className="h-3.5 w-3.5" />
                  {uploadName} pronto para envio
                </p>
              )}
            </section>
          )}

          {source === "template" && (
            <section className="space-y-4">
              <div className="flex items-center justify-between">
                <h3 className="text-xs font-bold uppercase tracking-wider text-foreground">Modelo *</h3>
                <button
                  type="button"
                  onClick={() => {
                    setSource(null);
                    setEditorFile(null);
                    setSelectedTemplate(null);
                    setVariableKeys([]);
                    setAppliedVariables(false);
                    setVariableFormOpen(false);
                  }}
                  className="text-xs font-medium text-primary-600 hover:underline"
                >
                  Trocar forma
                </button>
              </div>

              {loadingTemplates ? (
                <p className="flex items-center gap-2 text-sm text-muted-foreground">
                  <Loader2 className="h-4 w-4 animate-spin" /> Carregando modelos…
                </p>
              ) : templates.length === 0 ? (
                <p className="rounded-xl border border-border bg-muted p-4 text-sm text-muted-foreground">
                  Nenhum modelo cadastrado. Cadastre em <strong>Modelos de Contrato</strong> ou use o carregamento de
                  documento.
                </p>
              ) : (
                <select
                  value={selectedTemplate?.id ?? ""}
                  onChange={(event) => {
                    const template = templates.find((item) => item.id === event.target.value);
                    if (template) handlePickTemplate(template);
                  }}
                  className="h-10 w-full rounded-lg border border-border bg-card px-3 text-sm outline-none focus:ring-2 focus:ring-primary-500"
                >
                  <option value="">Selecione um modelo…</option>
                  {templates.map((template) => (
                    <option key={template.id} value={template.id}>
                      {template.name}
                    </option>
                  ))}
                </select>
              )}

              {editorFile && variableFormOpen && (
                /*
                  Sem contêiner com scroll próprio: `PreviewVariableForm` já limita
                  a altura e rola internamente. Aninhar produzia duas barras de
                  rolagem na mesma área.
                */
                <PreviewVariableForm
                  templateVariables={variableKeys}
                  autoFilledData={autoFilledData}
                  onConfirm={handleVariableConfirm}
                  onCancel={() => setVariableFormOpen(false)}
                  // O aditivo vai para assinatura eletrônica: variável sem valor
                  // fica vazia, não com a linha de preenchimento.
                  emptyValue=""
                />
              )}

            </section>
          )}

          {editorFile && !variableFormOpen && (
            <div
              className={clsx(
                "flex flex-col overflow-hidden rounded-xl border border-border bg-card",
                // Sem tela cheia o bloco ocupa a altura que sobra do painel — é o
                // que dá área útil ao documento. Em tela cheia, a tela inteira.
                isFullScreen ? "fixed inset-4 z-[60]" : "flex-1",
              )}
            >
              <div className="flex items-center justify-between gap-2 border-b border-border bg-muted px-3 py-2">
                <span className="truncate text-xs text-muted-foreground">
                  {appliedVariables
                    ? "Documento com as variáveis aplicadas — edite antes de enviar"
                    : "Edite o documento antes de enviar"}
                </span>
                <div className="flex shrink-0 items-center gap-1">
                  {variableKeys.length > 0 && (
                    <Button variant="ghost" size="sm" className="gap-1 text-xs" onClick={() => setVariableFormOpen(true)}>
                      <RefreshCw className="h-3.5 w-3.5" />
                      Variáveis
                    </Button>
                  )}
                  <Button
                    variant="ghost"
                    size="sm"
                    className="gap-1 text-xs"
                    onClick={() => setIsFullScreen((value) => !value)}
                  >
                    {isFullScreen ? <Minimize2 className="h-3.5 w-3.5" /> : <Maximize2 className="h-3.5 w-3.5" />}
                    {isFullScreen ? "Sair" : "Tela cheia"}
                  </Button>
                </div>
              </div>
              <div
                ref={pageRef}
                className={clsx(
                  "flex min-h-0 justify-center bg-[#f3f3f3]",
                  isFullScreen ? "flex-1 overflow-y-auto py-6" : "flex-1 overflow-hidden",
                )}
              >
                <div className={clsx("h-full min-w-0", isFullScreen && "w-full max-w-[850px] bg-card shadow-xl")}>
                  <div className="h-full overflow-y-auto">
                    <SuperDocEditor
                      ref={editorRef}
                      document={editorFile}
                      user={{ name: "Operador", email: "operador@suitee.com" }}
                      isInternal
                      hideToolbar={false}
                    />
                  </div>
                </div>
              </div>
            </div>
          )}

          {!editing && <section>
            <label htmlFor="amend-reason" className="mb-1.5 block text-xs font-bold uppercase tracking-wider text-foreground">
              Motivo do aditivo *
            </label>
            <textarea
              id="amend-reason"
              value={reason}
              onChange={(event) => setReason(event.target.value)}
              rows={2}
              placeholder="Ex.: prorrogação de prazo por 12 meses; mudança de fiador"
              className="w-full resize-none rounded-lg border border-border bg-muted px-3 py-2 text-sm outline-none focus:bg-card focus:ring-2 focus:ring-primary-500"
            />
          </section>}

          <p className="rounded-lg border border-border bg-muted/60 px-3 py-2 text-[11px] leading-snug text-muted-foreground">
            Os signatários do contrato atual são copiados para o aditivo e precisam assinar novamente. Ao ser
            assinado, o contrato original passa para <em>expirado</em> e o aditivo se torna o vigente. O
            acompanhamento fica na aba <strong>Assinatura</strong> do aditivo.
          </p>
        </div>

        {editing && !podeEnviar && contract.status !== ContractStatus.DRAFT && (
          <p className="border-t border-border bg-muted/30 px-6 pt-3 text-[11px] text-muted-foreground">
            Este contrato não está em rascunho, então não é possível enviar para assinatura por aqui.
            {contract.signature_locked_at ? " O envio já foi feito — acompanhe na aba Assinatura." : ""}
          </p>
        )}
        <div className="flex flex-wrap items-center justify-end gap-3 border-t border-border bg-muted/30 px-6 py-4">
          <Button
            variant="outline"
            onClick={() => {
              reset();
              onClose();
            }}
            disabled={submitting}
          >
            Cancelar
          </Button>
          <Button variant="outline" onClick={() => void handleSubmit()} disabled={!canSubmit}>
            {submitting ? <Loader2 className="h-4 w-4 animate-spin" /> : <FileSignature className="h-4 w-4" />}
            {editing ? "Salvar alterações" : "Criar aditivo"}
          </Button>
          {podeEnviar && (
            <Button onClick={() => void handleSubmit(true)} disabled={!canSubmit}>
              Salvar e revisar assinatura
            </Button>
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}
