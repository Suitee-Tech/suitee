import os
import requests
import json

API_KEY = os.getenv("EMAIL_API_KEY")
BASE_URL = "https://api.resend.com/templates"

HEADERS = {
    "Authorization": f"Bearer {API_KEY}",
    "Content-Type": "application/json"
}

BASE_HTML = """<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>Suitee Notificação</title>
  <!--[if mso]>
  <style type="text/css">
    body, table, td {font-family: Arial, Helvetica, sans-serif !important;}
  </style>
  <![endif]-->
  <style>
    body { margin: 0; padding: 0; background-color: #f9fafb; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased; }
    table { border-spacing: 0; border-collapse: collapse; width: 100%; }
    td { padding: 0; }
    img { border: 0; }
    .container { max-width: 600px; margin: 0 auto; width: 100%; }
    .card { background-color: #ffffff; border-radius: 12px; border: 1px solid #e5e7eb; overflow: hidden; box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.05); }
    .content-pad { padding: 40px 32px; }
    @media screen and (max-width: 600px) {
      .content-pad { padding: 32px 24px !important; }
      .container { width: 100% !important; }
      .btn { display: block !important; width: 100% !important; box-sizing: border-box !important; text-align: center !important; }
    }
  </style>
</head>
<body style="background-color: #f9fafb; margin: 0; padding: 40px 20px;">
  <table role="presentation" style="width: 100%; background-color: #f9fafb;">
    <tr>
      <td align="center">
        <table role="presentation" class="container" style="max-width: 600px; margin: 0 auto;">
          <tr>
            <td align="center" style="padding-bottom: 24px;">
              <img src="{{{LOGO_URL}}}" alt="Suitee Logo" width="160" style="display: block; width: 160px; max-width: 100%; height: auto;">
            </td>
          </tr>
          <tr>
            <td class="card" style="background-color: #ffffff; border-radius: 12px; border: 1px solid #e5e7eb;">
              <table role="presentation" style="width: 100%;">
                <tr>
                  <td class="content-pad" style="padding: 40px 32px;">
                    <h1 style="margin: 0 0 16px 0; font-size: 24px; font-weight: 700; color: #111827; line-height: 1.25;">
                      {{TITLE}}
                    </h1>
                    {{BODY_CONTENT}}
                    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin: 0 0 32px 0;">
                      <tr>
                        <td align="center">
                          <table role="presentation" cellpadding="0" cellspacing="0" style="border-collapse: separate; width: auto;">
                            <tr>
                              <td align="center" bgcolor="#1C905B" style="border-radius: 8px;">
                                <a class="btn" href="{{ACTION_URL}}" target="_blank" style="display: block; box-sizing: border-box; max-width: 100%; padding: 14px 28px; font-size: 16px; font-weight: 600; color: #ffffff; text-decoration: none; border-radius: 8px; background-color: #1C905B; border: 1px solid #1C905B; mso-padding-alt: 14px 28px; white-space: normal; word-break: break-word;">
                                  {{ACTION_TEXT}}
                                </a>
                              </td>
                            </tr>
                          </table>
                        </td>
                      </tr>
                    </table>
                    <hr style="border: none; border-top: 1px solid #e5e7eb; margin: 0 0 24px 0;">
                    <p style="margin: 0; font-size: 14px; color: #6b7280; line-height: 1.5;">
                      Se você não solicitou este e-mail, por favor, ignore esta mensagem. Para suporte técnico ou comercial, entre em contato com nossa equipe através da plataforma.
                    </p>
                  </td>
                </tr>
              </table>
            </td>
          </tr>
          <tr>
            <td align="center" style="padding-top: 32px; padding-bottom: 32px;">
              <p style="margin: 0 0 8px 0; font-size: 12px; color: #9ca3af; line-height: 1.5;">
                &copy; 2026 Suitee Inteligência Imobiliária. Todos os direitos reservados.
              </p>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>"""

TEMPLATES = [
    {
        "name": "welcome",
        "title": "Olá, {{{NAME}}}!",
        "body": "<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>Bem-vindo(a) à <strong>Suitee</strong>! Sua conta foi criada com sucesso e estamos felizes em tê-lo(a) conosco.</p><p style='margin: 0 0 32px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>Acesse sua conta para configurar seu perfil e começar a utilizar nossos recursos.</p>",
        "action_url": "{{{SUITEE_URL}}}",
        "action_text": "Acessar Plataforma",
        "id": "fc0c800d-ebad-46bf-96f5-279b9f08ef5b"
    },
    {
        "name": "reset-password",
        "title": "Redefinição de Senha",
        "body": "<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>Recebemos uma solicitação para redefinir a senha da sua conta.</p><p style='margin: 0 0 32px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>Clique no botão abaixo para criar uma nova senha. Este link é válido por 1 hora.</p>",
        "action_url": "{{{SUITEE_URL}}}",
        "action_text": "Redefinir Minha Senha",
        "id": "91bf153d-a3d5-4f84-af68-9e49ee710613"
    },
    {
        "name": "invite-company",
        "title": "Convite para {{{COMPANY_NAME}}}",
        "body": "<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>Você foi convidado(a) para participar da empresa <strong>{{{COMPANY_NAME}}}</strong> na Suitee.</p><p style='margin: 0 0 32px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>Clique no botão abaixo para aceitar o convite e criar sua conta (ou entrar, caso já tenha uma).</p>",
        "action_url": "{{{SUITEE_URL}}}",
        "action_text": "Aceitar Convite",
        "id": "0551da37-6822-4675-b08f-9d2b9d0ee87b"
    }
]

def update_templates():
    for t in TEMPLATES:
        html_content = BASE_HTML.replace("{{TITLE}}", t["title"])
        html_content = html_content.replace("{{BODY_CONTENT}}", t["body"])
        html_content = html_content.replace("{{ACTION_URL}}", t["action_url"])
        html_content = html_content.replace("{{ACTION_TEXT}}", t["action_text"])
        
        payload = {
            "subject": t["title"],
            "html": html_content,
            "text": ""
        }
        
        with open(f"{t['name']}.html", "w") as f:
            f.write(html_content)
        
        import re
        variables = re.findall(r"{{{([^}]+)}}}", html_content)
        
        var_list = [{"key": v, "type": "string"} for v in set(variables)]
        
        if var_list:
            payload["variables"] = var_list
        print(payload["variables"])
        
        res = requests.patch(f"{BASE_URL}/{t['id']}", headers=HEADERS, json=payload)
        print(f"Update {t['name']}: {res.status_code} {res.text}")

if __name__ == "__main__":
    update_templates()
