// Package handlers — LGPD endpoint (Art. 9 direito de acesso). Issue #475.
//
// O que esse endpoint deve fazer (LGPD Art. 9):
//
//	"O titular tem direito ao acesso facilitado às informações sobre o
//	tratamento de seus dados [...] que deverão ser disponibilizadas
//	de forma clara, adequada e ostensiva."
//
// Implementação MVP — toca-auth devolve em JSON tudo que ele tem
// sobre o usuário corrente (identity + preferences + audit trail
// relevante). Dados em outros services (leads, deals, charges,
// inspections) são apenas referenciados — quando o usuário precisar
// deles, pode usar o async export do #474 ou pedir ao DPO via
// /lgpd/me/account (#476 follow-up).
//
// Por que MVP em vez de fan-out cross-service?
//  1. Atende a primeira premissa do Art. 9: "informações sobre o
//     tratamento" — toca-auth é o único service com dados de
//     identificação pessoal direta (CPF, nome legal, e-mail).
//  2. Cross-service exige N HTTP calls com tratamento de falha
//     parcial — significativamente mais complexo. Ship fast +
//     iterate.
//  3. LGPD não exige "tudo em uma chamada" — pode ser fragmentado
//     desde que o usuário consiga acessar; aqui informamos onde
//     buscar o resto.
package handlers

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca-auth/internal/api/v1/dto"
	"github.com/oginoino/toca-auth/internal/service"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/gdpr"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
)

type LGPDHandler struct {
	authService  *service.AuthService
	prefsService *service.PreferencesService
}

func NewLGPDHandler(authSvc *service.AuthService, prefsSvc *service.PreferencesService) *LGPDHandler {
	return &LGPDHandler{authService: authSvc, prefsService: prefsSvc}
}

// PersonalDataResponse — shape do payload retornado por GET
// /lgpd/me/data. Estrutura plana + extensível por section (cada
// service futuramente pode aparecer aqui sob sua chave).
type PersonalDataResponse struct {
	// Identity — dados de cadastro mantidos pelo toca-auth.
	Identity *dto.UserResponse `json:"identity,omitempty"`

	// Preferences — preferências + consentimentos LGPD (Art. 8).
	Preferences *preferencesSection `json:"preferences,omitempty"`

	// Companies — empresas que o usuário pertence (membership).
	Companies []dto.CompanyResponse `json:"companies,omitempty"`

	// Generated — quando o snapshot foi montado (UTC RFC3339).
	GeneratedAt string `json:"generated_at"`

	// OtherDataSources — referência para dados que vivem em outros
	// services. UI pode renderizar instruções ao usuário sobre como
	// acessar (ex.: export async #474, contato com DPO etc.).
	OtherDataSources []dataSourceRef `json:"other_data_sources,omitempty"`

	// Notice — disclaimer LGPD com link para política completa.
	Notice string `json:"notice"`
}

type preferencesSection struct {
	Language   string                 `json:"language"`
	Timezone   string                 `json:"timezone"`
	DateFormat string                 `json:"date_format"`
	Consent    map[string]interface{} `json:"consent,omitempty"`
}

type dataSourceRef struct {
	Service     string `json:"service"`
	Description string `json:"description"`
	HowToAccess string `json:"how_to_access"`
}

