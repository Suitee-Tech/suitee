package services

import (
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// PartyFieldCatalog devolve o catálogo de campos mapeáveis da party — "tudo que
// for campo mapeado na plataforma" para o alvo do documento. O toca-ai extrai
// exatamente estes campos; Target diz onde cada valor é gravado (party.* direto
// ou party.custom_fields.*). É a fonte única do que a IA tenta identificar.
func PartyFieldCatalog(kind deal.PartyKind) []clients.AIFieldSpec {
	common := []clients.AIFieldSpec{
		{Name: "name", Label: partyNameLabel(kind), Type: "text", Target: "party.name", Required: true},
		{Name: "document", Label: partyDocLabel(kind), Type: "cpf_cnpj", Target: "party.document", Required: true},
		{Name: "email", Label: "E-mail", Type: "email", Target: "party.email"},
		{Name: "phone", Label: "Telefone", Type: "phone", Target: "party.phone"},
		// Endereço -> custom_fields (a party não tem campos de endereço dedicados).
		{Name: "cep", Label: "CEP", Type: "cep", Target: "party.custom_fields.cep"},
		{Name: "logradouro", Label: "Logradouro", Type: "text", Target: "party.custom_fields.logradouro"},
		{Name: "numero", Label: "Número", Type: "text", Target: "party.custom_fields.numero"},
		{Name: "complemento", Label: "Complemento", Type: "text", Target: "party.custom_fields.complemento"},
		{Name: "bairro", Label: "Bairro", Type: "text", Target: "party.custom_fields.bairro"},
		{Name: "cidade", Label: "Cidade", Type: "text", Target: "party.custom_fields.cidade"},
		{Name: "uf", Label: "UF", Type: "uf", Target: "party.custom_fields.uf"},
	}

	if kind == deal.PartyKindCompany {
		return append(common,
			clients.AIFieldSpec{Name: "razao_social", Label: "Razão social", Type: "text", Target: "party.custom_fields.razao_social"},
			clients.AIFieldSpec{Name: "nome_fantasia", Label: "Nome fantasia", Type: "text", Target: "party.custom_fields.nome_fantasia"},
			clients.AIFieldSpec{Name: "inscricao_estadual", Label: "Inscrição estadual", Type: "text", Target: "party.custom_fields.inscricao_estadual"},
		)
	}

	// Pessoa física.
	return append(common,
		clients.AIFieldSpec{Name: "birth_date", Label: "Data de nascimento", Type: "date", Target: "party.birth_date"},
		clients.AIFieldSpec{Name: "rg", Label: "RG", Type: "text", Target: "party.custom_fields.rg"},
		clients.AIFieldSpec{Name: "nacionalidade", Label: "Nacionalidade", Type: "text", Target: "party.custom_fields.nacionalidade"},
		clients.AIFieldSpec{Name: "estado_civil", Label: "Estado civil", Type: "text", Target: "party.custom_fields.estado_civil"},
		clients.AIFieldSpec{Name: "profissao", Label: "Profissão", Type: "text", Target: "party.custom_fields.profissao"},
	)
}

func partyNameLabel(kind deal.PartyKind) string {
	if kind == deal.PartyKindCompany {
		return "Razão social"
	}
	return "Nome completo"
}

func partyDocLabel(kind deal.PartyKind) string {
	if kind == deal.PartyKindCompany {
		return "CNPJ"
	}
	return "CPF"
}
