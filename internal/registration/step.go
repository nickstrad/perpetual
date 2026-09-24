package registration

import (
	"time"

	"perpetual/internal/invariant"
)

// NewState reconstructs only volatile coordination state. Durable records are
// recovered through the store, never through a previous job's phase.
func NewState() State { return State{Phase: PhaseNew} }

func IsCurrentEpoch(epoch string, effect EffectID) bool {
	return epoch != "" && effect.Epoch == epoch
}

func Step(state State, event Event) (State, []Effect) {
	switch event := event.(type) {
	case Submit:
		invariant.Check(state.Phase == PhaseNew, "job submitted twice")
		invariant.Check(event.EffectID.Epoch != "" && event.EffectID.Sequence != 0, "invalid effect ID")
		next := State{Phase: PhaseReserving, EffectID: event.EffectID, Request: event.Request}
		return next, []Effect{Reserve{EffectID: event.EffectID, Request: event.Request}}
	case Completed:
		invariant.Check(state.Phase == PhaseReserving, "completion without pending effect")
		invariant.Check(state.EffectID == event.EffectID, "completion for another effect")
		CheckOutcome(state.Request, event.Outcome)
		next := state
		next.Phase = PhaseResolved
		if event.Outcome.Kind == OutcomeUnknown {
			next.Phase = PhaseUncertain
		}
		next.Outcome = event.Outcome
		return next, []Effect{Reply{Outcome: event.Outcome}}
	default:
		invariant.Fail("unsupported registration event")
		return state, nil
	}
}

func CheckOutcome(request Request, outcome Outcome) {
	invariant.Check(outcome.Kind >= OutcomeCreated && outcome.Kind <= OutcomeUnknown, "invalid outcome kind")
	if outcome.Kind != OutcomeCreated && outcome.Kind != OutcomeExisting {
		invariant.Check(outcome.Record == (Record{}), "non-success outcome contains record")
		return
	}
	record := outcome.Record
	invariant.Check(record.RequestID == request.ID, "outcome request does not match")
	invariant.Check(record.Parameters == request.Parameters, "outcome parameters do not match")
	invariant.Check(record.Fingerprint == request.Fingerprint, "outcome fingerprint does not match")
	invariant.Check(ValidateMachineID(string(record.MachineID)) == nil, "outcome machine ID invalid")
	invariant.Check(!record.CreatedAt.IsZero(), "outcome timestamp missing")
	invariant.Check(record.CreatedAt.Location() == time.UTC, "outcome timestamp must be UTC")
}
