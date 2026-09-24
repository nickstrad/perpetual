package invariant

type Violation struct{ Message string }

func (v Violation) Error() string { return v.Message }

func Check(condition bool, message string) {
	if !condition {
		panic(Violation{Message: message})
	}
}
