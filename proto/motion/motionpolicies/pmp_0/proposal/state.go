package proposal

import (
	"github.com/gov4git/gov4git/v2/proto/ballot/ballotproto"
	"github.com/gov4git/gov4git/v2/proto/motion/motionpolicies/pmp_0"
	"github.com/gov4git/gov4git/v2/proto/motion/motionproto"
)

const StateFilebase = "state.json"

type ProposalState struct {
	ApprovalPoll        ballotproto.BallotID `json:"approval_poll"`
	LatestApprovalScore float64              `json:"latest_approval_score"`
	EligibleConcerns    motionproto.Refs     `json:"eligible_concerns"`
}

func NewProposalState(id motionproto.MotionID) *ProposalState {
	return &ProposalState{
		ApprovalPoll: pmp_0.ProposalApprovalPollName(id),
	}
}