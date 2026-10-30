package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/oginoino/toca-auth/internal/domain/group"
	"github.com/oginoino/toca/pkg/database"
)

type groupPostgresRepository struct {
	db *database.PostgresDB
}

func NewGroupPostgresRepository(db *database.PostgresDB) group.Repository {
	return &groupPostgresRepository{db: db}
}

func (r *groupPostgresRepository) Create(ctx context.Context, g *group.Group) error {
	if g.CreatedAt.IsZero() {
		g.CreatedAt = time.Now()
	}
	g.UpdatedAt = time.Now()
	metadata, _ := json.Marshal(g)
	_, err := r.db.ExecContext(ctx, `INSERT INTO toca_auth.groups (id, company_id, metadata, created_at, updated_at) VALUES ($1, $2, $3, $4, $5)`, g.ID, g.CompanyID, string(metadata), g.CreatedAt, g.UpdatedAt)
	return err
}

func (r *groupPostgresRepository) GetByID(ctx context.Context, id string) (*group.Group, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM toca_auth.groups WHERE id = $1", id).Scan(&metadata)
	if err != nil {
		return nil, nil
	}
	var g group.Group
	_ = json.Unmarshal(metadata, &g)
	return &g, nil
}

func (r *groupPostgresRepository) ListByCompanyID(ctx context.Context, companyID string) ([]*group.Group, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT metadata FROM toca_auth.groups WHERE company_id = $1 ORDER BY created_at ASC", companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*group.Group
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var g group.Group
		if err := json.Unmarshal(metadata, &g); err != nil {
			return nil, err
		}
		groups = append(groups, &g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return groups, nil
}

func (r *groupPostgresRepository) Update(ctx context.Context, g *group.Group) error {
	g.UpdatedAt = time.Now()
	metadata, _ := json.Marshal(g)
	// P2/item 36: a escrita é escopada pela EMPRESA (company_id é o tenant nas
	// tabelas de identidade do toca_auth).
	_, err := r.db.ExecContext(ctx, "UPDATE toca_auth.groups SET metadata = $1, updated_at = $2 WHERE id = $3 AND company_id = $4", string(metadata), g.UpdatedAt, g.ID, g.CompanyID)
	return err
}

func (r *groupPostgresRepository) Delete(ctx context.Context, companyID, id string) error {
	companyID = strings.TrimSpace(companyID)
	if companyID == "" {
		return fmt.Errorf("group not found: tenant context required")
	}
	_, err := r.db.ExecContext(ctx, "DELETE FROM toca_auth.groups WHERE id = $1 AND company_id = $2", id, companyID)
	return err
}

func (r *groupPostgresRepository) AddMember(ctx context.Context, groupID string, member group.GroupMember) error {
	g, err := r.GetByID(ctx, groupID)
	if err != nil {
		return err
	}
	if g == nil {
		return nil
	}
	g.Members = append(g.Members, member)
	return r.Update(ctx, g)
}

func (r *groupPostgresRepository) RemoveMember(ctx context.Context, groupID, accountID string) error {
	g, err := r.GetByID(ctx, groupID)
	if err != nil {
		return err
	}
	if g == nil {
		return nil
	}
	next := make([]group.GroupMember, 0, len(g.Members))
	for _, m := range g.Members {
		if m.AccountID != accountID {
			next = append(next, m)
		}
	}
	g.Members = next
	return r.Update(ctx, g)
}

func (r *groupPostgresRepository) RemoveMemberFromAllGroups(ctx context.Context, companyID, accountID string) error {
	groups, err := r.ListByCompanyID(ctx, companyID)
	if err != nil {
		return err
	}
	for _, g := range groups {
		next := make([]group.GroupMember, 0, len(g.Members))
		changed := false
		for _, m := range g.Members {
			if m.AccountID != accountID {
				next = append(next, m)
			} else {
				changed = true
			}
		}
		if changed {
			g.Members = next
			if err := r.Update(ctx, g); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *groupPostgresRepository) GetByMemberAndCompany(ctx context.Context, memberID, companyID string) ([]*group.Group, error) {
	groups, err := r.ListByCompanyID(ctx, companyID)
	if err != nil {
		return nil, err
	}
	var result []*group.Group
	for _, g := range groups {
		for _, m := range g.Members {
			if m.AccountID == memberID {
				result = append(result, g)
				break
			}
		}
	}
	return result, nil
}
