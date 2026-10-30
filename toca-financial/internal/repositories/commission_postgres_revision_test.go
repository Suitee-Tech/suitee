package repositories

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeVersionedCommissionUsesPhysicalRevision(t *testing.T) {
	metadataRevision := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	physicalRevision := metadataRevision.Add(5 * time.Minute)
	encoded, err := json.Marshal(&commission.Commission{ID: "commission-revision", UpdatedAt: metadataRevision})
	require.NoError(t, err)

	decoded, err := decodeVersionedCommission(encoded, physicalRevision)

	require.NoError(t, err)
	assert.Equal(t, physicalRevision, decoded.UpdatedAt)
	assert.NotEqual(t, metadataRevision, decoded.UpdatedAt)
}
