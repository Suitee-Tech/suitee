import { useEffect, useMemo, useState, type ReactNode } from "react";
import { clsx } from "clsx";
import {
  BellRing,
  Banknote,
  ChevronDown,
  ChevronRight,
  ClipboardCheck,
  CornerDownRight,
  Eye,
  EyeOff,
  FileCheck2,
  FilePlus2,
  FileQuestion,
  FileSignature,
  IdCard,
  Image as ImageIcon,
  Landmark,
  Layers,
  Receipt,
  Scale,
  ShieldCheck,
  Stamp,
  Star,
  Tag,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { DOCUMENT_CATEGORIES } from "../../lib/documents";
import { getCategoryMeta } from "../../lib/documents";
import {
  DOCUMENT_COLLECTIONS,
  documentFolderKeyToString,
  normalizeFolderKeyString,
  type DocumentDriveCounts,
  type DocumentEntityNode,
  type DocumentFolderKey,
} from "../../documents/drive";
import { DOCUMENT_ORIGINS, type DocumentOriginId } from "../../documents/origins";
import type { DocumentCategory } from "../../types/contractDocument";

/**
 * Árvore de navegação da Central de Documentos.
 *
 * Três eixos, na ordem em que a operação pensa:
 * 1. Coleções  — o que precisa de atenção (assinatura, recentes, vencidos).
 * 2. Origem    — de onde o documento veio (contrato, imóvel, cliente...),
 *                expansível até o registro específico.
 * 3. Tipo/Tags — recortes auxiliares do arquivo.
 *
 * Pastas sem nenhum documento ficam ocultas por padrão: a lista de zeros era
 * exatamente o ruído que impedia enxergar o acervo real.
 */
export interface DocumentFolderTreeProps {
  counts: DocumentDriveCounts;
  entitiesByOrigin: Partial<Record<DocumentOriginId, DocumentEntityNode[]>>;
  activeKey: DocumentFolderKey;
  onSelect: (key: DocumentFolderKey) => void;
  pinnedKeys: string[];
  onTogglePin: (key: DocumentFolderKey) => void;
  hidePinning?: boolean;
  /** Expõe coleções, origens e tipos sem nenhum documento. */
  showEmpty?: boolean;
  onToggleShowEmpty?: () => void;
}

const CATEGORY_ICONS: Record<DocumentCategory, LucideIcon> = {
  contrato: FileSignature,
  aditivo: FilePlus2,
  laudo: ShieldCheck,
  vistoria: ClipboardCheck,
  procuracao: Stamp,
  recibo: Receipt,
  notificacao: BellRing,
  financeiro: Banknote,
  juridico: Scale,
  fiscal: Landmark,
  identidade: IdCard,
  comprovante: FileCheck2,
  foto: ImageIcon,
  outros: FileQuestion,
};

interface RowProps {
  icon: LucideIcon;
  label: string;
  hint?: string;
  count: number;
  active: boolean;
  pinned?: boolean;
  hidePinning?: boolean;
  onSelect: () => void;
  onTogglePin?: () => void;
  depth?: 0 | 1;
  expandable?: boolean;
  expanded?: boolean;
  onToggleExpand?: () => void;
}

function TreeRow({
  icon: Icon,
  label,
  hint,
  count,
  active,
  pinned,
  hidePinning,
  onSelect,
  onTogglePin,
  depth = 0,
  expandable,
  expanded,
  onToggleExpand,
}: RowProps) {
  return (
    <div
      className={clsx(
        "group flex items-center gap-1 rounded-lg transition-colors",
        active ? "bg-primary-50 text-primary-700" : "text-foreground hover:bg-muted",
      )}
    >
      {expandable ? (
        <button
          type="button"
          onClick={onToggleExpand}
          title={expanded ? "Recolher" : "Expandir"}
          aria-label={expanded ? `Recolher ${label}` : `Expandir ${label}`}
          aria-expanded={expanded}
          className={clsx(
            "ml-1 p-0.5 rounded text-muted-foreground hover:text-foreground shrink-0",
            expanded && "text-foreground",
          )}
        >
          {expanded ? <ChevronDown className="w-3.5 h-3.5" /> : <ChevronRight className="w-3.5 h-3.5" />}
        </button>
      ) : (
        <span className="ml-1 w-4.5 shrink-0" aria-hidden />
      )}

      <button
        type="button"
        onClick={onSelect}
        title={hint}
        className={clsx("flex-1 min-w-0 flex items-center gap-2 py-1.5 text-left", depth === 1 && "pl-3")}
      >
        <Icon
          className={clsx(
            "shrink-0",
            depth === 1 ? "w-3.5 h-3.5" : "w-4 h-4",
            active ? "text-primary-600" : "text-muted-foreground",
          )}
        />
        <span
          className={clsx(
            "flex-1 truncate",
            depth === 1 && "text-caption text-muted-foreground",
            active && "font-bold",
            active && depth === 1 && "text-primary-700",
          )}
        >
          {label}
        </span>
        <span
          className={clsx(
            "text-xs tabular-nums shrink-0",
            active ? "text-primary-600" : "text-muted-foreground",
          )}
        >
          {count}
        </span>
      </button>

      {!hidePinning && onTogglePin && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onTogglePin();
          }}
          title={pinned ? "Desafixar pasta" : "Fixar pasta"}
          className={clsx(
            "mr-1.5 p-1 rounded transition-opacity shrink-0",
            pinned
              ? "opacity-100 text-status-warning"
              : "opacity-0 group-hover:opacity-100 text-muted-foreground hover:text-foreground",
          )}
        >
          <Star className={clsx("w-3.5 h-3.5", pinned && "fill-current")} />
        </button>
      )}
    </div>
  );
}

