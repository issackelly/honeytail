package main

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/honeycombio/honeytail/parsers/htjson"
	"github.com/honeycombio/honeytail/parsers/mongodb"
	"github.com/honeycombio/honeytail/parsers/mysql"
	"github.com/honeycombio/honeytail/parsers/nginx"
	"github.com/honeycombio/honeytail/tail"
	"github.com/honeycombio/libhoney-go"
	flag "github.com/jessevdk/go-flags"
)

// BuildID is set by Travis CI
var BuildID string

// internal version identifier
var version string

var validParsers = []string{
	"nginx",
	"mongo",
	"json",
	"mysql",
}

// GlobalOptions has all the top level CLI flags that honeytail supports
type GlobalOptions struct {
	APIHost string `hidden:"true" long:"api_host" description:"Host for the Honeycomb API" default:"https://api.honeycomb.io/"`

	SampleRate     uint `short:"r" long:"samplerate" description:"Only send 1 / N log lines" default:"1"`
	NumSenders     uint `short:"P" long:"poolsize" description:"Number of concurrent connections to open to Honeycomb" default:"10"`
	Debug          bool `long:"debug" description:"Print debugging output"`
	StatusInterval uint `long:"status_interval" description:"how frequently, in seconds, to print out summary info" default:"60"`

	ScrubFields []string `long:"scrub_field" description:"for the field listed, apply a one-way hash to the field content. May be specified multiple times"`
	DropFields  []string `long:"drop_field" description:"do not send the field to Honeycomb. May be specified multiple times"`
	AddFields   []string `long:"add_field" description:"add the field to every event. Field should be key=val. May be specified multiple times"`

	Reqs  RequiredOptions `group:"Required Options"`
	Modes OtherModes      `group:"Other Modes"`

	Tail tail.TailOptions `group:"Tail Options" namespace:"tail"`

	Nginx nginx.Options   `group:"Nginx Parser Options" namespace:"nginx"`
	JSON  htjson.Options  `group:"JSON Parser Options" namespace:"json"`
	MySQL mysql.Options   `group:"MySQL Parser Options" namespace:"mysql"`
	Mongo mongodb.Options `group:"MongoDB Parser Options" namespace:"mongo"`
}

type RequiredOptions struct {
	ParserName string   `short:"p" long:"parser" description:"Parser module to use. Use --list to list available options."`
	WriteKey   string   `short:"k" long:"writekey" description:"Team write key"`
	LogFiles   []string `short:"f" long:"file" description:"Log file(s) to parse. Use '-' for STDIN, use this flag multiple times to tail multiple files, or use a glob (/path/to/foo-*.log)"`
	Dataset    string   `short:"d" long:"dataset" description:"Name of the dataset"`
}

type OtherModes struct {
	Help        bool `short:"h" long:"help" description:"Show this help message"`
	ListParsers bool `short:"l" long:"list" description:"List available parsers"`
	Version     bool `short:"V" long:"version" description:"Show version"`

	WriteManPage bool `hidden:"true" long:"write-man-page" description:"Write out a man page"`
}

func main() {
	var options GlobalOptions
	flagParser := flag.NewParser(&options, flag.PrintErrors)
	flagParser.Usage = "-p <parser> -k <writekey> -f </path/to/logfile> -d <mydata>"
	if extraArgs, err := flagParser.Parse(); err != nil || len(extraArgs) != 0 {
		fmt.Println("Error: failed to parse the command line.")
		if err != nil {
			fmt.Printf("\t%s\n", err)
		} else {
			fmt.Printf("\tUnexpected extra arguments: %s\n", strings.Join(extraArgs, " "))
		}
		os.Exit(1)
	}
	rand.Seed(time.Now().UnixNano())

	if options.Debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	setVersion()
	handleOtherModes(flagParser, options)
	sanityCheckOptions(options)

	run(options)
}

// setVersion sets the internal version ID and updates libhoney's user-agent
func setVersion() {
	if BuildID == "" {
		version = "dev"
	} else {
		version = BuildID
	}
	libhoney.UserAgentAddition = fmt.Sprintf("honeytail/%s", version)
}

// handleOtherModes takse care of all flags that say we should just do something
// and exit rather than actually parsing logs
func handleOtherModes(fp *flag.Parser, options GlobalOptions) {
	if options.Modes.Version {
		fmt.Println("Honeytail version", version)
		os.Exit(0)
	}
	if options.Modes.Help {
		fp.WriteHelp(os.Stdout)
		fmt.Println("")
		os.Exit(0)
	}
	if options.Modes.WriteManPage {
		fp.WriteManPage(os.Stdout)
		os.Exit(0)
	}

	if options.Modes.ListParsers {
		fmt.Println("Available parsers:", strings.Join(validParsers, ", "))
		os.Exit(0)
	}
}

func sanityCheckOptions(options GlobalOptions) {
	switch {
	case options.Reqs.ParserName == "":
		logrus.Fatal("parser required")
	case options.Reqs.WriteKey == "" || options.Reqs.WriteKey == "NULL":
		logrus.Fatal("write key required")
	case len(options.Reqs.LogFiles) == 0:
		logrus.Fatal("log file name or '-' required")
	case options.Reqs.Dataset == "":
		logrus.Fatal("dataset name required")
	case options.Tail.ReadFrom == "end" && options.Tail.Stop:
		logrus.Fatal("Reading from the end and stopping when we get there. Zero lines to process. Ok, all done! ;)")
	case len(options.Reqs.LogFiles) > 1 && options.Tail.StateFile != "":
		logrus.Fatal("Statefile can not be set when tailing from multiple files")
	case options.Tail.StateFile != "":
		files, err := filepath.Glob(options.Reqs.LogFiles[0])
		if err != nil {
			logrus.Fatalf("Trying to glob log file %s failed: %+v\n",
				options.Reqs.LogFiles[0], err)
		}
		if len(files) > 1 {
			logrus.Fatal("Statefile can not be set when tailing from multiple files")
		}
	}
}
