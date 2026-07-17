package app

// Screen identifies the active TUI view.
type Screen int

const (
	ScreenPlay Screen = iota
	ScreenAuth
	ScreenSessions
	ScreenPickTurn
	ScreenTextForm
	ScreenBranches
	ScreenRevisePreview
)

// String returns a stable name for tests and diagnostics.
func (s Screen) String() string {
	switch s {
	case ScreenPlay:
		return "play"
	case ScreenAuth:
		return "auth"
	case ScreenSessions:
		return "sessions"
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

// FocusArea is which pane accepts keyboard navigation on the play screen.
type FocusArea int

const (
	FocusInput FocusArea = iota
	FocusHistory
)

// String returns a stable focus name for tests.
func (f FocusArea) String() string {
	switch f {
	case FocusInput:
		return "input"
	case FocusHistory:
		return "history"
	default:
		return "unknown"
	}
}

// Modal is a centered overlay on the main play surface.
type Modal int

const (
	ModalNone     Modal = iota
	ModalSettings       // model list
	ModalEffort         // effort for pending model
	ModalRename         // rename current session
)

// String returns a stable modal name for tests.
func (m Modal) String() string {
	switch m {
	case ModalNone:
		return "none"
	case ModalSettings:
		return "settings"
	case ModalEffort:
		return "effort"
	case ModalRename:
		return "rename"
	default:
		return "unknown"
	}
}

// TextFormKind selects what a free-text form submits to.
type TextFormKind int

const (
	formFeedback TextFormKind = iota
	formEditContent
	formReviseInstruction
)
