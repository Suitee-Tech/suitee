package services

import (
	"context"
	"errors"
	"testing"

	"github.com/oginoino/toca/pkg/events/contracts"
)

type fakeSyncer struct {
	tenant   string
	property string
	err      error
	calls    int
}

func (f *fakeSyncer) HandlePropertyEvent(_ context.Context, tenantID, propertyID string) error {
	f.calls++
	f.tenant, f.property = tenantID, propertyID
	return f.err
}

func TestPropertyRefFromPayloadAcceptsTypedAndMap(t *testing.T) {
	typed := contracts.PropertyUpdatedEventPayload{TenantID: testTenant, PropertyID: testProperty}
	if tenant, property := propertyRefFromPayload(typed); tenant != testTenant || property != testProperty {
		t.Fatalf("typed: %q %q", tenant, property)
	}
	asMap := map[string]interface{}{"tenant_id": testTenant, "property_id": testProperty}
	if tenant, property := propertyRefFromPayload(asMap); tenant != testTenant || property != testProperty {
		t.Fatalf("map: %q %q", tenant, property)
	}
	if tenant, property := propertyRefFromPayload("garbage"); tenant != "" || property != "" {
		t.Fatalf("payload inválido: %q %q", tenant, property)
	}
}

func TestPropertyConsumerDispatchesToSync(t *testing.T) {
	sync := &fakeSyncer{}
	consumer := NewPropertyConsumer(nil, sync)
	event := contracts.PropertyUpdatedEvent{
		EventIDVal: "evt-1",
		Data:       contracts.PropertyUpdatedEventPayload{TenantID: testTenant, PropertyID: testProperty},
	}
	if err := consumer.handleUploadedOrUpdated(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if sync.calls != 1 || sync.tenant != testTenant || sync.property != testProperty {
		t.Fatalf("sync=%+v", sync)
	}
}

func TestPropertyConsumerIgnoresIncompletePayload(t *testing.T) {
	sync := &fakeSyncer{}
	consumer := NewPropertyConsumer(nil, sync)
	event := contracts.PropertyUpdatedEvent{Data: contracts.PropertyUpdatedEventPayload{TenantID: testTenant}}
	if err := consumer.handleUploadedOrUpdated(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if sync.calls != 0 {
		t.Fatal("evento sem property_id não pode acionar o sync")
	}
}

func TestPropertyConsumerPropagatesSyncFailure(t *testing.T) {
	sync := &fakeSyncer{err: errors.New("db down")}
	consumer := NewPropertyConsumer(nil, sync)
	event := contracts.PropertyCapturedEvent{
		Data: contracts.PropertyCapturedEventPayload{TenantID: testTenant, PropertyID: testProperty},
	}
	if err := consumer.handleUploadedOrUpdated(context.Background(), event); err == nil {
		t.Fatal("falha do sync precisa subir para o RabbitMQ reentregar")
	}
}
