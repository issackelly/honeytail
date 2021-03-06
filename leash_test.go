package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/Sirupsen/logrus"
	"github.com/honeycombio/honeytail/tail"
)

// defaultOptions is a fully populated GlobalOptions with good defaults to start from
var defaultOptions = GlobalOptions{
	// each test will have to populate APIHost with the location of its test server
	APIHost:    "",
	SampleRate: 1,
	NumSenders: 1,
	Reqs: RequiredOptions{
		// using the json parser for everything because we're not testing parsers here.
		ParserName: "json",
		WriteKey:   "abcabc123123",
		// each test will specify its own logfile
		// LogFiles:   []string{tmpdir + ""},
		Dataset: "pika",
	},
	Tail: tail.TailOptions{
		ReadFrom: "start",
		Stop:     true,
	},
	StatusInterval: 1,
}

// test testing framework
func TestHTTPtest(t *testing.T) {
	ts := &testSetup{}
	ts.start(t, &GlobalOptions{})
	defer ts.close()
	ts.rsp.responseBody = "whatup pikachu"
	res, err := http.Get(ts.server.URL)
	if err != nil {
		log.Fatal(err)
	}
	greeting, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		log.Fatal(err)
	}
	testEquals(t, res.StatusCode, 200)
	testEquals(t, string(greeting), "whatup pikachu")
	testEquals(t, ts.rsp.reqCounter, 1)
}

func TestBasicSend(t *testing.T) {
	opts := defaultOptions
	ts := &testSetup{}
	ts.start(t, &opts)
	defer ts.close()
	logFileName := ts.tmpdir + "/first.log"
	fh, err := os.Create(logFileName)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	fmt.Fprintf(fh, `{"format":"json"}`)
	opts.Reqs.LogFiles = []string{logFileName}
	run(opts)
	testEquals(t, ts.rsp.reqCounter, 1)
	testEquals(t, ts.rsp.reqBody, `{"format":"json"}`)
	teamID := ts.rsp.req.Header.Get("X-Honeycomb-Team")
	testEquals(t, teamID, "abcabc123123")
	request_url := ts.rsp.req.URL.Path
	testEquals(t, request_url, "/1/events/pika")
	sampleRate := ts.rsp.req.Header.Get("X-Honeycomb-Samplerate")
	testEquals(t, sampleRate, "1")
}

func TestSetVersion(t *testing.T) {
	opts := defaultOptions
	ts := &testSetup{}
	ts.start(t, &opts)
	defer ts.close()
	logFileName := ts.tmpdir + "/setv.log"
	fh, _ := os.Create(logFileName)
	defer fh.Close()
	fmt.Fprintf(fh, `{"format":"json"}`)
	opts.Reqs.LogFiles = []string{logFileName}
	run(opts)
	userAgent := ts.rsp.req.Header.Get("User-Agent")
	testEquals(t, userAgent, "libhoney-go/1.1.0")
	setVersion()
	run(opts)
	userAgent = ts.rsp.req.Header.Get("User-Agent")
	testEquals(t, userAgent, "libhoney-go/1.1.0 honeytail/dev")
	BuildID = "test"
	setVersion()
	run(opts)
	userAgent = ts.rsp.req.Header.Get("User-Agent")
	testEquals(t, userAgent, "libhoney-go/1.1.0 honeytail/test")
}

func TestDropField(t *testing.T) {
	opts := defaultOptions
	ts := &testSetup{}
	ts.start(t, &opts)
	defer ts.close()
	logFileName := ts.tmpdir + "/drop.log"
	fh, _ := os.Create(logFileName)
	defer fh.Close()
	fmt.Fprintf(fh, `{"dropme":"chew","format":"json","reallygone":"notyet"}`)
	opts.Reqs.LogFiles = []string{logFileName}
	run(opts)
	testEquals(t, ts.rsp.reqCounter, 1)
	testEquals(t, ts.rsp.reqBody, `{"dropme":"chew","format":"json","reallygone":"notyet"}`)
	opts.DropFields = []string{"dropme"}
	run(opts)
	testEquals(t, ts.rsp.reqCounter, 2)
	testEquals(t, ts.rsp.reqBody, `{"format":"json","reallygone":"notyet"}`)
	opts.DropFields = []string{"dropme", "reallygone"}
	run(opts)
	testEquals(t, ts.rsp.reqCounter, 3)
	testEquals(t, ts.rsp.reqBody, `{"format":"json"}`)
}

func TestScrubField(t *testing.T) {
	opts := defaultOptions
	ts := &testSetup{}
	ts.start(t, &opts)
	defer ts.close()
	logFileName := ts.tmpdir + "/scrub.log"
	fh, _ := os.Create(logFileName)
	defer fh.Close()
	fmt.Fprintf(fh, `{"format":"json","name":"hidden"}`)
	opts.Reqs.LogFiles = []string{logFileName}
	opts.ScrubFields = []string{"name"}
	run(opts)
	testEquals(t, ts.rsp.reqCounter, 1)
	testEquals(t, ts.rsp.reqBody, `{"format":"json","name":"e564b4081d7a9ea4b00dada53bdae70c99b87b6fce869f0c3dd4d2bfa1e53e1c"}`)
}