// GetMyData godoc
// @Summary      LGPD Art. 9 — direito de acesso aos próprios dados (#475)
// @Description  Retorna em JSON tudo que toca-auth tem sobre o usuário
// @Description  autenticado: identity (cadastro), preferences, companies
// @Description  associadas + referências para dados em outros services.
// @Description  Cada chamada é auditada (lgpd.data_export_requested).
// @Tags         lgpd
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} response.Response{data=PersonalDataResponse}
// @Failure      401 {object} response.Response
// @Failure      404 {object} response.Response
// @Router       /lgpd/me/data [get]
func (h *LGPDHandler) GetMyData(c *gin.Context) {
	userIDVal, _ := c.Get("user_id")
	userID, ok := userIDVal.(string)
	if !ok || userID == "" {
		response.Unauthorized(c, "user_id missing from token")
		return
	}

	// Identity — busca account + individual.
	acc, err := h.authService.GetUserInfo(c.Request.Context(), userID)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	if acc == nil {
		response.NotFound(c, "account not found")
		return
	}

	out := PersonalDataResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Notice: "Este payload contém os dados pessoais mantidos pela Suitee (Art. 9 LGPD). " +
			"Dados em outros sistemas (cobranças, contratos, vistorias) estão listados em " +
			"'other_data_sources' com instruções de acesso. Para solicitar exclusão definitiva, " +
			"use DELETE /lgpd/me/account.",
		OtherDataSources: []dataSourceRef{
			{Service: "toca-deal", Description: "Negociações + contratos assinados", HowToAccess: "Portal cliente → Meus negócios"},
			{Service: "toca-financial", Description: "Cobranças, repasses, comissões", HowToAccess: "Portal cliente → Finanças"},
			{Service: "toca-ops", Description: "Vistorias + tickets de manutenção", HowToAccess: "Portal cliente → Locação → Manutenção"},
			{Service: "toca-crm", Description: "Histórico de interações como lead", HowToAccess: "Solicitar ao DPO via privacidade@suitee.com.br"},
		},
	}

	if acc.Individual != nil {
		if dtoUser := dto.MapUserToDTO(acc.Individual); dtoUser != nil {
			out.Identity = dtoUser
		}
	}

	// Preferences — best-effort (não bloqueia se falhar).
	if prefs, err := h.prefsService.GetPreferences(c.Request.Context(), userID); err == nil && prefs != nil {
		consent := map[string]interface{}{
			"marketing_emails":   prefs.Consent.MarketingEmails,
			"marketing_sms":      prefs.Consent.MarketingSMS,
			"marketing_whatsapp": prefs.Consent.MarketingWhatsApp,
			"terms_version":      prefs.Consent.TermsVersion,
		}
		if prefs.Consent.MarketingEmailsAt != nil {
			consent["marketing_emails_at"] = prefs.Consent.MarketingEmailsAt.UTC().Format(time.RFC3339)
		}
		if prefs.Consent.MarketingSMSAt != nil {
			consent["marketing_sms_at"] = prefs.Consent.MarketingSMSAt.UTC().Format(time.RFC3339)
		}
		if prefs.Consent.MarketingWhatsAppAt != nil {
			consent["marketing_whatsapp_at"] = prefs.Consent.MarketingWhatsAppAt.UTC().Format(time.RFC3339)
		}
		if prefs.Consent.TermsAcceptedAt != nil {
			consent["terms_accepted_at"] = prefs.Consent.TermsAcceptedAt.UTC().Format(time.RFC3339)
		}
		out.Preferences = &preferencesSection{
			Language:   prefs.Language,
			Timezone:   prefs.Timezone,
			DateFormat: prefs.DateFormat,
			Consent:    consent,
		}
	}

	// Audit — LGPD Art. 9 §1º: o exercício do direito deve ser
	// comprovável. Gravamos a invocação para que, em fiscalização,
	// possamos demonstrar que o usuário X teve acesso aos próprios
	// dados no momento Y.
	audit.Record(c.Request.Context(), audit.Entry{
		Action:     "lgpd.data_export_requested",
		EntityType: "User",
		EntityID:   userID,
		Metadata: map[string]any{
			"has_identity":    out.Identity != nil,
			"has_preferences": out.Preferences != nil,
		},
	})

	response.Success(c, "LGPD personal data exported", out)
}

// DeleteMyAccountRequest — payload com confirmação textual. LGPD não
// exige, mas operações irreversíveis pedem fricção UX explícita pra
// evitar deleção acidental (click errado, browser autofill).
type DeleteMyAccountRequest struct {
	// Confirm — texto literal "EXCLUIR MINHA CONTA" exigido pelo
	// frontend. Backend re-valida pra impedir bypass.
	Confirm string `json:"confirm" binding:"required"`
	// Reason — motivo livre. Opcional mas LGPD exige rastrear pra que
	// possamos identificar padrões (ex.: bug específico levando users
	// a sair).
	Reason string `json:"reason,omitempty"`
}

