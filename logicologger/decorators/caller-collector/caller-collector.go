package caller_collector

import (
	"runtime"
	"strings"

	logicologger "github.com/gnemade360/go-logger/logicologger"
)

type CallersCollector struct {
	Skip int
}

func (c *CallersCollector) Decorate(entry *logicologger.LogEvent) error {

	callers := GetStack(c.Skip)
	d := entry.Data
	d["callers"] = callers
	entry.Data = d
	return nil
}

func GetStack(skip int) []*Caller {
	calls := make([]*Caller, 0)
	i := skip
	for {
		if pc, file, line, ok := runtime.Caller(i); ok {

			fnc := runtime.FuncForPC(pc)

			caller := &Caller{
				Pc:   pc,
				File: file,
				Line: line,
				Ok:   ok,
			}
			separatePackageAndFunctionName(fnc, caller)
			calls = append(calls, caller)
		} else {
			break
		}
		i = i + 1
	}
	return calls
}

func separatePackageAndFunctionName(f *runtime.Func, caller *Caller) {
	if f == nil {
		return
	}
	funcName := f.Name()
	lastSlash := strings.LastIndexByte(funcName, '/')
	if lastSlash < 0 {
		lastSlash = 0
	}
	lastDot := strings.LastIndexByte(funcName[lastSlash:], '.') + lastSlash
	packageN := funcName[:lastDot]
	funcN := funcName[lastDot+1:]

	ls := strings.LastIndexByte(packageN, '/')
	structN := ""
	if ls > -1 {

		ldi := strings.LastIndexByte(packageN[ls:], '.') + ls
		if ldi > -1 {
			structN = packageN[ldi+1:]
		}
	}
	if caller != nil {
		caller.Package = packageN
		caller.Func = funcN
		caller.FullName = funcName
		caller.Struct = structN
	}
}

type Caller struct {
	Package  string
	Struct   string
	Func     string
	File     string
	Line     int
	FullName string
	Pc       uintptr
	Ok       bool
}
