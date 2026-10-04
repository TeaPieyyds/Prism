package sctp

import (
	"fmt"
	"os"
	"strings"
)

func sctpDebugf(format string, args ...any) {
	if os.Getenv("TANDBG") == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "[TANDBG] "+format+"\n", args...)
}

func chunkNames(chunks []chunk) string {
	if len(chunks) == 0 {
		return "none"
	}
	names := make([]string, 0, len(chunks))
	for _, c := range chunks {
		names = append(names, chunkName(c))
	}
	return strings.Join(names, ",")
}

func chunkName(c chunk) string {
	switch c.(type) {
	case *chunkPayloadData:
		return ctPayloadData.String()
	case *chunkInit:
		return ctInit.String()
	case *chunkInitAck:
		return ctInitAck.String()
	case *chunkSelectiveAck:
		return ctSack.String()
	case *chunkHeartbeat:
		return ctHeartbeat.String()
	case *chunkHeartbeatAck:
		return ctHeartbeatAck.String()
	case *chunkAbort:
		return ctAbort.String()
	case *chunkShutdown:
		return ctShutdown.String()
	case *chunkShutdownAck:
		return ctShutdownAck.String()
	case *chunkError:
		return ctError.String()
	case *chunkCookieEcho:
		return ctCookieEcho.String()
	case *chunkCookieAck:
		return ctCookieAck.String()
	case *chunkShutdownComplete:
		return ctShutdownComplete.String()
	case *chunkReconfig:
		return ctReconfig.String()
	case *chunkForwardTSN:
		return ctForwardTSN.String()
	default:
		return fmt.Sprintf("%T", c)
	}
}
