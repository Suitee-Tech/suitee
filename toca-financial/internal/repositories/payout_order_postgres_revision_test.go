package repositories

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/payout_order"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeVersionedPayoutOrderUsesPhysicalRevision(t *testing.T) {
	metadataRevision := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	physicalRevision := metadataRevision.Add(5 * time.Minute)
	encoded, err := json.Marshal(&payout_order.PayoutOrder{ID: "payout-revision", UpdatedAt: metadataRevision})
	require.NoError(t, err)

	decoded, err := decodeVersionedPayoutOrder(encoded, physicalRevision)

	require.NoError(t, err)
	assert.Equal(t, physicalRevision, decoded.UpdatedAt)
	assert.NotEqual(t, metadataRevision, decoded.UpdatedAt)
}

func TestSourceEventLookupSelectsLatestActivePayout(t *testing.T) {
	query := strings.ToUpper(getActivePayoutOrderBySourceEventQuery)
	assert.Contains(t, query, "<> 'CANCELLED'")
	assert.Contains(t, query, "ORDER BY CREATED_AT DESC")
	assert.Contains(t, query, "LIMIT 1")
}
