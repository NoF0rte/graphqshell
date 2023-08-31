package fuzz

import "regexp"

type Regexes []*regexp.Regexp

func (r Regexes) MatchString(s string) bool {
	for _, re := range r {
		if re.MatchString(s) {
			return true
		}
	}

	return false
}

func (r Regexes) FindAllString(s string) []string {
	for _, re := range r {
		matches := re.FindAllString(s, -1)
		if len(matches) != 0 {
			return matches
		}
	}

	return nil
}

func (r Regexes) FindAllStringSubmatch(text string) [][]string {
	for _, re := range r {
		matches := re.FindAllStringSubmatch(text, -1)
		if len(matches) != 0 {
			return matches
		}
	}

	return nil
}
