package services

import (
	"context"
	"errors"
	"testing"

	"github.com/oginoino/toca-core/internal/addressverification"
	"github.com/oginoino/toca-core/internal/domain/core/address"
	"github.com/oginoino/toca-core/internal/domain/core/property"
	appctx "github.com/oginoino/toca/pkg/context"
)

type propertyRepoStub struct {
	createCalls int
	updateCalls int
}

func (r *propertyRepoStub) Create(context.Context, *property.Property) error {
	r.createCalls++
	return nil
}

func (*propertyRepoStub) GetByID(context.Context, string, string) (*property.Property, error) {
	return nil, errors.New("not implemented")
}

func (*propertyRepoStub) GetByIDs(context.Context, []string, string) ([]*property.Property, error) {
	return nil, errors.New("not implemented")
}

func (*propertyRepoStub) GetAll(context.Context, property.PropertyFilter) ([]*property.Property, int64, error) {
	return nil, 0, errors.New("not implemented")
}

func (r *propertyRepoStub) Update(context.Context, *property.Property) error {
	r.updateCalls++
	return nil
}

func (*propertyRepoStub) AppendImage(context.Context, string, string, property.Image) (*property.Property, error) {
	return nil, errors.New("not implemented")
}

func (*propertyRepoStub) Delete(context.Context, string, string) error {
	return errors.New("not implemented")
}

func (*propertyRepoStub) ListSharedForPartner(context.Context, string, []string, []string) ([]*property.Property, error) {
	return nil, errors.New("not implemented")
}

type addressProcessorStub struct {
	calls int
	err   error
}

func (p *addressProcessorStub) Process(_ context.Context, value *address.Address) error {
	p.calls++
	if p.err != nil {
		return p.err
	}
	value.PostalCode = "01001000"
	value.IsVerified = true
	return nil
}

func TestPropertyServiceProcessesAddressBeforeCreateAndUpdate(t *testing.T) {
	repo := &propertyRepoStub{}
	processor := &addressProcessorStub{}
	service := NewPropertyService(repo)
	service.SetAddressProcessor(processor)
	ctx := appctx.WithTenantID(context.Background(), "tenant-1")

	created := &property.Property{}
	if err := service.Create(ctx, created); err != nil {
		t.Fatal(err)
	}
	updated := &property.Property{ID: "property-1"}
	if err := service.Update(ctx, updated); err != nil {
		t.Fatal(err)
	}

	if processor.calls != 2 {
		t.Fatalf("processor calls = %d, want 2", processor.calls)
	}
	if repo.createCalls != 1 || repo.updateCalls != 1 {
		t.Fatalf("repo calls create=%d update=%d, want 1 each", repo.createCalls, repo.updateCalls)
	}
	if !created.Address.IsVerified || created.Address.PostalCode != "01001000" {
		t.Fatalf("create did not persist processed address: %+v", created.Address)
	}
	if !updated.Address.IsVerified || updated.Address.PostalCode != "01001000" {
		t.Fatalf("update did not persist processed address: %+v", updated.Address)
	}
}

func TestPropertyServiceDoesNotPersistWhenAddressProcessingFails(t *testing.T) {
	repo := &propertyRepoStub{}
	processor := &addressProcessorStub{err: addressverification.ErrPostalCodeNotFound}
	service := NewPropertyService(repo)
	service.SetAddressProcessor(processor)
	ctx := appctx.WithTenantID(context.Background(), "tenant-1")

	err := service.Create(ctx, &property.Property{})
	if !errors.Is(err, addressverification.ErrPostalCodeNotFound) {
		t.Fatalf("error = %v, want ErrPostalCodeNotFound", err)
	}
	if repo.createCalls != 0 {
		t.Fatalf("repository called %d times after address failure", repo.createCalls)
	}
}
