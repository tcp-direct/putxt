package putxt

import "unicode/utf8"

// slightly modified from golang.org/x/tools/godoc/util to include carriage return for dirty windows users
func IsText(s []byte) bool {
	const max = 1024
	if len(s) > max {
		s = s[0:max]
	}
	for i, c := range string(s) {
		if i+utf8.UTFMax > len(s) {
			break
		}
		if c == 0xFFFD || c < ' ' && c != '\n' && c != '\t' && c != '\f' && c != '\r' {
			return false
		}
	}
	return true
}
