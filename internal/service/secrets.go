package service

import (
	"regexp"
	"strconv"
	"strings"
)

// LifeOS never stores a card number, a government ID number or a secret
// (AGENTS.md rule 7). The functions in this file find the ones that have a
// pattern. A password in plain words and a bank account number have no
// pattern, so no code can find them.

var (
	privateKeyPattern = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)
	tokenPatterns     = []*regexp.Regexp{
		regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`),  // AWS access key ID
		regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}`),   // GitHub token
		regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{22,}`), // GitHub fine-grained token
		regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}`), // Slack token
		regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}`),        // API key of an AI service
	}
	// The fourth letter of a PAN says who holds it: P is a person, C a
	// company, and so on.
	panPattern = regexp.MustCompile(`\b[A-Z]{3}[ABCFGHJLPT][A-Z][0-9]{4}[A-Z]\b`)
	// A run is groups of digits with one space or one hyphen between them.
	digitRunPattern = regexp.MustCompile(`[0-9]+(?:[ -][0-9]+)*`)
	groupSeparator  = regexp.MustCompile(`[ -]`)
	// The names that people write next to an Aadhaar number: Aadhaar and
	// its common spellings, UID, UIDAI, and the word in Hindi.
	aadhaarWordPattern = regexp.MustCompile(`(?i)\ba{1,2}dha{1,2}r\b|\buid(?:ai)?\b|आधार`)
)

// aadhaarWordDistance is how far from a number, in bytes, the name of
// Aadhaar can be.
const aadhaarWordDistance = 40

// The ways that a card number and an Aadhaar number are written in groups.
var (
	cardGroupings = [][]int{
		{4, 4, 4, 4},    // most cards
		{4, 4, 4, 4, 3}, // a card with 19 digits
		{4, 6, 5},       // American Express
		{4, 6, 4},       // Diners Club
	}
	aadhaarGrouping = []int{4, 4, 4}
)

// secretKind returns what kind of secret text holds, in words for an error
// message, or "" when it finds none.
func secretKind(text string) string {
	if privateKeyPattern.MatchString(text) {
		return "a private key"
	}
	for _, pattern := range tokenPatterns {
		if pattern.MatchString(text) {
			return "an access key or a token"
		}
	}
	for _, at := range digitRunPattern.FindAllStringIndex(text, -1) {
		around := text[max(0, at[0]-aadhaarWordDistance):min(len(text), at[1]+aadhaarWordDistance)]
		named := aadhaarWordPattern.MatchString(around)
		if kind := numberKind(groupSeparator.Split(text[at[0]:at[1]], -1), named); kind != "" {
			return kind
		}
	}
	if panPattern.MatchString(text) {
		return "a PAN"
	}
	return ""
}

// numberKind looks at one run of digit groups. A number is one group, or
// neighbouring groups in one of the known ways to write it.
//
// 12 digits in one piece are an Aadhaar number only when named is true:
// the name of Aadhaar is near the run. A phone number with its country
// code and the reference number of a UPI payment have 12 digits too, and 1
// of 10 has a correct check digit. The form 1234 5678 9012 is how an
// Aadhaar number is printed, so it needs no name.
func numberKind(groups []string, named bool) string {
	for i, group := range groups {
		if isCardNumber(group) {
			return "a card number"
		}
		if named && isAadhaarNumber(group) {
			return "an Aadhaar number"
		}
		for _, grouping := range cardGroupings {
			if number, ok := grouped(groups, i, grouping); ok && isCardNumber(number) {
				return "a card number"
			}
		}
		if number, ok := grouped(groups, i, aadhaarGrouping); ok && isAadhaarNumber(number) {
			return "an Aadhaar number"
		}
	}
	return ""
}

// grouped returns the digits of the groups that start at index first, when
// their lengths are the lengths in grouping. A neighbour of four digits
// means that the groups are a part of a longer number, so that is no match.
func grouped(groups []string, first int, grouping []int) (string, bool) {
	last := first + len(grouping) - 1
	if last >= len(groups) {
		return "", false
	}
	for i, length := range grouping {
		if len(groups[first+i]) != length {
			return "", false
		}
	}
	if first > 0 && len(groups[first-1]) == 4 {
		return "", false
	}
	if last+1 < len(groups) && len(groups[last+1]) == 4 {
		return "", false
	}
	return strings.Join(groups[first:last+1], ""), true
}

// isCardNumber reports whether digits has the first digits of a card
// network, a length that this network uses, and a correct Luhn check digit.
// The length matters: the IMEI of a phone has 15 digits and a Luhn check
// digit, and most IMEIs start with 35 or 49.
func isCardNumber(digits string) bool {
	n := len(digits)
	if n < 13 || n > 19 {
		return false
	}
	two, _ := strconv.Atoi(digits[:2])
	four, _ := strconv.Atoi(digits[:4])
	network := false
	switch {
	case digits[0] == '4': // Visa
		network = n == 13 || n == 16 || n == 19
	case (two >= 51 && two <= 55) || (four >= 2221 && four <= 2720): // Mastercard
		network = n == 16
	case two == 34 || two == 37: // American Express
		network = n == 15
	case two == 30 || two == 36 || two == 38 || two == 39: // Diners Club
		network = n == 14 || n >= 16
	case two == 35: // JCB, RuPay
		network = n >= 16
	case digits[0] == '6', two == 50, two >= 56 && two <= 58: // Discover, RuPay, UnionPay, Maestro
		network = true
	case two == 81 || two == 82: // RuPay
		network = n == 16
	}
	return network && luhnValid(digits)
}

// isAadhaarNumber reports whether digits is 12 digits that do not start
// with 0 or 1 and that have a correct Verhoeff check digit.
func isAadhaarNumber(digits string) bool {
	return len(digits) == 12 && digits[0] >= '2' && verhoeffValid(digits)
}

// luhnValid reports whether the last digit is the Luhn check digit of the
// digits before it.
func luhnValid(digits string) bool {
	sum := 0
	for i := 0; i < len(digits); i++ {
		n := int(digits[len(digits)-1-i] - '0')
		if i%2 == 1 {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		sum += n
	}
	return sum%10 == 0
}

// The tables of the Verhoeff algorithm: the multiplication in the group
// D5, and the permutation that depends on the position of a digit.
var (
	verhoeffMultiply = [10][10]int{
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		{1, 2, 3, 4, 0, 6, 7, 8, 9, 5},
		{2, 3, 4, 0, 1, 7, 8, 9, 5, 6},
		{3, 4, 0, 1, 2, 8, 9, 5, 6, 7},
		{4, 0, 1, 2, 3, 9, 5, 6, 7, 8},
		{5, 9, 8, 7, 6, 0, 4, 3, 2, 1},
		{6, 5, 9, 8, 7, 1, 0, 4, 3, 2},
		{7, 6, 5, 9, 8, 2, 1, 0, 4, 3},
		{8, 7, 6, 5, 9, 3, 2, 1, 0, 4},
		{9, 8, 7, 6, 5, 4, 3, 2, 1, 0},
	}
	verhoeffPermute = [8][10]int{
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		{1, 5, 7, 6, 2, 8, 3, 0, 9, 4},
		{5, 8, 0, 3, 7, 9, 6, 1, 4, 2},
		{8, 9, 1, 6, 0, 4, 3, 5, 2, 7},
		{9, 4, 5, 3, 1, 2, 6, 8, 7, 0},
		{4, 2, 8, 6, 5, 7, 3, 9, 0, 1},
		{2, 7, 9, 3, 8, 0, 6, 4, 1, 5},
		{7, 0, 4, 6, 9, 1, 3, 2, 5, 8},
	}
)

// verhoeffValid reports whether the last digit is the Verhoeff check digit
// of the digits before it. Aadhaar uses this algorithm.
func verhoeffValid(digits string) bool {
	check := 0
	for i := 0; i < len(digits); i++ {
		n := int(digits[len(digits)-1-i] - '0')
		check = verhoeffMultiply[check][verhoeffPermute[i%8][n]]
	}
	return check == 0
}
