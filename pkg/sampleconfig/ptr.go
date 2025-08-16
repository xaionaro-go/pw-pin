package sampleconfig

func ptr[T any](value T) *T {
	return &value
}
