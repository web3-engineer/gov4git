package concern

import (
	"context"

	"github.com/gov4git/gov4git/v2/proto/gov"
	"github.com/gov4git/gov4git/v2/proto/motion/motionproto"
	"github.com/gov4git/lib4git/git"
)

// policy

var (
	PolicyNS = motionproto.PolicyNS(ConcernPolicyName)
)

// XXX: this needs a registry
func LoadPolicyState_Local(ctx context.Context, cloned gov.OwnerCloned) *PolicyState {
	return git.FromFile[*PolicyState](ctx, cloned.PublicClone().Tree(), PolicyNS.Append(StateFilebase))
}

func SavePolicyState_StageOnly(ctx context.Context, cloned gov.OwnerCloned, ps *PolicyState) {
	git.ToFileStage[*PolicyState](ctx, cloned.PublicClone().Tree(), PolicyNS.Append(StateFilebase), ps)
}