package pwpin

func ptr[T any](value T) *T {
	return &value
}
