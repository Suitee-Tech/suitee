import PizZip from 'pizzip';
import Docxtemplater from 'docxtemplater';
import { resolveContractDateVariable } from '../contracts/contractDateVariables';

// ── Tipos ──────────────────────────────────────────────────────────

/** Classificação do tipo de dado de uma variável de contrato. */
export type VariableDataType = 'string' | 'number' | 'boolean' | 'object' | 'array' | 'null';

/** Metadado de uma variável extraído dos dados concretos. */
export interface VariableMeta {
  key: string;
  type: VariableDataType;
  /** Prévia do valor para exibição em formulários (máximo ~60 chars). */
  preview: string;
}

// ── Classificação de variáveis ─────────────────────────────────────

/**
 * Classifica o tipo de dado de um valor e gera uma prévia legível.
 * Usado pelo PreviewVariableForm para mostrar o valor real em vez de "Objeto".
 */
export function classifyVariableValue(value: unknown): { type: VariableDataType; preview: string } {
  if (value === null || value === undefined) {
    return { type: 'null', preview: '—' };
  }

  if (Array.isArray(value)) {
    const len = value.length;
    if (len === 0) return { type: 'array', preview: 'Vazio' };
    const names = value
      .slice(0, 3)
      .map((item) => (item && typeof item === 'object' ? (item as any).name || '' : String(item)))
      .filter(Boolean);
    const suffix = len > 3 ? ` +${len - 3}` : '';
    return {
      type: 'array',
      preview: names.length > 0 ? `${names.join(', ')}${suffix}` : `${len} item(ns)`,
    };
  }

  if (typeof value === 'object') {
    const obj = value as Record<string, any>;
    const name = obj.name || obj.label || obj.title || '';
    const extra = obj.email ? ` (${obj.email})` : '';
    return { type: 'object', preview: name ? `${name}${extra}` : 'Objeto' };
  }

  if (typeof value === 'string') {
    return { type: 'string', preview: value.length > 60 ? value.slice(0, 57) + '...' : value };
  }

  if (typeof value === 'number') {
    return { type: 'number', preview: String(value) };
  }

  if (typeof value === 'boolean') {
    return { type: 'boolean', preview: value ? 'Sim' : 'Não' };
  }

  return { type: 'string', preview: String(value) };
}

/**
 * Extrai metadados de todas as variáveis de um objeto de dados.
 * Retorna um mapa chave → VariableMeta.
 */
export function classifyAllVariables(data: Record<string, any>): Record<string, VariableMeta> {
  const result: Record<string, VariableMeta> = {};
  for (const [key, value] of Object.entries(data)) {
    const { type, preview } = classifyVariableValue(value);
    result[key] = { key, type, preview };
  }
  return result;
}

// ── Resolução e validação de templates ─────────────────────────────

const TEMPLATE_DELIMITERS = { start: '{{', end: '}}' } as const;
const FORBIDDEN_PATH_SEGMENTS = new Set(['__proto__', 'prototype', 'constructor']);
export const DOCX_SCALAR_OVERRIDES_KEY = '__docx_scalar_overrides__';

export interface DocxTemplateIssue {
  id: string;
  explanation: string;
  file?: string;
  context?: string;
}

export interface DocxTemplateInspection {
  valid: boolean;
  variables: string[];
  issues: DocxTemplateIssue[];
}

export function formatDocxTemplateIssue(issue: DocxTemplateIssue): string {
  const context = issue.context
    ? ` próximo de "${issue.context.slice(0, 50)}"`
    : '';
  switch (issue.id) {
    case 'unopened_tag':
      return `há um fechamento }} sem abertura {{${context}`;
    case 'unclosed_tag':
      return `há uma abertura {{ sem fechamento }}${context}`;
    case 'unopened_loop':
      return `há um fechamento de lista sem abertura${context}`;
    case 'unclosed_loop':
      return `há uma lista sem tag de fechamento${context}`;
    case 'closing_tag_does_not_match_opening_tag':
      return `as tags de abertura e fechamento de uma lista não correspondem${context}`;
    default:
      return issue.explanation;
  }
}

