package repositories

import "fmt"

// brokerVisibility devolve o predicado SQL de visibilidade do corretor.
//
// Um negócio é do corretor quando ele é o RESPONSÁVEL (`broker_id`) ou quando
// ele PARTICIPA da divisão de comissão (`broker_participations`): captador,
// co-captador, visitador, atendente ou fechamento.
//
// O escopo por `broker_id` sozinho deixava o papel de participação decorativo:
// o corretor nomeado como captador e como responsável pelo fechamento não
// enxergava o negócio (lista vazia, detalhe 404) enquanto o perfil
// administrativo via tudo. A revisão de permissões mantém o corretor na própria
// carteira — a carteira passa a incluir os negócios que ele divide.
//
// A escrita continua na matriz de permissões (cada rota exige o seu
// `deals:*`); o que muda é a leitura, que é o que decide a Esteira e o detalhe.
// Excluir negócio segue restrito ao responsável (ver DeleteScoped).
func brokerVisibility(metadata string, argRef string) string {
	return fmt.Sprintf(
		"((%[1]s->>'broker_id') = %[2]s OR EXISTS (SELECT 1 FROM jsonb_array_elements(COALESCE(%[1]s->'broker_participations','[]'::jsonb)) p WHERE COALESCE(p->>'broker_id','') <> '' AND p->>'broker_id' = %[2]s))",
		metadata, argRef)
}