// DeleteMyAccount godoc
// @Summary      LGPD Art. 18 — direito ao esquecimento (#476)
// @Description  Anonimiza os dados pessoais do usuário corrente em
// @Description  TODOS os domains registrados (auth, crm.lead, deal.party,
// @Description  etc.). PII é substituído por hash determinístico do
// @Description  pkg/gdpr; foreign keys (account_id) preservadas pra que
// @Description  histórico de negócio continue íntegro. Requer
// @Description  confirmação textual no body. Irreversível.
// @Tags         lgpd
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body DeleteMyAccountRequest true "Confirmação"
// @Success      200 {object} response.Response
// @Failure      400 {object} response.Response "confirmação inválida"
// @Failure      401 {object} response.Response
// @Router       /lgpd/me/account [delete]
func (h *LGPDHandler) DeleteMyAccount(c *gin.Context) {
	userIDVal, _ := c.Get("user_id")
	userID, ok := userIDVal.(string)
	if !ok || userID == "" {
		response.Unauthorized(c, "user_id missing from token")
		return
	}

	var req DeleteMyAccountRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	// Confirmação textual — case-sensitive, sem acentuação, sem
	// margem para autofill. Erro 400 com mensagem clara ajuda UX
	// (frontend pode espelhar a string esperada).
	const expectedConfirm = "EXCLUIR MINHA CONTA"
	if strings.TrimSpace(req.Confirm) != expectedConfirm {
		response.ValidationError(c, "Para confirmar, envie confirm='"+expectedConfirm+"'")
		return
	}

	// Audit ANTES da execução — se a anonymização travar a metade,
	// o trail ainda registra a intenção. Garantia comprobatória de
	// quem pediu, quando, e por quê (LGPD Art. 18 §1º).
	audit.Record(c.Request.Context(), audit.Entry{
		Action:     "lgpd.account_deletion_requested",
		EntityType: "User",
		EntityID:   userID,
		Metadata: map[string]any{
			"reason_length": len(req.Reason),
			// Razão não vai no audit por LGPD (texto livre pode ter PII
			// do próprio user — ex.: "vou pedir reembolso porque CPF
			// X..."). Só o comprimento, para análise agregada se quiser.
		},
	})

	// Executa anonymização cross-service. Cada Anonymizer registrado
	// roda independente; falhas parciais são preservadas pro retry.
	results := gdpr.AnonymizeAccount(c.Request.Context(), userID)

	// Summary do resultado pra response. Se algum domain falhou,
	// expomos o erro pro user retentar — mas audit já gravou.
	var failed []string
	var totalTouched int
	domainSummary := make(map[string]int, len(results))
	for _, r := range results {
		domainSummary[r.Domain] = r.RowsTouched
		totalTouched += r.RowsTouched
		if r.Err != nil {
			failed = append(failed, r.Domain+": "+r.Err.Error())
		}
	}

	// Audit DEPOIS com o sumário — operadores veem antes/depois.
	audit.Record(c.Request.Context(), audit.Entry{
		Action:     "lgpd.account_anonymized",
		EntityType: "User",
		EntityID:   userID,
		Metadata: map[string]any{
			"domains":       domainSummary,
			"total_touched": totalTouched,
			"failed":        failed,
		},
	})

	if len(failed) > 0 {
		// Não 500 — anonimização parcial é o estado real. Frontend
		// vê o que falhou e pode chamar de novo (idempotente).
		response.Success(c, "Account partially anonymized; retry to complete failed domains", gin.H{
			"summary":       domainSummary,
			"total_touched": totalTouched,
			"failed":        failed,
		})
		return
	}

	response.Success(c, "Account anonymized. Você foi deslogado.", gin.H{
		"summary":       domainSummary,
		"total_touched": totalTouched,
	})
}
