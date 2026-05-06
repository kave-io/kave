package runtime

// ObservedSpan is work Kave can see inside an intercepted or reported trace,
// but cannot block at that boundary. It is written as SpanKindObservedAction.
type ObservedSpan struct {
	Name      string
	Type      ActionType
	Connector string
	Method    string
	Model     string
	Input     *[]byte
	Output    *[]byte
	Error     *string
	Attrs     map[string]any
}
