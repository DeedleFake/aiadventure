package app

// Screen identifies the active TUI view.
type Screen int

const (
	ScreenHub Screen = iota
	ScreenAuth
	ScreenModel
	ScreenEffort
	ScreenSessions
	ScreenNewSession
	ScreenPlay
	ScreenPlayMenu
	ScreenPickTurn
	ScreenTextForm
	ScreenBranches
	ScreenRevisePreview
)

// ScreenName returns a stable name for tests and diagnostics.
func (s Screen) String() string {
	switch s {
	case ScreenHub:
		return "hub"
	case ScreenAuth:
		return "auth"
	case ScreenModel:
		return "model"
	case ScreenEffort:
		return "effort"
	case ScreenSessions:
		return "sessions"
	case ScreenNewSession:
		return "new_session"
	case ScreenPlay:
		return "play"
	case ScreenPlayMenu:
		return "play_menu"
	case ScreenPickTurn:
		return "pick_turn"
	case ScreenTextForm:
		return "text_form"
	case ScreenBranches:
		return "branches"
	case ScreenRevisePreview:
		return "revise_preview"
	default:
		return "unknown"
	}
}

// Hub items (cursor indices).
const (
	hubSignIn = iota
	hubSignOut
	hubModel
	hubNewSession
	hubSessions
	hubQuit
	hubItemCount
)

var hubLabels = []string{
	"Sign in to xAI (OAuth)",
	"Sign out",
	"Select model / effort",
	"New adventure session",
	"Browse / search sessions",
	"Quit",
}

// Play menu actions.
const (
	playActPhase = iota
	playActEdit
	playActRevise
	playActFeedback
	playActBranch
	playActModel
	playActBackHub
	playActCount
)

var playActLabels = []string{
	"Toggle phase (brainstorm ↔ adventure)",
	"Edit a prior turn (manual fork)",
	"Revise AI turn with AI (fork)",
	"Add out-of-band feedback",
	"Switch branch",
	"Change model / effort",
	"Save & return to hub",
}

// TextFormKind selects what a free-text form submits to.
type TextFormKind int

const (
	formFeedback TextFormKind = iota
	formEditContent
	formReviseInstruction
)
