package htjson

import (
	"reflect"
	"testing"
	"time"
)

type FakeNower struct{}

func (f *FakeNower) Now() time.Time {
	fakeTime, _ := time.Parse(time.RFC3339, "2010-06-21T15:04:05Z")
	return fakeTime
}

type testLineMap struct {
	input    string
	expected map[string]interface{}
}

var tlms = []testLineMap{
	{ // strings, floats, and ints
		input: `{"mystr": "myval", "myint": 3, "myfloat": 4.234}`,
		expected: map[string]interface{}{
			"mystr":   "myval",
			"myint":   float64(3),
			"myfloat": 4.234,
		},
	},
	{ // time
		input: `{"time": "2014-03-10 19:57:38.123456789 -0800 PST", "myint": 3, "myfloat": 4.234}`,
		expected: map[string]interface{}{
			"time":    "2014-03-10 19:57:38.123456789 -0800 PST",
			"myint":   float64(3),
			"myfloat": 4.234,
		},
	},
	{ // non-flat json object
		input: `{"array": [3, 4, 6], "myfloat": 4.234}`,
		expected: map[string]interface{}{
			"array":   "[3,4,6]",
			"myfloat": 4.234,
		},
	},
}

func TestParseLine(t *testing.T) {
	jlp := JSONLineParser{}
	for _, tlm := range tlms {
		resp, err := jlp.ParseLine(tlm.input)
		if err != nil {
			t.Error("jlp.ParseLine unexpectedly returned error ", err)
		}
		if !reflect.DeepEqual(resp, tlm.expected) {
			t.Errorf("response %+v didn't match expected %+v", resp, tlm.expected)
		}
	}
}

type testTimestamp struct {
	format    string                 // the format this test's time is in
	fieldName string                 // the field in the map containing the time
	input     map[string]interface{} // the map to send in as the test
	expected  time.Time              // the expected time object to get back
}

var tts = []testTimestamp{
	{
		format:    "2006-01-02 15:04:05.999999999 -0700 MST",
		fieldName: "time",
		input:     map[string]interface{}{"time": "2014-03-10 19:57:38.123456789 -0800 PST"},
	},
	{
		format:    time.RFC3339Nano,
		fieldName: "timestamp",
		input:     map[string]interface{}{"timestamp": "2014-04-10T19:57:38.123456789-08:00"},
	},
	{
		format:    time.RFC3339,
		fieldName: "Date",
		input:     map[string]interface{}{"Date": "2014-04-10T19:57:38-08:00"},
	},
	{
		format:    time.RubyDate,
		fieldName: "datetime",
		input:     map[string]interface{}{"datetime": "Thu Apr 10 19:57:38.123456789 -0800 2014"},
	},
	{
		format:    time.UnixDate,
		fieldName: "DateTime",
		input:     map[string]interface{}{"DateTime": "Thu Apr 10 19:57:38 PST 2014"},
	},
}

func TestGetTimestampValid(t *testing.T) {
	p := &Parser{nower: &FakeNower{}}
	for i, tTimeSet := range tts {
		testTime, _ := time.Parse(tTimeSet.format, tTimeSet.input[tTimeSet.fieldName].(string))
		resp := p.getTimestamp(tTimeSet.input)
		if !resp.Equal(testTime) {
			t.Errorf("time %d: resp time %s didn't match expected time %s", i, resp, testTime)
		}
	}
}

func TestGetTimestampInvalid(t *testing.T) {
	p := &Parser{nower: &FakeNower{}}
	// time field missing
	resp := p.getTimestamp(map[string]interface{}{"noTimeField": "not used"})
	if !resp.Equal(p.nower.Now()) {
		t.Errorf("resp time %s didn't match expected time %s", resp, p.nower.Now())
	}
	// time field unparsable
	resp = p.getTimestamp(map[string]interface{}{"time": "not a valid date"})
	if !resp.Equal(p.nower.Now()) {
		t.Errorf("resp time %s didn't match expected time %s", resp, p.nower.Now())
	}
}

func TestGetTimestampCustomFormat(t *testing.T) {
	weirdFormat := "Mon // 02 ---- Jan ... 06 15:04:05 MST"

	testStr := "Mon // 09 ---- Aug ... 10 15:34:56 PST"
	testTime, _ := time.Parse(weirdFormat, testStr)

	// with just Format defined
	p := &Parser{
		nower: &FakeNower{},
		conf:  Options{Format: weirdFormat},
	}
	resp := p.getTimestamp(map[string]interface{}{"timestamp": testStr})
	if !resp.Equal(testTime) {
		t.Errorf("resp time %s didn't match expected time %s", resp, testTime)
	}

	// with just TimeFieldName defined
	p = &Parser{
		nower: &FakeNower{},
		conf: Options{
			TimeFieldName: "funkyTime",
		},
	}
	// use one of the expected/fallback formats
	resp = p.getTimestamp(map[string]interface{}{"funkyTime": testTime.Format(time.RubyDate)})
	if !resp.Equal(testTime) {
		t.Errorf("resp time %s didn't match expected time %s", resp, testTime)
	}

	// Now with both defined
	p = &Parser{
		nower: &FakeNower{},
		conf: Options{
			TimeFieldName: "funkyTime",
			Format:        weirdFormat,
		},
	}
	resp = p.getTimestamp(map[string]interface{}{"funkyTime": testStr})
	if !resp.Equal(testTime) {
		t.Errorf("resp time %s didn't match expected time %s", resp, testTime)
	}
	// don't parse the "time" field if we're told to look for time in "funkyTime"
	resp = p.getTimestamp(map[string]interface{}{"time": "2014-04-10 19:57:38.123456789 -0800 PST"})
	if !resp.Equal(p.nower.Now()) {
		t.Errorf("resp time %s didn't match expected time %s", resp, p.nower.Now())
	}
}

func TestCommaInTimestamp(t *testing.T) {
	p := &Parser{
		nower: &FakeNower{},
		conf:  Options{},
	}
	commaTimes := []testTimestamp{
		{ // test commas as the fractional portion separator
			format:    "2006-01-02 15:04:05,999999999 -0700 MST",
			fieldName: "time",
			input:     map[string]interface{}{"time": "2014-03-10 12:57:38,123456789 -0700 PDT"},
			expected:  time.Unix(1394481458, 123456789),
		},
		{
			format:    "2006-01-02 15:04:05.999999999 -0700 MST",
			fieldName: "time",
			input:     map[string]interface{}{"time": "2014-03-10 12:57:38,123456789 -0700 PDT"},
			expected:  time.Unix(1394481458, 123456789),
		},
	}
	for i, tTimeSet := range commaTimes {
		p.conf.Format = tTimeSet.format
		expectedTime := tTimeSet.expected
		resp := p.getTimestamp(tTimeSet.input)
		if !resp.Equal(expectedTime) {
			t.Errorf("time %d: resp time %s didn't match expected time %s", i, resp, expectedTime)
		}
	}

}
