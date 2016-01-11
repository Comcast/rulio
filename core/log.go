// Copyright 2015 Comcast Cable Communications Management, LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// End Copyright

package core

// "error": error in system (not due to user input).
//
// "uerr": error due to user input.
//
// "when": string indicating where in some operation we are.  Note
// that the code below automatically addes filename:linenum.
//
// "loc": string naming the location.
//
// "ruleId": string naming the rule ID.
//
// "rule": string of rule JSON.
//
// "rm": map representation of a rule.
//
// "eid": string naming the event ID.
//
// "em": map representation of an event.
//
// "event": string of event JSON.
//
// "fact": string of event JSON.
//
// "fm": map representation of a fact.
//
// "pat": string of pattern JSON.
//
// "pm": map representation of a pattern.
//
// "bss": Bindingss.
//
// "bs": Bindings.
//
// "elapsed": Number representing nanoseconds.  Also see 'Timer',
// which helps generate timings.

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/robertkrimen/otto"
)

const (
	// The log record property given to the first string arg to
	// Log().
	LogKeyOp = "op"
)

// Logger is a simple interface to a mostly generic logging functionality.
type Logger interface {
	Log(level LogLevel, args ...interface{})
	Metric(name string, args ...interface{})
}

var DefaultLogger Logger = NewSimpleLogger(os.Stdout)

var BenchLogger Logger = NewSimpleLogger(ioutil.Discard)

var LogFormatFromEnv = os.Getenv("RULES_LOGS")

func init() {
	// log.SetFlags(0) // log.Lmicroseconds | log.Ldate | log.Lshortfile)
	log.SetFlags(log.Lmicroseconds | log.Ldate | log.Lshortfile)
}

// Core logging granularity
//
// We have several ubiquitous dimensions for log records.  Severity,
// "origin", and component.  "Origin" should indicate the source of
// the data that triggered the log record.  Examples: internal code or
// data given by a user.
//
// We hopfully will just log everything.  If so, we won't actually
// need to filter log records based on these standard dimensions.  But
// in case we do want to do some filtering, we can.  Filtering is
// convenient during development.
//
// Filtering aside, we provide some gear to make these dimensions
// explicit and hopefully efficient.
//
// We still use the "level" terminology even though a "level" is no
// longer just a (severity) level.  How a "level" encodes each of
// these dimensions: severity, origin, and component.

// LogLevel is really a bit field.
//
// Confusingly still using the old name.
type LogLevel uint64

const (
	// SEVMASK is the list of severity bits.  Severities are
	// defined below.
	SEVMASK LogLevel = 0xff

	// ORIMASK is the list of origin bits.  An "origin" could be
	// user data, an external system, Core code itself, etc.
	ORIMASK LogLevel = 0xff00

	// COMPMASK is the list of component bits.  Components are
	// defined below.
	COMPMASK LogLevel = 0xffff0000
)

const (
	// We have three banks of logging flags: severity, origin, and
	// component.  All are packed into a uint64 (though could be a
	// uint32).
	//
	// Severity almost follows the usual nomenclature, but we just
	// use single bits.

	// Be careful when modifying this stuff.  We assign each bit
	// in order.

	CRIT LogLevel = 1 << iota
	ERROR
	WARN
	POINT // Might not belong here.
	TIMER // Might not belong here.
	INFO
	DEBUG
	ABSURD

	// The origin attempts indicate the source of the log message.

	// SYS origin means the core itself.
	SYS
	// USR origin means the log message was caused by user data.
	USR
	// APP origin means the log message was caused by an
	// "application", which typically means an external service.
	APP
	// METRIC is a "metric"
	METRIC

	// We have some unused bits to round out the origin byte.
	_
	_
	_
	_

	// Since we now understand the main Core components, we can
	// enumerate them.  We don't need to confine ourselves to a
	// byte.  We can use the rest of the word.  But we'll mask
	// 0xffff0000 for now to give us 16 bits for components.

	// MISC is the catch-all component.
	MISC
	// MATCH is for pattern matching code.
	MATCH
	// STATE is for the in-memory state indexes.
	STATE
	// STORAGE is for the persistence implementations and associated layer.
	STORAGE
	// EVENT is for event processing.
	EVENT
	// SYSTEM is for sys/system.go: The outer-most package API layer.
	SYSTEM
	// EXTERN for external components.  Kinda sad.
	EXTERN
)