function SectionTitle({ children, action }: { children: ReactNode; action?: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-2 px-3 pt-2 pb-1">
      <span className="text-micro font-bold uppercase text-muted-foreground tracking-wider">{children}</span>
      {action}
    </div>
  );
}

export function DocumentFolderTree({
  counts,
  entitiesByOrigin,
  activeKey,
  onSelect,
  pinnedKeys,
  onTogglePin,
  hidePinning,
  showEmpty = false,
  onToggleShowEmpty,
}: DocumentFolderTreeProps) {
  const activeStr = documentFolderKeyToString(activeKey);
  const activeOrigin: DocumentOriginId | null =
    activeKey.kind === "origin" ? activeKey.origin : activeKey.kind === "origin-entity" ? activeKey.origin : null;

  const [expandedOrigins, setExpandedOrigins] = useState<DocumentOriginId[]>(activeOrigin ? [activeOrigin] : []);

  // A origem do filtro ativo permanece aberta, sem descartar o que o usuário abriu.
  useEffect(() => {
    if (!activeOrigin) return;
    setExpandedOrigins((current) => (current.includes(activeOrigin) ? current : [...current, activeOrigin]));
  }, [activeOrigin]);

  const collections = useMemo(
    () =>
      DOCUMENT_COLLECTIONS.filter(
        (collection) => showEmpty || collection.id === "all" || counts.collections[collection.id] > 0,
      ),
    [counts.collections, showEmpty],
  );

  const originSections = useMemo(
    () =>
      DOCUMENT_ORIGINS.map((origin, index) => ({
        origin,
        index,
        count: counts.origins[origin.id] ?? 0,
        nodes: entitiesByOrigin[origin.id] ?? [],
      }))
        .filter((section) => showEmpty || section.count > 0)
        .sort((a, b) => b.count - a.count || a.index - b.index),
    [counts.origins, entitiesByOrigin, showEmpty],
  );

  const categorySections = useMemo(
    () =>
      DOCUMENT_CATEGORIES.map((category) => ({ category, count: counts.categories[category] ?? 0 }))
        .filter((entry) => showEmpty || entry.count > 0)
        .sort(
          (a, b) =>
            b.count - a.count ||
            getCategoryMeta(a.category).label.localeCompare(getCategoryMeta(b.category).label, "pt-BR"),
        ),
    [counts.categories, showEmpty],
  );

  const tagSections = useMemo(
    () =>
      Object.entries(counts.tags)
        .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0], "pt-BR"))
        .slice(0, 10),
    [counts.tags],
  );

  const allRows = useMemo<{ key: DocumentFolderKey; label: string; count: number; icon: LucideIcon }[]>(
    () => [
      ...collections.map((collection) => ({
        key: { kind: "collection", id: collection.id } as DocumentFolderKey,
        label: collection.label,
        count: counts.collections[collection.id],
        icon: collection.icon,
      })),
      ...originSections.map((section) => ({
        key: { kind: "origin", origin: section.origin.id } as DocumentFolderKey,
        label: section.origin.label,
        count: section.count,
        icon: section.origin.icon,
      })),
      ...originSections.flatMap((section) =>
        section.nodes.map((node) => ({
          key: {
            kind: "origin-entity",
            origin: section.origin.id,
            entityId: node.entityId,
          } as DocumentFolderKey,
          label: `${section.origin.label} · ${node.label}`,
          count: node.count,
          icon: CornerDownRight,
        })),
      ),
      ...categorySections.map(({ category, count }) => ({
        key: { kind: "category", category } as DocumentFolderKey,
        label: getCategoryMeta(category).label,
        count,
        icon: CATEGORY_ICONS[category],
      })),
      ...tagSections.map(([tag, count]) => ({
        key: { kind: "tag", tag } as DocumentFolderKey,
        label: tag,
        count,
        icon: Tag,
      })),
    ],
    [collections, counts.collections, originSections, categorySections, tagSections],
  );

  const pinnedSet = useMemo(
    () => new Set(pinnedKeys.map(normalizeFolderKeyString)),
    [pinnedKeys],
  );
  const pinnedRows = useMemo(
    () => allRows.filter((row) => pinnedSet.has(documentFolderKeyToString(row.key))),
    [allRows, pinnedSet],
  );

  const isActive = (key: DocumentFolderKey) => documentFolderKeyToString(key) === activeStr;
  const isPinned = (key: DocumentFolderKey) => pinnedSet.has(documentFolderKeyToString(key));

  const toggleOrigin = (origin: DocumentOriginId) => {
    setExpandedOrigins((current) =>
      current.includes(origin) ? current.filter((o) => o !== origin) : [...current, origin],
    );
  };

  const renderRow = (row: { key: DocumentFolderKey; label: string; count: number; icon: LucideIcon }, hint?: string) => (
    <TreeRow
      key={`row-${documentFolderKeyToString(row.key)}`}
      icon={row.icon}
      label={row.label}
      hint={hint}
      count={row.count}
      active={isActive(row.key)}
      pinned={isPinned(row.key)}
      hidePinning={hidePinning}
      onSelect={() => onSelect(row.key)}
      onTogglePin={() => onTogglePin(row.key)}
    />
  );

  return (
    <nav className="flex flex-col gap-0.5 p-2 text-sm">
      {pinnedRows.length > 0 && (
        <>
          <SectionTitle>Pastas fixadas</SectionTitle>
          {pinnedRows.map((row) => renderRow(row))}
          <div className="my-1.5 border-t border-border" />
        </>
      )}

      <SectionTitle>Coleções</SectionTitle>
      {collections.map((collection) =>
        renderRow(
          {
            key: { kind: "collection", id: collection.id },
            label: collection.label,
            count: counts.collections[collection.id],
            icon: collection.icon,
          },
          collection.description,
        ),
      )}

      {originSections.length > 0 && (
        <>
          <div className="my-1.5 border-t border-border" />
          <SectionTitle action={<Layers className="w-3 h-3 text-muted-foreground" aria-hidden />}>Origem</SectionTitle>
          {originSections.map((section) => {
            const key: DocumentFolderKey = { kind: "origin", origin: section.origin.id };
            const expanded = expandedOrigins.includes(section.origin.id);
            return (
              <div key={`origin-${section.origin.id}`} className="flex flex-col">
                <TreeRow
                  icon={section.origin.icon}
                  label={section.origin.label}
                  hint={section.origin.description}
                  count={section.count}
                  active={isActive(key)}
                  pinned={isPinned(key)}
                  hidePinning={hidePinning}
                  expandable={section.nodes.length > 0}
                  expanded={expanded}
                  onToggleExpand={() => toggleOrigin(section.origin.id)}
                  onSelect={() => onSelect(key)}
                  onTogglePin={() => onTogglePin(key)}
                />
                {expanded &&
                  section.nodes.map((node) => {
                    const nodeKey: DocumentFolderKey = {
                      kind: "origin-entity",
                      origin: section.origin.id,
                      entityId: node.entityId,
                    };
                    return (
                      <TreeRow
                        key={documentFolderKeyToString(nodeKey)}
                        depth={1}
                        icon={CornerDownRight}
                        label={node.label}
                        hint={node.detail}
                        count={node.count}
                        active={isActive(nodeKey)}
                        pinned={isPinned(nodeKey)}
                        hidePinning={hidePinning}
                        onSelect={() => onSelect(nodeKey)}
                        onTogglePin={() => onTogglePin(nodeKey)}
                      />
                    );
                  })}
              </div>
            );
          })}
        </>
      )}

      <div className="my-1.5 border-t border-border" />
      <SectionTitle>Tipo de documento</SectionTitle>
      {categorySections.length > 0 ? (
        categorySections.map(({ category, count }) =>
          renderRow({
            key: { kind: "category", category },
            label: getCategoryMeta(category).label,
            count,
            icon: CATEGORY_ICONS[category],
          }),
        )
      ) : (
        <p className="px-3 py-1.5 text-caption text-muted-foreground">Nenhum documento catalogado ainda.</p>
      )}

      {tagSections.length > 0 && (
        <>
          <div className="my-1.5 border-t border-border" />
          <SectionTitle>Tags</SectionTitle>
          {tagSections.map(([tag, count]) =>
            renderRow({ key: { kind: "tag", tag }, label: tag, count, icon: Tag }),
          )}
        </>
      )}

      {onToggleShowEmpty && (
        <>
          <div className="my-1.5 border-t border-border" />
          <button
            type="button"
            onClick={onToggleShowEmpty}
            className="w-full flex items-center gap-2 px-3 py-1.5 text-caption text-muted-foreground hover:text-foreground transition-colors"
          >
            {showEmpty ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
            {showEmpty ? "Ocultar pastas vazias" : "Mostrar pastas vazias"}
          </button>
        </>
      )}
    </nav>
  );
}

export { documentFolderKeyToString };
export type { DocumentFolderKey };