/** Resolve uma chave literal, uma data calculada e então um caminho pontuado. */
export function resolveTemplateValue(source: unknown, key: string): unknown {
  if (source === null || source === undefined || !key) return undefined;
  if (key === '.') return source;

  if (
    typeof source === 'object' &&
    Object.prototype.hasOwnProperty.call(source, key)
  ) {
    return (source as Record<string, unknown>)[key];
  }

  const dateValue = resolveContractDateVariable(source, key);
  if (dateValue !== undefined) return dateValue;

  let current: unknown = source;
  for (const segment of key.split('.')) {
    if (!segment || FORBIDDEN_PATH_SEGMENTS.has(segment)) return undefined;
    if (current === null || current === undefined || typeof current !== 'object') {
      return undefined;
    }
    if (!Object.prototype.hasOwnProperty.call(current, segment)) return undefined;
    current = (current as Record<string, unknown>)[segment];
  }
  return current;
}

/**
 * Converte um array de objetos em uma string legível (join de nomes).
 * Ex: [{name:"João"}, {name:"Maria"}] → "João, Maria"
 */
function arrayToNamesString(arr: any[]): string {
  return arr
    .map((item) => {
      if (item === null || item === undefined) return '';
      if (typeof item === 'object') {
        const displayValue = (item as any).name || (item as any).label || (item as any).title;
        return displayValue === null || displayValue === undefined ? '' : String(displayValue);
      }
      return String(item);
    })
    .filter((item) => item !== '')
    .join(', ');
}

export function formatTemplateScalarValue(value: unknown): unknown {
  if (Array.isArray(value)) return arrayToNamesString(value);
  if (value && typeof value === 'object') {
    const objectValue = value as Record<string, unknown>;
    return objectValue.name || objectValue.label || objectValue.title || '';
  }
  return value;
}

function templateParser(tag: string) {
  const key = tag.trim();
  return {
    get(scope: unknown, context: any) {
      const isLoop = context?.meta?.part?.module === 'loop';
      if (!isLoop) {
        const rootScope = context?.scopeList?.[0];
        const overrides = rootScope?.[DOCX_SCALAR_OVERRIDES_KEY];
        if (
          overrides &&
          typeof overrides === 'object' &&
          Object.prototype.hasOwnProperty.call(overrides, key)
        ) {
          return overrides[key];
        }
      }
      const value = resolveTemplateValue(scope, key);
      return isLoop
        ? value
        : formatTemplateScalarValue(value);
    },
  };
}

function normalizeBase64Payload(content: string): string {
  const commaIndex = content.indexOf(',');
  let payload = commaIndex >= 0 ? content.slice(commaIndex + 1) : content;
  payload = payload.replace(/-/g, '+').replace(/_/g, '/').replace(/\s/g, '');
  while (payload.length % 4 !== 0) payload += '=';
  return payload;
}

function decodeBase64Bytes(content: string): Uint8Array {
  const binary = atob(normalizeBase64Payload(content));
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index++) {
    bytes[index] = binary.charCodeAt(index);
  }
  return bytes;
}

export interface DocxTextReplacementResult {
  content: ArrayBuffer;
  replacements: number;
}

export type DocxVariableMarker = '' | '#' | '^' | '/';

interface LogicalVariableTag {
  sourceTag: string;
  marker: DocxVariableMarker;
  key: string;
}

interface XmlRange {
  start: number;
  end: number;
}

type LiteralBrace = '{' | '}';

const DOCX_TEXT_PART_PATTERN =
  /^word\/(?:document|header[^/]*|footer[^/]*|footnotes|endnotes|comments)\.xml$/;
