package statemachine
package statemachine

import "testing"

func TestValidateTransition_Allowed(t *testing.T) {
    cases := []struct{ from, to State }{
        {StateDraft, StateActive},
        {StateActive, StateLocked},
        {StateLocked, StateDrawing},
        {StateDrawing, StateKYCPending},
        {StateFrozen, StateDraft},
    }

    for _, c := range cases {
        if err := ValidateTransition(c.from, c.to); err != nil {
            t.Fatalf("expected transition %s->%s to be allowed: %v", c.from, c.to, err)
        }
    }
}

func TestValidateTransition_Disallowed(t *testing.T) {
    if err := ValidateTransition(StateCompleted, StateActive); err == nil {
        t.Fatalf("expected transition COMPLETED->ACTIVE to be invalid")
    }
}
