//go:build !windows

package diagnosis

func platformErrorCategory(_ error) string {
	return CategoryUnknown
}
