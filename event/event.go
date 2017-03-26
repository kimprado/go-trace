package event

import (
	"bytes"
	"fmt"
)

const (
	// Guards against a bad trace file or decoder bug from causing oom
	maxMakeSize  = 1e6
	maxStackSize = 1e3

	// Shift of the number of arguments in the first event byte.
	//
	//   src/runtime/trace.go:85~ traceArgCountShift = 6
	traceArgCountShift = 6
)

// These are the types of events that may be emitted. They are copied directly
// from the runtime/trace.go source file.
const (
	EvNone              Type = 0  // unused
	EvBatch             Type = 1  // start of per-P batch of events [pid, timestamp]
	EvFrequency         Type = 2  // contains tracer timer frequency [frequency (ticks per second)]
	EvStack             Type = 3  // stack [stack id, number of PCs, array of {PC, func string ID, file string ID, line}]
	EvGomaxprocs        Type = 4  // current value of GOMAXPROCS [timestamp, GOMAXPROCS, stack id]
	EvProcStart         Type = 5  // start of P [timestamp, thread id]
	EvProcStop          Type = 6  // stop of P [timestamp]
	EvGCStart           Type = 7  // GC start [timestamp, seq, stack id]
	EvGCDone            Type = 8  // GC done [timestamp]
	EvGCScanStart       Type = 9  // GC mark termination start [timestamp]
	EvGCScanDone        Type = 10 // GC mark termination done [timestamp]
	EvGCSweepStart      Type = 11 // GC sweep start [timestamp, stack id]
	EvGCSweepDone       Type = 12 // GC sweep done [timestamp]
	EvGoCreate          Type = 13 // goroutine creation [timestamp, new goroutine id, new stack id, stack id]
	EvGoStart           Type = 14 // goroutine starts running [timestamp, goroutine id, seq]
	EvGoEnd             Type = 15 // goroutine ends [timestamp]
	EvGoStop            Type = 16 // goroutine stops (like in select{}) [timestamp, stack]
	EvGoSched           Type = 17 // goroutine calls Gosched [timestamp, stack]
	EvGoPreempt         Type = 18 // goroutine is preempted [timestamp, stack]
	EvGoSleep           Type = 19 // goroutine calls Sleep [timestamp, stack]
	EvGoBlock           Type = 20 // goroutine blocks [timestamp, stack]
	EvGoUnblock         Type = 21 // goroutine is unblocked [timestamp, goroutine id, seq, stack]
	EvGoBlockSend       Type = 22 // goroutine blocks on chan send [timestamp, stack]
	EvGoBlockRecv       Type = 23 // goroutine blocks on chan recv [timestamp, stack]
	EvGoBlockSelect     Type = 24 // goroutine blocks on select [timestamp, stack]
	EvGoBlockSync       Type = 25 // goroutine blocks on Mutex/RWMutex [timestamp, stack]
	EvGoBlockCond       Type = 26 // goroutine blocks on Cond [timestamp, stack]
	EvGoBlockNet        Type = 27 // goroutine blocks on network [timestamp, stack]
	EvGoSysCall         Type = 28 // syscall enter [timestamp, stack]
	EvGoSysExit         Type = 29 // syscall exit [timestamp, goroutine id, seq, real timestamp]
	EvGoSysBlock        Type = 30 // syscall blocks [timestamp]
	EvGoWaiting         Type = 31 // denotes that goroutine is blocked when tracing starts [timestamp, goroutine id]
	EvGoInSyscall       Type = 32 // denotes that goroutine is in syscall when tracing starts [timestamp, goroutine id]
	EvHeapAlloc         Type = 33 // memstats.heap_live change [timestamp, heap_alloc]
	EvNextGC            Type = 34 // memstats.next_gc change [timestamp, next_gc]
	EvTimerGoroutine    Type = 35 // denotes timer goroutine [timer goroutine id]
	EvFutileWakeup      Type = 36 // denotes that the previous wakeup of this goroutine was futile [timestamp]
	EvString            Type = 37 // string dictionary entry [ID, length, string]
	EvGoStartLocal      Type = 38 // goroutine starts running on the same P as the last event [timestamp, goroutine id]
	EvGoUnblockLocal    Type = 39 // goroutine is unblocked on the same P as the last event [timestamp, goroutine id, stack]
	EvGoSysExitLocal    Type = 40 // syscall exit on the same P as the last event [timestamp, goroutine id, real timestamp]
	EvGoStartLabel      Type = 41 // goroutine starts running with label [timestamp, goroutine id, seq, label string id]
	EvGoBlockGC         Type = 42 // goroutine blocks on GC assist [timestamp, stack]
	EvGCMarkAssistStart Type = 43 // GC mark assist start [timestamp, stack]
	EvGCMarkAssistDone  Type = 44 // GC mark assist done [timestamp]
	EvCount             Type = 45
)

