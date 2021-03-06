package goData

import "testing"
import "time"

func TestTimeToDie(t *testing.T) {
	var (
		in   = time.Date(2014, 8, 17, 20, 01, 00, 0, time.UTC)
		want = true
	)
	got, err := timeToDie(in)
	//sucess
	if got != want {
		t.Errorf("timeToDie should have returned true, but returned %v, we want %v", got, want)
	}

	loc, _ := time.LoadLocation("Europe/Berlin")
	const shortForm = "2006-Jan-02"
	inNotUTC, _ := time.ParseInLocation(shortForm, "2012-Jul-09", loc)

	got, err = timeToDie(inNotUTC)
	if err == nil {
		t.Errorf("timeToDie should through timeZone Error")
	}
}

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

func TestPrepareURL(t *testing.T) {
	var (
		urlTemplate = "www.test.de/?id=[[?]]/differentID=[[?]]"
		id1         = "123"
		id2         = "321"
		want        = "www.test.de/?id=123/differentID=321"
	)
	// success
	got, err := prepareURL(urlTemplate, id1, id2)

	if err != nil {
		t.Errorf("prepareURL(%s, %s, %s) returned err(%v); want %s", urlTemplate, id1,
			id2, err, want)
	} else if want != got {
		t.Errorf("prepareURL(%s, %s, %s), got \"%s\", want \"%s\"", urlTemplate, id1, id2,
			got, want)
	}

	// err expected if no parameters are provided
	got, err = prepareURL("")
	if err == nil {
		t.Errorf("prepareURL(\"\") should return err " +
			"'no arguments given'")
	}

	// if too less parameters - err expected
	got, err = prepareURL(urlTemplate, id1)
	if err == nil {
		t.Errorf("prepareURL(%s, %s) should return "+
			"'not enough parameters provided' ", urlTemplate, id1)
	}

	// if too many parameters - err expected
	got, err = prepareURL(urlTemplate, id1, id2, id1)
	if err == nil {
		t.Errorf("prepareURL(%s, %s, %s, %s) should return "+
			"'too many parameters provided'", urlTemplate, id1, id2, id1)
	}
}