// getCoreLogLevel returns a string for the severity of the given "level".
//
// Could generate Stringer methods ...
func getCoreLogLevel(level LogLevel) string {
	switch level & SEVMASK {
	case CRIT:
		return "crit"
	case ERROR:
		return "error"
	case WARN:
		return "warn"
	case POINT:
		return "point"
	case TIMER:
		return "timer"
	case INFO:
		return "info"
	case DEBUG:
		return "debug"
	case ABSURD:
		return "absurd"
	default:
		return "unknown"
	}
}

// getLogOrigin returns a string for the origin of the given "level".
func getLogOrigin(level LogLevel) string {
	switch level & ORIMASK {
	case APP:
		return "app"
	case SYS:
		return "sys"
	case USR:
		return "usr"
	default:
		return "unknown"
	}
}

// getLogComponent returns a string for the component of the given "level".
//
// Could generate Stringer methods ...
func getLogComponent(level LogLevel) string {
	switch level & COMPMASK {
	case MISC:
		return "misc"
	case MATCH:
		return "match"
	case STATE:
		return "state"
	case STORAGE:
		return "storage"
	case EVENT:
		return "event"
	case SYSTEM:
		return "system"
	case EXTERN:
		return "extern"
	default:
		return "unknown"
	}
}

const (

	// ALLSEV means any severities.
	ANYSEV = SEVMASK

	// ANYORI means any "origin".
	ANYORI = ORIMASK

	// ANYCOMP means any component.
	ANYCOMP = COMPMASK

	// NOTHING is a mask that should result in no logs.
	NOTHING LogLevel = 0x0
	// EVERYTHING is a mask that should result in logging everything.
	EVERYTHING LogLevel = ^NOTHING

	// UERR is a user "error".
	UERR = ERROR | USR
	// APERR is an application "error".
	APERR = ERROR | APP

	// ANYINFO logs anything at or above the INFO level.  Also
	// logs all timers.
	ANYINFO = TIMER | CRIT | ERROR | WARN | INFO | ANYORI | ANYCOMP

	// ANYWARN logs anything at or about the WARN level.
	ANYWARN = CRIT | ERROR | WARN | ANYORI | ANYCOMP
)

// ParseVerbosity parses and evals and log mask.
//
// This function is a little crazy.  It uses Javascript to parse and
// eval the given string.  The various log constants are in the
// Javascript environment.  For example, the string "ERROR|APP" would
// parse/eval 'ERROR|APP'.  Since we're using Javascript, you can use
// Javascript numerics, too.  Example: "0xffffffff".
//
// The empty string is interpreted as 'EVERYTHING'.  Use 'NOTHING' to
// get that.
func ParseVerbosity(s string) (LogLevel, error) {

	if s == "" {
		s = "EVERYTHING"
	}

	js := otto.New()

	js.Set("CRIT", CRIT)
	js.Set("ERROR", ERROR)
	js.Set("WARN", WARN)
	js.Set("POINT", POINT)
	js.Set("TIMER", TIMER)
	js.Set("INFO", INFO)
	js.Set("DEBUG", DEBUG)
	js.Set("ABSURD", ABSURD)
	js.Set("SYS", SYS)
	js.Set("USR", USR)
	js.Set("APP", APP)
	js.Set("MISC", MISC)
	js.Set("MATCH", MATCH)
	js.Set("STATE", STATE)
	js.Set("STORAGE", STORAGE)
	js.Set("EVENT", EVENT)
	js.Set("SYSTEM", SYSTEM)
	js.Set("EXTERN", EXTERN)
	js.Set("NOTHING", NOTHING)
	js.Set("EVERYTHING", EVERYTHING)
	js.Set("UERR", UERR)
	js.Set("APERR", APERR)
	js.Set("ANYSEV", ANYSEV)
	js.Set("ANYORI", ANYORI)
	js.Set("ANYCOMP", ANYCOMP)
	js.Set("ANYINFO", ANYINFO)
	js.Set("ANYWARN", ANYWARN)

	v, err := js.Run(s)
	if err != nil {
		return NOTHING, err
	}
	level, err := v.Export()
	if err != nil {
		return NOTHING, err
	}
	switch n := level.(type) {
	case float64:
		return LogLevel(n), nil
	case int32:
		return LogLevel(n), nil
	case int64:
		return LogLevel(n), nil
	case uint64:
		return LogLevel(n), nil
	default:
		return NOTHING, fmt.Errorf("can't handle %T (%T)", level, level)
	}
}

// defaultLogFields makes sure we have at least one bit set for each
// of SEVMASK, ORIMASK, and COMPMASK.
//
// This function is called on given log levels in the 'Log()'
// function.  Any 'Log()' call will at least show up as 'DEBUG'
// (level), 'SYS' (component), 'MISC' (origin) if not otherwise
// specified.
func defaultLogFields(n LogLevel) LogLevel {
	if 0 == SEVMASK&n {
		n = n | DEBUG
	}
	if 0 == ORIMASK&n {
		n = n | SYS
	}
	if 0 == COMPMASK&n {
		n = n | MISC
	}
	return n
}

