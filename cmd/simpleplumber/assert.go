package main

func assertNoError(err error) {
	if err != nil {
		panic(err)
	}
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}
