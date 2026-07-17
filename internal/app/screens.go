package app

// Screen identifies the active TUI view.
// List/menu UIs (sessions, branch, pick-turn) are modals over ScreenPlay.
type Screen int

const (
	ScreenPlay Screen = iota
	ScreenAuth
	ScreenTextForm
	ScreenRevisePreview
)

// String returns a stable name for tests and diagnostics.
func (s Screen) String() string {
	switch s {
	case ScreenPlay:
		return "play"
	case ScreenAuth:
		return "auth"
	case ScreenTextForm:
		return "text_form"
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
	ModalSessions       // session browser
	ModalPickTurn       // pick turn for edit/revise
	ModalBranches       // branch tips list
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
	case ModalSessions:
		return "sessions"
	case ModalPickTurn:
		return "pick_turn"
	case ModalBranches:
		return "branches"
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