const OPEN_BRACE_ENTITY = '&#123;';
const CLOSE_BRACE_ENTITY = '&#125;';
const LEADING_OPEN_BRACE_PATTERN = /^(?:\{|&#(?:0*123|x0*7b);)/i;
const LEADING_CLOSE_BRACE_PATTERN = /^(?:\}|&#(?:0*125|x0*7d);)/i;
const TRAILING_OPEN_BRACE_PATTERN = /(?:\{|&#(?:0*123|x0*7b);)$/i;
const TRAILING_CLOSE_BRACE_PATTERN = /(?:\}|&#(?:0*125|x0*7d);)$/i;

function getOutermostParagraphRanges(xml: string): XmlRange[] {
  const tagPattern = /<w:p(?:\s[^>]*)?>|<\/w:p>/g;
  const ranges: XmlRange[] = [];
  let depth = 0;
  let outerStart = -1;
  let match: RegExpExecArray | null;

  while ((match = tagPattern.exec(xml)) !== null) {
    const tag = match[0];
    if (tag.startsWith('</')) {
      if (depth === 0) continue;
      depth--;
      if (depth === 0 && outerStart >= 0) {
        ranges.push({ start: outerStart, end: match.index + tag.length });
        outerStart = -1;
      }
    } else if (!tag.endsWith('/>')) {
      if (depth === 0) outerStart = match.index;
      depth++;
    }
  }

  return ranges;
}

function mapDocxParagraphs(
  xml: string,
  mapper: (paragraph: string) => string,
): string {
  const ranges = getOutermostParagraphRanges(xml);
  if (ranges.length === 0) return xml;
  const output: string[] = [];
  let cursor = 0;
  for (const range of ranges) {
    output.push(xml.slice(cursor, range.start), mapper(xml.slice(range.start, range.end)));
    cursor = range.end;
  }
  output.push(xml.slice(cursor));
  return output.join('');
}

function getLeadingLiteralBrace(content: string): LiteralBrace | null {
  if (LEADING_OPEN_BRACE_PATTERN.test(content)) return '{';
  if (LEADING_CLOSE_BRACE_PATTERN.test(content)) return '}';
  return null;
}

function getTrailingLiteralBrace(content: string): LiteralBrace | null {
  if (TRAILING_OPEN_BRACE_PATTERN.test(content)) return '{';
  if (TRAILING_CLOSE_BRACE_PATTERN.test(content)) return '}';
  return null;
}

function encodeLeadingLiteralBrace(content: string, brace: LiteralBrace): string {
  return brace === '{'
    ? content.replace(LEADING_OPEN_BRACE_PATTERN, OPEN_BRACE_ENTITY)
    : content.replace(LEADING_CLOSE_BRACE_PATTERN, CLOSE_BRACE_ENTITY);
}

/**
 * O Docxtemplater concatena todos os `<w:t>` de uma parte e ignora limites
 * visuais como parágrafos e `<w:br/>`. Sem esta barreira, duas chaves simples
 * em linhas diferentes podem virar artificialmente `{{` ou `}}`.
 *
 * A segunda chave é codificada como entidade XML, que o Word continua exibindo
 * normalmente, mas que não participa da gramática de delimitadores. Runs comuns
 * não são barreiras, portanto uma tag real dividida entre runs segue funcionando.
 */
function protectBracesAcrossStructuralBreaks(xml: string): string {
  const structuralTokenPattern =
    /(<w:t\b[^>]*>)([\s\S]*?)(<\/w:t>)|<\/?w:p\b[^>]*>|<w:(?:br|cr|tab)\b[^>]*\/?\s*>/g;
  let previousTrailingBrace: LiteralBrace | null = null;
  let crossedStructuralBreak = false;

  return xml.replace(
    structuralTokenPattern,
    (
      token,
      textOpen: string | undefined,
      content: string | undefined,
      textClose: string | undefined,
    ) => {
      if (textOpen !== undefined && content !== undefined && textClose !== undefined) {
        if (content.length === 0) return token;

        const leadingBrace = getLeadingLiteralBrace(content);
        const boundaryBrace = crossedStructuralBreak &&
          leadingBrace !== null &&
          leadingBrace === previousTrailingBrace
          ? leadingBrace
          : null;
        const protectedContent = boundaryBrace !== null
          ? encodeLeadingLiteralBrace(content, boundaryBrace)
          : content;

        previousTrailingBrace = getTrailingLiteralBrace(protectedContent);
        crossedStructuralBreak = false;
        return `${textOpen}${protectedContent}${textClose}`;
      }

      crossedStructuralBreak = true;
      return token;
    },
  );
}

function protectDocxStructuralBraceBoundaries(input: ArrayBuffer): ArrayBuffer {
  const zip = new PizZip(input);
  const partNames = Object.keys(zip.files).filter((name) =>
    DOCX_TEXT_PART_PATTERN.test(name),
  );
  let changed = false;
  for (const partName of partNames) {
    const file = zip.file(partName);
    if (!file) continue;
    const xml = file.asText();
    const protectedXml = protectBracesAcrossStructuralBreaks(xml);
    if (protectedXml !== xml) {
      zip.file(partName, protectedXml);
      changed = true;
    }
  }
  return changed ? zip.generate({ type: 'arraybuffer' }) : input;
}

function collectVariableTags(input: ArrayBuffer): LogicalVariableTag[] {
  const zip = new PizZip(input);
  const textNodePattern = /<w:t\b[^>]*>([\s\S]*?)<\/w:t>/g;
  const variablePattern = /\{\{\s*([#^/]?)\s*([a-zA-Z0-9_][a-zA-Z0-9_.]*)\s*\}\}/g;
  const found = new Map<string, LogicalVariableTag>();

  for (const partName of Object.keys(zip.files).filter((name) => DOCX_TEXT_PART_PATTERN.test(name))) {
    const xml = zip.file(partName)?.asText();
    if (!xml) continue;
    for (const range of getOutermostParagraphRanges(xml)) {
      const paragraph = xml.slice(range.start, range.end);
      const logicalText = Array.from(paragraph.matchAll(textNodePattern))
        .map((match) => match[1])
        .join('');
      variablePattern.lastIndex = 0;
      let variableMatch: RegExpExecArray | null;
      while ((variableMatch = variablePattern.exec(logicalText)) !== null) {
        const tag: LogicalVariableTag = {
          sourceTag: variableMatch[0],
          marker: variableMatch[1] as DocxVariableMarker,
          key: variableMatch[2],
        };
        found.set(tag.sourceTag, tag);
      }
    }
  }

  return Array.from(found.values());
}

function collectVariableTagVariants(input: ArrayBuffer, key: string): string[] {
  return collectVariableTags(input)
    .filter((tag) => tag.key === key)
    .map((tag) => tag.sourceTag);
}

function escapeDocxText(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}

function normalizeXmlTextEntities(value: string): string {
  return value
    .replace(/&#(?:0*38|x0*26);/gi, '&amp;')
    .replace(/&#(?:0*60|x0*3c);/gi, '&lt;')
    .replace(/&#(?:0*62|x0*3e);/gi, '&gt;')
    .replace(/&#(?:0*34|x0*22);/gi, '"')
    .replace(/&#(?:0*39|x0*27);/gi, "'")
    .replace(/&#(?:0*123|x0*7b);/gi, '{')
    .replace(/&#(?:0*125|x0*7d);/gi, '}');
}

function replaceTextAcrossRuns(
  paragraphXml: string,
  search: string,
  replacement: string,
  skip: number,
  limit: number,
): { xml: string; matches: number; replacements: number } {
  if (!search || limit <= 0) {
    return { xml: paragraphXml, matches: 0, replacements: 0 };
  }

  const textNodeRegex = /(<w:t\b[^>]*>)([\s\S]*?)(<\/w:t>)/g;
  const runs: Array<{
    content: string;
    contentStart: number;
    contentEnd: number;
    logicalStart: number;
    logicalEnd: number;
  }> = [];
  let logicalText = '';
  let match: RegExpExecArray | null;

  while ((match = textNodeRegex.exec(paragraphXml)) !== null) {
    const logicalStart = logicalText.length;
    const content = normalizeXmlTextEntities(match[2]);
    logicalText += content;
    runs.push({
      content,
      contentStart: match.index + match[1].length,
      contentEnd: match.index + match[1].length + match[2].length,
      logicalStart,
      logicalEnd: logicalText.length,
    });
  }

  const allPositions: number[] = [];
  let cursor = 0;
  while (true) {
    const position = logicalText.indexOf(search, cursor);
    if (position < 0) break;
    allPositions.push(position);
    cursor = position + search.length;
  }
  const positions = allPositions.slice(skip, skip + limit);

  for (let positionIndex = positions.length - 1; positionIndex >= 0; positionIndex--) {
    const start = positions[positionIndex];
    const end = start + search.length;
    const firstRunIndex = runs.findIndex(
      (run) => start >= run.logicalStart && start < run.logicalEnd,
    );
    const lastRunIndex = runs.findIndex(
      (run) => end - 1 >= run.logicalStart && end - 1 < run.logicalEnd,
    );
    if (firstRunIndex < 0 || lastRunIndex < 0) continue;

    const firstRun = runs[firstRunIndex];
    const lastRun = runs[lastRunIndex];
    const startInFirst = start - firstRun.logicalStart;
    const endInLast = end - lastRun.logicalStart;

    if (firstRunIndex === lastRunIndex) {
      firstRun.content =
        firstRun.content.slice(0, startInFirst) +
        replacement +
        firstRun.content.slice(endInLast);
      continue;
    }

    firstRun.content = firstRun.content.slice(0, startInFirst) + replacement;
    for (let runIndex = firstRunIndex + 1; runIndex < lastRunIndex; runIndex++) {
      runs[runIndex].content = '';
    }
    lastRun.content = lastRun.content.slice(endInLast);
  }

  if (positions.length === 0) {
    return { xml: paragraphXml, matches: allPositions.length, replacements: 0 };
  }

  const output: string[] = [];
  let xmlCursor = 0;
  for (const run of runs) {
    output.push(paragraphXml.slice(xmlCursor, run.contentStart), run.content);
    xmlCursor = run.contentEnd;
  }
  output.push(paragraphXml.slice(xmlCursor));
  return {
    xml: output.join(''),
    matches: allPositions.length,
    replacements: positions.length,
  };
}

/**
 * Substitui texto no conteúdo lógico do Word, mesmo quando ele está dividido
 * entre vários `<w:t>`. Usado ao mapear variáveis no editor de modelos.
 */
export function replaceDocxText(
  input: ArrayBuffer,
  searchText: string,
  replacementText: string,
  replaceAll = true,
  occurrenceIndex = 0,
): DocxTextReplacementResult {
  const zip = new PizZip(input);
  const search = escapeDocxText(searchText);
  const replacement = escapeDocxText(replacementText);
  let remaining = replaceAll ? Number.POSITIVE_INFINITY : 1;
  let remainingToSkip = replaceAll ? 0 : Math.max(0, occurrenceIndex);
  let replacements = 0;

  const partNames = Object.keys(zip.files)
    .filter((name) => DOCX_TEXT_PART_PATTERN.test(name))
    .sort((left, right) => {
      if (left === 'word/document.xml') return -1;
      if (right === 'word/document.xml') return 1;
      return left.localeCompare(right);
    });

  for (const partName of partNames) {
    if (remaining <= 0) break;
    const file = zip.file(partName);
    if (!file) continue;
    const xml = file.asText();
    const updated = mapDocxParagraphs(xml, (paragraph) => {
      if (remaining <= 0) return paragraph;
      const result = replaceTextAcrossRuns(
        paragraph,
        search,
        replacement,
        remainingToSkip,
        remaining,
      );
      remainingToSkip = Math.max(0, remainingToSkip - result.matches);
      remaining -= result.replacements;
      replacements += result.replacements;
      return result.xml;
    });
    if (updated !== xml) zip.file(partName, updated);
  }

  return {
    content: zip.generate({ type: 'arraybuffer' }),
    replacements,
  };
}

/** Remapeia uma variável escalar ou todas as suas formas de lista. */
export function replaceDocxVariable(
  input: ArrayBuffer,
  oldKey: string,
  newKey: string,
  options?: {
    replaceAll?: boolean;
    marker?: DocxVariableMarker;
    occurrenceIndex?: number;
    sourceTag?: string;
  },
): DocxTextReplacementResult {
  const replaceAll = options?.replaceAll ?? true;
  if (!replaceAll) {
    const marker = options?.marker ?? '';
    const sourceTag = options?.sourceTag ?? `{{${marker}${oldKey}}}`;
    return replaceDocxText(
      input,
      sourceTag,
      sourceTag.replace(oldKey, newKey),
      false,
      options?.occurrenceIndex,
    );
  }

  let content = input;
  let replacements = 0;
  for (const sourceTag of collectVariableTagVariants(input, oldKey)) {
    const result = replaceDocxText(
      content,
      sourceTag,
      sourceTag.replace(oldKey, newKey),
    );
    content = result.content;
    replacements += result.replacements;
  }

  return { content, replacements };
}

function bytesToArrayBuffer(bytes: Uint8Array): ArrayBuffer {
  const buffer = new ArrayBuffer(bytes.byteLength);
  new Uint8Array(buffer).set(bytes);
  return buffer;
}

function normalizeLoopTagSpacing(input: ArrayBuffer): ArrayBuffer {
  let content = input;
  for (const tag of collectVariableTags(input)) {
    if (!tag.marker) continue;
    const normalizedTag = tag.sourceTag.replace(
      /^\{\{\s*([#^/])\s*/,
      '{{$1',
    );
    if (normalizedTag === tag.sourceTag) continue;
    content = replaceDocxText(content, tag.sourceTag, normalizedTag).content;
  }
  return content;
}

function prepareDocxContent(base64Content: string): ArrayBuffer {
  const decoded = bytesToArrayBuffer(decodeBase64Bytes(base64Content));
  // A proteção precisa vir antes da normalização: o scanner de loops também
  // concatena runs e poderia criar `{{#lista}}` através de uma quebra visual.
  // A segunda passagem mantém a barreira caso uma substituição legítima tenha
  // reserializado entidades numéricas em outro trecho do mesmo parágrafo.
  const protectedInput = protectDocxStructuralBraceBoundaries(decoded);
  const normalized = normalizeLoopTagSpacing(protectedInput);
  return normalized === protectedInput
    ? normalized
    : protectDocxStructuralBraceBoundaries(normalized);
}

function createDocxtemplater(
  zip: PizZip,
  { allowLegacyUnclosedOpening = false }: {
    allowLegacyUnclosedOpening?: boolean;
  } = {},
): Docxtemplater {
  return new Docxtemplater(zip, {
    paragraphLoop: true,
    linebreaks: true,
    delimiters: TEMPLATE_DELIMITERS,
    parser: templateParser,
    errorLogging: false,
    // JSON e outros trechos literais podem terminar objetos aninhados com `}}`
    // (duas chaves simples adjacentes). Esses caracteres não são uma tag sem
    // abertura. A inspeção ainda rejeita `{{tag` incompleta; somente preview e
    // geração preservam esse caso para compatibilidade com modelos legados.
    syntax: {
      allowUnopenedTag: true,
      allowUnclosedTag: allowLegacyUnclosedOpening,
    },
    nullGetter(part) {
      if (!part.module) return `{{${part.value}}}`;
      return '';
    },
  });
}

function extractTemplateVariables(
  doc: Docxtemplater,
  rootData?: Record<string, unknown>,
): string[] {
  const found = new Set<string>();
  const compiled = (doc as any).compiled as Record<string, unknown> | undefined;
  const partNames = compiled ? Object.keys(compiled) : ['word/document.xml'];
  const variableRegex = /\{\{\s*([#\^\/]?)\s*([a-zA-Z0-9_][a-zA-Z0-9_.]*)\s*\}\}/g;

  for (const partName of partNames) {
    let text: string;
    try {
      text = doc.getFullText(partName);
    } catch {
      continue;
    }

    const loopStack: Array<{
      key: string;
      localScopes: unknown[];
      suppressUnknownFields: boolean;
    }> = [];
    variableRegex.lastIndex = 0;
    let match: RegExpExecArray | null;
    while ((match = variableRegex.exec(text)) !== null) {
      const marker = match[1];
      const key = match[2];
      if (marker === '#' || marker === '^') {
        found.add(key);
        const sectionValue = rootData === undefined
          ? undefined
          : resolveTemplateValue(rootData, key);
        const createsChildScope = marker === '#' && !!sectionValue && typeof sectionValue === 'object';
        const localScopes = createsChildScope
          ? (Array.isArray(sectionValue) ? sectionValue : [sectionValue])
          : [];
        loopStack.push({
          key,
          localScopes,
          // Uma lista vazia não renderiza o conteúdo interno e não oferece
          // amostra suficiente para separar campos do item de campos raiz.
          suppressUnknownFields: createsChildScope && Array.isArray(sectionValue) && sectionValue.length === 0,
        });
      } else if (marker === '/') {
        const index = loopStack.map((section) => section.key).lastIndexOf(key);
        if (index >= 0) loopStack.splice(index, loopStack.length - index);
      } else if (loopStack.length === 0) {
        found.add(key);
      } else if (rootData !== undefined) {
        const resolvesInLocalScope = loopStack.some((section) =>
          section.localScopes.some((localScope) => resolveTemplateValue(localScope, key) !== undefined),
        );
        const hiddenByEmptyArray = loopStack.some((section) => section.suppressUnknownFields);
        if (!resolvesInLocalScope && !hiddenByEmptyArray) found.add(key);
      }
    }
  }

  return Array.from(found);
}

function toTemplateIssues(error: unknown): DocxTemplateIssue[] {
  const root = error as any;
  const errors = Array.isArray(root?.properties?.errors)
    ? root.properties.errors
    : [root];

  return errors.map((item: any) => ({
    id: item?.properties?.id || 'invalid_docx_template',
    explanation: item?.properties?.explanation || item?.message || 'Template DOCX inválido',
    file: item?.properties?.file,
    context: item?.properties?.context,
  }));
}

/**
 * Compila o template e extrai variáveis do DOCX lógico, inclusive entre runs.
 * Fechamentos formados por chaves JSON são conteúdo literal; aberturas de tag
 * incompletas e erros estruturais de tags completas continuam inválidos.
 */
export function inspectDocxTemplate(base64Content: string): DocxTemplateInspection {
  try {
    const doc = createDocxtemplater(new PizZip(prepareDocxContent(base64Content)));
    return { valid: true, variables: extractTemplateVariables(doc), issues: [] };
  } catch (error) {
    return { valid: false, variables: [], issues: toTemplateIssues(error) };
  }
}

/** Extrai variáveis para o preview sem confundir chaves literais com tags. */
export function extractDocxTemplateVariables(
  base64Content: string,
  data?: Record<string, unknown>,
): string[] {
  try {
    const doc = createDocxtemplater(
      new PizZip(prepareDocxContent(base64Content)),
      { allowLegacyUnclosedOpening: true },
    );
    return extractTemplateVariables(doc, data);
  } catch {
    return [];
  }
}

// ── Preenchimento do template ──────────────────────────────────────

/**
 * Preenche um template DOCX (base64) com os dados fornecidos.
 *
 * Regras de substituição:
 * - {{variavel}}         → valor da variável (strings, números, booleanos)
 * - {{objeto}}            → nome do objeto (ex: "João Silva")
 * - {{#array}}{{name}}{{/array}} → loop sobre itens do array
 * - {{objeto.campo}}     → resolve caminhos pontuados
 *
 * @param base64Content - Template DOCX em base64 (com ou sem prefixo data:)
 * @param data           - Dados para preencher o template
 * @returns DOCX preenchido em base64 (com prefixo data:)
 */
export const fillDocxTemplate = (
  base64Content: string,
  data: Record<string, any>,
): string => {
  try {
    const zip = new PizZip(prepareDocxContent(base64Content));
    // Delimitadores órfãos são texto literal; erros estruturais de loop continuam estritos.
    const doc = createDocxtemplater(zip, { allowLegacyUnclosedOpening: true });
    doc.render(data);

    const outputBase64 = doc.getZip().generate({ type: 'base64' });
    return `data:application/vnd.openxmlformats-officedocument.wordprocessingml.document;base64,${outputBase64}`;
  } catch (error: any) {
    console.error('Error filling DOCX template', error);
    if (error.properties?.errors instanceof Array) {
      const messages = error.properties.errors
        .map((e: any) => e.properties?.explanation || e.message)
        .join('\n');
      console.error('Docxtemplater errors:', messages);
    }
    throw error;
  }
};

export const base64ToFile = (base64: string, filename: string): File => {
  try {
    const u8arr = decodeBase64Bytes(base64);
    const buffer = bytesToArrayBuffer(u8arr);
    return new File([buffer], filename, {
      type: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    });
  } catch (e) {
    console.error('Failed to convert base64 to file', e);
    return new File([], filename);
  }
};

/**
 * Lê um arquivo como data URL base64.
 *
 * Estava duplicado dentro de uma página; o painel de aditivo precisa do mesmo
 * comportamento ao exportar do editor e ao subir um documento.
 */
export const fileToBase64 = (file: File | Blob): Promise<string> =>
  new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.readAsDataURL(file);
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = (error) => reject(error);
  });

/** Remove o prefixo `data:...;base64,` quando presente. */
export const stripDataUrlPrefix = (value: string): string =>
  value.includes(",") ? value.split(",")[1] : value;
