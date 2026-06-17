//go:build !darwin

package hardware

import "os"

func readFileOS(path string) ([]byte, error) {
	return os.ReadFile(path)
}
