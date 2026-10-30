import React, { useState, useMemo, useRef, useEffect, useCallback } from "react";
import {
 Plus,
 Upload,
 FileText,
 File,
 Eye,
 Download,
 Trash2,
 X,
 Search,
 CheckCircle2,
 Clock,
 AlertCircle,
 PenTool,
 Send,
 Users,
 Calendar,
 ChevronDown,
 MoreHorizontal,
 Shield,
 FilePlus,
 FolderOpen,
 ClipboardList,
 ArrowUpRight,
 Mail,
 UserPlus,
 Eraser,
 Check,
 ZoomIn,
 ZoomOut,
 History,
 ChevronsUpDown,
 Loader2,
 User,
} from "lucide-react";
import { clsx } from "clsx";
import { motion, AnimatePresence } from "motion/react";
import { toast } from "sonner";
import {
  type Contract,
} from "../contracts/types";
import type { ContractDocument, DocumentCategory, DocumentStatus, DocumentSigner } from "../types/contractDocument";
import { useDocumentsContext } from "../context/DocumentsContext";
import { useConfirmAction } from "../hooks/useConfirmAction";
import { SignaturePad, formatFileSize } from "./DocumentsTabSignaturePad";
import { AuditTrailPanel } from "./AuditTrailPanel";
import { ExpandableSidePanel } from "./ExpandableSidePanel";
import { Button } from "../ui/button";
import {
 Dialog,
 DialogContent,
 DialogDescription,
 DialogTitle,
} from "../ui/dialog";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/popover";
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList, CommandSeparator } from "../ui/command";
import { leadService, ApiLead } from "../services/lead.service";
import { formatPhoneBR } from "../utils/masks";
import { CATEGORY_OPTIONS, getCategoryLabel, getCategoryMeta, getStatusMeta } from "../lib/documents";
import { useAuditLogContext } from "../context/AuditLogContext";


// Categories surfaced in the upload dropdown — uses the canonical option
// list from lib/documents.ts so any new backend category shows up here
// automatically.
const UPLOAD_CATEGORIES = CATEGORY_OPTIONS;


// ========================
interface DocumentsTabProps {
 contract: Contract;
}

