package repository

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/oginoino/toca/toca-comm/internal/domain/message"
)

// Resolve only identities supplied by an authorized channel. CRM data is scoped
// to that channel's tenant; account data exposes only name and avatar.
func (r *messagePostgresRepository) GetParticipantProfiles(ctx context.Context, tenantID string, ids []string) ([]message.ParticipantProfile, error) {
	valid := []string{}
	for _, id := range ids {
		if _, err := uuid.Parse(id); err == nil {
			valid = append(valid, id)
		}
	}
	if len(valid) == 0 {
		return nil, nil
	}
	payload, _ := json.Marshal(valid)
	rows, err := r.db.QueryServiceRows(ctx, `
 WITH requested AS (SELECT jsonb_array_elements_text($2::jsonb) AS id)
 SELECT p.id,
 COALESCE(NULLIF(l.metadata->>'Name', ''), NULLIF(b.metadata->>'Name', ''),
 NULLIF(a.metadata->'Individual'->>'FullName', ''),
 NULLIF(trim(concat_ws(' ', a.metadata->'Individual'->>'FirstName', a.metadata->'Individual'->>'LastName')), ''), '') AS name,
 COALESCE(NULLIF(a.metadata->'Individual'->>'AvatarURL', ''), NULLIF(l.metadata->>'Avatar', ''), NULLIF(b.metadata->>'Avatar', ''), '') AS avatar
 FROM requested p
 LEFT JOIN toca_auth.accounts a ON a.id = p.id::uuid
 LEFT JOIN LATERAL (
 SELECT metadata FROM crm.leads
 WHERE tenant_id = $1 AND deleted_at IS NULL
 AND (id = p.id::uuid OR metadata->>'AccountID' = p.id)
 ORDER BY updated_at DESC, id LIMIT 1
 ) l ON true
 LEFT JOIN LATERAL (
 SELECT metadata FROM crm.brokers
 WHERE tenant_id = $1 AND (id = p.id::uuid OR
 (NULLIF(a.metadata->'Individual'->>'Email', '') IS NOT NULL AND lower(metadata->>'Email') = lower(a.metadata->'Individual'->>'Email')))
 ORDER BY updated_at DESC, id LIMIT 1
 ) b ON true`, tenantID, string(payload))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	profiles := []message.ParticipantProfile{}
	for rows.Next() {
		var p message.ParticipantProfile
		if err := rows.Scan(&p.ID, &p.Name, &p.AvatarURL); err != nil {
			return nil, err
		}
		if p.Name != "" || p.AvatarURL != "" {
			profiles = append(profiles, p)
		}
	}
	return profiles, rows.Err()
}
