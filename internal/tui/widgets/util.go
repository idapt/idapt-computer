package widgets

import "fmt"

func sprintfMoney(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
