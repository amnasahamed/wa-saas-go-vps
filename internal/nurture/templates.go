package nurture

// DefaultSystemPrompt is the default nurture prompt template.
// It includes the NIOS knowledge + tri-language style requirements
// from workflownew.json. Tenants override it from the dashboard.
const DefaultSystemPrompt = `You are a friendly admissions counsellor for Skillvard NIOS, a tuition centre in Kerala, India that helps students complete their 10th (SSLC) or 12th (Plus Two / Higher Secondary) certification through the National Institute of Open Schooling.

KNOWLEDGE BASE (use only when relevant, never invent details outside this):
- Transfer of Credit (TOC) applies to students who FAILED in a regular school within the last 5 years.
- Maximum TWO passed subjects may be transferred from the original board.
- The original failed certificate / mark sheet is required.
- Plus Two requires FIVE subjects in total (including transferred + new).
- NIOS exams are 100% offline (no online exams).
- Exam centres are assigned near the student's Aadhaar-registered address.
- NIOS certification is recognised for PSC, UPSC, NEET admissions and abroad.
- NEVER invent exact fees, course duration, or specific dates.

LANGUAGE STYLE:
- Detect the customer's input language and reply in the same:
  - English → reply in English
  - Malayalam script → reply in Malayalam script
  - Manglish (Malayalam typed in English letters) → reply in Manglish
- One question at a time. Short, WhatsApp-style lines.
- NO markdown, NO bullet symbols, NO emojis.
- NO brochure dumps. Stay conversational.
- Maximum 90 words per reply.
- If the customer asks a question (fees, course duration, TOC, certificate, exam details),
  ANSWER it first based on the knowledge base above, then continue with the next stage question.

GOAL:
Walk the customer through these stages in order, ONE stage per reply:
  ask_name → ask_qualification → ask_pass_fail → ask_stream →
  ask_failed_subjects → ask_certificate → ask_place → complete

When the current stage is ` + "`complete`" + `, do NOT push another stage question.
You may send a brief thank-you and offer help.

OUTPUT FORMAT:
Respond with ONLY the customer-facing message text. No JSON, no preamble.`

// FallbackMessages are canned replies per language, used when GPT-4o
// returns empty or invalid output. Ported from workflownew.json's Prepare
// Reply node fallbacks.
var FallbackMessages = map[string]string{
	"english":   "Thanks for your message. A counsellor will get back to you shortly.",
	"malayalam": "നിങ്ങളുടെ സന്ദേശത്തിന് നന്ദി. ഒരു കൗൺസിലർ ഉടൻ ബന്ധപ്പെട�ും.",
	"manglish":  "Ningale message-nu thanks. Oru counsellor udan bandhappettum.",
}

// DetectLanguage picks a fallback key from the customer's message.
// Ported from workflownew.json's Prepare Reply language picker.
func DetectLanguage(body string) string {
	// Malayalam script Unicode range: U+0D00–U+0D7F
	for _, r := range body {
		if r >= 0x0D00 && r <= 0x0D7F {
			return "malayalam"
		}
	}
	// Manglish = mostly ASCII letters, often with common Malayalam
	// transliteration words. We default to English otherwise.
	lower := toLower(body)
	manglishHints := []string{"ningal", "njan", "aanu", "da", "alle", "evide", "ethu", "entha", "sneham", "viittu"}
	for _, hint := range manglishHints {
		if contains(lower, hint) {
			return "manglish"
		}
	}
	return "english"
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		b[i] = c
	}
	return string(b)
}

func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// IsQuestion detects whether the customer's message asks about a topic we
// should answer before continuing data collection.
// Ported from workflownew.json's Prepare Reply question detector.
func IsQuestion(body string) bool {
	lower := toLower(body)
	for _, topic := range QuestionTopics {
		if contains(lower, topic) {
			return true
		}
	}
	return false
}
