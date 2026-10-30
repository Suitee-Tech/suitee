# 🎨 Toka (Frontend)

O **Toka** é o frontend web do ecossistema **Suitee**, desenvolvido como uma Single Page Application (SPA) moderna, rápida e altamente reativa.

---

## 🚀 Tecnologias e Stack

O projeto frontend utiliza as seguintes tecnologias:

- **Framework**: React 18
- **Build Tool**: Vite (Rápido e otimizado)
- **Linguagem**: TypeScript (Tipagem estática e segurança)
- **Estilização**: TailwindCSS (Utility-first) + CSS Modules
- **Componentes UI**: [shadcn/ui](https://ui.shadcn.com/) (Componentes acessíveis e customizáveis)
- **Gerenciamento de Pacotes**: pnpm (Rápido e eficiente com espaço em disco)

---

## 🛠️ Como Iniciar (Ambiente Local)

### Pré-requisitos
- Node.js 24.22.1
- pnpm 10.15.0 (versão fixada em `package.json`)
- O backend rodando localmente (`make up ENV=local`)

### Passo a Passo

1. **Instale as dependências**:
   A partir da raiz, execute:
   ```bash
   make frontend-install
   ```

2. **Variáveis de Ambiente (Opcionais)**:
   Por padrão, o frontend vai tentar se conectar na porta `:8080` (Gateway do backend local). Caso o seu backend rode em outra porta/host, você pode configurar criando um arquivo `.env`:
   ```env
   VITE_API_URL=http://localhost:8080/api/v1
   VITE_WS_URL=ws://localhost:8080/api/v1/ws
   ```

3. **Inicie o Servidor de Desenvolvimento**:
   ```bash
   pnpm dev
   ```
   Acesse a aplicação no navegador em `http://localhost:5173`. O servidor de desenvolvimento tem suporte a Hot Module Replacement (HMR).

---

## 🐳 Rodando via Docker (Opcional)

Se você preferir não instalar o Node.js/pnpm localmente, pode subir o frontend pelo Docker (junto com a stack):

A partir da raiz do projeto, execute:
```bash
make up ENV=local
```

O contêiner irá iniciar o Nginx servindo o build de produção ou o ambiente de desenvolvimento, e a interface estará disponível na porta `http://localhost:3000`. Ele também já configura proxy reverso para os endpoints de `/api` e `/uploads`.

---

## 📦 Build para Produção

Para gerar o build otimizado de produção:

```bash
pnpm build:ci
```

Os arquivos estáticos serão gerados na pasta `dist/`, prontos para serem servidos por um Nginx, S3, Vercel, Netlify, ou similares.

---

## 🧪 Testes E2E (Playwright)

Os testes End-to-End do frontend e integrações estão configurados com o Playwright.

Para rodar o gate determinístico usado pelo CI:
```bash
pnpm test:e2e:ci
```
Os testes e fixtures encontram-se na pasta `e2e/`.
