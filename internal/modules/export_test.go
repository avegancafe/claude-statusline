package modules

// BuildBarForTest exposes buildBar to the external test package. Kept in an
// _test.go file so it never becomes part of the package's public API.
func BuildBarForTest(pct float64, width int, fill, empty string) string {
	return buildBar(pct, width, fill, empty)
}
