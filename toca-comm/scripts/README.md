# toca-comm/scripts

Scripts Python utilitários para manutenção do serviço de comunicação (`toca-comm`), principalmente templates de e-mail do Resend.

## Scripts

### `update_templates.py`
Atualiza (via API do Resend) os templates de e-mail usados pelo serviço:
- `welcome` — e-mail de boas-vindas
- `reset-password` — redefinição de senha
- `invite-company` — convite para empresa

Lê o HTML base embutido no próprio script, substitui placeholders por conteúdo específico de cada template e faz PATCH em `https://api.resend.com/templates/{id}`.

### `test_send.py`
Testa o envio de um e-mail usando `template_id`.

### `test_send_vars.py`
Testa o envio com o objeto `template` contendo variáveis (formato mais novo da API Resend).

### `test_regex.py`
Testa a extração de variáveis `{{ }}` do HTML base via regex. Depende de `update_templates.py` (import).

## Pré-requisitos

- Python 3.13+
- `requests`: `pip install requests`
- Variável de ambiente `EMAIL_API_KEY` (API key do Resend)

## Uso

```bash
cd toca-comm/scripts
export EMAIL_API_KEY=re_xxxxxxxxxxxx
python update_templates.py   # atualiza templates em produção
python test_send.py           # envia e-mail de teste
```

**Atenção:** `update_templates.py` modifica templates reais no Resend. Teste em ambiente sandbox antes de rodar em produção.
