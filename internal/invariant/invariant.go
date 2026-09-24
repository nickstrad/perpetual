package invariant

type Violation struct{ Message string }

func (v Violation) Error() string { return v.Message }

func Check(condition bool, message string) {
	if !condition {
		Fail(message)
	}
}

// Fail reports a violated invariant unconditionally. Use it where a code path
// itself is the violation, instead of Check(false, ...).
func Fail(message string) {
	panic(Violation{Message: message})
}
