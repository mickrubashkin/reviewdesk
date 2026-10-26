package main

import "time"

// Email is an active email of the board with its portal sends over the
// synced window.
type Email struct {
	ID            string
	Slug          string
	Title         string
	Stage         string
	SortOrder     int
	Language      string
	Variant       string
	AdaptationKey string
	Sends         int
	LastSent      *time.Time
	// Live is set when the email already carries a live mark, e.g. from an
	// earlier run; such emails are left alone.
	Live bool
}

type Action string

const (
	// ActionArchive: the old version is no longer sent.
	ActionArchive Action = "archive"
	// ActionPromote: the old version is the only one and it is sent, so it is
	// the live email of its slot.
	ActionPromote Action = "promote"
	// ActionStillInRobot: the old version is still sent although a new one
	// exists; it stays, marked live, until Bitrix24 gets the new one.
	ActionStillInRobot Action = "still_in_robot"
	// ActionCheck: not seen in the portal and the slot has nothing else, so
	// it may be sent from elsewhere (e.g. the partner admin panel). Left as
	// is for a person to decide.
	ActionCheck Action = "check"
)

const stillInRobotNote = "Old version still in the Bitrix24 robot (portal sync). Upload the new version, then archive this one."
const promotedNote = "Sent from the Bitrix24 robot (portal sync); the old version became the main one."

type Step struct {
	Email  Email
	Action Action
	// New is the new version of the slot, if any.
	New *Email
	// Note overrides the default live note of a promote step.
	Note string
}

type slotKey struct {
	stage, language, adaptation string
	sortOrder                   int
}

func keyOf(e Email) slotKey {
	return slotKey{e.Stage, e.Language, e.AdaptationKey, e.SortOrder}
}

// BuildPlan decides what happens to every old email: archived when it is not
// sent and a new version replaces it, left for a check when it is not sent
// but is the slot's only version, promoted when it is the only version and
// is sent, kept as "still in the robot" when a new version exists but the
// old one is what goes out.
func BuildPlan(emails []Email) []Step {
	newBySlot := map[slotKey]*Email{}
	for i := range emails {
		if emails[i].Variant == "new" {
			newBySlot[keyOf(emails[i])] = &emails[i]
		}
	}
	var steps []Step
	for _, email := range emails {
		if email.Variant != "old" || email.Live {
			continue
		}
		newVersion := newBySlot[keyOf(email)]
		step := Step{Email: email, New: newVersion}
		switch {
		case email.Sends == 0 && newVersion != nil:
			step.Action = ActionArchive
		case email.Sends == 0:
			step.Action = ActionCheck
		case newVersion == nil:
			step.Action = ActionPromote
		default:
			step.Action = ActionStillInRobot
		}
		steps = append(steps, step)
	}
	return steps
}
