// Package log provides a structured logger.
//
// Structured logging produces logs easily consumed later by humans or
// machines. Humans might be interested in debugging errors, or tracing
// specific requests. Machines might be interested in counting interesting
// events, or aggregating information for off-line processing. In both cases,
// it is important that the log messages are structured and actionable.
// Package log is designed to encourage both of these best practices.
//
// Basic Usage
//
// The fundamental interface is Logger. Loggers create log events from
// key/value data. The Logger interface has a single method, Log, which
// accepts a sequence of alternating key/value pairs, which this package names
// keyvals.
//
//    type Logger interface {
//        Log(keyvals ...interface{}) error
//    }
//
// Here is an example of a function using a Logger to create log events.
//
//    func RunTask(task Task, logger log.Logger) string {
//        logger.Log("taskID", task.ID, "event", "starting task")
//        ...
//        logger.Log("taskID", task.ID, "event", "task complete")
//    }
//
// The keys in the above example are "taskID" and "event". The values are
// task.ID, "starting task", and "task complete". Every key is followed
// immediately by its value.
//
// Keys are usually plain strings. Values may be any type that has a sensible
// encoding in the chosen log format. With structured logging it is a good
// idea to log simple values without formatting them. This practice allows
// the chosen logger to encode values in the most appropriate way.
//
// Log Context
//
// A log context stores keyvals that it includes in all log events. Building
// appropriate log contexts reduces repetition and aids consistency in the
// resulting log output. We can use a context to improve the RunTask example.
//
//    func RunTask(task Task, logger log.Logger) string {
//        logger = log.NewContext(logger).With("taskID", task.ID)
//        logger.Log("event", "starting task")
//        ...
//        taskHelper(task.Cmd, logger)
//        ...
//        logger.Log("event", "task complete")
//    }
//
// The improved version emits the same log events as the original for the
// first and last calls to Log. The call to taskHelper highlights that a
// context may be passed as a logger to other functions. Each log event
// created by the called function will include the task.ID even though the
// function does not have access to that value. Using log contexts this way
// simplifies producing log output that enables tracing the life cycle of
// individual tasks. (See the Context example for the full code of the
// above snippet.)
//
// Dynamic Context Values
//
// A Valuer function stored in a log context generates a new value each time
// the context logs an event. The Valuer example demonstrates how this
// feature works.
//
// Valuers provide the basis for consistently logging timestamps and source
// code location. The log package defines several valuers for that purpose.
// See Timestamp, DefaultTimestamp, DefaultTimestampUTC, Caller, and
// DefaultCaller. A common logger initialization sequence that ensures all log
// entries contain a timestamp and source location looks like this:
//
//    logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout))
//    logger = log.NewContext(logger).With("ts", log.DefaultTimestampUTC, "caller", log.DefaultCaller)
//
// Concurrent Safety
//
// Applications with multiple goroutines want each log event written to the
// same logger to remain separate from other log events. Package log provides
// two simple solutions for concurrent safe logging.
//
// NewSyncWriter wraps an io.Writer and serializes each call to its Write
// method. Using a SyncWriter has the benefit that the smallest practical
// portion of the logging logic is performed within a mutex, but it requires
// the formatting Logger to make only one call to Write per log event.
//
// NewSyncLogger wraps any Logger and serializes each call to its Log method.
// Using a SyncLogger has the benefit that it guarantees each log event is
// handled atomically within the wrapped logger, but it typically serializes
// both the formatting and output logic. Use a SyncLogger if the formatting
// logger may perform multiple writes per log event.
package log