func TestAddField(t *testing.T) {
	opts := defaultOptions
	ts := &testSetup{}
	ts.start(t, &opts)
	defer ts.close()
	logFileName := ts.tmpdir + "/add.log"
	logfh, _ := os.Create(logFileName)
	defer logfh.Close()
	fmt.Fprintf(logfh, `{"format":"json"}`)
	opts.Reqs.LogFiles = []string{logFileName}
	opts.AddFields = []string{`newfield=newval`}
	run(opts)
	testEquals(t, ts.rsp.reqBody, `{"format":"json","newfield":"newval"}`)
	opts.AddFields = []string{"newfield=newval", "second=new"}
	run(opts)
	testEquals(t, ts.rsp.reqBody, `{"format":"json","newfield":"newval","second":"new"}`)
}

func TestSampleRate(t *testing.T) {
	opts := defaultOptions
	ts := &testSetup{}
	ts.start(t, &opts)
	defer ts.close()
	rand.Seed(1)
	sampleLogFile := ts.tmpdir + "/sample.log"
	logfh, _ := os.Create(sampleLogFile)
	defer logfh.Close()
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(logfh, `{"format":"json%d"}`+"\n", i)
	}
	opts.Reqs.LogFiles = []string{sampleLogFile}
	run(opts)
	// with no sampling, 1000 lines -> 1000 requests
	testEquals(t, ts.rsp.reqCounter, 1000)
	testEquals(t, ts.rsp.reqBody, `{"format":"json999"}`)
	sampleRate := ts.rsp.req.Header.Get("X-Honeycomb-Samplerate")
	testEquals(t, sampleRate, "1")
	opts.SampleRate = 20
	ts.rsp.reset()
	run(opts)
	// setting a sample rate of 20 and a rand seed of 1, 49 requests.
	testEquals(t, ts.rsp.reqCounter, 49)
	testEquals(t, ts.rsp.reqBody, `{"format":"json996"}`)
	sampleRate = ts.rsp.req.Header.Get("X-Honeycomb-Samplerate")
	testEquals(t, sampleRate, "20")
}

func TestReadFromOffset(t *testing.T) {
	opts := defaultOptions
	ts := &testSetup{}
	ts.start(t, &opts)
	defer ts.close()
	offsetLogFile := ts.tmpdir + "/offset.log"
	offsetStateFile := ts.tmpdir + "/offset.leash.state"
	logfh, _ := os.Create(offsetLogFile)
	defer logfh.Close()
	logStat := unix.Stat_t{}
	unix.Stat(offsetLogFile, &logStat)
	for i := 0; i < 10; i++ {
		fmt.Fprintf(logfh, `{"format":"json%d"}`+"\n", i)
	}
	opts.Reqs.LogFiles = []string{offsetLogFile}
	opts.Tail.ReadFrom = "last"
	osf, _ := os.Create(offsetStateFile)
	defer osf.Close()
	fmt.Fprintf(osf, `{"INode":%d,"Offset":38}`, logStat.Ino)
	run(opts)
	testEquals(t, ts.rsp.reqCounter, 8)
}

// boilerplate to spin up a httptest server, create tmpdir, etc.
// to create an environment in which to run these tests
type testSetup struct {
	server *httptest.Server
	rsp    *responder
	tmpdir string
}

func (t *testSetup) start(tst *testing.T, opts *GlobalOptions) {
	logrus.SetOutput(ioutil.Discard)
	t.rsp = &responder{}
	t.server = httptest.NewServer(http.HandlerFunc(t.rsp.serveResponse))
	tmpdir, err := ioutil.TempDir(os.TempDir(), "test")
	if err != nil {
		tst.Fatal(err)
	}
	t.tmpdir = tmpdir
	opts.APIHost = t.server.URL
	t.rsp.responseCode = 200
}
func (t *testSetup) close() {
	t.server.Close()
	os.RemoveAll(t.tmpdir)
}

type responder struct {
	req          *http.Request // the most recent request answered by the server
	reqBody      string        // the body sent along with the request
	reqCounter   int           // the number of requests answered since last reset
	responseCode int           // the http status code with which to respond
	responseBody string        // the body to send as the response
}

func (r *responder) serveResponse(w http.ResponseWriter, req *http.Request) {
	r.req = req
	r.reqCounter += 1
	body, _ := ioutil.ReadAll(req.Body)
	req.Body.Close()
	r.reqBody = string(body)
	w.WriteHeader(r.responseCode)
	fmt.Fprintf(w, r.responseBody)
}
func (r *responder) reset() {
	r.reqCounter = 0
	r.responseCode = 200
}

// helper function
func testEquals(t testing.TB, actual, expected interface{}, msg ...string) {
	if !reflect.DeepEqual(actual, expected) {
		message := strings.Join(msg, ", ")
		_, file, line, _ := runtime.Caller(1)

		t.Errorf(
			"%s:%d: %s -- actual(%T): %v, expected(%T): %v",
			filepath.Base(file),
			line,
			message,
			testDeref(actual),
			testDeref(actual),
			testDeref(expected),
			testDeref(expected),
		)
	}
}
func testDeref(v interface{}) interface{} {
	switch t := v.(type) {
	case *string:
		return fmt.Sprintf("*(%v)", *t)
	case *int64:
		return fmt.Sprintf("*(%v)", *t)
	case *float64:
		return fmt.Sprintf("*(%v)", *t)
	case *bool:
		return fmt.Sprintf("*(%v)", *t)
	default:
		return v
	}
}
