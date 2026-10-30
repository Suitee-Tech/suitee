#!/bin/sh
set -eu

ENV_JS_PATH=/usr/share/nginx/html/env.js
INDEX_HTML_PATH=/usr/share/nginx/html/index.html
ROBOTS_TXT_PATH=/usr/share/nginx/html/robots.txt

json_escape() {
  printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'
}

# Chaves de runtime que o navegador pode ver. TUDO aqui é público por
# definição: URLs de infra, flags de UI e a versão. NENHUMA credencial entra
# nesta lista — não há chave de API, token ou segredo em /env.js. O CI
# (check-frontend-env) falha o build se aparecer qualquer nome com cara de
# segredo aqui, nos compose do frontend ou no source.
#
# VITE_SUPABASE_URL é a URL pública do projeto (usada para montar URLs de mídia
# e documentos); a antiga VITE_SUPABASE_ANON_KEY saiu quando o reset de senha
# passou a ser mediado pelo backend (POST /auth/reset-password).
keys="VITE_APP_VERSION VITE_API_URL VITE_MEDIA_URL VITE_WS_URL VITE_IMGPROXY_URL VITE_MULTI_PARTY_UI VITE_SUPABASE_URL VITE_INVOICE_API_URL VITE_API_DEBUG"

# Sem estas o app cai nos defaults de BUILD (localhost) e nada funciona —
# melhor recusar subir do que servir um env.js vazio.
required_keys="VITE_API_URL VITE_SUPABASE_URL"

write_env_js() {
  for key in $required_keys; do
    value=$(printenv "$key" 2>/dev/null || true)
    if [ -z "${value:-}" ]; then
      echo "env.js: $key nao definido no container — recusando subir (servir env.js vazio quebraria o app silenciosamente)" >&2
      exit 1
    fi
  done

  printf '%s\n' 'window.__TOCA_ENV__ = {' > "$ENV_JS_PATH"

  first=1
  for key in $keys; do
    value=$(printenv "$key" 2>/dev/null || true)
    if [ -z "${value:-}" ]; then
      continue
    fi

    if [ "$first" -eq 0 ]; then
      printf '%s\n' ',' >> "$ENV_JS_PATH"
    fi
    first=0

    esc=$(json_escape "$value")
    printf '  "%s": "%s"' "$key" "$esc" >> "$ENV_JS_PATH"
  done

  printf '%s\n' '' >> "$ENV_JS_PATH"
  printf '%s\n' '};' >> "$ENV_JS_PATH"
}

write_env_js

# env.js is (re)written at runtime; ensure the unprivileged nginx worker can
# read it regardless of the mode inherited from the write-once source tree.
chmod 644 "$ENV_JS_PATH"

# `SITE_NOINDEX` bloqueia a indexação do ambiente (ligado no overlay de beta).
# A mesma imagem serve beta e produção, então o robots.txt e o meta são
# resolvidos em runtime — nunca no build.
is_noindex() {
  case "$(printf '%s' "${SITE_NOINDEX:-}" | tr '[:upper:]' '[:lower:]')" in
    1 | true | yes | on | sim) return 0 ;;
    *) return 1 ;;
  esac
}

write_indexing_policy() {
  # A source é publicada write-once (arquivos 0400); remova antes de regravar.
  rm -f "$ROBOTS_TXT_PATH"

  if is_noindex; then
    {
      printf '%s\n' '# Ambiente de BETA — nao indexar em mecanismos de busca.'
      printf '%s\n' 'User-agent: *'
      printf '%s\n' 'Disallow: /'
    } >"$ROBOTS_TXT_PATH"

    # O robots.txt impede o rastreamento; o meta `noindex` impede a indexação
    # caso a página ainda seja buscada ou o robots.txt seja ignorado. O build já
    # traz `<meta name="robots" content="index, follow">` (padrão de produção):
    # o conteúdo é SUBSTITUÍDO quando o meta existe, ou injetado antes de
    # `</head>` quando não existe. Só `awk` (o `sed -i` diverge entre busybox e
    # BSD/macOS).
    if [ -f "$INDEX_HTML_PATH" ]; then
      awk -v m='  <meta name="robots" content="noindex, nofollow" />' '
        {
          if (!replaced && $0 ~ /<meta[^>]*name="robots"/) {
            sub(/content="[^"]*"/, "content=\"noindex, nofollow\"")
            replaced = 1
          } else if (!replaced && !injected && $0 ~ /<\/head>/) {
            sub(/<\/head>/, m "\n</head>")
            injected = 1
          }
          print
        }
      ' "$INDEX_HTML_PATH" >"$INDEX_HTML_PATH.tmp"
      mv "$INDEX_HTML_PATH.tmp" "$INDEX_HTML_PATH"
    fi
  else
    {
      printf '%s\n' 'User-agent: *'
      printf '%s\n' 'Allow: /'
    } >"$ROBOTS_TXT_PATH"
  fi

  chmod 644 "$ROBOTS_TXT_PATH" "$INDEX_HTML_PATH"
}

write_indexing_policy

exec nginx -g 'daemon off;'
