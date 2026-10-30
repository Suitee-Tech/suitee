# Central de Ajuda — Suitee (fonte única de documentação)

> Esta pasta (`toka/src/help/docs`) é a **fonte única** da documentação de uso
> da aplicação. Ela vive dentro do `toka` porque o build do frontend (contexto
> `./toka`) precisa empacotá-la, e é consumida por três superfícies, sempre do
> mesmo conteúdo:

| Superfície | Como consome |
|---|---|
| **Página de ajuda** (`/ajuda` no app admin) | `toka` empacota `topicos/*.md` + `img/*.png` via `import.meta.glob` e renderiza com `react-markdown` |
| **Suitee IA (chat)** | `toca-mcp` copia esta pasta para `/help-corpus` no Dockerfile (contexto do build = raiz do repo) e expõe as tools `search_help` e `get_help_topic` |
| **Testes** | `toca-mcp/internal/helpcorpus` tem testes de carga/busca; o frontend tem um teste de sincronia manifest × arquivos |

**Regra de ouro:** não edite o conteúdo em nenhuma das superfícies. Edite aqui
e as três passam a refletir a mesma versão no próximo build/deploy.

## Conteúdo

- `index.md` — página inicial da ajuda (mapa de assuntos).
- `manifest.yaml` — índice estruturado: ordem, título, resumo, tags e perfis de
  cada tópico (usado pela navegação do frontend e pela busca do MCP).
- `topicos/` — um arquivo Markdown por assunto.
- `img/` — prints das telas (PNG), referenciados como `../img/<nome>.png` a
  partir de `topicos/` e como `img/<nome>.png` a partir de `index.md`.

## Formato de um tópico (`topicos/<nn>-<slug>.md`)

Todo arquivo começa com um bloco de frontmatter mínimo (sem biblioteca YAML
externa — apenas `chave: valor` por linha):

```markdown
---
title: CRM de Leads
summary: Como captar, qualificar e converter leads no CRM.
tags: [leads, captação, qualificação, roleta]
roles: [admin, gestor, corretor, operacoes, marketing]
---
```

| Campo | Obrigatório | Descrição |
|---|---|---|
| `title` | sim | Título curto do tópico (exibido na navegação e nos resultados de busca) |
| `summary` | sim | 1–2 frases; é o resumo exibido nos resultados de busca |
| `tags` | sim | Palavras-chave de busca (lista inline) |
| `roles` | sim | Perfis que usam o tópico (ver `docs/features/ROLES-MATRIX.md`) |

**Seções do corpo** (nesta ordem; use as que fizerem sentido):

```markdown
## O que é
1–2 frases diretas sobre a área/funcionalidade.

## Onde fica
Menu → item (ex.: *Portfólio & Negócios → Imóveis*), com a rota `/imoveis`.

## Passo a passo: <tarefa principal>
1. Passos numerados, curtos e acionáveis.
2. Comece sempre com verbo.

## Exemplo prático
> Cenário com personagem (use os nomes das jornadas BDD: Ana, Bruno, Carla,
> Diego, Eliane, Fábio, Gabriela) e números de exemplo. Termine com o resultado.

## Dicas
- 3–5 dicas curtas.

## Perfis e permissões
| Perfil | O que pode |
|---|---|
| `admin` | ... |

## Veja também
- [Tópico relacionado](./04-roleta.md)
```

## Regras de estilo

1. **Direto**: sem introduções longas; vá ao procedimento.
2. **Contextualizado**: todo tópico diz onde fica, quem pode usar e quando usar.
3. **Prints**: 1–2 por tópico (`` com legenda `*Print: ...*`).
4. **Exemplo prático**: sempre que o fluxo tiver mais de 2 passos.
5. **Linguagem**: pt-BR, tratamento "você"; sem jargão interno (tenant, UUID).
6. **Nomes reais de menu**: copie os rótulos de `toka/src/shared/admin/nav.ts`.
7. Ao citar a IA, use o nome **Suitee IA**.

## Como validar

```sh
# sincronia manifest × tópicos + links e imagens da Central de Ajuda
node scripts/docs/check-help-center.mjs

# testes da busca (MCP)
cd toca-mcp && GOWORK=off go test ./internal/helpcorpus/
```
