package deviceflow

import "os"

func osLookupEnv(key string) (string, bool) {
	return os.LookupEnv(key)
}
