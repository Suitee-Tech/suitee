package contract

import (
	"github.com/oginoino/toca/pkg/events/contracts"
)

// Use the central definition from pkg/events/contracts
type ContractSignedEvent = contracts.ContractSignedEvent
type ContractSignedEventPayload = contracts.ContractSignedEventPayload
type BrokerParticipationPayload = contracts.BrokerParticipation
type PartyPayload = contracts.PartyPayload

// Lifecycle events used by toca-financial / toca-comm to react to operator
// actions taken from the contract management surface.
type ContractTerminatedEvent = contracts.ContractTerminatedEvent
type ContractTerminatedEventPayload = contracts.ContractTerminatedEventPayload

type ContractReadjustedEvent = contracts.ContractReadjustedEvent
type ContractReadjustedEventPayload = contracts.ContractReadjustedEventPayload

type ContractAmendedEvent = contracts.ContractAmendedEvent
type ContractAmendedEventPayload = contracts.ContractAmendedEventPayload
