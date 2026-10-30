---
title: Imóveis e Captações
summary: Portfólio de imóveis, captações e mídia.
tags: [imóveis, imoveis, portfólio, captação, galeria, publicar]
roles: [admin, gestor, corretor, operacoes, marketing]
---

# Imóveis e Captações

## O que é

A área de imóveis reúne o portfólio completo da imobiliária: o estoque de
imóveis com seus status, a ficha de cada imóvel (mídia, financeiro,
documentação, histórico) e o pipeline de captações de novos imóveis.

## Onde fica

Menu **Portfólio & Negócios → Imóveis**, rota `/imoveis`. As captações
ficam em `/captacoes` — acesse pelo botão **Iniciar Captação** em
**Leads (CRM)** ou digitando a rota.

## Passo a passo: encontrar e filtrar imóveis

1. Em **Imóveis** (**Estoque de Imóveis**), use a busca por ID, bairro ou
   título.
2. Filtre por tipo de negócio: **Todos**, **Venda** ou **Locação**.
3. Use as abas de status: **Todos**, **Captação & Portfólio**,
   **Anunciado**, **Em Negociação** e **Fechados**.
4. Alterne entre as visões lista e mapa conforme a necessidade.
5. Clique no imóvel para abrir o detalhe em `/imoveis/:id`.

*Print: listagem de imóveis com filtros.*

## Passo a passo: trabalhar o detalhe do imóvel

1. No detalhe, confira a **galeria de mídia** (carrossel de fotos e **Ver
   fotos**) e o resumo: valor, quartos, suítes e banheiros.
2. Use o menu de status no cabeçalho para mudar a situação do imóvel — de
   **Em Captação** e **Em Portfólio** até **Anunciado**, **Em Negociação**,
   **Alugado** ou **Vendido**.
3. Complete as abas: **Detalhes & Características**,
   **Financeiro & Avaliação**, **Documentação** e **Histórico & CRM**.
4. Em **Histórico & CRM**, acompanhe a performance e os leads ativos
   interessados no imóvel.
5. Clique em **Editar** para atualizar dados, fotos e condições.

*Print: detalhe do imóvel com galeria de mídia.*

## Passo a passo: captar um imóvel novo

1. Em **Captações** (`/captacoes`), clique em **Iniciar Captação**.
2. Preencha o wizard em até 6 passos: **Origem do imóvel** (novo ou já
   cadastrado), **Proprietário**, **Imóvel** (título, tipo, valor),
   **Endereço**, **Mídia** (opcional) e **Equipe e comissões**.
3. Em **Equipe e comissões**, confirme o captador e o percentual de
   comissão (padrão 100% para o captador).
4. Clique em **Registrar Captação**: o imóvel entra no kanban na coluna
   **Em Captação** e o proprietário vira cliente.
5. Arraste o card para **Em Portfólio** quando o cadastro estiver completo
   e, depois, para **Anunciado** para publicar nos portais configurados em
   **Configurações → Integrações**.

## Exemplo prático

> Fábio procura a imobiliária para alugar o apartamento de 78 m² na Vila
> Olímpia. Carla registra o lead de captação e inicia o wizard: origem
> "imóvel novo", proprietário Fábio, valor R$ 3.500/mês. Bruno promove o
> card para **Em Portfólio**, completa mídia e financeiro no detalhe e muda
> o status para **Anunciado**. Com o aluguel fechado na esteira, o status
> vira **Alugado** e o imóvel sai das listas públicas.

## Dicas

- Fotos boas vendem: preencha a galeria antes de mudar para **Anunciado**.
- Use **Captação & Portfólio** para achar imóveis parados há muito tempo.
- A troca de status em lote pode ser feita card a card; priorize os que
  estão em **Em Negociação**.
- Publique somente imóveis com documentação aprovada na aba
  **Documentação**.

## Perfis e permissões

| Perfil | O que pode |
|---|---|
| `admin`, `gestor`, `corretor`, `operacoes`, `marketing` | Criar captações, editar imóveis, trocar status e publicar. |
| `admin`, `gestor`, `operacoes` | Excluir imóveis. |
| `financeiro`, `auditor` | Consulta do portfólio, sem edição. |
| `parceiro`, `cliente` | Veem o catálogo pelo Portal, com acesso parcial. |

## Veja também

- [CRM de Leads](./03-leads.md)
- [Esteira Digital](./06-esteira.md)
- [Agenda](./09-agenda.md)
- [Contratos](./07-contratos.md)
