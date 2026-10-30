package lead

import (
	"fmt"

	"github.com/oginoino/toca/toca-crm/internal/domain/broker"
)

// ConfigWarnings retorna mensagens humanas para configurações tecnicamente
// válidas que provavelmente não terão o efeito esperado em runtime — o que
// o review chamou de "silent misconfiguration trap".
//
// Casos cobertos hoje:
//   - distMethod=weighted mas nenhum broker no pool tem Weight > 0 → todos
//     viram peso 1.0 (uniforme random), efetivamente sequential aleatório.
//   - distMethod=priority mas nenhum broker no pool tem IsPriority=true →
//     o algoritmo cai pra sequential silenciosamente.
//   - Pool de uma stage está vazio ou todos os brokers configurados estão
//     ausentes do brokerLookup (foram deletados/desativados).
//   - Roleta ENABLED mas ActiveBrokers vazio e nenhuma StageBrokerPools
//     configurada — o algoritmo nunca encontra um pool válido.
//
// brokerLookup é o mesmo map que o picker usa (broker.Repository.GetByIDs).
// Pode ser nil para uma checagem "rasa" que só inspeciona a config; nesse
// caso, regras que dependem de Weight/IsPriority são puladas.
//
// Wave P1.4 — fixa o "silent misconfiguration trap" detectado no review.
// O FE exibe estas mensagens como banner para o admin saber que precisa
// configurar Weight ou IsPriority nos brokers para o método escolhido
// funcionar de verdade.
func ConfigWarnings(cfg *RouletteConfig, brokerLookup map[string]*broker.Broker) []string {
	if cfg == nil {
		return nil
	}
	var out []string

	// 1) Roleta enabled mas sem pool configurada em lugar nenhum.
	if cfg.Enabled && len(cfg.ActiveBrokers) == 0 && len(cfg.StageBrokerPools) == 0 {
		out = append(out, "Roleta está habilitada mas nenhum corretor está configurado. Adicione brokers em \"Corretores ativos\" ou crie pools por etapa.")
	}

	// 2) StageBrokerPools com pools vazios ou referenciando brokers ausentes.
	for stage, pool := range cfg.StageBrokerPools {
		if len(pool) == 0 {
			out = append(out, fmt.Sprintf("A etapa \"%s\" está configurada com pool vazio — leads nessa etapa caem no fallback global.", stage))
			continue
		}
		if brokerLookup != nil {
			missing := 0
			for _, id := range pool {
				if _, ok := brokerLookup[id]; !ok {
					missing++
				}
			}
			if missing == len(pool) {
				out = append(out, fmt.Sprintf("Todos os corretores configurados na etapa \"%s\" estão inativos ou foram removidos.", stage))
			}
		}
	}

	// 3) Weighted/Priority sem dados de suporte. Só vale se houver pool.
	if brokerLookup != nil && hasAnyPool(cfg) {
		switch cfg.DistMethod {
		case DistMethodWeighted:
			if !anyBrokerHasWeight(cfg, brokerLookup) {
				out = append(out, "Método de distribuição \"weighted\" está configurado, mas nenhum corretor do pool tem peso > 0. Resultado: distribuição efetivamente aleatória uniforme. Configure pesos por corretor para que o método tenha efeito.")
			}
		case DistMethodPriority:
			if !anyBrokerHasPriority(cfg, brokerLookup) {
				out = append(out, "Método de distribuição \"priority\" está configurado, mas nenhum corretor do pool está marcado como prioridade. Resultado: distribuição cai para sequential. Marque ao menos um corretor como prioridade para que o método tenha efeito.")
			}
		}
	}

	return out
}

func hasAnyPool(cfg *RouletteConfig) bool {
	if len(cfg.ActiveBrokers) > 0 {
		return true
	}
	for _, p := range cfg.StageBrokerPools {
		if len(p) > 0 {
			return true
		}
	}
	return false
}

func brokersInScope(cfg *RouletteConfig) []string {
	seen := make(map[string]bool, len(cfg.ActiveBrokers))
	out := make([]string, 0, len(cfg.ActiveBrokers))
	for _, id := range cfg.ActiveBrokers {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, pool := range cfg.StageBrokerPools {
		for _, id := range pool {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out
}

func anyBrokerHasWeight(cfg *RouletteConfig, lookup map[string]*broker.Broker) bool {
	for _, id := range brokersInScope(cfg) {
		if b, ok := lookup[id]; ok && b != nil && b.Weight > 0 {
			return true
		}
	}
	return false
}

func anyBrokerHasPriority(cfg *RouletteConfig, lookup map[string]*broker.Broker) bool {
	for _, id := range brokersInScope(cfg) {
		if b, ok := lookup[id]; ok && b != nil && b.IsPriority {
			return true
		}
	}
	return false
}
