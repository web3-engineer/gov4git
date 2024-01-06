package panorama

import (
	"context"

	"github.com/gov4git/gov4git/v2/proto/account"
	"github.com/gov4git/gov4git/v2/proto/ballot/ballotapi"
	"github.com/gov4git/gov4git/v2/proto/gov"
	"github.com/gov4git/gov4git/v2/proto/id"
	"github.com/gov4git/gov4git/v2/proto/member"
	"github.com/gov4git/gov4git/v2/proto/motion/motionapi"
	"github.com/gov4git/gov4git/v2/proto/motion/motionproto"
)

type Panoramic struct {
	RealBalance      float64                 `json:"real_balance"`
	EffectiveBalance float64                 `json:"effective_balance"`
	Motions          motionproto.MotionViews `json:"motions"`
}

func Panorama(
	ctx context.Context,
	addr gov.Address,
	voterAddr id.OwnerAddress,

) *Panoramic {

	voterOwner := id.CloneOwner(ctx, voterAddr)
	return Panorama_Local(ctx, gov.Clone(ctx, addr), voterAddr, voterOwner)
}

func Panorama_Local(
	ctx context.Context,
	cloned gov.Cloned,
	voterAddr id.OwnerAddress,
	voterOwner id.OwnerCloned,

) *Panoramic {

	voterUser := member.FindClonedUser_Local(ctx, cloned, voterOwner)
	voterAccountID := member.UserAccountID(voterUser)
	voterAccount := account.Get_Local(ctx, cloned, account.AccountID(voterAccountID))
	real := voterAccount.Balance(account.PluralAsset).Quantity

	mvs := motionapi.TrackMotionBatch_Local(ctx, cloned, voterAddr, voterOwner)

	// apply pending votes to governance
	for _, ad := range ballotapi.List_Local(ctx, cloned) {
		vs := ballotapi.Track_StageOnly(
			ctx,
			voterAddr,
			voterOwner,
			cloned,
			ad.ID,
		)
		fetchedVote := ballotapi.FetchedVote{
			Voter:     voterUser,
			Address:   voterAddr.Public,
			Elections: vs.PendingVotes,
		}
		ballotapi.TallyFetchedVotes_StageOnly(
			ctx,
			cloned,
			ad.ID,
			ballotapi.FetchedVotes{fetchedVote},
		)
	}

	eff := voterAccount.Balance(account.PluralAsset).Quantity

	return &Panoramic{
		RealBalance:      real,
		EffectiveBalance: eff,
		Motions:          mvs,
	}
}