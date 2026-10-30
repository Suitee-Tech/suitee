package lead

import (
	"testing"

	"github.com/oginoino/toca/toca-crm/internal/domain/broker"
)

func TestConfigWarnings(t *testing.T) {
	withWeight := &broker.Broker{ID: "w-1", Weight: 1.5}
	withoutWeight := &broker.Broker{ID: "p-1"}
	priorityBroker := &broker.Broker{ID: "pri-1", IsPriority: true}
	regularBroker := &broker.Broker{ID: "reg-1"}

	cases := []struct {
		name     string
		cfg      *RouletteConfig
		lookup   map[string]*broker.Broker
		mustHave []string // substrings the warning list must contain
		mustNot  []string // substrings the warning list must NOT contain
	}{
		{
			name: "nil cfg returns nil",
			cfg:  nil,
		},
		{
			name: "enabled without any pool surfaces banner",
			cfg:  &RouletteConfig{Enabled: true},
			mustHave: []string{
				"Roleta está habilitada mas nenhum corretor",
			},
		},
		{
			name: "weighted without weighted broker warns",
			cfg: &RouletteConfig{
				Enabled:       true,
				DistMethod:    DistMethodWeighted,
				ActiveBrokers: []string{"p-1"},
			},
			lookup: map[string]*broker.Broker{"p-1": withoutWeight},
			mustHave: []string{
				"weighted",
				"distribuição efetivamente aleatória",
			},
		},
		{
			name: "weighted with at least one weighted broker is silent",
			cfg: &RouletteConfig{
				Enabled:       true,
				DistMethod:    DistMethodWeighted,
				ActiveBrokers: []string{"w-1", "p-1"},
			},
			lookup:  map[string]*broker.Broker{"w-1": withWeight, "p-1": withoutWeight},
			mustNot: []string{"weighted"},
		},
		{
			name: "priority without priority broker warns",
			cfg: &RouletteConfig{
				Enabled:       true,
				DistMethod:    DistMethodPriority,
				ActiveBrokers: []string{"reg-1"},
			},
			lookup: map[string]*broker.Broker{"reg-1": regularBroker},
			mustHave: []string{
				"priority",
				"cai para sequential",
			},
		},
		{
			name: "priority with at least one priority broker is silent",
			cfg: &RouletteConfig{
				Enabled:       true,
				DistMethod:    DistMethodPriority,
				ActiveBrokers: []string{"reg-1", "pri-1"},
			},
			lookup:  map[string]*broker.Broker{"reg-1": regularBroker, "pri-1": priorityBroker},
			mustNot: []string{"priority"},
		},
		{
			name: "empty stage pool surfaces banner",
			cfg: &RouletteConfig{
				Enabled:          true,
				StageBrokerPools: map[string][]string{"NEW": {}, "CONTACTED": {"any"}},
			},
			lookup: map[string]*broker.Broker{"any": regularBroker},
			mustHave: []string{
				"\"NEW\"",
				"pool vazio",
			},
		},
		{
			name: "stage pool with only deleted brokers warns",
			cfg: &RouletteConfig{
				Enabled:          true,
				StageBrokerPools: map[string][]string{"NEW": {"gone-1", "gone-2"}},
			},
			lookup: map[string]*broker.Broker{},
			mustHave: []string{
				"\"NEW\"",
				"inativos ou foram removidos",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ConfigWarnings(tc.cfg, tc.lookup)
			joined := ""
			for _, w := range got {
				joined += w + "\n"
			}
			for _, needle := range tc.mustHave {
				if !contains(joined, needle) {
					t.Fatalf("missing %q in warnings:\n%s", needle, joined)
				}
			}
			for _, forbidden := range tc.mustNot {
				if contains(joined, forbidden) {
					t.Fatalf("unexpected %q in warnings:\n%s", forbidden, joined)
				}
			}
		})
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
