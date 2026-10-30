/**
 * Constantes LGPD compartilhadas entre admin e portal. Issue #477.
 *
 * Incrementar CURRENT_TERMS_VERSION quando houver mudança material nos
 * Termos de Uso ou na Política de Privacidade. A API registra uma única
 * versão para o aceite do pacote legal.
 * O TermsAcceptanceModal vai detectar o mismatch entre o que está
 * salvo em UserPreferences.Consent.TermsVersion e este valor, e
 * forçar re-aceite no próximo login do usuário.
 *
 * Convenção: SemVer-like simples (1.0, 1.1, 2.0). Major bump quando
 * mudanças materiais (escopo de coleta, finalidades, retenção);
 * minor bump pra correções de redação.
 */
export const CURRENT_TERMS_VERSION = "2.0";
export const CURRENT_PRIVACY_VERSION = "2.0";
export const CURRENT_TERMS_DOCUMENT_VERSION = "1.0";
/**
 * URLs públicas (servidas pelo Vite/CDN) dos documentos legais.
 * As páginas legais fazem fetch desses arquivos markdown em vez de
 * manter os textos longos inline em TSX; isso mantém os diffs legíveis
 * e permite linkagem direta e preservação de versões anteriores.
 */
export const PRIVACY_POLICY_URL = "/legal/politica-privacidade.md";
export const TERMS_OF_USE_URL = "/legal/termos-de-uso.md";
/**
 * Resumo curto exibido no modal de aceite. Não substitui os documentos;
 * ajuda o usuário a entender o que está aceitando antes de clicar.
 *
 * Atualizar junto com a versão sempre que mudar conteúdo material.
 */
export const TERMS_SUMMARY = [
  "Suitee Inteligence é um produto oferecido por [CNPJ da empresa] Suitee Tech, CNPJ [CNPJ da empresa].",
  "Os Termos de Uso explicam o acesso ao produto e as responsabilidades de cada usuário.",
  "A Política de Privacidade explica quando o fornecedor atua como controlador ou operador.",
  "Recursos de inteligência artificial podem enviar mensagens e contexto necessário à API da DeepSeek.",
  "Marketing depende de escolha específica; comunicações operacionais podem continuar quando necessárias.",
];