// Type represents the type of trace event.
type Type byte

// Valid returns true if the event Type is valid, false otherwise.
func (t Type) Valid() bool {
	return EvNone < t && t < EvCount
}

// Name returns the name of this event type.
func (t Type) Name() string {
	return schemas[t%EvCount].Name
}

// Since returns the version that this event was introduced.
func (t Type) Since() Version {
	return schemas[t%EvCount].Since
}

// Args returns an ordered list of arguments this type of event will contain.
func (t Type) Args() []string {
	return schemas[t%EvCount].Args
}

// String implements fmt.Stringer by returning a helpful string describing this
// event type.
func (t Type) String() string {
	return fmt.Sprintf(`encoding.%v`, t.Name())
}

type Event struct {

	// Trace is the Go trace this event belongs to, will always be non-nil for
	// traces produced by this library.
	// Trace *Trace

	// Type is the type of this Event.
	Type Type

	// Args will contain all the Event specific arguments, excluding sequences
	// and timestamps. All uleb128 values are decoded here including arbitrary
	// length events like Stack.
	Args []uint64

	// Data may be nil or a slice containing Event data for arguments that are not
	// uleb128 encoded. Currently only the string event fits this criteria.
	//
	// @TODO Remove all together in favor of storing in *Trace?
	Data []byte

	// Id's of the P and G associated with this event. With G being a goroutine
	// and P a resource that is required to execute Go code.
	P, G int64

	// Ts is the timestamp of the event.
	Ts int64

	// Off is the offset of the first byte for this Event relative to the begining
	// of the input stream.
	Off int

	// // Seq is the sequence of the event.
	// //
	// // For Version1 a sequence was emitted in EvBatch to seed the next increment
	// // operation of the ongoing sequence counter to be used for event ordering.
	// // For Version2 and later this is set to the order the Event was emitted from
	// // the input stream although it is not used.
	// Seq uint64

	// // Stack is the Stack trace associated with this event, if any. It's possible
	// // that events which should have a stack are the zero value for one of two
	// // reasons, the stack event was not yet sent over the wire or the Stack was
	// // omitted entirely by the runtime. Stacks may be shared across multiple
	// // events and should not be mutated, make a copy instead.
	// Stack Stack
}

// Reset will reset this event for reuse.
func (e *Event) Reset() {
	args, data := e.Args[0:0], e.Data[0:0]
	*e = Event{Args: args, Data: data}
}

// String implements fmt.Stringer by returning a helpful string describing this
// event type.
func (e *Event) String() string {
	switch e.Type {
	case EvString:
		return fmt.Sprintf(`encoding.%v(%q)`, schemas[e.Type%EvCount].Name, string(e.Data))
	case EvFrequency:
		return fmt.Sprintf(`encoding.%v(%v)`, schemas[e.Type%EvCount].Name, e.Args[0])
	}
	return fmt.Sprintf(`encoding.%v`, schemas[e.Type%EvCount].Name)
}

type Stack []Frame

func (s Stack) Empty() bool {
	return len(s) == 0
}

func (s Stack) String() string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("encoding.Stack[%v]:\n", len(s)))
	for _, frame := range s {
		buf.WriteString(frame.String() + "\n")
	}
	return buf.String()
}

type Frame struct {
	PC, Line   uint64
	Func, File string
}

func (f Frame) String() string {
	return fmt.Sprintf("%v [PC %v]\n\t%v:%v", f.Func, f.PC, f.File, f.Line)
}