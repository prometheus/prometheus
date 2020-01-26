package agent

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/armon/circbuf"
	"github.com/armon/go-metrics"
	"github.com/hashicorp/serf/serf"
)

const (
	windows = "windows"

	// maxBufSize limits how much data we collect from a handler.
	// This is to prevent Serf's memory from growing to an enormous
	// amount due to a faulty handler.
	maxBufSize = 8 * 1024

	// warnSlow is used to warn about a slow handler invocation
	warnSlow = time.Second
)

var sanitizeTagRegexp = regexp.MustCompile(`[^A-Z0-9_]`)

// invokeEventScript will execute the given event script with the given
// event. Depending on the event, the semantics of how data are passed
// are a bit different. For all events, the SERF_EVENT environmental
// variable is the type of the event. For user events, the SERF_USER_EVENT
// environmental variable is also set, containing the name of the user
// event that was fired.
//
// In all events, data is passed in via stdin to facilitate piping. See
// the various stdin functions below for more information.
func invokeEventScript(logger *log.Logger, script string, self serf.Member, event serf.Event) error {
	defer metrics.MeasureSince([]string{"agent", "invoke", script}, time.Now())
	output, _ := circbuf.NewBuffer(maxBufSize)

	// Determine the shell invocation based on OS
	var shell, flag string
	if runtime.GOOS == windows {
		shell = "cmd"
		flag = "/C"
	} else {
		shell = "/bin/sh"
		flag = "-c"
	}

	cmd := exec.Command(shell, flag, script)
	cmd.Env = append(os.Environ(),
		"SERF_EVENT="+event.EventType().String(),
		"SERF_SELF_NAME="+self.Name,
		"SERF_SELF_ROLE="+self.Tags["role"],
	)
	cmd.Stderr = output
	cmd.Stdout = output

	// Add all the tags
	for name, val := range self.Tags {
		//http://stackoverflow.com/questions/2821043/allowed-characters-in-linux-environment-variable-names
		//(http://pubs.opengroup.org/onlinepubs/000095399/basedefs/xbd_chap08.html for the long version)
		//says that env var names must be in [A-Z0-9_] and not start with [0-9].
		//we only care about the first part, so convert all chars not in [A-Z0-9_] to _
		sanitizedName := sanitizeTagRegexp.ReplaceAllString(strings.ToUpper(name), "_")
		tag_env := fmt.Sprintf("SERF_TAG_%s=%s", sanitizedName, val)
		cmd.Env = append(cmd.Env, tag_env)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	switch e := event.(type) {
	case serf.MemberEvent:
		go memberEventStdin(logger, stdin, &e)
	case serf.UserEvent:
		cmd.Env = append(cmd.Env, "SERF_USER_EVENT="+e.Name)
		cmd.Env = append(cmd.Env, fmt.Sprintf("SERF_USER_LTIME=%d", e.LTime))
		go streamPayload(logger, stdin, e.Payload)
	case *serf.Query:
		cmd.Env = append(cmd.Env, "SERF_QUERY_NAME="+e.Name)
		cmd.Env = append(cmd.Env, fmt.Sprintf("SERF_QUERY_LTIME=%d", e.LTime))
		go streamPayload(logger, stdin, e.Payload)
	default:
		return fmt.Errorf("Unknown event type: %s", event.EventType().String())
	}

	// Start a timer to warn about slow handlers
	slowTimer := time.AfterFunc(warnSlow, func() {
		logger.Printf("[WARN] agent: Script '%s' slow, execution exceeding %v",
			script, warnSlow)
	})

	if err := cmd.Start(); err != nil {
		return err
	}

	// Warn if buffer is overritten
	if output.TotalWritten() > output.Size() {
		logger.Printf("[WARN] agent: Script '%s' generated %d bytes of output, truncated to %d",
			script, output.TotalWritten(), output.Size())
	}

	err = cmd.Wait()
	slowTimer.Stop()
	logger.Printf("[DEBUG] agent: Event '%s' script output: %s",
		event.EventType().String(), output.String())
	if err != nil {
		return err
	}

	// If this is a query and we have output, respond
	if query, ok := event.(*serf.Query); ok && output.TotalWritten() > 0 {
		if err := query.Respond(output.Bytes()); err != nil {
			logger.Printf("[WARN] agent: Failed to respond to query '%s': %s",
				event.String(), err)
		}
	}

	return nil
}

// eventClean cleans a value to be a parameter in an event line.
func eventClean(v string) string {
	v = strings.Replace(v, "\t", "\\t", -1)
	v = strings.Replace(v, "\n", "\\n", -1)
	return v
}

// Sends data on stdin for a member event.
//
// The format for the data is unix tool friendly, separated by whitespace
// and newlines. The structure of each line for any member event is:
// "NAME    ADDRESS    ROLE    TAGS" where the whitespace is actually tabs.
// The name and role are cleaned so that newlines and tabs are replaced
// with "\n" and "\t" respectively.
func memberEventStdin(logger *log.Logger, stdin io.WriteCloser, e *serf.MemberEvent) {
	defer stdin.Close()
	for _, member := range e.Members {
		// Format the tags as tag1=v1,tag2=v2,...
		var tagPairs []string
		for name, value := range member.Tags {
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s", name, value))
		}
		tags := strings.Join(tagPairs, ",")

		// Send the entire line
		_, err := stdin.Write([]byte(fmt.Sprintf(
			"%s\t%s\t%s\t%s\n",
			eventClean(member.Name),
			member.Addr.String(),
			eventClean(member.Tags["role"]),
			eventClean(tags))))
		if err != nil {
			return
		}
	}
}

// Sends data on stdin for an event. The stdin simply contains the
// payload (if any).
// Most shells read implementations need a newline, force it to be there
func streamPayload(logger *log.Logger, stdin io.WriteCloser, buf []byte) {
	defer stdin.Close()

	// Append a newline to payload if missing
	payload := buf
	if len(payload) > 0 && payload[len(payload)-1] != '\n' {
		payload = append(payload, '\n')
	}

	if _, err := stdin.Write(payload); err != nil {
		logger.Printf("[ERR] Error writing payload: %s", err)
		return
	}
}
