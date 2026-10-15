package loader

import "unicode/utf8"

func validUTF8(data []byte) bool {
	return utf8.Valid(data)
}
