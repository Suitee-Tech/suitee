import { readdirSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import { parse } from '@babel/parser';

/**
 * Copy de interface em pt-BR não pode perder acentuação nem vazar jargão
 * interno. O scanner olha apenas contextos que chegam à tela — texto JSX,
 * atributos de apresentação, propriedades de objeto com nome de copy e
 * chamadas de toast/erro — para não confundir rota (`/configuracoes`), enum de
 * API (`"locacao"`) ou chave técnica com frase para o usuário.
 */

const WORDS = [
  'Configuracoes', 'configuracoes', 'Configuracao', 'configuracao',
  'Imoveis', 'imoveis', 'Imovel', 'imovel',
  'Relatorios', 'relatorios', 'Relatorio', 'relatorio',
  'Comissoes', 'comissoes', 'Comissao', 'comissao',
  'Cobrancas', 'cobrancas', 'Cobranca', 'cobranca',
  'Inadimplencia', 'inadimplencia',
  'Locacao', 'locacao',
  'Manutencao', 'manutencao',
  'Alteracoes', 'alteracoes', 'Alteracao', 'alteracao',
  'Endereco', 'endereco', 'Enderecos', 'enderecos',
  'Usuario', 'usuario', 'Usuarios', 'usuarios',
  'Historico', 'historico',
  'Analise', 'analise',
  'Avaliacao', 'avaliacao',
  'Descricao', 'descricao',
  'Observacao', 'observacao',
  'Informacoes', 'informacoes', 'Informacao', 'informacao',
  'Notificacoes', 'notificacoes', 'Notificacao', 'notificacao',
  'Permissoes', 'permissoes', 'Permissao', 'permissao',
  'Autenticacao', 'autenticacao',
  'Aparencia', 'aparencia',
  'Integracoes', 'integracoes', 'Integracao', 'integracao',
  'Seguranca', 'seguranca',
  'Sessao', 'sessao',
  'Formulario', 'formulario', 'Formularios', 'formularios',
  'Qualificacao', 'qualificacao',
  'Padrao', 'padrao',
  'Opcao', 'opcao', 'Opcoes', 'opcoes',
  'Numero', 'numero',
  'Codigo', 'codigo',
  'Area', 'area',
  'Ultimo', 'ultimo', 'Ultima', 'ultima',
  'Proximo', 'proximo', 'Proxima', 'proxima',
  'Obrigatorio', 'obrigatorio', 'Obrigatoria', 'obrigatoria',
  'Disponivel', 'disponivel',
  'Possivel', 'possivel',
  'Pagina', 'pagina',
  'Conteudo', 'conteudo',
  'Inicio', 'inicio',
  'Comunicacao', 'comunicacao',
  'Visualizacao', 'visualizacao',
  'Confirmacao', 'confirmacao',
  'Operacao', 'operacao', 'Operacoes', 'operacoes',
  'Exclusao', 'exclusao',
  'Edicao', 'edicao',
  'Selecao', 'selecao',
  'Exportacao', 'exportacao',
  'Referencia', 'referencia',
  'Competencia', 'competencia',
  'Experiencia', 'experiencia',
  'Inteligencia', 'inteligencia',
  'Servico', 'servico', 'Servicos', 'servicos',
  'Beneficio', 'beneficio', 'Beneficios', 'beneficios',
  'Nao', 'nao',
  'Voce', 'voce',
  'Tambem', 'tambem',
  'Apos', 'apos',
  'Sera', 'sera', 'Serao', 'serao',
  'Sao', 'sao',
  'Orgao', 'orgao',
  'Situacao', 'situacao',
  'Acao', 'acao', 'Acoes', 'acoes',
  'Impressao', 'impressao',
  'Lancamento', 'lancamento', 'Lancamentos', 'lancamentos',
  'Nivel', 'nivel', 'Urgencia', 'urgencia',
  'Horario', 'horario', 'Horarios', 'horarios', 'Contabil', 'contabil', 'Emissao', 'emissao',
  'Titulo', 'titulo', 'Subtitulo', 'subtitulo', 'Versao', 'versao', 'Automatico', 'automatico', 'Automatica', 'automatica', 'Diaria', 'diaria', 'Marco', 'marco',
  'Atribuido', 'atribuido', 'Interacao', 'interacao', 'Publicacao', 'publicacao', 'Anuncio', 'anuncio', 'Anuncios', 'anuncios',
  'Distribuicao', 'distribuicao', 'Sincronizacao', 'sincronizacao', 'Frequencia', 'frequencia',
  'Restauracao', 'restauracao', 'Localizacao', 'localizacao', 'Fisica', 'fisica', 'Mudanca', 'mudanca',
  'Rotulos', 'rotulos', 'Escalacao', 'escalacao', 'Automacao', 'automacao', 'Rotina', 'rotina',
  'Imobiliaria', 'imobiliaria', 'Imobiliarias', 'imobiliarias',
  'Proprietario', 'proprietario', 'Proprietarios', 'proprietarios',
  'Locatario', 'locatario', 'Locatarios', 'locatarios',
  'Conciliacao', 'conciliacao', 'Rescisao', 'rescisao', 'Renovacao', 'renovacao',
  'Orcamento', 'orcamento', 'Metricas', 'metricas', 'Importacao', 'importacao',
  'Validacao', 'validacao', 'Simulacao', 'simulacao', 'Aprovacao', 'aprovacao', 'Reprovacao', 'reprovacao',
];

const WORD_RE = new RegExp(`(?<![\\p{L}\\p{N}])(?:${WORDS.join('|')})(?![\\p{L}\\p{N}])`, 'gu');
const JARGON = [
  { pattern: /\bAudit Trail\b/g, message: 'use "Trilha de auditoria"' },
  { pattern: /\bWhite-label\b/gi, message: 'use "marca própria"' },
  { pattern: /\btenant\b/gi, message: 'use "empresa"' },
  { pattern: /\bpayout\b/gi, message: 'use "ordem de pagamento" ou "repasse"' },
];

const UI_ATTRIBUTES = new Set([
  'label', 'title', 'subtitle', 'description', 'message', 'placeholder',
  'aria-label', 'alt', 'confirmLabel', 'cancelLabel', 'loadingText', 'empty',
  'help', 'hint', 'text', 'labelText',
]);
const UI_OBJECT_KEYS = new Set([
  'label', 'title', 'subtitle', 'description', 'message', 'placeholder',
  'confirmLabel', 'cancelLabel', 'loadingText', 'empty', 'help', 'hint',
  'text', 'labelText', 'desc',
]);
const UI_CALLS = new Set([
  'toast.success', 'toast.error', 'toast.info', 'toast.warning',
  'setError', 'notifyApiError', 'setStatusMessage',
]);

function calleeName(node) {
  if (!node) return '';
  if (node.type === 'Identifier') return node.name;
  if (node.type === 'MemberExpression' && !node.computed) {
    return `${calleeName(node.object)}.${node.property.name || ''}`;
  }
  return '';
}

function isUiContext(node) {
  let current = node;
  for (let depth = 0; depth < 8 && current; depth += 1) {
    const parent = current.__parent;
    if (!parent) return false;
    if (parent.type === 'JSXText') return true;
    if (parent.type === 'JSXAttribute') {
      const name = parent.name?.type === 'JSXIdentifier' ? parent.name.name : '';
      return UI_ATTRIBUTES.has(name);
    }
    if (parent.type === 'ObjectProperty') {
      const key = parent.key?.type === 'Identifier' ? parent.key.name : parent.key?.value;
      if (UI_OBJECT_KEYS.has(key)) return true;
      if (['response', 'data', 'payload', 'config', 'body'].includes(key)) return false;
    }
    if (parent.type === 'CallExpression') {
      return UI_CALLS.has(calleeName(parent.callee));
    }
    if (['TSAsExpression', 'TSTypeAssertion', 'ParenthesizedExpression', 'ConditionalExpression', 'LogicalExpression', 'StringLiteral'].includes(parent.type)) {
      current = parent;
      continue;
    }
    if (['JSXExpressionContainer', 'JSXElement', 'JSXFragment'].includes(parent.type)) {
      current = parent;
      continue;
    }
    return false;
  }
  return false;
}

function isExcludedValue(value) {
  const text = value.trim();
  if (!text) return true;
  if (text.startsWith('/') || text.startsWith('#') || text.startsWith('http')) return true;
  if (text.includes('://') || text.includes('=') || text.includes('?')) return true;
  if (/\.(pdf|docx?|png|jpe?g|webp)$/i.test(text)) return true;
  if (!/\s/.test(text) && /[._:/@]/.test(text)) return true;
  if (!/\s/.test(text) && /^[a-z0-9-]+$/.test(text)) return true;
  if (!/\s/.test(text) && /^[a-z][a-z0-9]*$/.test(text)) return true;
  if (/^(bg|text|border|ring|shadow|from|via|to|hover|focus|grid|flex|px|py|mt|mb|w|h)-/.test(text)) return true;
  return false;
}

function collectStrings(ast) {
  const found = [];
  const walk = (node, parent) => {
    if (!node) return;
    node.__parent = parent;
    if (node.type === 'JSXText') {
      found.push({ value: node.value, loc: node.loc });
    } else if (node.type === 'StringLiteral') {
      found.push({ value: node.value, loc: node.loc, node });
    } else if (node.type === 'TemplateElement') {
      found.push({ value: node.value.cooked || node.value.raw, loc: node.loc });
    }
    for (const [key, value] of Object.entries(node)) {
      if (['loc', 'start', 'end', 'extra', 'leadingComments', 'trailingComments', 'innerComments', '__parent'].includes(key)) continue;
      if (Array.isArray(value)) {
        for (const child of value) {
          if (child && typeof child.type === 'string') walk(child, node);
        }
      } else if (value && typeof value.type === 'string') {
        walk(value, node);
      }
    }
  };
  walk(ast.program, null);
  return found;
}

export function findCopyIssues(source, file = 'unknown.tsx') {
  const ast = parse(source, {
    sourceType: 'unambiguous',
    plugins: ['typescript', 'jsx'],
    errorRecovery: true,
  });
  const issues = [];
  for (const { value, loc, node } of collectStrings(ast)) {
    if (!value || isExcludedValue(value)) continue;
    if (node && node.type === 'StringLiteral' && !isUiContext(node) && !value.includes(' ')) continue;
    if (node && node.type === 'StringLiteral' && !isUiContext(node)) {
      // Frases longas fora de contexto de UI continuam sendo candidatas apenas
      // se estiverem num atributo/propriedade de apresentação.
      continue;
    }
    const words = [...new Set(value.match(WORD_RE) || [])];
    const jargon = JARGON.filter((rule) => rule.pattern.test(value)).map((rule) => rule.message);
    if (words.length === 0 && jargon.length === 0) continue;
    issues.push({
      file,
      line: loc?.start.line ?? 0,
      text: value.trim().replace(/\s+/g, ' ').slice(0, 140),
      words,
      jargon: [...new Set(jargon)],
    });
  }
  return issues;
}

export function scanDirectory(root) {
  const issues = [];
  const walk = (dir) => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const file = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        if (file === path.join(root, 'app')) continue;
        walk(file);
        continue;
      }
      if (!/\.(ts|tsx)$/.test(entry.name) || /\.(test|spec)\./.test(entry.name)) continue;
      const source = readFileSync(file, 'utf8');
      issues.push(...findCopyIssues(source, path.relative(root, file)));
    }
  };
  walk(root);
  return issues;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const root = path.resolve('src');
  const issues = scanDirectory(root);
  for (const issue of issues) {
    const details = [...issue.words, ...issue.jargon].join('; ');
    console.error(`${issue.file}:${issue.line}: ${details} :: ${issue.text}`);
  }
  if (issues.length > 0) {
    console.error(`\nCopy: ${issues.length} ocorrência(s) de acentuação/jargão em contexto de interface.`);
    process.exitCode = 1;
  } else {
    console.log('Copy: nenhuma ocorrência de acentuação/jargão em contexto de interface.');
  }
}
