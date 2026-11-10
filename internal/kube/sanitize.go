package kube

import "strings"

func nameRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.'
}

// Sanitize derives a short cluster name from an arbitrary context name.
// For EKS ARNs ("arn:aws:eks:...:cluster/foo") only the part after the last
// slash is kept; other unsafe runes become '-'.
func Sanitize(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 && i < len(s)-1 {
		s = s[i+1:]
	} else if strings.HasSuffix(s, "/") && len(s) > 1 {
		s = strings.TrimRight(s, "/")
	}
	b := []rune(s)
	for i, r := range b {
		if !nameRune(r) {
			b[i] = '-'
		}
	}
	out := strings.Trim(string(b), "-.")
	if out == "" {
		return "cluster"
	}
	return out
}