export function DocumentsTab({ contract }: DocumentsTabProps) {
 const { confirmAction, confirmationDialog } = useConfirmAction();
 const { documents, addDocument, updateDocument, deleteDocument } = useDocumentsContext();
 const { getAuditLog } = useAuditLogContext();
 const [filter, setFilter] = useState<string>("Todos");
 const [searchTerm, setSearchTerm] = useState("");
 const [showUploadModal, setShowUploadModal] = useState(false);
 const [showSignModalId, setShowSignModalId] = useState<string | null>(null);
 const [documentMutationPending, setDocumentMutationPending] = useState(false);
 const [showDocDetailId, setShowDocDetailId] = useState<string | null>(null);
 const [isDragging, setIsDragging] = useState(false);
 const [auditDocId, setAuditDocId] = useState<string | null>(null);
 const [auditDocLabel, setAuditDocLabel] = useState("");

 // Upload form state
 const [uploadForm, setUploadForm] = useState({
 name: "",
 category: "contrato" as DocumentCategory,
 requireSignatures: false,
 signerName: "",
 signerEmail: "",
 signerRole: "Inquilino",
 signers: [] as Omit<DocumentSigner, "id" | "status" | "signedAt">[],
 notes: "",
 });
 const [uploadedFileName, setUploadedFileName] = useState("");
 const [uploadedFileSize, setUploadedFileSize] = useState(0);
 const fileInputRef = useRef<HTMLInputElement>(null);

 // Lead Combobox state
 const [searchLeadTerm, setSearchLeadTerm] = useState('');
 const [suggestedLeads, setSuggestedLeads] = useState<ApiLead[]>([]);
 const [searchingLeads, setSearchingLeads] = useState(false);
 const [selectedLead, setSelectedLead] = useState<ApiLead | null>(null);
 const [leadComboOpen, setLeadComboOpen] = useState(false);
 const [manualEntryMode, setManualEntryMode] = useState(false);

 useEffect(() => {
 if (!leadComboOpen) return;

 if (!searchLeadTerm || searchLeadTerm.length < 2) {
 if (suggestedLeads.length === 0) {
 setSearchingLeads(true);
 leadService.getAll({ limit: 10 })
 .then(data => setSuggestedLeads(data.data || []))
 .catch(() => {})
 .finally(() => setSearchingLeads(false));
 }
 return;
 }

 const timer = setTimeout(() => {
 setSearchingLeads(true);
 leadService.getAll({ search: searchLeadTerm, limit: 10 })
 .then(data => setSuggestedLeads(data.data || []))
 .catch(() => {})
 .finally(() => setSearchingLeads(false));
 }, 300);

 return () => clearTimeout(timer);
 }, [searchLeadTerm, leadComboOpen]);

 const mapLeadRoleToSignerRole = (role: string): string | null => {
 const roleLower = role?.toLowerCase() || '';
 if (roleLower === 'buyer') return 'Comprador';
 if (roleLower === 'seller') return 'Vendedor';
 if (roleLower === 'tenant') return 'Inquilino';
 if (roleLower === 'landlord') return 'Proprietário';
 if (roleLower === 'owner') return 'Locador';
 if (roleLower === 'guarantor') return 'Fiador';
 return null;
 };

 const translateLeadRole = (role: string) => {
 const map: Record<string, string> = {
 buyer: 'Comprador',
 seller: 'Vendedor',
 tenant: 'Inquilino',
 landlord: 'Proprietário',
 owner: 'Locador',
 guarantor: 'Fiador'
 };
 return map[role?.toLowerCase()] || role || 'Desconhecido';
 };

 const leadRoleToBadgeColor = (role: string) => {
 const r = role?.toLowerCase();
 if (r === 'buyer') return 'bg-status-info-soft text-status-info';
 if (r === 'seller') return 'bg-stage-contract/15 text-stage-contract';
 if (r === 'tenant') return 'bg-status-ok-soft text-status-ok';
 if (r === 'landlord' || r === 'owner') return 'bg-status-warning-soft text-status-warning';
 if (r === 'guarantor') return 'bg-status-critical-soft text-status-critical';
 return 'bg-muted text-foreground';
 };

 const handleSelectLead = (lead: ApiLead) => {
 setSelectedLead(lead);
 setUploadForm(prev => ({
 ...prev,
 signerName: lead.name,
 signerEmail: lead.email,
 signerRole: mapLeadRoleToSignerRole(lead.role || '')?.toLowerCase() || 'Inquilino'
 }));
 setLeadComboOpen(false);
 };

 const handleClearSelection = () => {
 setSelectedLead(null);
 setUploadForm(prev => ({
 ...prev,
 signerName: "",
 signerEmail: ""
 }));
 setManualEntryMode(false);
 };

 const handleEnterManualMode = () => {
 setManualEntryMode(true);
 setSelectedLead(null);
 setLeadComboOpen(false);
 };

 const contractDocs = useMemo(
 () =>
 documents
 .filter((d) => d.contractId === contract.id)
 .sort((a, b) => {
 // Legacy "pendente" sorts next to pendente_assinatura since they
 // collapse to the same bucket in the UI (see normalizeBackendStatus
 // in lib/documents.ts).
 const statusOrder: Record<DocumentStatus, number> = {
 pendente: 0,
 pendente_assinatura: 0,
 parcialmente_assinado: 1,
 rascunho: 2,
 assinado: 3,
 expirado: 4,
 cancelado: 5,
 };
 return statusOrder[a.status] - statusOrder[b.status];
 }),
 [documents, contract.id]
 );

 const filteredDocs = useMemo(() => {
 let docs = contractDocs;
 if (filter !== "Todos") {
 docs = docs.filter(
 (d) =>
 getCategoryLabel(d.category) === filter ||
 getStatusMeta(d.status).label === filter
 );
 }
 if (searchTerm) {
 docs = docs.filter((d) =>
 d.name.toLowerCase().includes(searchTerm.toLowerCase())
 );
 }
 return docs;
 }, [contractDocs, filter, searchTerm]);

 const stats = useMemo(
 () => ({
 total: contractDocs.length,
 signed: contractDocs.filter((d) => d.status === "assinado").length,
 pending: contractDocs.filter(
 (d) =>
 d.status === "pendente_assinatura" ||
 d.status === "parcialmente_assinado"
 ).length,
 draft: contractDocs.filter((d) => d.status === "rascunho").length,
 }),
 [contractDocs]
 );

 // --- File Upload Handlers ---
 const handleFilePick = (e: React.ChangeEvent<HTMLInputElement>) => {
 const file = e.target.files?.[0];
 if (!file) return;
 setUploadedFileName(file.name);
 setUploadedFileSize(file.size);
 if (!uploadForm.name) {
 setUploadForm((prev) => ({
 ...prev,
 name: file.name.replace(/\.[^/.]+$/, ""),
 }));
 }
 };

 const handleDragOver = (e: React.DragEvent) => {
 e.preventDefault();
 setIsDragging(true);
 };

 const handleDragLeave = () => setIsDragging(false);

 const handleDrop = (e: React.DragEvent) => {
 e.preventDefault();
 setIsDragging(false);
 const file = e.dataTransfer.files[0];
 if (!file) return;
 setUploadedFileName(file.name);
 setUploadedFileSize(file.size);
 if (!uploadForm.name) {
 setUploadForm((prev) => ({
 ...prev,
 name: file.name.replace(/\.[^/.]+$/, ""),
 }));
 }
 };

 const addSigner = () => {
 if (!uploadForm.signerName.trim() || !uploadForm.signerEmail.trim()) return;
 setUploadForm((prev) => ({
 ...prev,
 signers: [
 ...prev.signers,
 {
 name: prev.signerName,
 email: prev.signerEmail,
 role: prev.signerRole,
 },
 ],
 signerName: "",
 signerEmail: "",
 signerRole: "Inquilino",
 }));
 };

 const removeSigner = (index: number) => {
 setUploadForm((prev) => ({
 ...prev,
 signers: prev.signers.filter((_, i) => i !== index),
 }));
 };

 const handleUploadSubmit = async () => {
 if (!uploadedFileName || !uploadForm.name.trim()) return;

 const ext = uploadedFileName.split(".").pop()?.toUpperCase() || "PDF";
 const signers: DocumentSigner[] = uploadForm.signers.map((s, i) => ({
 id: `S-NEW-${Date.now()}-${i}`,
 name: s.name,
 email: s.email,
 role: s.role,
 status: "pendente" as const,
 }));

 const newDoc: ContractDocument = {
 id: `DOC-${contract.id.replace("CTR-", "")}-${Date.now()}`,
 contractId: contract.id,
 name: uploadForm.name,
 category: uploadForm.category,
 fileType: ext,
 fileSize: uploadedFileSize,
 uploadDate: "12/03/2026",
 uploadedBy: "Joao Corretor",
 status:
 signers.length > 0
 ? "pendente_assinatura"
 : uploadForm.notes
 ? "rascunho"
 : "assinado",
 signers,
 notes: uploadForm.notes || undefined,
 };

 setDocumentMutationPending(true);
 try {
  const applied = await addDocument({
  ...newDoc,
  file_type: newDoc.fileType,
  });
  if (!applied) return;
  resetUploadForm();
  setShowUploadModal(false);
 } catch {
  // The context already surfaced the API error; keep the form open.
 } finally {
  setDocumentMutationPending(false);
 }
 };

 const resetUploadForm = () => {
 setUploadForm({
 name: "",
 category: "contrato",
 requireSignatures: false,
 signerName: "",
 signerEmail: "",
 signerRole: "Inquilino",
 signers: [],
 notes: "",
 });
 setUploadedFileName("");
 setUploadedFileSize(0);
 };

 // --- Signature Handler ---
 const handleSign = async (docId: string, signerId: string) => {
 const doc = documents.find((d) => d.id === docId);
 if (!doc) return;

 const updatedSigners = doc.signers.map((s) =>
 s.id === signerId
 ? { ...s, status: "assinado" as const, signedAt: "12/03/2026 " + new Date().toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit" }) }
 : s
 );

 const allSigned = updatedSigners.every((s) => s.status === "assinado");
 const someSigned = updatedSigners.some((s) => s.status === "assinado");

 setDocumentMutationPending(true);
 try {
  const applied = await updateDocument(docId, {
  signers: updatedSigners,
  status: allSigned
  ? "assinado"
  : someSigned
  ? "parcialmente_assinado"
  : "pendente_assinatura",
  });
  if (!applied) return;

  setShowSignModal(null);
  setShowDocDetail(null);
 } catch {
  // Keep the signature flow open so the user can retry.
 } finally {
  setDocumentMutationPending(false);
 }
 };

 // --- Send Reminder ---
 const handleSendReminder = (doc: ContractDocument) => {
 const pending = doc.signers.filter((s) => s.status === "pendente");
 if (pending.length > 0) {
 const names = pending.map((s) => s.name).join(", ");
 toast.success(`Lembrete enviado com sucesso`, {
 description: `Notificação enviada para: ${names}`,
 duration: 4000,
 });
 } else {
 toast.info("Nenhum signatario pendente para notificar.");
 }
 };

 // --- PDF Preview State ---
 const [showPdfPreviewId, setShowPdfPreviewId] = useState<string | null>(null);
 const [pdfZoom, setPdfZoom] = useState(100);
 const [pdfPage, setPdfPage] = useState(1);
 const showSignModal = useMemo(
 () => documents.find((document) => document.id === showSignModalId) ?? null,
 [documents, showSignModalId],
 );
 const showDocDetail = useMemo(
 () => documents.find((document) => document.id === showDocDetailId) ?? null,
 [documents, showDocDetailId],
 );
 const showPdfPreview = useMemo(
 () => documents.find((document) => document.id === showPdfPreviewId) ?? null,
 [documents, showPdfPreviewId],
 );
 const setShowSignModal = (document: ContractDocument | null) => setShowSignModalId(document?.id ?? null);
 const setShowDocDetail = (document: ContractDocument | null) => setShowDocDetailId(document?.id ?? null);
 const setShowPdfPreview = (document: ContractDocument | null) => setShowPdfPreviewId(document?.id ?? null);

 // ======== UPLOAD MODAL ========
 const handleCloseUpload = () => {
 if (documentMutationPending) return;
 setShowUploadModal(false);
 resetUploadForm();
 };
 const renderUploadModal = () => (
 <ExpandableSidePanel
 open={showUploadModal}
 onClose={handleCloseUpload}
 defaultSize="xl"
 expandedSize="full"
 bgClass="bg-muted"
 renderHeader={({ expandButton, closeButton }) => (
 <div className="bg-card px-6 py-4 border-b border-border flex items-center justify-between flex-shrink-0">
 <div className="flex items-center gap-3">
 <div className="p-2 bg-primary-50 text-primary-700 rounded-lg">
 <Upload className="w-5 h-5" />
 </div>
 <div>
 <h2 className="text-lg font-bold text-foreground">
 Upload de Documento
 </h2>
 <p className="text-sm text-muted-foreground">
 {contract.id} - {contract.client}
 </p>
 </div>
 </div>
 <div className="flex items-center gap-2">
 {expandButton}
 {closeButton}
 </div>
 </div>
 )}
 >

 {/* Content */}
 <div className="flex-1 overflow-y-auto p-6 space-y-6">
 {/* Dropzone */}
 <div
 onDragOver={handleDragOver}
 onDragLeave={handleDragLeave}
 onDrop={handleDrop}
 onClick={() => fileInputRef.current?.click()}
 className={clsx(
 "border-2 border-dashed rounded-xl p-8 text-center cursor-pointer transition-all",
 isDragging
 ? "border-primary-400 bg-primary-50"
 : uploadedFileName
 ? "border-status-ok/40 bg-status-ok-soft"
 : "border-border bg-card hover:border-border hover:bg-muted"
 )}
 >
 <input
 ref={fileInputRef}
 type="file"
 accept=".pdf,.doc,.docx,.jpg,.jpeg,.png"
 onChange={handleFilePick}
 className="hidden"
 />
 {uploadedFileName ? (
 <div className="flex flex-col items-center gap-2">
 <div className="w-12 h-12 rounded-xl bg-status-ok-soft flex items-center justify-center">
 <CheckCircle2 className="w-6 h-6 text-status-ok" />
 </div>
 <p className="text-sm font-bold text-foreground">
 {uploadedFileName}
 </p>
 <p className="text-xs text-muted-foreground">
 {formatFileSize(uploadedFileSize)}
 </p>
 <Button
 variant="ghost"
 size="sm"
 onClick={(e) => {
 e.stopPropagation();
 setUploadedFileName("");
 setUploadedFileSize(0);
 }}
 className="mt-1 h-auto px-2 py-1 text-xs font-medium text-status-critical hover:text-status-critical"
 >
 Remover arquivo
 </Button>
 </div>
 ) : (
 <div className="flex flex-col items-center gap-3">
 <div className="w-14 h-14 rounded-xl bg-muted flex items-center justify-center">
 <Upload className="w-7 h-7 text-muted-foreground" />
 </div>
 <div>
 <p className="text-sm font-bold text-foreground">
 Arraste o arquivo aqui ou clique para selecionar
 </p>
 <p className="text-xs text-muted-foreground mt-1">
 PDF, DOC, DOCX, JPG, PNG (max. 25MB)
 </p>
 </div>
 </div>
 )}
 </div>

 {/* Document Name */}
 <div>
 <label className="block text-sm font-bold text-foreground mb-1.5">
 Nome do Documento *
 </label>
 <input
 type="text"
 value={uploadForm.name}
 onChange={(e) =>
 setUploadForm((prev) => ({ ...prev, name: e.target.value }))
 }
 placeholder="Ex: Contrato de Locação Residencial"
 className="w-full px-4 py-2.5 bg-card border border-border rounded-xl text-sm outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 transition-all"
 />
 </div>

 {/* Category */}
 <div>
 <label className="block text-sm font-bold text-foreground mb-2">
 Categoria *
 </label>
 <div className="grid grid-cols-4 gap-2">
 {UPLOAD_CATEGORIES.map((cat) => (
 <button
 key={cat.value}
 type="button"
 onClick={() =>
 setUploadForm((prev) => ({
 ...prev,
 category: cat.value,
 }))
 }
 className={clsx(
 "px-3 py-2 rounded-xl border text-xs font-medium transition-all text-center",
 uploadForm.category === cat.value
 ? "bg-primary-50 border-primary-300 text-primary-700 ring-2 ring-primary-600/20"
 : "bg-card border-border text-muted-foreground hover:border-border"
 )}
 >
 {cat.label}
 </button>
 ))}
 </div>
 </div>

 {/* Require Signatures Toggle */}
 <div className="bg-card p-4 rounded-xl border border-border">
 <div className="flex items-center justify-between">
 <div className="flex items-center gap-3">
 <PenTool className="w-5 h-5 text-primary-600" />
 <div>
 <p className="text-sm font-bold text-foreground">
 Assinatura Digital
 </p>
 <p className="text-xs text-muted-foreground">
 Solicitar assinatura dos envolvidos
 </p>
 </div>
 </div>
 <button
 onClick={() =>
 setUploadForm((prev) => ({
 ...prev,
 requireSignatures: !prev.requireSignatures,
 }))
 }
 className={clsx(
 "w-11 h-6 rounded-full transition-colors relative",
 uploadForm.requireSignatures
 ? "bg-primary-600"
 : "bg-muted"
 )}
 >
 <div
 className={clsx(
 "absolute top-0.5 w-5 h-5 rounded-full bg-card shadow transition-transform",
 uploadForm.requireSignatures
 ? "translate-x-[22px]"
 : "translate-x-0.5"
 )}
 />
 </button>
 </div>

 {/* Signers */}
 {uploadForm.requireSignatures && (
 <motion.div
 initial={{ opacity: 0, height: 0 }}
 animate={{ opacity: 1, height: "auto" }}
 className="mt-4 space-y-3 border-t border-border pt-4"
 >
 {/* Added signers */}
 {uploadForm.signers.map((s, i) => (
 <div
 key={i}
 className="flex items-center justify-between bg-muted px-3 py-2 rounded-lg"
 >
 <div className="flex items-center gap-2">
 <div className="w-7 h-7 rounded-full bg-primary-100 flex items-center justify-center text-primary-700">
 <Users className="w-3.5 h-3.5" />
 </div>
 <div>
 <p className="text-xs font-bold text-foreground">
 {s.name}
 </p>
 <p className="text-micro text-muted-foreground">
 {s.email} - {s.role}
 </p>
 </div>
 </div>
 <Button
 variant="ghost"
 size="icon"
 onClick={() => removeSigner(i)}
 className="size-7 text-status-critical hover:text-status-critical"
 >
 <X className="w-3.5 h-3.5" />
 </Button>
 </div>
 ))}

 {/* Add signer form */}
 <div className="space-y-3 mt-2 border-t border-border pt-3">
 {!manualEntryMode ? (
 <div className="flex gap-2">
 <Popover open={leadComboOpen} onOpenChange={(open) => {
 setLeadComboOpen(open);
 if (open) {
 setSearchLeadTerm('');
 setSuggestedLeads([]);
 }
 }}>
 <PopoverTrigger asChild>
 <button
 role="combobox"
 aria-expanded={leadComboOpen}
 className="flex-1 flex items-center justify-between px-3 py-2 bg-card border border-border rounded-lg text-sm hover:bg-muted focus:outline-none focus:ring-2 focus:ring-primary-500 min-h-[38px]"
 >
 {selectedLead ? (
 <div className="flex items-center gap-2 truncate">
 <User className="w-4 h-4 text-muted-foreground shrink-0" />
 <span className="font-medium text-foreground truncate">{selectedLead.name}</span>
 <span className="text-xs text-muted-foreground truncate">{selectedLead.email}</span>
 </div>
 ) : (
 <span className="text-muted-foreground">Buscar lead ou cliente...</span>
 )}
 <ChevronsUpDown className="w-4 h-4 text-muted-foreground shrink-0 ml-2" />
 </button>
 </PopoverTrigger>
 <PopoverContent className="w-[var(--radix-popover-trigger-width)] p-0 bg-card border border-border shadow-xl rounded-lg z-50" align="start">
 <Command shouldFilter={false}>
 <CommandInput
 placeholder="Buscar por nome ou email..."
 onValueChange={setSearchLeadTerm}
 />
 <CommandList>
 {searchingLeads && (
 <div className="flex items-center justify-center py-4">
 <Loader2 className="w-4 h-4 text-primary-500 animate-spin" />
 </div>
 )}
 {!searchingLeads && suggestedLeads.length === 0 && searchLeadTerm.length > 1 && (
 <CommandEmpty>Nenhum contato encontrado.</CommandEmpty>
 )}
 {!searchingLeads && suggestedLeads.length === 0 && searchLeadTerm.length <= 1 && (
 <CommandEmpty>Digite para buscar...</CommandEmpty>
 )}
 {suggestedLeads.length > 0 && (
 <CommandGroup heading="Leads / Clientes">
 {suggestedLeads.map(lead => (
 <CommandItem
 key={lead.id}
 value={lead.id}
 onSelect={() => handleSelectLead(lead)}
 className="flex items-center justify-between cursor-pointer"
 >
 <div className="flex items-center gap-2 truncate">
 <Check className={clsx("w-4 h-4 shrink-0", selectedLead?.id === lead.id ? "opacity-100 text-primary-600" : "opacity-0")} />
 <div className="truncate">
 <p className="font-medium text-foreground truncate text-sm">{lead.name}</p>
 <p className="text-xs text-muted-foreground truncate">{lead.email}{lead.phone ? ` · ${formatPhoneBR(lead.phone)}` : ''}</p>
 </div>
 </div>
 <div className="flex items-center gap-1.5 shrink-0 ml-2">
 <span className={`text-micro px-1.5 py-0.5 rounded font-bold ${leadRoleToBadgeColor(lead.role || '')?.toLowerCase() || 'inquilino'}`}>
 {translateLeadRole(lead.role || '')?.toLowerCase() || 'inquilino'}
 </span>
 </div>
 </CommandItem>
 ))}
 </CommandGroup>
 )}
 <CommandSeparator />
 <CommandGroup>
 <CommandItem
 onSelect={handleEnterManualMode}
 className="cursor-pointer text-primary-600"
 >
 <UserPlus className="w-4 h-4 mr-2" />
 <span className="font-medium">Destinatário não cadastrado</span>
 </CommandItem>
 </CommandGroup>
 </CommandList>
 </Command>
 </PopoverContent>
 </Popover>
 {selectedLead && (
 <Button
 type="button"
 variant="ghost"
 size="icon"
 onClick={handleClearSelection}
 className="size-9 text-muted-foreground"
 title="Limpar seleção"
 >
 <X className="w-4 h-4" />
 </Button>
 )}
 </div>
 ) : (
 <div className="space-y-2">
 <div className="flex items-center justify-between">
 <span className="text-xs font-bold text-muted-foreground uppercase">Preenchimento Manual</span>
 <Button
 type="button"
 variant="link"
 size="sm"
 onClick={() => setManualEntryMode(false)}
 className="h-auto p-0 text-xs font-bold text-primary"
 >
 Buscar existente
 </Button>
 </div>
 <div className="grid grid-cols-2 gap-2">
 <input
 type="text"
 value={uploadForm.signerName}
 onChange={(e) =>
 setUploadForm((prev) => ({
 ...prev,
 signerName: e.target.value,
 }))
 }
 placeholder="Nome"
 className="px-3 py-2 bg-card border border-border rounded-lg text-sm outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600"
 />
 <input
 type="email"
 value={uploadForm.signerEmail}
 onChange={(e) =>
 setUploadForm((prev) => ({
 ...prev,
 signerEmail: e.target.value,
 }))
 }
 placeholder="Email"
 className="px-3 py-2 bg-card border border-border rounded-lg text-sm outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600"
 />
 </div>
 </div>
 )}

 <div className="flex items-center gap-2">
 <div className="relative flex-1">
 <select
 value={uploadForm.signerRole}
 onChange={(e) =>
 setUploadForm((prev) => ({
 ...prev,
 signerRole: e.target.value,
 }))
 }
 className="w-full px-3 py-2 bg-card border border-border rounded-lg text-sm outline-none appearance-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600"
 >
 <option>Inquilino</option>
 <option>Proprietário</option>
 <option>Locador</option>
 <option>Locatário</option>
 <option>Comprador</option>
 <option>Vendedor</option>
 <option>Testemunha</option>
 <option>Fiador</option>
 <option>Avalista</option>
 <option>Representante Legal</option>
 </select>
 <ChevronDown className="w-4 h-4 text-muted-foreground absolute right-2 top-1/2 -translate-y-1/2 pointer-events-none" />
 </div>
 <Button
 type="button"
 onClick={() => {
 addSigner();
 handleClearSelection();
 }}
 >
 <UserPlus className="w-4 h-4" /> Adicionar
 </Button>
 </div>
 </div>
 </motion.div>
 )}
 </div>

 {/* Notes */}
 <div>
 <label className="block text-sm font-bold text-foreground mb-1.5">
 Observacoes{" "}
 <span className="text-muted-foreground font-normal">(opcional)</span>
 </label>
 <textarea
 value={uploadForm.notes}
 onChange={(e) =>
 setUploadForm((prev) => ({
 ...prev,
 notes: e.target.value,
 }))
 }
 rows={3}
 placeholder="Notas internas sobre este documento..."
 className="w-full px-4 py-3 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none resize-none transition-all"
 />
 </div>
 </div>

 {/* Footer */}
 <div className="bg-card px-6 py-4 border-t border-border flex items-center justify-end gap-3">
 <Button
 variant="outline"
 onClick={handleCloseUpload}
 disabled={documentMutationPending}
 >
 Cancelar
 </Button>
 <Button
 onClick={handleUploadSubmit}
 disabled={documentMutationPending || !uploadedFileName || !uploadForm.name.trim()}
 >
 <Upload className="w-4 h-4" />
 {uploadForm.requireSignatures && uploadForm.signers.length > 0
 ? "Enviar para Assinatura"
 : "Fazer Upload"}
 </Button>
 </div>
 </ExpandableSidePanel>
 );

 // ======== SIGN MODAL ========
 const renderSignModal = () => (
 <Dialog open={!!showSignModal} onOpenChange={(next) => { if (!next && !documentMutationPending) setShowSignModal(null); }}>
 {showSignModal && (
 <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-lg">
 {/* Header */}
 <div className="px-6 py-4 border-b border-border flex items-center gap-3 pr-12">
 <div className="p-2 bg-primary-50 text-primary-700 rounded-lg">
 <PenTool className="w-5 h-5" />
 </div>
 <div>
 <DialogTitle className="text-base font-bold text-foreground">
 Assinatura Digital
 </DialogTitle>
 <DialogDescription className="text-xs truncate max-w-[280px]">
 {showSignModal.name}
 </DialogDescription>
 </div>
 </div>

 {/* Signers Status */}
 <div className="px-6 py-4 bg-muted border-b border-border">
 <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-3">
 Signatarios
 </p>
 <div className="space-y-2">
 {showSignModal.signers.map((signer) => (
 <div
 key={signer.id}
 className="flex items-center justify-between"
 >
 <div className="flex items-center gap-2">
 <div
 className={clsx(
 "w-7 h-7 rounded-full flex items-center justify-center",
 signer.status === "assinado"
 ? "bg-status-ok-soft text-status-ok"
 : "bg-muted text-muted-foreground"
 )}
 >
 {signer.status === "assinado" ? (
 <Check className="w-3.5 h-3.5" />
 ) : (
 <Clock className="w-3.5 h-3.5" />
 )}
 </div>
 <div>
 <p className="text-sm font-medium text-foreground">
 {signer.name}
 </p>
 <p className="text-micro text-muted-foreground">
 {signer.role}
 </p>
 </div>
 </div>
 {signer.status === "assinado" ? (
 <span className="text-micro font-medium text-status-ok">
 {signer.signedAt}
 </span>
 ) : (
 <span className="text-micro font-medium text-status-warning">
 Pendente
 </span>
 )}
 </div>
 ))}
 </div>
 </div>

 {/* Signature Pad Area */}
 <div className="px-6 py-5">
 {showSignModal.signers.filter((s) => s.status === "pendente")
 .length > 0 ? (
 <>
 <p className="text-xs text-muted-foreground mb-3">
 Assinando como:{" "}
 <span className="font-bold text-foreground">
 {
 showSignModal.signers.find(
 (s) => s.status === "pendente"
 )?.name
 }
 </span>{" "}
 (
 {
 showSignModal.signers.find(
 (s) => s.status === "pendente"
 )?.role
 }
 )
 </p>
 <SignaturePad
 onSave={async () => {
 const pendingSigner = showSignModal.signers.find(
 (s) => s.status === "pendente"
 );
 if (pendingSigner) {
 await handleSign(showSignModal.id, pendingSigner.id);
 }
 }}
 onCancel={() => setShowSignModal(null)}
 isSaving={documentMutationPending}
 />
 </>
 ) : (
 <div className="text-center py-6">
 <CheckCircle2 className="w-12 h-12 text-status-ok mx-auto mb-3" />
 <p className="text-sm font-bold text-foreground">
 Todas as assinaturas foram coletadas
 </p>
 <p className="text-xs text-muted-foreground mt-1">
 Este documento esta totalmente assinado
 </p>
 <Button
 onClick={() => setShowSignModal(null)}
 className="mt-4"
 >
 Fechar
 </Button>
 </div>
 )}
 </div>
 </DialogContent>
 )}
 </Dialog>
 );

 // ======== DOCUMENT DETAIL MODAL ========
 const renderDocDetailModal = () => (
 <ExpandableSidePanel
 open={!!showDocDetail}
 onClose={() => { if (!documentMutationPending) setShowDocDetail(null); }}
 defaultSize="lg"
 expandedSize="full"
 renderHeader={({ expandButton, closeButton }) => showDocDetail ? (
 <div className="px-6 py-4 border-b border-border flex items-center justify-between flex-shrink-0">
 <div className="flex items-center gap-3">
 <div className="p-2.5 bg-muted rounded-lg">
 <FileText className="w-5 h-5 text-muted-foreground" />
 </div>
 <div>
 <h3 className="text-base font-bold text-foreground">
 Detalhes do Documento
 </h3>
 <p className="text-xs text-muted-foreground">{showDocDetail.id}</p>
 </div>
 </div>
 <div className="flex items-center gap-2">
 {expandButton}
 {closeButton}
 </div>
 </div>
 ) : null}
 >

 {showDocDetail && (
 <>
 {/* Content */}
 <div className="flex-1 overflow-y-auto p-6 space-y-6">
 {/* Doc Info */}
 <div className="space-y-4">
 <div>
 <h4 className="text-lg font-bold text-foreground">
 {showDocDetail.name}
 </h4>
 <div className="flex items-center gap-2 mt-2">
 {(() => {
 const cm = getCategoryMeta(showDocDetail.category);
 const sm = getStatusMeta(showDocDetail.status);
 const SIcon = sm.icon;
 return (
 <>
 <span
 className={clsx(
 "inline-flex items-center gap-1 py-1 px-2.5 rounded-full text-caption font-bold border",
 cm.badgeClass
 )}
 >
 {cm.label}
 </span>
 <span
 className={clsx(
 "inline-flex items-center gap-1 py-1 px-2.5 rounded-full text-caption font-bold border",
 sm.badgeClass
 )}
 >
 <SIcon className="w-3 h-3" />
 {sm.label}
 </span>
 </>
 );
 })()}
 </div>
 </div>

 <div className="grid grid-cols-2 gap-4 bg-muted p-4 rounded-xl">
 <div>
 <p className="text-micro font-medium text-muted-foreground uppercase tracking-wider mb-1">
 Formato
 </p>
 <p className="text-sm font-bold text-foreground">
 {showDocDetail.fileType}
 </p>
 </div>
 <div>
 <p className="text-micro font-medium text-muted-foreground uppercase tracking-wider mb-1">
 Tamanho
 </p>
 <p className="text-sm font-bold text-foreground">
 {formatFileSize(showDocDetail.fileSize)}
 </p>
 </div>
 <div>
 <p className="text-micro font-medium text-muted-foreground uppercase tracking-wider mb-1">
 Upload em
 </p>
 <p className="text-sm font-bold text-foreground">
 {showDocDetail.uploadDate}
 </p>
 </div>
 <div>
 <p className="text-micro font-medium text-muted-foreground uppercase tracking-wider mb-1">
 Enviado por
 </p>
 <p className="text-sm font-bold text-foreground">
 {showDocDetail.uploadedBy}
 </p>
 </div>
 {showDocDetail.expiresAt && (
 <div className="col-span-2">
 <p className="text-micro font-medium text-muted-foreground uppercase tracking-wider mb-1">
 Expira em
 </p>
 <p className="text-sm font-bold text-status-critical">
 {showDocDetail.expiresAt}
 </p>
 </div>
 )}
 </div>
 </div>

 {/* Signers */}
 {showDocDetail.signers.length > 0 && (
 <div>
 <h4 className="text-sm font-bold text-foreground mb-3 flex items-center gap-2">
 <PenTool className="w-4 h-4 text-primary-600" />
 Assinaturas ({showDocDetail.signers.filter((s) => s.status === "assinado").length}/{showDocDetail.signers.length})
 </h4>
 <div className="space-y-2">
 {showDocDetail.signers.map((signer) => (
 <div
 key={signer.id}
 className="flex items-center justify-between bg-muted p-3 rounded-xl"
 >
 <div className="flex items-center gap-3">
 <div
 className={clsx(
 "w-9 h-9 rounded-full flex items-center justify-center",
 signer.status === "assinado"
 ? "bg-status-ok-soft text-status-ok"
 : signer.status === "recusado"
 ? "bg-status-critical-soft text-status-critical"
 : "bg-status-warning-soft text-status-warning"
 )}
 >
 {signer.status === "assinado" ? (
 <CheckCircle2 className="w-4 h-4" />
 ) : signer.status === "recusado" ? (
 <X className="w-4 h-4" />
 ) : (
 <Clock className="w-4 h-4" />
 )}
 </div>
 <div>
 <p className="text-sm font-bold text-foreground">
 {signer.name}
 </p>
 <div className="flex items-center gap-2">
 <p className="text-micro text-muted-foreground">
 {signer.role}
 </p>
 <span className="text-micro text-muted-foreground/60">
 |
 </span>
 <p className="text-micro text-muted-foreground">
 {signer.email}
 </p>
 </div>
 </div>
 </div>
 <div className="text-right">
 {signer.status === "assinado" ? (
 <p className="text-micro font-medium text-status-ok">
 {signer.signedAt}
 </p>
 ) : (
 <span className="text-micro font-bold text-status-warning uppercase">
 Pendente
 </span>
 )}
 </div>
 </div>
 ))}
 </div>
 </div>
 )}

 {/* Notes */}
 {showDocDetail.notes && (
 <div>
 <h4 className="text-sm font-bold text-foreground mb-2">
 Observacoes
 </h4>
 <p className="text-sm text-muted-foreground bg-muted p-4 rounded-xl">
 {showDocDetail.notes}
 </p>
 </div>
 )}
 </div>

 {/* Footer Actions */}
 <div className="bg-card px-6 py-4 border-t border-border flex items-center gap-3 flex-wrap">
 {showDocDetail.signers.some((s) => s.status === "pendente") && (
 <>
 <Button
 onClick={() => {
 setShowDocDetail(null);
 setShowSignModal(showDocDetail);
 }}
 className="flex-1"
 >
 <PenTool className="w-4 h-4" /> Assinar
 </Button>
 <Button
 variant="outline"
 onClick={() => handleSendReminder(showDocDetail)}
 >
 <Send className="w-4 h-4" /> Lembrete
 </Button>
 </>
 )}
 {showDocDetail.fileType === "PDF" && (
 <Button
 variant="outline"
 onClick={() => {
 const doc = showDocDetail;
 setShowDocDetail(null);
 setShowPdfPreview(doc);
 setPdfZoom(100);
 setPdfPage(1);
 }}
 className="border-status-critical/30 text-status-critical hover:bg-status-critical-soft hover:text-status-critical"
 >
 <Eye className="w-4 h-4" /> Preview PDF
 </Button>
 )}
 <Button
 variant="outline"
 onClick={() => { setAuditDocId(showDocDetail.id); setAuditDocLabel(showDocDetail.name); }}
 >
 <History className="w-4 h-4" /> Histórico
 </Button>
 <Button variant="outline">
 <Download className="w-4 h-4" /> Download
 </Button>
 <Button
 variant="outline"
 size="icon"
 onClick={() => {
  confirmAction(
   `Excluir o documento "${showDocDetail.name}"? Esta ação não pode ser desfeita.`,
   async () => {
    setDocumentMutationPending(true);
    try {
     const applied = await deleteDocument(showDocDetail.id);
     if (!applied) return;
     setShowDocDetail(null);
    } finally {
     setDocumentMutationPending(false);
    }
   },
   { title: "Excluir documento", confirmLabel: "Excluir", variant: "danger" },
  );
 }}
 disabled={documentMutationPending}
 aria-label="Excluir documento"
 title="Excluir documento"
 className="border-status-critical/30 text-status-critical hover:bg-status-critical-soft hover:text-status-critical"
 >
 <Trash2 className="w-4 h-4" />
 </Button>
 </div>
 </>
 )}
 </ExpandableSidePanel>
 );

 // ======== PDF PREVIEW MODAL ========
 const renderPdfPreview = () => (
 <AnimatePresence>
 {showPdfPreview && (
 <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
 <motion.div
 initial={{ opacity: 0 }}
 animate={{ opacity: 1 }}
 exit={{ opacity: 0 }}
 className="absolute inset-0 bg-foreground/60 backdrop-blur-sm"
 onClick={() => { setShowPdfPreview(null); setPdfZoom(100); setPdfPage(1); }}
 />
 <motion.div
 initial={{ scale: 0.9, opacity: 0 }}
 animate={{ scale: 1, opacity: 1 }}
 exit={{ scale: 0.9, opacity: 0 }}
 transition={{ type: "spring", bounce: 0, duration: 0.3 }}
 className="relative w-full max-w-4xl h-[85vh] bg-card rounded-2xl shadow-2xl overflow-hidden flex flex-col"
 >
 {/* Header */}
 <div className="px-6 py-3 border-b border-border flex items-center justify-between bg-muted">
 <div className="flex items-center gap-3 min-w-0">
 <div className="p-2 bg-status-critical-soft rounded-lg">
 <FileText className="w-5 h-5 text-status-critical" />
 </div>
 <div className="min-w-0">
 <h3 className="text-sm font-bold text-foreground truncate">{showPdfPreview.name}</h3>
 <p className="text-micro text-muted-foreground">{showPdfPreview.fileType} - {formatFileSize(showPdfPreview.fileSize)}</p>
 </div>
 </div>
 <div className="flex items-center gap-2">
 {/* Zoom controls */}
 <div className="flex items-center gap-1 bg-card border border-border rounded-lg px-1">
 <Button variant="ghost" size="icon" onClick={() => setPdfZoom(z => Math.max(50, z - 25))} className="size-8 text-muted-foreground hover:text-foreground" title="Reduzir">
 <ZoomOut className="w-4 h-4" />
 </Button>
 <span className="text-xs font-bold text-foreground w-10 text-center">{pdfZoom}%</span>
 <Button variant="ghost" size="icon" onClick={() => setPdfZoom(z => Math.min(200, z + 25))} className="size-8 text-muted-foreground hover:text-foreground" title="Ampliar">
 <ZoomIn className="w-4 h-4" />
 </Button>
 </div>
 {/* Page indicator */}
 <div className="flex items-center gap-1 bg-card border border-border rounded-lg px-2 py-1">
 <span className="text-xs font-bold text-foreground">Pag {pdfPage}</span>
 <span className="text-xs text-muted-foreground">/ {Math.max(1, Math.ceil(showPdfPreview.fileSize / 500000))}</span>
 </div>
 {/* Actions */}
 <Button variant="ghost" size="icon" className="size-9 text-muted-foreground hover:text-primary-600 hover:bg-primary-50" title="Download">
 <Download className="w-4 h-4" />
 </Button>
 {showPdfPreview.signers.some(s => s.status === "pendente") && (
 <Button
 size="sm"
 onClick={() => { setShowPdfPreview(null); setShowSignModal(showPdfPreview); }}
 className="text-xs"
 >
 <PenTool className="w-3 h-3" /> Assinar
 </Button>
 )}
 <Button
 variant="ghost"
 size="icon"
 onClick={() => { setShowPdfPreview(null); setPdfZoom(100); setPdfPage(1); }}
 className="size-9 text-muted-foreground"
 >
 <X className="w-5 h-5" />
 </Button>
 </div>
 </div>

 {/* PDF Preview Body */}
 <div className="flex-1 overflow-auto bg-muted p-6 flex justify-center">
 <div
 className="bg-card shadow-lg rounded-sm border border-border transition-transform origin-top"
 style={{
 width: `${Math.round(595 * (pdfZoom / 100))}px`,
 minHeight: `${Math.round(842 * (pdfZoom / 100))}px`,
 transform: `scale(1)`,
 }}
 >
 {/* Simulated PDF content */}
 <div className="p-8 space-y-6" style={{ fontSize: `${Math.round(12 * (pdfZoom / 100))}px` }}>
 {/* Header */}
 <div className="text-center border-b border-border pb-4">
 <div className="flex justify-center mb-3">
 <div className="w-16 h-16 bg-primary-100 rounded-full flex items-center justify-center">
 <FileText className="w-8 h-8 text-primary-600" />
 </div>
 </div>
 <h2 className="font-bold text-foreground" style={{ fontSize: `${Math.round(18 * (pdfZoom / 100))}px` }}>
 {showPdfPreview.name}
 </h2>
 <p className="text-muted-foreground mt-1" style={{ fontSize: `${Math.round(11 * (pdfZoom / 100))}px` }}>
 Documento ID: {showPdfPreview.id} | Contrato: {showPdfPreview.contractId}
 </p>
 <div className="flex items-center justify-center gap-2 mt-2">
 {(() => {
 const cm = getCategoryMeta(showPdfPreview.category);
 const sm = getStatusMeta(showPdfPreview.status);
 const SIcon = sm.icon;
 return (
 <>
 <span className={clsx(
 "inline-flex items-center gap-1 py-0.5 px-2 rounded-full border",
 cm.badgeClass
 )} style={{ fontSize: `${Math.round(10 * (pdfZoom / 100))}px` }}>
 {cm.label}
 </span>
 <span className={clsx(
 "inline-flex items-center gap-1 py-0.5 px-2 rounded-full border",
 sm.badgeClass
 )} style={{ fontSize: `${Math.round(10 * (pdfZoom / 100))}px` }}>
 <SIcon className="w-3 h-3" />
 {sm.label}
 </span>
 </>
 );
 })()}
 </div>
 </div>

 {/* Document Body */}
 <div className="space-y-4 text-foreground">
 <div>
 <p className="font-bold text-foreground mb-1">1. DAS PARTES</p>
 <div className="bg-muted rounded-lg p-3 space-y-1">
 {showPdfPreview.signers.length > 0 ? (
 showPdfPreview.signers.map((s, i) => (
 <p key={s.id} className="flex items-center gap-2">
 <span className="font-bold text-foreground">{s.role}:</span> {s.name} ({s.email})
 {s.status === "assinado" ? (
 <span className="text-status-ok flex items-center gap-0.5" style={{ fontSize: `${Math.round(10 * (pdfZoom / 100))}px` }}>
 <CheckCircle2 style={{ width: `${Math.round(10 * (pdfZoom / 100))}px`, height: `${Math.round(10 * (pdfZoom / 100))}px` }} /> Assinado
 </span>
 ) : (
 <span className="text-status-warning flex items-center gap-0.5" style={{ fontSize: `${Math.round(10 * (pdfZoom / 100))}px` }}>
 <Clock style={{ width: `${Math.round(10 * (pdfZoom / 100))}px`, height: `${Math.round(10 * (pdfZoom / 100))}px` }} /> Pendente
 </span>
 )}
 </p>
 ))
 ) : (
 <p className="text-muted-foreground italic">Sem signatarios registrados</p>
 )}
 </div>
 </div>

 <div>
 <p className="font-bold text-foreground mb-1">2. DO OBJETO</p>
 <p className="text-muted-foreground leading-relaxed">
 O presente documento, de categoria <strong>{getCategoryLabel(showPdfPreview.category)}</strong>,
 refere-se ao contrato <strong>{showPdfPreview.contractId}</strong> e foi
 enviado em <strong>{showPdfPreview.uploadDate}</strong> por <strong>{showPdfPreview.uploadedBy}</strong>.
 </p>
 </div>

 {showPdfPreview.notes && (
 <div>
 <p className="font-bold text-foreground mb-1">3. OBSERVACOES</p>
 <div className="bg-status-warning-soft border border-status-warning/20 rounded-lg p-3">
 <p className="text-foreground">{showPdfPreview.notes}</p>
 </div>
 </div>
 )}

 <div>
 <p className="font-bold text-foreground mb-1">{showPdfPreview.notes ? "4" : "3"}. CLAUSULAS E CONDICOES</p>
 <div className="space-y-2 text-muted-foreground leading-relaxed">
 <p>Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris.</p>
 <p>Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.</p>
 <p>Sed ut perspiciatis unde omnis iste natus error sit voluptatem accusantium doloremque laudantium, totam rem aperiam, eaque ipsa quae ab illo inventore veritatis et quasi architecto beatae vitae dicta sunt explicabo.</p>
 </div>
 </div>

 {showPdfPreview.expiresAt && (
 <div className="bg-status-critical-soft border border-status-critical/20 rounded-lg p-3 flex items-center gap-2">
 <AlertCircle className="w-4 h-4 text-status-critical shrink-0" />
 <p className="text-status-critical font-medium" style={{ fontSize: `${Math.round(11 * (pdfZoom / 100))}px` }}>
 Prazo de validade: {showPdfPreview.expiresAt}
 </p>
 </div>
 )}
 </div>

 {/* Signature Area */}
 {showPdfPreview.signers.length > 0 && (
 <div className="border-t border-border pt-6 mt-8">
 <p className="font-bold text-foreground mb-4 text-center" style={{ fontSize: `${Math.round(11 * (pdfZoom / 100))}px` }}>
 ASSINATURAS
 </p>
 <div className="grid grid-cols-2 gap-6">
 {showPdfPreview.signers.map(s => (
 <div key={s.id} className="text-center">
 <div className={clsx(
 "h-16 border-b-2 mb-2 flex items-end justify-center pb-1",
 s.status === "assinado" ? "border-border" : "border-dashed border-border"
 )}>
 {s.status === "assinado" ? (
 <span className="font-bold text-muted-foreground italic" style={{ fontFamily: "cursive", fontSize: `${Math.round(16 * (pdfZoom / 100))}px` }}>
 {s.name}
 </span>
 ) : (
 <span className="text-muted-foreground italic" style={{ fontSize: `${Math.round(10 * (pdfZoom / 100))}px` }}>
 Aguardando assinatura...
 </span>
 )}
 </div>
 <p className="font-bold text-foreground" style={{ fontSize: `${Math.round(10 * (pdfZoom / 100))}px` }}>{s.name}</p>
 <p className="text-muted-foreground" style={{ fontSize: `${Math.round(9 * (pdfZoom / 100))}px` }}>{s.role}</p>
 {s.signedAt && (
 <p className="text-status-ok" style={{ fontSize: `${Math.round(9 * (pdfZoom / 100))}px` }}>{s.signedAt}</p>
 )}
 </div>
 ))}
 </div>
 </div>
 )}
 </div>
 </div>
 </div>

 {/* Footer Pagination */}
 <div className="px-6 py-2 border-t border-border bg-muted flex items-center justify-between">
 <Button
 variant="outline"
 size="sm"
 onClick={() => setPdfPage(p => Math.max(1, p - 1))}
 disabled={pdfPage <= 1}
 className="text-xs font-bold"
 >
 Anterior
 </Button>
 <span className="text-xs text-muted-foreground">
 Página {pdfPage} de {Math.max(1, Math.ceil(showPdfPreview.fileSize / 500000))}
 </span>
 <Button
 variant="outline"
 size="sm"
 onClick={() => setPdfPage(p => Math.min(Math.max(1, Math.ceil(showPdfPreview!.fileSize / 500000)), p + 1))}
 disabled={pdfPage >= Math.max(1, Math.ceil(showPdfPreview.fileSize / 500000))}
 className="text-xs font-bold"
 >
 Próxima
 </Button>
 </div>
 </motion.div>
 </div>
 )}
 </AnimatePresence>
 );

 // ======== MAIN RENDER ========
 return (
 <>
 {renderUploadModal()}
 {renderSignModal()}
 {renderDocDetailModal()}
 {renderPdfPreview()}
 {confirmationDialog}

 <motion.div
 initial={{ opacity: 0, y: 8 }}
 animate={{ opacity: 1, y: 0 }}
 transition={{ duration: 0.25 }}
 className="space-y-6"
 >
 {/* Header */}
 <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
 <div>
 <h3 className="text-base font-bold text-foreground">
 Documentos do Contrato
 </h3>
 <p className="text-sm text-muted-foreground mt-1">
 Gerencie documentos, assinaturas digitais e arquivos do contrato
 </p>
 </div>
 <Button onClick={() => setShowUploadModal(true)}>
 <Upload className="w-4 h-4" /> Upload de Documento
 </Button>
 </div>

 {/* Stats */}
 <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
 <div className="rounded-xl p-3 border bg-muted border-border">
 <div className="flex items-center gap-2 mb-1">
 <FolderOpen className="w-4 h-4 text-muted-foreground" />
 <p className="text-micro font-medium text-muted-foreground">
 Total de Documentos
 </p>
 </div>
 <p className="text-lg font-bold text-foreground">{stats.total}</p>
 </div>
 <div className="rounded-xl p-3 border bg-status-ok-soft border-status-ok/20">
 <div className="flex items-center gap-2 mb-1">
 <CheckCircle2 className="w-4 h-4 text-status-ok" />
 <p className="text-micro font-medium text-status-ok">
 Assinados
 </p>
 </div>
 <p className="text-lg font-bold text-status-ok">{stats.signed}</p>
 </div>
 <div className="rounded-xl p-3 border bg-status-warning-soft border-status-warning/20">
 <div className="flex items-center gap-2 mb-1">
 <Clock className="w-4 h-4 text-status-warning" />
 <p className="text-micro font-medium text-status-warning">
 Aguardando Assinatura
 </p>
 </div>
 <p className="text-lg font-bold text-status-warning">{stats.pending}</p>
 </div>
 <div className="rounded-xl p-3 border bg-status-info-soft border-status-info/20">
 <div className="flex items-center gap-2 mb-1">
 <File className="w-4 h-4 text-status-info" />
 <p className="text-micro font-medium text-status-info">
 Rascunhos
 </p>
 </div>
 <p className="text-lg font-bold text-status-info">{stats.draft}</p>
 </div>
 </div>

 {/* Search & Filters */}
 <div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-3">
 <div className="relative flex-1 max-w-sm">
 <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
 <input
 type="text"
 value={searchTerm}
 onChange={(e) => setSearchTerm(e.target.value)}
 placeholder="Buscar documento..."
 className="w-full pl-9 pr-4 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none transition-all"
 />
 </div>
 <div className="flex items-center gap-2 flex-wrap">
 {[
 "Todos",
 "Aguardando Assinatura",
 "Assinado",
 "Contrato",
 "Vistoria",
 "Aditivo",
 ].map((f) => (
 <button
 key={f}
 onClick={() => setFilter(f)}
 className={clsx(
 "px-3 py-1.5 text-xs font-bold rounded-lg transition-all border",
 filter === f
 ? "bg-primary text-primary-foreground border-primary"
 : "bg-card text-muted-foreground border-border hover:bg-muted"
 )}
 >
 {f}
 </button>
 ))}
 </div>
 </div>

 {/* Document Cards */}
 <div className="space-y-3">
 {filteredDocs.map((doc) => {
 const categoryMeta = getCategoryMeta(doc.category);
 const statusMeta = getStatusMeta(doc.status);
 const StatusIcon = statusMeta.icon;
 const signedCount = doc.signers.filter(
 (s) => s.status === "assinado"
 ).length;
 const totalSigners = doc.signers.length;
 const hasPending = doc.signers.some(
 (s) => s.status === "pendente"
 );

 return (
 <motion.div
 key={doc.id}
 layout
 initial={{ opacity: 0, y: 8 }}
 animate={{ opacity: 1, y: 0 }}
 className="bg-card rounded-xl border border-border shadow-sm hover:border-border transition-colors overflow-hidden"
 >
 <div className="p-4 flex items-start gap-4">
 {/* File Icon */}
 <div
 className={clsx(
 "w-12 h-14 rounded-lg flex flex-col items-center justify-center border-2 flex-shrink-0",
 doc.status === "assinado"
 ? "border-status-ok/30 bg-status-ok-soft"
 : hasPending
 ? "border-status-warning/30 bg-status-warning-soft"
 : "border-border bg-muted"
 )}
 >
 <FileText
 className={clsx(
 "w-5 h-5",
 doc.status === "assinado"
 ? "text-status-ok"
 : hasPending
 ? "text-status-warning"
 : "text-muted-foreground"
 )}
 />
 <span
 className={clsx(
 "text-caption font-bold mt-0.5",
 doc.status === "assinado"
 ? "text-status-ok"
 : hasPending
 ? "text-status-warning"
 : "text-muted-foreground"
 )}
 >
 {doc.fileType}
 </span>
 </div>

 {/* Content */}
 <div className="flex-1 min-w-0">
 <div className="flex items-start justify-between gap-2">
 <div className="min-w-0">
 <h4 className="text-sm font-bold text-foreground truncate">
 {doc.name}
 </h4>
 <div className="flex items-center gap-2 mt-1 flex-wrap">
 <span
 className={clsx(
 "inline-flex items-center gap-1 py-0.5 px-2 rounded-full text-micro font-bold border",
 categoryMeta.badgeClass
 )}
 >
 {categoryMeta.label}
 </span>
 <span
 className={clsx(
 "inline-flex items-center gap-1 py-0.5 px-2 rounded-full text-micro font-bold border",
 statusMeta.badgeClass
 )}
 >
 <StatusIcon className="w-3 h-3" />
 {statusMeta.label}
 </span>
 </div>
 </div>

 {/* Actions */}
 <div className="flex items-center gap-1 flex-shrink-0">
 {doc.fileType === "PDF" && (
 <Button
 variant="ghost"
 size="sm"
 onClick={() => { setShowPdfPreview(doc); setPdfZoom(100); setPdfPage(1); }}
 className="text-xs font-bold text-muted-foreground hover:text-status-critical hover:bg-status-critical-soft"
 title="Preview do documento"
 >
 <Eye className="w-3 h-3" /> Preview
 </Button>
 )}
 {hasPending && (
 <Button
 variant="ghost"
 size="sm"
 onClick={() => setShowSignModal(doc)}
 className="text-xs font-bold text-primary-600 hover:text-primary-800 hover:bg-primary-50"
 >
 <PenTool className="w-3 h-3" /> Assinar
 </Button>
 )}
 <Button
 variant="ghost"
 size="sm"
 onClick={() => setShowDocDetail(doc)}
 className="text-xs font-bold text-muted-foreground hover:text-foreground"
 >
 <FileText className="w-3 h-3" /> Detalhes
 </Button>
 <Button
 variant="ghost"
 size="sm"
 onClick={() => { setAuditDocId(doc.id); setAuditDocLabel(doc.name); }}
 className="text-xs font-bold text-muted-foreground hover:text-foreground"
 title="Histórico de alterações"
 >
 <History className="w-3 h-3" />
 </Button>
 </div>
 </div>

 {/* Meta row */}
 <div className="flex items-center gap-4 mt-2.5 text-caption text-muted-foreground">
 <span className="flex items-center gap-1">
 <Calendar className="w-3 h-3" /> {doc.uploadDate}
 </span>
 <span>{formatFileSize(doc.fileSize)}</span>
 <span>{doc.uploadedBy}</span>
 {totalSigners > 0 && (
 <span className="flex items-center gap-1">
 <PenTool className="w-3 h-3" /> {signedCount}/
 {totalSigners} assinaturas
 </span>
 )}
 </div>

 {/* Signer Avatars (if any pending) */}
 {totalSigners > 0 && (
 <div className="flex items-center gap-1.5 mt-2.5">
 {doc.signers.map((s) => (
 <div
 key={s.id}
 title={`${s.name} (${s.role}) - ${s.status === "assinado" ? "Assinado" : "Pendente"
 }`}
 className={clsx(
 "w-6 h-6 rounded-full flex items-center justify-center text-caption font-bold border-2",
 s.status === "assinado"
 ? "bg-status-ok-soft text-status-ok border-status-ok/30"
 : "bg-status-warning-soft text-status-warning border-status-warning/30"
 )}
 >
 {s.name
 .split(" ")
 .map((n) => n[0])
 .join("")
 .slice(0, 2)
 .toUpperCase()}
 </div>
 ))}
 </div>
 )}
 </div>
 </div>

 {/* Expiration Warning */}
 {doc.expiresAt && doc.status !== "assinado" && (
 <div className="px-4 py-2 bg-status-warning-soft border-t border-status-warning/20 flex items-center gap-2">
 <AlertCircle className="w-3.5 h-3.5 text-status-warning" />
 <p className="text-caption font-medium text-status-warning">
 Este documento expira em {doc.expiresAt}. Solicite a
 assinatura antes do prazo.
 </p>
 </div>
 )}
 </motion.div>
 );
 })}

 {filteredDocs.length === 0 && (
 <div className="bg-card rounded-2xl border border-border p-12 text-center">
 <FolderOpen className="w-12 h-12 mx-auto mb-3 text-muted-foreground/60" />
 <p className="text-sm font-bold text-foreground mb-1">
 Nenhum documento encontrado
 </p>
 <p className="text-xs text-muted-foreground mb-4">
 {searchTerm || filter !== "Todos"
 ? "Tente ajustar os filtros ou busca"
 : "Faca o upload do primeiro documento deste contrato"}
 </p>
 {!searchTerm && filter === "Todos" && (
 <Button onClick={() => setShowUploadModal(true)}>
 <Upload className="w-4 h-4" /> Upload de Documento
 </Button>
 )}
 </div>
 )}
 </div>
 </motion.div>

 {/* Audit Trail Panel for documents */}
 {auditDocId && (
 <AuditTrailPanel
 entityId={auditDocId}
 entityLabel={auditDocLabel}
 onClose={() => { setAuditDocId(null); setAuditDocLabel(""); }}
   getAuditLog={getAuditLog}
 />
 )}
 </>
 );
}
