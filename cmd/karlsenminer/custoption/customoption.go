package custoption

import (
	"io/fs"
	"runtime"
	"unicode/utf8"
)

type Option struct {
	NumThreads          uint32
	Path                string
	DisableLocalDagFile bool
}

func CheckPath(name string) error {
	if !validPath(name) || runtime.GOOS == "windows" && containsAny(name, `\:`) {
		return &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	return nil
}

// This function is copied from fs.go but allow path starting with a slash
func validPath(name string) bool {
	if !utf8.ValidString(name) {
		return false
	}

	if len(name) == 1 && name == "/" {
		return false
	}

	// Iterate over elements in name, checking each.
	for {
		i := 0
		if len(name) > 0 && name[0] == '/' {
			i++
		}
		for i < len(name) && name[i] != '/' {
			i++
		}
		elem := name[:i]
		if elem == "" || elem == "." || elem == ".." {
			return false
		}
		if i == len(name) {
			return true // reached clean ending
		}
		name = name[i+1:]
	}
}

func containsAny(s, chars string) bool {
	for i := 0; i < len(s); i++ {
		for j := 0; j < len(chars); j++ {
			if s[i] == chars[j] {
				return true
			}
		}
	}
	return false
}
