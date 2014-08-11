package main

import "testing"

func TestExtractNotationIDs(t *testing.T) {
	var (
		in   = "dfkjhfskejfhkjf@IdNotation(isin='DE0009652388', id='95018393,100236746,105304140')"
		want = []string{"95018393", "100236746", "105304140"}
	)
	out, err := extractNotationIDs(in)

	if err != nil {
		t.Errorf("extractNotationIDs(\"%v\") returned an error %v, want %v", in, err.Error(), want)
	}

	if out[0] != want[0] ||
		out[1] != want[1] ||
		out[2] != want[2] {
		t.Errorf("extractNotationIDs(\"%v\") = %v, want %v", in, out, want)
	}

}
