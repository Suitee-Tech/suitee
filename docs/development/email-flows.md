# Fluxos de e-mail

Este documento é o inventário dos e-mails transacionais e da forma como cada
ambiente os entrega. O local usa o Mailpit do Supabase; beta e produção usam o
Resend (ou um SMTP próprio, quando configurado).

## Como o transporte é escolhido

| Configuração | Transporte | Uso |
|---|---|---|
| `SMTP_HOST` preenchido | SMTP direto | Mailpit local (`host.docker.internal:54325`) ou SMTP próprio. |
| `SMTP_HOST` vazio | API do Resend | Beta/produção. Templates de boas-vindas, reset e convite usam os IDs do Resend; os demais fluxos usam HTML. |
| `COMM_EMAIL_ENABLED=false` | nenhum e-mail de notificação | Default de produção; o compose local liga `true`. |
| `COMM_EMAIL_IGNORE_PRESENCE=true` | envia mesmo online | Apenas compose local, para observar notificações no Mailpit. |

O caminho local não substitui o comportamento de produção: quando `SMTP_HOST`
está vazio, os templates do Resend continuam sendo usados e os testes de
contrato cobrem reset, convite e boas-vindas.

## Matriz de cobertura

| Fluxo | Origem | Local | Beta/produção |
|---|---|---|---|
| Confirmação de cadastro | Supabase Auth/GoTrue | SMTP do Supabase → Mailpit | Provedor de e-mail do Supabase |
| Boas-vindas após cadastro | `auth.user.registered` → `toca-comm` | HTML local via SMTP | Template Resend `fc0c800d-ebad-46bf-96f5-279b9f08ef5b` |
| Esqueceu a senha | `auth.password_reset.requested` → `toca-comm` | HTML local via SMTP | Template Resend `91bf153d-a3d5-4f84-af68-9e49ee710613` |
| Convite de equipe/reenvio | `auth.invitation.created` → `toca-comm` | HTML local via SMTP | Template Resend `0551da37-6822-4675-b08f-9d2b9d0ee87b` |
| Convite do portal do cliente | `auth.client.invited` → `toca-comm` | HTML local via SMTP | HTML via Resend |
| Código OTP do portal | `auth.otp.requested` → `toca-comm` | HTML local via SMTP | HTML via Resend |
| Lead atribuído | evento de lead → `EmailNotifier` | SMTP local; presença ignorada no compose local | Resend; presença pode suprimir |
| Atualizações de negócio | eventos de deal → `EmailNotifier` | SMTP local | Resend |
| Propostas/contrapropostas e follow-up | eventos de proposta → `EmailNotifier` | SMTP local | Resend |
| Agenda, visitas e lembretes | eventos de agenda → `EmailNotifier` | SMTP local | Resend |
| Recomendações do corretor | eventos de recomendação → `EmailNotifier` | SMTP local | Resend |
| Lembretes de chat | `chat_reminder_engine` → `EmailNotifier` | SMTP local | Resend |
| Cobranças e repasses | `toca-financial` → Comm interno → `NotificationService` | SMTP local | Resend |
| Notificação genérica interna | `POST /api/v1/comm/internal/notifications/send` | SMTP local | Resend |

## Cobertura automatizada

- `TestLocalSMTP_WelcomePasswordResetAndInvitation` cobre boas-vindas, reset,
  convite, OTP e convite do portal contra um servidor SMTP de teste.
- `TestResendContract_SendTemplateEmail` e
  `TestResendContract_PasswordResetAndInvitationTemplates` cobrem o caminho de
  produção sem SMTP, verificando os IDs dos templates.
- `TestEmailNotifier_LocalIgnorePresence` cobre a exceção local de presença.
- `TestRequireActiveSubscription` cobre a rota de consentimento em plano vencido.

## Limitações conhecidas

- Confirmação de cadastro é responsabilidade do GoTrue/Supabase; se o projeto
  local estiver com auto-confirm ligado, essa mensagem específica não é gerada.
- Em produção, o kill-switch `COMM_EMAIL_ENABLED` continua desligado por padrão.
  O `docker-compose.prod.yml` não recebe as flags locais.
- O Mailpit é o Inbucket do Supabase local; a caixa visual fica em
  `http://localhost:54324` e a porta SMTP em `54325`.
