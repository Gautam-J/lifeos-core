package service

import (
	"strings"
	"testing"
)

// The tests build each token from parts. A token that is written in one
// piece makes the secret scanner of GitHub refuse the push.
var (
	awsKey      = "AKIA" + "IOSFODNN7EXAMPLE"
	githubToken = "ghp" + "_" + strings.Repeat("a1B2", 9)
	githubPAT   = "github" + "_pat_" + strings.Repeat("a1B2_", 5)
	slackToken  = "xox" + "b-" + "123456789012-abcdefghij"
	skKey       = "sk" + "-" + "ant-api03-" + strings.Repeat("x9", 12)
	privateKey  = "-----BEGIN RSA " + "PRIVATE KEY-----\nMIIEow\n-----END RSA " + "PRIVATE KEY-----"
)

func TestSecretKindFindsEachKindOfSecret(t *testing.T) {
	cases := []struct{ name, text, want string }{
		{"Visa", "my card is 4111111111111111", "a card number"},
		{"Visa with 13 digits", "4222222222222", "a card number"},
		{"Visa with 19 digits", "4111111111111111110", "a card number"},
		{"Mastercard", "5555555555554444", "a card number"},
		{"Mastercard that starts with 2", "2223003122003222", "a card number"},
		{"American Express", "378282246310005", "a card number"},
		{"Discover", "6011111111111117", "a card number"},
		{"JCB", "3530111333300000", "a card number"},
		{"Diners Club", "30569309025904", "a card number"},
		{"RuPay", "6521500000000006", "a card number"},
		{"RuPay that starts with 508", "5085000000000007", "a card number"},
		{"groups of four with spaces", "pay with 4111 1111 1111 1111 today", "a card number"},
		{"groups of four with hyphens", "4111-1111-1111-1111", "a card number"},
		{"the groups of American Express", "3782 822463 10005", "a card number"},
		{"the groups of Diners Club", "3056 930902 5904", "a card number"},
		{"a card after a date", "2026-10-11 4111 1111 1111 1111", "a card number"},
		{"a card between letters", "card:4111111111111111.", "a card number"},
		{"a card on its own line", "notes\n5555 5555 5555 4444\nend", "a card number"},

		{"Aadhaar", "234567890124", "an Aadhaar number"},
		{"Aadhaar in groups", "my Aadhaar is 2345 6789 0124", "an Aadhaar number"},
		{"Aadhaar with hyphens", "2345-6789-0124", "an Aadhaar number"},
		{"Aadhaar after a date", "2026-10-11 2345 6789 0124", "an Aadhaar number"},

		{"PAN of a person", "PAN ABCPE1234F", "a PAN"},
		{"PAN of a company", "AAACR5055K", "a PAN"},

		{"private key", privateKey, "a private key"},
		{"AWS access key", "key " + awsKey, "an access key or a token"},
		{"GitHub token", githubToken, "an access key or a token"},
		{"GitHub fine-grained token", githubPAT, "an access key or a token"},
		{"Slack token", slackToken, "an access key or a token"},
		{"API key that starts with sk-", "export KEY=" + skKey, "an access key or a token"},
	}
	for _, c := range cases {
		if got := secretKind(c.text); got != c.want {
			t.Errorf("%s: secretKind = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSecretKindAcceptsTextWithoutASecret(t *testing.T) {
	cases := []struct{ name, text string }{
		{"empty", ""},
		{"plain words", "spent the entire day planning a second brain"},
		{"a card number with a wrong check digit", "4111111111111112"},
		{"16 digits that no card network uses", "1234567890123452"},
		{"20 digits", "41111111111111111111"},
		{"five groups of four", "4111 1111 1111 1111 1111"},
		{"a phone number", "call 9876543210"},
		{"a phone number with a country code", "+91 98765 43210"},
		{"12 digits with a wrong check digit", "234567890123"},
		{"12 digits that start with 1", "123456789012"},
		{"three of the four groups of a longer number", "9999 2345 6789 0124"},
		{"a date and a time", "2026-10-11 07:30"},
		{"an ISBN", "9780441172719"},
		{"a timestamp in milliseconds", "1791676800000"},
		{"an amount", "paid 12,50,000 rupees on 2026-10-11"},
		{"ten capital letters and digits that are not a PAN", "ABCDE1234F"},
		{"a PAN inside a longer word", "XABCPE1234FX"},
		{"a word that starts with sk-", "sk-learn is a library"},
		{"the words private key", "I lost my private key yesterday"},
		{"a short token prefix", "ghp_short"},
	}
	for _, c := range cases {
		if got := secretKind(c.text); got != "" {
			t.Errorf("%s: secretKind = %q, want no secret", c.name, got)
		}
	}
}

func TestLuhnAndVerhoeffCheckTheLastDigit(t *testing.T) {
	if !luhnValid("4111111111111111") || luhnValid("4111111111111112") {
		t.Error("luhnValid gives the wrong answer for the Visa test number")
	}
	if !verhoeffValid("234567890124") || verhoeffValid("234567890123") {
		t.Error("verhoeffValid gives the wrong answer")
	}
	// Verhoeff, not Luhn, finds two neighbours that changed places.
	if verhoeffValid("234567890214") {
		t.Error("verhoeffValid accepts two digits that changed places")
	}
}
