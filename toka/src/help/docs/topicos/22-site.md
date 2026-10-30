---
title: Site de imóveis
summary: Ative e configure a vitrine pública da imobiliária, acompanhe TLS/publicação e veja os resultados.
tags: [site, site de imóveis, vitrine, publicar site, domínio, tls, https, marca, branding, seo, leads do site]
roles: [admin, gestor, marketing]
---

# Site de imóveis

## O que é

O **Site de imóveis** é a vitrine pública da sua imobiliária: um endereço
próprio, com a sua marca, onde o comprador encontra os imóveis publicados e
entra em contato. A configuração fica no painel; o cadastro dos imóveis
continua em **Imóveis**.

## Onde fica

Menu **Portfólio & Negócios → Site de imóveis**, rota `/site`.

O endereço público é formado pela marca escolhida + domínio da Suitee, por
exemplo `https://imobiliaria-Exemplo.imoveis.suitee.com.br`.

## Passo a passo: ativar o site

1. Abra **Portfólio & Negócios → Site de imóveis**.
2. Informe o **nome** que aparecerá na vitrine e escolha a **marca** (o
   endereço). A marca é única e não pode ser reservada, numérica ou malformada.
3. Confira o preview do endereço e ative o site.
4. Salve. A imobiliária é provisionada na vitrine e o estoque já cadastrado é
   publicado em segundo plano; imóveis em rascunho ficam fora.

## Passo a passo: configurar a identidade

1. Na seção de identidade, envie o **logo**, a **capa** e escolha as cores
   principal e de destaque.
2. Use **Usar identidade do painel** para reaproveitar logo e cores de
   **Configurações → Aparência**.
3. Preencha a frase de destaque (até 160 caracteres) e salve.

Se a vitrine ainda não tiver logo, o logo da empresa é aplicado
automaticamente; sem logo da empresa, a vitrine usa a logo da Suitee.

## Passo a passo: contato e páginas

1. Em **Contato no site**, confira e-mail, telefone, WhatsApp, endereço e
   CRECI. **Usar dados da empresa** restaura os valores de Configurações.
2. Nas abas **Início**, **Sobre** e **Contato**, escreva o título, o subtítulo
   e o conteúdo e clique em **Publicar página**.
3. As alterações são publicadas na vitrine; não é necessário deploy.

## Passo a passo: liberar o endereço com segurança

1. Depois de salvar, o painel mostra **“Publicando seu site com segurança”**.
2. O sistema valida o certificado HTTPS e a resposta da página inicial.
3. Aguarde alguns instantes, sem recarregar: o painel acompanha o status.
4. Quando aparecer **“Site publicado com HTTPS válido”**, **Abrir site** e
   **Copiar link** são liberados.

Se o site estiver desligado, o endereço não abre. Ao religar, o estoque atual
é reprojetado.

## Acompanhar resultados

- **Visualizações (7d)** e **Leads (7d)** mostram o desempenho da vitrine.
- **Imóveis publicados** e **Imóveis na vitrine** comparam o portfólio com o
  que está no ar.
- Os últimos leads aparecem no painel; quando já estão no CRM, o status mostra
  **No CRM** e o nome abre o lead.
- **Sincronizar** força uma nova publicação do estoque em segundo plano.

## Publicar um imóvel no site

Publicar não é feito em `/site`. Abra o imóvel em **Imóveis**, mude o status
comercial para **Anunciado** ou use o controle **Publicar no site** no detalhe
do imóvel. O anúncio aparece na vitrine sem passo extra.

## SEO e indexação

Em produção, a vitrine é indexável: o `robots.txt` libera o rastreamento e o
`sitemap.xml` responde. No ambiente de beta, a indexação fica bloqueada para
evitar conteúdo de teste nos buscadores. Essa configuração é do ambiente, não
do painel.

## Exemplo prático

> Ana é `admin` da Imobiliária Exemplo. Ela abre **Site de imóveis**, define a
> marca `imobiliaria-Exemplo`, reaproveita o logo e as cores do painel e publica
> as páginas **Sobre** e **Contato**. Enquanto o certificado é emitido, o painel
> mostra “Publicando seu site com segurança” e mantém o link bloqueado. Pouco
> depois, o endereço fica verde e ela copia o link para divulgar. Os imóveis
> continuam sendo publicados individualmente em **Imóveis**.

## Dicas

- A marca vira parte do endereço público: escolha algo curto e fácil de
  divulgar.
- Preencha o contato antes de divulgar; ele aparece no rodapé e nos anúncios.
- Imóvel em rascunho não aparece para o comprador.
- Para tirar o site do ar, desligue-o no painel; as métricas históricas
  continuam disponíveis.

## Perfis e permissões

| Perfil | O que pode |
|---|---|
| `admin` | Ativar, configurar, publicar páginas e sincronizar o site. |
| `gestor` | Configurar a vitrine conforme as permissões da empresa. |
| `marketing` | Acompanhar e ajustar a presença da marca, quando liberado. |
| `corretor` | Publica imóveis pelo cadastro do imóvel; não administra o site. |
| `cliente` | Não acessa a gestão do site. |

## Veja também

- [Imóveis e Captações](./05-imoveis.md)
- [Configurações e Assinatura](./16-configuracoes.md)
- [Suitee IA](./17-inteligencia.md)
- [Perguntas frequentes](./20-faq.md)
