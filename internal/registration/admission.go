package registration

import "perpetual/internal/invariant"

// The caller must hold the registration gate while collecting this observation.
// A plain absent lookup is never authority to create a record.
func DecideAdmission(request Request, observed Observation) Admission {
	invariant.Check(observed.Limit > 0, "registration limit must be positive")
	invariant.Check(observed.Used <= observed.Limit, "registration count exceeds limit")
	if observed.HasRequest {
		invariant.Check(observed.Record.RequestID == request.ID, "lookup returned another request")
		if observed.Record.Parameters != request.Parameters {
			return Admission{Kind: AdmissionRequestConflict}
		}
		if observed.Record.Fingerprint != request.Fingerprint {
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
