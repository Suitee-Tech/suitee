package deal

// ChecklistTemplate is a named bundle of checklist items operators can
// apply to a Deal in one click (see DealService / DocumentationTab UI).
//
// Templates are mirrored from the historical FE-only constant
// (toka/src/app/data.ts:DEFAULT_CHECKLIST_TEMPLATES). Moving the
// source of truth to the domain layer is roadmap §4/W4 — this is the
// first step: a `GET /api/v1/checklist-templates` endpoint that hands
// back the built-in set so the FE can stop hardcoding it. Persisted
// per-tenant customisation lives behind a migration and admin UI yet
// to land; both will plug into this same struct.
type ChecklistTemplate struct {
	ID    string                  `json:"id"`
	Label string                  `json:"label"`
	Items []ChecklistTemplateItem `json:"items"`
}

type ChecklistTemplateItem struct {
	Name        string                `json:"name"`
	Category    ChecklistItemCategory `json:"category"`
	Required    bool                  `json:"required"`
	Description string                `json:"description,omitempty"`
	// RequestMode (opcional): "form" = apenas preenchimento de formulário
	// (sem anexo) — ex.: CPF e telefone digitados. Vazio = "upload".
	RequestMode ChecklistItemRequestMode `json:"request_mode,omitempty"`
	// RequestedFields (opcional) são os campos solicitados ao cliente
	// quando RequestMode = "form" (ou complemento do anexo).
	RequestedFields []ChecklistRequestedField `json:"requested_fields,omitempty"`
}

// BuiltinChecklistTemplates returns the canonical template set bundled
// with the application. Operators get these out-of-the-box; custom
// per-tenant templates (once persistent storage lands) layer on top.
//
// Keep the IDs aligned with `ChecklistTemplateId` in
// `toka/src/app/data.ts` — the FE keys saved selections by these
// strings, so renaming silently breaks existing operator state.
func BuiltinChecklistTemplates() []ChecklistTemplate {
	return []ChecklistTemplate{
		{
			ID:    "pf_compra_financiamento",
			Label: "PF — Compra com Financiamento",
			Items: []ChecklistTemplateItem{
				{Name: "Documento de Identidade (RG/CNH)", Category: CategoryPessoal, Required: true},
				{Name: "CPF", Category: CategoryPessoal, Required: true, RequestMode: RequestModeForm, RequestedFields: []ChecklistRequestedField{{Key: "cpf", Label: "CPF", Type: "text", Required: true}}},
				{Name: "Certidão de Nascimento ou Casamento", Category: CategoryPessoal, Required: true},
				{Name: "Comprovante de Residência", Category: CategoryPessoal, Required: true},
				{Name: "Comprovante de Renda (3 últimos meses)", Category: CategoryFinanceiro, Required: true},
				{Name: "Declaração de Imposto de Renda Completa", Category: CategoryFinanceiro, Required: true},
				{Name: "Extrato do FGTS", Category: CategoryFinanceiro, Required: true},
				{Name: "Carteira de Trabalho (páginas de identificação)", Category: CategoryFinanceiro, Required: false},
				{Name: "Telefone/WhatsApp", Category: CategoryPessoal, Required: false, RequestMode: RequestModeForm, RequestedFields: []ChecklistRequestedField{{Key: "phone", Label: "Telefone/WhatsApp", Type: "text", Required: true}}},
			},
		},
		{
			ID:    "pf_compra_avista",
			Label: "PF — Compra à Vista",
			Items: []ChecklistTemplateItem{
				{Name: "Documento de Identidade (RG/CNH)", Category: CategoryPessoal, Required: true},
				{Name: "CPF", Category: CategoryPessoal, Required: true, RequestMode: RequestModeForm, RequestedFields: []ChecklistRequestedField{{Key: "cpf", Label: "CPF", Type: "text", Required: true}}},
				{Name: "Certidão de Nascimento ou Casamento", Category: CategoryPessoal, Required: true},
				{Name: "Comprovante de Residência", Category: CategoryPessoal, Required: true},
				{Name: "Comprovante de Renda ou Declaração de IR", Category: CategoryFinanceiro, Required: true},
			},
		},
		{
			ID:    "pj_compra",
			Label: "PJ — Compra",
			Items: []ChecklistTemplateItem{
				{Name: "Contrato Social e Alterações", Category: CategoryJuridico, Required: true},
				{Name: "Cartão CNPJ", Category: CategoryJuridico, Required: true},
				{Name: "Procuração do Representante Legal", Category: CategoryJuridico, Required: true},
				{Name: "Documentos dos Sócios (RG/CPF)", Category: CategoryPessoal, Required: true},
				{Name: "Balanço Patrimonial (último exercício)", Category: CategoryFinanceiro, Required: true},
				{Name: "Certidões Negativas (Federal, Estadual, Municipal)", Category: CategoryJuridico, Required: true},
			},
		},
		{
			ID:    "locacao_pf",
			Label: "PF — Locação",
			Items: []ChecklistTemplateItem{
				{Name: "Documento de Identidade (RG/CNH)", Category: CategoryPessoal, Required: true},
				{Name: "CPF", Category: CategoryPessoal, Required: true, RequestMode: RequestModeForm, RequestedFields: []ChecklistRequestedField{{Key: "cpf", Label: "CPF", Type: "text", Required: true}}},
				{Name: "Comprovante de Residência Atual", Category: CategoryPessoal, Required: true},
				{Name: "Comprovante de Renda (3 últimos meses)", Category: CategoryFinanceiro, Required: true},
				{
					Name:        "Documentos do Fiador (RG, CPF, Renda, Matrícula do Imóvel)",
					Category:    CategoryPessoal,
					Required:    false,
					Description: "Caso a garantia seja fiança",
				},
				{Name: "Telefone/WhatsApp", Category: CategoryPessoal, Required: false, RequestMode: RequestModeForm, RequestedFields: []ChecklistRequestedField{{Key: "phone", Label: "Telefone/WhatsApp", Type: "text", Required: true}}},
			},
		},
		{
			ID:    "locacao_pj",
			Label: "PJ — Locação",
			Items: []ChecklistTemplateItem{
				{Name: "Contrato Social e Alterações", Category: CategoryJuridico, Required: true},
				{Name: "Cartão CNPJ", Category: CategoryJuridico, Required: true},
				{Name: "Documentos dos Sócios (RG/CPF)", Category: CategoryPessoal, Required: true},
				{Name: "Comprovante de Endereço da Sede", Category: CategoryPessoal, Required: true},
				{Name: "Balanço ou Faturamento Recente", Category: CategoryFinanceiro, Required: true},
			},
		},
	}
}

// BuiltinChecklistTemplateByID is a convenience lookup matching the FE
// `ChecklistTemplateId` union — useful for handlers that need to apply
// a single template without iterating the slice every time. Returns
// nil for unknown IDs so the caller can decide whether to 404 or fall
// back to the empty checklist.
func BuiltinChecklistTemplateByID(id string) *ChecklistTemplate {
	for _, t := range BuiltinChecklistTemplates() {
		if t.ID == id {
			return &t
		}
	}
	return nil
}