// getVerbosity attempts to find the current LogLevel.
//
// By default, it's 'EVERYTHING'.  'ctx.Verbosity' overrides that
// default, and 'ctx.GetLoc().Control.Verbosity' overrides that
// override.
func getVerbosity(ctx *Context) LogLevel {
	// ctx.GetLoc().Control.Verbosity overrides ctx.Verbosity
	verbosity := EVERYTHING

	if ctx != nil {
		verbosity = ctx.Verbosity
		loc := ctx.GetLoc()
		if loc != nil {
			c := loc.Control()
			if c != nil {
				if c.Verbosity != NOTHING {
					verbosity = c.Verbosity
				}
			}
		}
	}

	return verbosity
}

// loggable determines if we should emit a log record at the given level.
//
// The decision is based on a call to 'getVerbosity(ctx)'.  A message
// is loggable if each of SEVMASK, ORIMASK, and COMPMASK masks are
// non-zero.  In other words, a severity, origin, and component all
// have to match something.
func loggable(ctx *Context, level LogLevel) bool {
	return loggableFor(level, getVerbosity(ctx))
}

func loggableFor(level LogLevel, given LogLevel) bool {
	vl := given & level
	return 0 < SEVMASK&vl && 0 < ORIMASK&vl && 0 < COMPMASK&vl
}

// MakeLogRecord is used by Log() to add log data to a context.
func MakeLogRecord(args []interface{}) map[string]interface{} {
	rec := make(map[string]interface{})
	n := len(args)
	for i := 0; i < n; i += 2 {
		var key string
		var val interface{}
		if i+1 < n {
			val = args[i+1]
		}
		switch s := args[i].(type) {
		case string:
			key = s
		default:
			key = fmt.Sprintf("%s", args[i])
		}
		if SystemParameters.LogRecordValueLimit < len(key) {
			key = key[0:SystemParameters.LogRecordValueLimit] + "..."
		}
		rec[key] = val
	}

	return rec
}

// abbreviateCodepath drops most of the leading directories from the given path.
//
// Used to strip irrelevant directories from source code paths.
//
// This function is called very often.  Should be cheap.
func abbreviateCodepath(path string) string {
	if i := strings.Index(path, "XfinityRulesService/csv-rules-core"); 0 < i {
		return "rules/" + path[i+30:]
	}
	return path
}

// getCallerLine looks up the filename:linenum in the call stack.
func getCallerLine(n int) string {
	_, file, line, _ := runtime.Caller(n)
	return abbreviateCodepath(file) + ":" + strconv.Itoa(line)
}

// addCallerLine, if LogCallerLine is true, adds a filename:linenum
// property to the given args.
//
// Looks up three levels in the call stack.
func addCallerLine(args []interface{}) []interface{} {
	if SystemParameters.LogCallerLine {
		return append(args, "_at", getCallerLine(3))
	}
	return args
}

type LogHook func(level LogLevel, args ...interface{})

