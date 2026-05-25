package statemachine

import (
	"fmt"
	"strings"
)

type State string

const (
	StateDraft          State = "DRAFT"
	StateActive         State = "ACTIVE"
	StateLocked         State = "LOCKED"
	StateDrawing        State = "DRAWING"
	StateKYCPending     State = "KYC_PENDING"
	StateGatewayRouting State = "GATEWAY_ROUTING"
	StatePaying         State = "PAYING"
	StateCompleted      State = "COMPLETED"
	StateCancelled      State = "CANCELLED"
	StateFrozen         State = "FROZEN"
)

var validTransitions = map[State][]State{
	StateDraft:          {StateActive, StateCancelled, StateFrozen},
	StateActive:         {StateLocked, StateCancelled, StateFrozen},
	StateLocked:         {StateDrawing, StateFrozen},
	StateDrawing:        {StateKYCPending, StateGatewayRouting, StatePaying, StateFrozen},
	StateKYCPending:     {StateGatewayRouting, StatePaying, StateFrozen},
	StateGatewayRouting: {StatePaying, StateCompleted, StateFrozen},
	StatePaying:         {StateCompleted, StateFrozen},
	StateCancelled:      {},
	StateCompleted:      {},
	StateFrozen:         {StateDraft, StateActive, StateLocked, StateDrawing, StateKYCPending, StateGatewayRouting, StatePaying, StateCompleted, StateCancelled}, // can transition back to original state upon unfreeze
}

func ValidateTransition(current, next State) error {
	currUpper := State(strings.ToUpper(string(current)))
	nextUpper := State(strings.ToUpper(string(next)))

	if currUpper == nextUpper {
		return nil
	}

	allowed, ok := validTransitions[currUpper]
	if !ok {
		return fmt.Errorf("invalid current state: %s", current)
	}

	for _, a := range allowed {
		if a == nextUpper {
			return nil
		}
	}

	return fmt.Errorf("invalid state transition from %s to %s", current, next)
}
