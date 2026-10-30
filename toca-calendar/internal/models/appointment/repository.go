package appointment

import "context"

type ListFilters struct {
	AgentID    string
	PropertyID string
	LeadID     string
	DealID     string
	Date       string
	StartDate  string
	EndDate    string
	Status     string
	Type       string
	ClientName string
	AgentName  string
	Priority   string
	Search     string
	// AccessAgentID and TeamScope are authorization filters, applied before
	// COUNT/LIMIT so pagination can never reveal or skip inaccessible events.
	AccessAgentID string
	TeamScope     bool
	Limit         int64
	Offset        int64
}

type Repository interface {
	Create(ctx context.Context, appt *Appointment) error
	GetByID(ctx context.Context, id string) (*Appointment, error)
	// GetByIDForLink atende o link assinado do e-mail, sem claims no contexto.
	GetByIDForLink(ctx context.Context, id string) (*Appointment, error)
	Update(ctx context.Context, appt *Appointment) error
	Delete(ctx context.Context, tenantID, id string) error
	List(ctx context.Context, tenantID string, filters ListFilters) ([]*Appointment, int64, error)
	FindConflicts(ctx context.Context, tenantID, agentID, date, startTime, endTime, excludeID string) ([]*Appointment, error)
}
