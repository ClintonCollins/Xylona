// Package diagnosis captures bounded evidence for failed game-server executions.
package diagnosis

import (
	"errors"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// Report describes one execution's failure without retaining its launch inputs.
type Report struct {
	ExecutionID       string
	AttemptStartedAt  time.Time
	OccurredAt        time.Time
	Stage             string
	Category          string
	Error             string
	Evidence          string
	MatchedEvidence   string
	Truncated         bool
	EvidenceAvailable bool
	ExitCode          *int
}

// Failure stages distinguish rejected starts from processes that actually ran.
const (
	StagePreStart       = "pre_start"
	StageLaunch         = "launch"
	StageRuntime        = "runtime"
	StageUnknownOutcome = "unknown_outcome"
	StageUnknown        = "unknown"
)

// Categories are conservative explanations supported by captured evidence.
const (
	CategoryUnknown           = "unknown"
	CategoryMissingExecutable = "missing_executable"
	CategoryPermissionDenied  = "permission_denied"
	CategoryPortInUse         = "port_in_use"
	CategoryDiskFull          = "disk_full"
	CategoryIncompleteSetup   = "incomplete_setup"
	CategoryNodeUnavailable   = "node_unavailable"
)

// Evidence limits apply before reports cross process or persistence boundaries.
const (
	MaxEvidenceBytes = 32 * 1024
	MaxEvidenceLines = 200
	MaxErrorBytes    = 4096
)

var (
	terminalEscape = regexp.MustCompile("\x1b(?:\\[[0-?]*[ -/]*[@-~]|\\][^\x07\x1b]*(?:\x07|\x1b\\\\))")
	playerChat     = regexp.MustCompile(`<[^>\n]+>\s`)
	logRules       = []struct {
		category string
		pattern  *regexp.Regexp
	}{
		{CategoryMissingExecutable, regexp.MustCompile(`(?i)(executable file not found in|exec(?:vp|ve)?: .*not found|(?:fork/exec|createprocess) .*: (?:no such file or directory|the system cannot find the file specified))`)},
		{CategoryPermissionDenied, regexp.MustCompile(`(?i)(?:open|mkdir|chdir|fork/exec|exec|createprocess|bind)(?:\([^\n]*\)|\s+[^\n]*|): (?:permission denied|access is denied)`)},
		{CategoryPortInUse, regexp.MustCompile(`(?i)(?:bind(?:\([^\n]*\))?: .*address already in use|(?:failed|unable) to bind[^\n]*(?:address already in use|only one usage of each socket address)|bindexception: address already in use)`)},
		{CategoryDiskFull, regexp.MustCompile(`(?i)(?:write|open|mkdir|fallocate|error|exception|failed)[^\n]*(?:no space left on device|there is not enough space on the disk|disk full)`)},
	}
)

// Capture redacts known secrets before bounding or classifying text. The caller
// supplies execution metadata and whether a console snapshot was available.
func Capture(err error, output string, secrets ...string) Report {
	report := Report{Category: CategoryUnknown, EvidenceAvailable: true}
	report.Evidence, report.Truncated = Tail(sanitize(output, secrets), MaxEvidenceLines, MaxEvidenceBytes)
	if err != nil {
		errorText, truncated := Tail(sanitize(err.Error(), secrets), MaxEvidenceLines, MaxErrorBytes)
		report.Error = errorText
		report.Truncated = report.Truncated || truncated
		report.Category = errorCategory(err)
	}
	if report.Category != CategoryUnknown {
		return report
	}
	report.Category, report.MatchedEvidence = classifyText(report.Error + "\n" + report.Evidence)
	return report
}

// Bound reapplies size limits to reports received across an untrusted boundary.
func Bound(report Report) Report {
	if report.AttemptStartedAt.IsZero() {
		report.AttemptStartedAt = AttemptTime(report.ExecutionID)
	}
	switch report.Stage {
	case StagePreStart, StageLaunch, StageRuntime, StageUnknownOutcome, StageUnknown:
	default:
		report.Stage = StageUnknown
	}
	switch report.Category {
	case CategoryUnknown, CategoryMissingExecutable, CategoryPermissionDenied, CategoryPortInUse, CategoryDiskFull, CategoryIncompleteSetup, CategoryNodeUnavailable:
	default:
		report.Category = CategoryUnknown
	}
	var truncated bool
	report.Evidence, truncated = Tail(sanitize(report.Evidence, nil), MaxEvidenceLines, MaxEvidenceBytes)
	report.Truncated = report.Truncated || truncated
	report.Error, truncated = Tail(sanitize(report.Error, nil), MaxEvidenceLines, MaxErrorBytes)
	report.Truncated = report.Truncated || truncated
	report.MatchedEvidence, truncated = Tail(sanitize(report.MatchedEvidence, nil), 1, MaxErrorBytes)
	report.Truncated = report.Truncated || truncated
	return report
}

// AttemptTime recovers controller launch ordering from an execution identifier
// even when an older node cannot echo the explicit attempt timestamp.
func AttemptTime(executionID string) time.Time {
	id, errParse := uuid.Parse(executionID)
	if errParse != nil || id.Version() != 7 {
		return time.Time{}
	}
	seconds, nanos := id.Time().UnixTime()
	return time.Unix(seconds, nanos).UTC()
}

// Redact removes known credentials and terminal control sequences from text.
func Redact(text string, secrets ...string) string {
	return sanitize(text, secrets)
}

// Tail retains the end of text within both limits without splitting UTF-8.
func Tail(text string, maxLines, maxBytes int) (string, bool) {
	text = strings.ToValidUTF8(text, "\uFFFD")
	text = strings.TrimRight(text, "\n")
	originalLength := len(text)
	lines := strings.Split(text, "\n")
	if len(lines) > maxLines {
		text = strings.Join(lines[len(lines)-maxLines:], "\n")
	}
	if len(text) > maxBytes {
		start := len(text) - maxBytes
		for start < len(text) && text[start]&0xc0 == 0x80 {
			start++
		}
		text = text[start:]
	}
	return text, len(text) < originalLength
}

func sanitize(text string, secrets []string) string {
	// Longest first prevents a short secret from exposing the rest of a longer
	// credential that contains it. Redact before truncating at any boundary.
	values := slices.Clone(secrets)
	slices.SortFunc(values, func(a, b string) int { return len(b) - len(a) })
	for _, value := range values {
		if value != "" {
			text = strings.ReplaceAll(text, value, "[redacted]")
		}
	}
	text = plainText(text)
	for _, value := range values {
		value = plainText(value)
		if value != "" {
			text = strings.ReplaceAll(text, value, "[redacted]")
		}
	}
	return text
}

func plainText(text string) string {
	text = terminalEscape.ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.Map(func(r rune) rune {
		if r == '\r' {
			return '\n'
		}
		if r != '\n' && r != '\t' && unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)
}

func errorCategory(err error) string {
	platformCategory := platformErrorCategory(err)
	if platformCategory != CategoryUnknown {
		return platformCategory
	}
	var executableError *exec.Error
	var pathError *os.PathError
	missingExecutable := errors.As(err, &executableError) && errors.Is(err, exec.ErrNotFound)
	if errors.As(err, &pathError) && (pathError.Op == "fork/exec" || pathError.Op == "exec" || pathError.Op == "CreateProcess") {
		missingExecutable = missingExecutable || errors.Is(err, os.ErrNotExist)
	}
	switch {
	case missingExecutable:
		return CategoryMissingExecutable
	case errors.Is(err, os.ErrPermission):
		return CategoryPermissionDenied
	case errors.Is(err, syscall.EADDRINUSE):
		return CategoryPortInUse
	case errors.Is(err, syscall.ENOSPC):
		return CategoryDiskFull
	default:
		return CategoryUnknown
	}
}

func classifyText(text string) (string, string) {
	category, evidence := CategoryUnknown, ""
	for line := range strings.SplitSeq(text, "\n") {
		lower := strings.ToLower(line)
		if playerChat.MatchString(line) || strings.Contains(lower, "[chat]") || strings.Contains(lower, "example") || strings.Contains(lower, "resolved") || strings.Contains(lower, "documentation") {
			continue
		}
		for _, rule := range logRules {
			if !rule.pattern.MatchString(line) {
				continue
			}
			if category != CategoryUnknown && category != rule.category {
				return CategoryUnknown, ""
			}
			category = rule.category
			evidence, _ = Tail(line, 1, MaxErrorBytes)
		}
	}
	return category, evidence
}
