package utils

import (
	"strings"
	"unicode"
)

func CapitalizeFirstLetter(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(string(s[0])) + s[1:]
}

func StartsWithLowerCase(s string) bool {
	if len(s) == 0 {
		return false // or true, depending on your definition for an empty string
	}
	return unicode.IsLower(rune(s[0]))
}

func IsValidIdentifier(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return true
}

func isUnderscope(r rune) bool {
	return r == '_'
}

func ContainsUnderscore(s string) bool {
	for _, r := range s {
		if isUnderscope(r) {
			return true
		}
	}
	return false
}

func RemoveUnderscore(s string) string {
	const underscore = "_"
	return strings.ReplaceAll(s, string(underscore), "")
}

func CamelToSnake(camel string) string {
	var result strings.Builder

	for i, r := range camel {
		if unicode.IsUpper(r) {
			if i > 0 {
				result.WriteRune('_')
			}
			result.WriteRune(unicode.ToLower(r))
		} else {
			result.WriteRune(r)
		}
	}

	return result.String()
}
