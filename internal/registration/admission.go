package registration

import "perpetual/internal/invariant"

// The caller must hold the registration gate while collecting this observation.
// A plain absent lookup is never authority to create a record.
func DecideAdmission(request Request, observed Observation) Admission {
	invariant.Check(observed.Limit > 0, "registration limit must be positive")
	invariant.Check(observed.Used <= observed.Limit, "registration count exceeds limit")
	if observed.HasRequest {
		invariant.Check(observed.Record.RequestID == request.ID, "lookup returned another request")
		if observed.Record.Parameters != request.Parameters || observed.Record.Fingerprint != request.Fingerprint {
			return Admission{Kind: AdmissionRequestConflict}
		}
		return Admission{Kind: AdmissionExisting, Record: observed.Record}
	}
	if observed.NameTaken {
		return Admission{Kind: AdmissionNameConflict}
	}
	if observed.Used == observed.Limit {
		return Admission{Kind: AdmissionCapacity}
	}
	return Admission{Kind: AdmissionCreate}
}

// Outcome translates a terminal admission decision into the outcome the store
// reports. AdmissionCreate has no outcome until the record is written.
func (a Admission) Outcome() Outcome {
	switch a.Kind {
	case AdmissionExisting:
		return Outcome{Kind: OutcomeExisting, Record: a.Record}
	case AdmissionRequestConflict:
		return Outcome{Kind: OutcomeRequestConflict}
	case AdmissionNameConflict:
		return Outcome{Kind: OutcomeNameConflict}
	case AdmissionCapacity:
		return Outcome{Kind: OutcomeCapacity}
	default:
		invariant.Fail("admission kind has no outcome")
		return Outcome{}
	}
}
