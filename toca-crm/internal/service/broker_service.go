package service

import (
	"context"
	"sync"
)

// BrokerService handles broker availability and listing
// In a real scenario, this would call toca-auth or toca-core to get users with role 'BROKER'
type BrokerService interface {
	GetAvailableBrokers(ctx context.Context, tenantID string) ([]string, error)
}

// InMemoryBrokerService is a stub implementation
type InMemoryBrokerService struct {
	// Simple map of tenantID -> list of broker IDs
	brokers map[string][]string
}

func NewInMemoryBrokerService() *InMemoryBrokerService {
	return &InMemoryBrokerService{
		brokers: map[string][]string{
			"default": {"broker-1", "broker-2", "broker-3"},
		},
	}
}

func (s *InMemoryBrokerService) GetAvailableBrokers(ctx context.Context, tenantID string) ([]string, error) {
	// Return stub data or empty list
	if brokers, ok := s.brokers[tenantID]; ok {
		return brokers, nil
	}
	// Fallback for development
	return []string{"broker-1", "broker-2"}, nil
}

// RouletteDistribution implements Round-Robin distribution
type RouletteDistribution struct {
	mu           sync.Mutex
	currentIndex map[string]int // tenantID -> current index
}

func NewRouletteDistribution() *RouletteDistribution {
	return &RouletteDistribution{
		currentIndex: make(map[string]int),
	}
}

func (r *RouletteDistribution) NextBroker(tenantID string, brokers []string) string {
	if len(brokers) == 0 {
		return ""
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	idx := r.currentIndex[tenantID]
	if idx >= len(brokers) {
		idx = 0
	}

	brokerID := brokers[idx]
	r.currentIndex[tenantID] = (idx + 1) % len(brokers)

	return brokerID
}
