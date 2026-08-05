package nurture

import (
	"regexp"
	"strings"

	"github.com/amnasahamed/niosengine/internal/leads"
)

// StageEvaluator is the default progressive stage machine.
//
// Rules ported from workflownew.json's "ask_name → ask_qualification →
// ask_pass_fail → ask_stream → ask_failed_subjects → ask_certificate →
// ask_place → complete" logic.
type defaultStageEvaluator struct{}

func NewStageEvaluator() StageEvaluator { return &defaultStageEvaluator{} }

// NextStage returns the stage that should be asked next given the current
// stage and the known lead fields.
//
// The transitions are:
//
//	ask_name            → ask_qualification   if name is set
//	ask_qualification   → ask_pass_fail       if class_level is set
//	ask_pass_fail       → ask_stream          if exam_result is set AND class_level looks like Plus Two
//	ask_pass_fail       → ask_failed_subjects if exam_result == "fail"
//	ask_pass_fail       → ask_certificate     if exam_result != "fail" AND not "appearing"
//	ask_stream          → ask_failed_subjects if exam_result == "fail"
//	ask_stream          → ask_certificate     otherwise
//	ask_failed_subjects → ask_certificate     if subjects_count > 0
//	ask_certificate     → ask_place           if has_certificate is set
//	ask_place           → complete            if place is set
//	complete            → complete            (terminal)
func (e *defaultStageEvaluator) NextStage(currentStage string, fields map[string]any) string {
	switch currentStage {
	case leads.StageAskName:
		if str(fields, "name") != "" {
			return leads.StageAskQualification
		}
		return leads.StageAskName

	case leads.StageAskQualification:
		if str(fields, "class_level") != "" {
			return leads.StageAskPassFail
		}
		return leads.StageAskQualification

	case leads.StageAskPassFail:
		exam := str(fields, "exam_result")
		if exam == "" {
			return leads.StageAskPassFail
		}
		if isHigherSecondary(str(fields, "class_level")) {
			return leads.StageAskStream
		}
		if exam == "fail" {
			return leads.StageAskFailedSubjects
		}
		return leads.StageAskCertificate

	case leads.StageAskStream:
		if str(fields, "stream") != "" {
			if str(fields, "exam_result") == "fail" {
				return leads.StageAskFailedSubjects
			}
			return leads.StageAskCertificate
		}
		return leads.StageAskStream

	case leads.StageAskFailedSubjects:
		if intVal(fields, "subjects_count") > 0 {
			return leads.StageAskCertificate
		}
		return leads.StageAskFailedSubjects

	case leads.StageAskCertificate:
		if str(fields, "has_certificate") != "" {
			return leads.StageAskPlace
		}
		return leads.StageAskCertificate

	case leads.StageAskPlace:
		if str(fields, "place") != "" {
			return leads.StageComplete
		}
		return leads.StageAskPlace

	case leads.StageComplete:
		return leads.StageComplete

	default:
		return leads.StageAskName
	}
}

// IsComplete returns true if every applicable field is filled.
// Used by ReplyGenerator to suppress further stage-progression questions.
func (e *defaultStageEvaluator) IsComplete(fields map[string]any) bool {
	if str(fields, "name") == "" {
		return false
	}
	if str(fields, "class_level") == "" {
		return false
	}
	exam := str(fields, "exam_result")
	if exam == "" {
		return false
	}
	if isHigherSecondary(str(fields, "class_level")) && str(fields, "stream") == "" {
		return false
	}
	if exam == "fail" && intVal(fields, "subjects_count") == 0 {
		return false
	}
	if str(fields, "has_certificate") == "" {
		return false
	}
	if str(fields, "place") == "" {
		return false
	}
	return true
}

// isHigherSecondary returns true if class_level indicates Plus Two / HS / 12th.
// These levels need a stream (Science/Commerce/Arts/Humanities).
var plusTwoPattern = regexp.MustCompile(`(?i)\b(plus\s*two|\+2|hssc|h\.s\.s\.c|12th|intermediate|higher\s*secondary|class\s*12)\b`)

func isHigherSecondary(classLevel string) bool {
	return plusTwoPattern.MatchString(classLevel)
}

// str extracts a non-empty string from a fields map.
func str(m map[string]any, k string) string {
	if v, ok := m[k]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// intVal extracts a positive integer from a fields map.
// Handles int, int64, float64 (JSON default) gracefully.
func intVal(m map[string]any, k string) int {
	if v, ok := m[k]; ok {
		switch x := v.(type) {
		case int:
			return x
		case int32:
			return int(x)
		case int64:
			return int(x)
		case float64:
			return int(x)
		}
	}
	return 0
}
