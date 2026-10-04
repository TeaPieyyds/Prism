package nethernet

import (
	"fmt"
	"os"

	"github.com/Happy2018new/nemc-tan-lobby-solver/core/webrtc"
)

func tanDebugf(format string, args ...any) {
	if os.Getenv("TANDBG") == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "[TANDBG] "+format+"\n", args...)
}

func candidateSummary(candidate webrtc.ICECandidate) string {
	related := ""
	if candidate.RelatedAddress != "" || candidate.RelatedPort != 0 {
		related = fmt.Sprintf(" related=%s:%d", candidate.RelatedAddress, candidate.RelatedPort)
	}
	return fmt.Sprintf(
		"%s/%s %s:%d priority=%d%s",
		candidate.Typ.String(),
		candidate.Protocol.String(),
		candidate.Address,
		candidate.Port,
		candidate.Priority,
		related,
	)
}