// Log is the top-level API for logging everything.
//
// 'Args' should have an odd number or args.  The first arg should be
// a string, which is typically the name of the calling function
// (usually qualified with the package name).  The rest of the args
// are implement key/value pairs.  The even args, which are property
// names, should be strings.  The odd args, which are the respective
// values, can be anything.
//
// If GetVerbosity() < level, then do nothing.
//
// If the given context has a 'LogAccumulator', then 'MakeLogRecord()'
// is called to generate a log record that is appended to that
// accumulator.
func Log(level LogLevel, ctx *Context, args ...interface{}) {

	level = defaultLogFields(level)

	if !loggable(ctx, level) {
		return
	}

	more := make([]interface{}, 0, 30)
	more = append(more, LogKeyOp)
	more = append(more, args...)
	if ctx != nil {
		for p, v := range ctx.logProps {
			more = append(more, p)
			more = append(more, v)
		}
	}
	args = more

	// =======
	// 	//args = append([]interface{}{LogKeyOP}, args...)
	// 	tmp := make([]interface{}, 1, len(args)+20)
	// 	tmp[0] = LogKeyOP
	// 	args = append(tmp, args...)

	locName := "none"
	if ctx != nil && ctx.GetLoc() != nil {
		locName = ctx.GetLoc().Name
	}

	args = append(args,
		"corelev", getCoreLogLevel(level),
		"origin", getLogOrigin(level),
		"comp", getLogComponent(level),
		"ctxLocation", locName)
	args = addCallerLine(args)

	{
		locName := "none"
		if ctx != nil && ctx.GetLoc() != nil {
			locName = ctx.GetLoc().Name
		}
		args = append(args, "ctxLocation", locName)
	}
	// =======
	// 	var format string
	// 	if ctx != nil && ctx.GetLoc() != nil {
	// 		c := ctx.GetLoc().Control()
	// 		if c != nil {
	// 			format = c.Logging
	// 		}
	// 	}
	// 	if format == "" && ctx != nil {
	// 		// ctx.Logging access is technically not thread-safe.
	// 		format = ctx.Logging
	// 	}

	// 	lvl := getExternalLogLevel(level)
	// >>>>>>> master

	logger := DefaultLogger
	if ctx != nil {

		// Add to the context's accumulator (if any)
		var acc *Accumulator
		ctx.RLock()
		if loggableFor(level, ctx.LogAccumulatorLevel) {
			acc = ctx.LogAccumulator
		}
		ctx.RUnlock()

		if ctx.LogHook != nil {
			ctx.LogHook(level, args...)
		}

		if acc != nil {
			acc.Add(MakeLogRecord(args))
		}
	}

	metricKey, ok := args[1].(string) // Already added "op"
	if !ok {
		metricKey = Gorep(args[1]) // Should say more
	}
	if level&METRIC == METRIC {
		// If we want only METRIC, then only do that.
		logger.Metric(metricKey, args[2:]...)
	} else {
		if 0 < level&METRIC {
			logger.Metric(metricKey, args[2:]...)
		}
		logger.Log(level, args...)
	}
}

func Metric(ctx *Context, args ...interface{}) {
	Log(METRIC, ctx, args...)
}

type PointHook func(ctx *Context, namespace string, metric string, val interface{}, unit string, more ...string)

// Point generates a log line that reports point that applications
// might want to monitor.
//
// A system dashboard is a good example.  This function is not a
// generic logging function, and it is not intended to carry much (or
// any) optional, fine-grained data.  Instead, it's intended to be
// relatively robust and simple in order to make dashboard processing
// relatively simple.  See 'tools/inload' for a rough example.
//
// 'Metric' is the label for what you are tracking.  Examples:
// 'RuleTriggered', 'RuleTimeElapsed' (nanoseconds),
// 'RuleActionExecutions'.
//
// 'Val' is the numeric value.  Don't use non-numeric values.  Units
// are implicitly specified by the metric.
//
// Ctx.app_id is added to the varargs: '"appid", Ctx.app_id'.
//
// Example usage:
//
//   Point(ctx, "", "RuleTimeElapsed", elapsed, "Microseconds")
//
// Log level is POINT.
//
// If ctx.PointHook is not nil, that function is called with almost
// the same arguments.
//
// This function is designed to be compatible with CloudWatch custom
// metrics, RRDB-style timeseries databases, and similar metrics
// systems.
//
// See example PointHook in 'rulesys/main.go'.
func Point(ctx *Context, namespace string, metric string, val interface{}, unit string, more ...string) {
	if ctx != nil {
		if app := ctx.Prop("app_id"); app != nil {
			s, ok := app.(string)
			if !ok {
				s = "unknown"
			}
			more = append(more, "app_id", s)
		}
	}

	if namespace == "" {
		namespace = "rulesservice"
	}

	if ctx != nil && ctx.PointHook != nil {
		ctx.PointHook(ctx, namespace, metric, val, unit, more...)
	}

	if 0 == len(more) {
		Log(POINT, ctx, "Point", "namespace", namespace, "metric", metric, "value", val, "unit", unit)
	} else {
		// Special case to skip some unnecessary allocation.
		args := []interface{}{"Point", "namespace", namespace, "metric", metric, "value", val, "unit", unit}
		// Can't more... because more is a []string, not []interface{}.
		for _, arg := range more {
			args = append(args, arg)
		}
		Log(POINT, ctx, args...)
	}
}

// logFacti generates a crude string representation of the given fact
// to give to 'Log'.
//
// We don't just let 'Log' generate a better representation because
// the resulting type(s) might conflict across calls for a "fact".
//
// "fact": a Go map
// "factjs": a JSON representation of an event,
// "factid": a string representation of a fact id,
// "facti": a string representation of an interface{} that's a fact: fmt.Sprintf("%#v", fact)`
//
// ToDo: Something better.
func logFacti(fact interface{}) string {
	return fmt.Sprintf("%#v", fact)
}
