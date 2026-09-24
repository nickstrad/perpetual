package registration

import "time"

type RequestID string
type MachineID string
type Parameters struct {
	Name      string `json:"name"`
	Image     string `json:"image"`
	VCPUs     uint32 `json:"vcpus"`
	MemoryMiB uint64 `json:"memory_mib"`
	DiskMiB   uint64 `json:"disk_mib"`
}
type Request struct {
	ID          RequestID
	CandidateID MachineID
	Parameters  Parameters
	Fingerprint [32]byte
}
type Record struct {
	RequestID   RequestID
	MachineID   MachineID
	Parameters  Parameters
	Fingerprint [32]byte
	CreatedAt   time.Time
}
type AdmissionKind uint8

const (
	AdmissionCreate AdmissionKind = iota + 1
	AdmissionExisting
	AdmissionRequestConflict
	AdmissionNameConflict
	AdmissionCapacity
)

type Observation struct {
	HasRequest bool
	Record     Record
	NameTaken  bool
	Used       uint64
	Limit      uint64
}
type Admission struct {
	Kind   AdmissionKind
	Record Record
}
type Phase uint8

const (
	PhaseNew Phase = iota
	PhaseReserving
	PhaseResolved
	PhaseUncertain
)

type OutcomeKind uint8

const (
	OutcomeCreated OutcomeKind = iota + 1
	OutcomeExisting
	OutcomeRequestConflict
	OutcomeNameConflict
	OutcomeCapacity
	OutcomeBusy
	OutcomeUnavailable
	OutcomeUnknown
)

type Outcome struct {
	Kind   OutcomeKind
	Record Record
}
type EffectID struct {
	Epoch    string
	Sequence uint64
}
type State struct {
	Phase    Phase
	EffectID EffectID
	Request  Request
	Outcome  Outcome
}
type Event interface{ registrationEvent() }
type Submit struct {
	Request  Request
	EffectID EffectID
}

func (Submit) registrationEvent() {}

type Completed struct {
	EffectID EffectID
	Outcome  Outcome
}

func (Completed) registrationEvent() {}

type Effect interface{ registrationEffect() }
type Reserve struct {
	EffectID EffectID
	Request  Request
}

func (Reserve) registrationEffect() {}

type Reply struct{ Outcome Outcome }

func (Reply) registrationEffect() {}
