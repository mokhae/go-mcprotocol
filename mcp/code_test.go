package mcp

import (
	"encoding/hex"
	"testing"
)

func TestCode_EncodeHex(t *testing.T) {
	cases := []struct {
		code     Code
		input    string
		expected string
	}{
		// binary mode stores each 2 byte word from lower byte to upper byte
		{code: Binary, input: "0401", expected: "0104"}, // read command
		{code: Binary, input: "1401", expected: "0114"}, // write command
		{code: Binary, input: "0001", expected: "0100"}, // bit unit sub command
		{code: Binary, input: "", expected: ""},
		// several words are swapped word by word, not reversed as a whole
		{code: Binary, input: "04010001", expected: "01040100"},

		// ascii mode sends the value as is, so the result is the ascii chars of input
		{code: Ascii, input: "0401", expected: "30343031"},
		{code: Ascii, input: "", expected: ""},
	}

	for _, v := range cases {
		actual, err := v.code.EncodeHex(v.input)
		if err != nil {
			t.Errorf("something wrong: input is %v: %v", v.input, err)
			continue
		}

		if hex.EncodeToString(actual) != v.expected {
			t.Errorf("wrong result: input %v expected is %v but actual is %v", v.input, v.expected, hex.EncodeToString(actual))
		}
	}
}

func TestCode_EncodeHex_error(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{name: "odd number of chars", input: "040"},
		{name: "not hexadecimal", input: "zz"},
		// binary mode words are 2 bytes, so an odd byte count has no word layout
		{name: "1 byte is not a word", input: "90"},
		{name: "3 bytes are not whole words", input: "000064"},
	}

	for _, v := range cases {
		t.Run(v.name, func(t *testing.T) {
			if _, err := Binary.EncodeHex(v.input); err == nil {
				t.Errorf("expected an error for input %v but got nil", v.input)
			}
		})
	}
}
