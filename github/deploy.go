package github

import (
	"context"
	crypto_rand "crypto/rand"
	_ "embed"
	"encoding/base64"
	"os"
	"path"
	"strconv"

	"github.com/google/go-github/v58/github"
	"github.com/gov4git/gov4git/v2/gov4git/api"
	"github.com/gov4git/gov4git/v2/proto/boot"
	"github.com/gov4git/gov4git/v2/proto/gov"
	"github.com/gov4git/gov4git/v2/proto/id"
	"github.com/gov4git/lib4git/base"
	"github.com/gov4git/lib4git/form"
	"github.com/gov4git/lib4git/git"
	"github.com/gov4git/lib4git/must"
	"github.com/gov4git/lib4git/ns"
	"github.com/gov4git/vendor4git"
	vendor "github.com/gov4git/vendor4git/github"
	"golang.org/x/crypto/nacl/box"
	"golang.org/x/oauth2"
)

func Deploy(
	ctx context.Context,
	token string, // permissions: read project issues, create/write govPrefix
	project Repo,
	govPrefix Repo,
	ghRelease string, // GitHub release of gov4git to install
) api.Config {

	// create authenticated GitHub client
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	ghClient := github.NewClient(tc)

	// create governance public and private repos
	v := vendor.NewGithubVendorWithClient(ctx, ghClient)

	govPublic := Repo{Owner: govPrefix.Owner, Name: govPrefix.Name + "-gov.public"}
	base.Infof("creating GitHub repository %v", govPublic)
	govPublicURLs, err := v.CreateRepo(ctx, govPublic.Name, govPublic.Owner, false)
	must.NoError(ctx, err)

	govPrivate := Repo{Owner: govPrefix.Owner, Name: govPrefix.Name + "-gov.private"}
	base.Infof("creating GitHub repository %v", govPrivate)
	govPrivateURLs, err := v.CreateRepo(ctx, govPrivate.Name, govPrivate.Owner, true)
	must.NoError(ctx, err)

	govOwnerAddr := gov.OwnerAddress{
		Public: id.PublicAddress{
			Repo:   git.URL(govPublicURLs.HTTPSURL),
			Branch: git.MainBranch,
		},
		Private: id.PrivateAddress{
			Repo:   git.URL(govPrivateURLs.HTTPSURL),
			Branch: git.MainBranch,
		},
	}

	// attach access token authentication to context for git use
	git.SetAuth(ctx, govOwnerAddr.Public.Repo, git.MakeTokenAuth(ctx, token))
	git.SetAuth(ctx, govOwnerAddr.Private.Repo, git.MakeTokenAuth(ctx, token))

	// initialize governance identity
	base.Infof("initializing governance for %v", project)
	boot.Boot(ctx, govOwnerAddr)

	// create GitHub environment for governance
	base.Infof("creating GitHub environment for governance in %v", govPublic)
	createDeployEnvironment(ctx, ghClient, token, project, govPublic, govPublicURLs, govPrivateURLs, ghRelease)

	// install github automation in the public governance repo
	base.Infof("installing GitHub actions for governance in %v, targetting %v", govPublic, project)
	installGithubActions(ctx, govOwnerAddr)

	// install governance labels in project repo
	createGovernanceIssueLabels(ctx, ghClient, project)

	// return config for gov4git administrator
	homeDir, err := os.UserHomeDir()
	must.NoError(ctx, err)
	return api.Config{
		Auth: map[git.URL]api.AuthConfig{
			git.URL(govPublicURLs.HTTPSURL):               {AccessToken: github.String(token)},
			git.URL(govPrivateURLs.HTTPSURL):              {AccessToken: github.String(token)},
			git.URL("YOUR_MEMBER_PUBLIC_REPO_HTTPS_URL"):  {AccessToken: github.String("YOUR_MEMBER_ACCESS_TOKEN")},
			git.URL("YOUR_MEMBER_PRIVATE_REPO_HTTPS_URL"): {AccessToken: github.String("YOUR_MEMBER_ACCESS_TOKEN")},
		},
		//
		GovPublicURL:     git.URL(govPublicURLs.HTTPSURL),
		GovPublicBranch:  git.MainBranch,
		GovPrivateURL:    git.URL(govPrivateURLs.HTTPSURL),
		GovPrivateBranch: git.MainBranch,
		//
		MemberPublicURL:     "YOUR_MEMBER_PUBLIC_REPO_HTTPS_URL",
		MemberPublicBranch:  git.MainBranch,
		MemberPrivateURL:    "YOUR_MEMBER_PRIVATE_REPO_HTTPS_URL",
		MemberPrivateBranch: git.MainBranch,
		//
		CacheDir:        path.Join(homeDir, ".gov4git", "cache"),
		CacheTTLSeconds: 0,
	}
}

var (
	//go:embed deploy/.github/scripts/gov4git_cron.sh
	cronSH string

	//go:embed deploy/.github/workflows/gov4git_cron.yml
	cronYML string

	//go:embed deploy/.github/python/requirements.txt
	pythonRequirementsTXT string
)

func installGithubActions(
	ctx context.Context,
	govOwnerAddr gov.OwnerAddress,
) {

	govCloned := git.CloneOne(ctx, git.Address(govOwnerAddr.Public))
	t := govCloned.Tree()

	// populate helper files for github actions
	git.StringToFileStage(ctx, t, ns.NS{".github", "scripts", "gov4git_cron.sh"}, cronSH)
	git.StringToFileStage(ctx, t, ns.NS{".github", "workflows", "gov4git_cron.yml"}, cronYML)
	git.StringToFileStage(ctx, t, ns.NS{".github", "python", "requirements.txt"}, pythonRequirementsTXT)

	git.Commit(ctx, t, "install gov4git github actions")
	govCloned.Push(ctx)
}

func createGovernanceIssueLabels(
	ctx context.Context,
	ghc *github.Client,
	project Repo,
) {

	for _, l := range GovernanceLabels {
		label := &github.Label{Name: github.String(l)}
		_, _, err := ghc.Issues.CreateLabel(ctx, project.Owner, project.Name, label)
		if IsLabelAlreadyExists(err) {
			base.Infof("github issue label %v already exists in %v", l, project)
			continue
		}
		must.NoError(ctx, err)
	}
}

func createDeployEnvironment(
	ctx context.Context,
	ghClient *github.Client,
	token string,
	project Repo,
	govPublic Repo,
	govPublicURLs *vendor4git.Repository,
	govPrivateURLs *vendor4git.Repository,
	ghRelease string,
) {

	// fetch repo id
	ghGovPubRepo, _, err := ghClient.Repositories.Get(ctx, govPublic.Owner, govPublic.Name)
	must.NoError(ctx, err)

	// create deploy environment
	createEnv := &github.CreateUpdateEnvironment{}
	env, _, err := ghClient.Repositories.CreateUpdateEnvironment(ctx, govPublic.Owner, govPublic.Name, DeployEnvName, createEnv)
	must.NoError(ctx, err)

	// create environment secrets
	envSecrets := map[string]string{
		"ORGANIZER_GITHUB_TOKEN": token,
	}

	govEnvPubKey, _, err := ghClient.Actions.GetEnvPublicKey(ctx, int(ghGovPubRepo.GetID()), env.GetName())
	// govPubPubKey, _, err := ghClient.Actions.GetRepoPublicKey(ctx, govPublic.Owner, govPublic.Name)
	must.NoError(ctx, err)

	for k, v := range envSecrets {
		encryptedValue := encryptValue(ctx, govEnvPubKey, v)
		encryptedSecret := &github.EncryptedSecret{
			Name:           k,
			KeyID:          govEnvPubKey.GetKeyID(),
			EncryptedValue: encryptedValue,
		}
		base.Infof("adding secret to environment: %v", form.SprintJSON(encryptedSecret))
		_, err := ghClient.Actions.CreateOrUpdateEnvSecret(ctx, int(ghGovPubRepo.GetID()), env.GetName(), encryptedSecret)
		must.NoError(ctx, err)
	}

	// create environment variables
	envVars := map[string]string{
		"GOV4GIT_RELEASE":      ghRelease,
		"PROJECT_OWNER":        project.Owner,
		"PROJECT_REPO":         project.Name,
		"GOV_PUBLIC_REPO_URL":  govPublicURLs.HTTPSURL,
		"GOV_PRIVATE_REPO_URL": govPrivateURLs.HTTPSURL,
		"SYNC_GITHUB_FREQ":     strconv.Itoa(DefaultGithubFreq),
		"SYNC_COMMUNITY_FREQ":  strconv.Itoa(DefaultCommunityFreq),
		"SYNC_FETCH_PAR":       strconv.Itoa(DefaultFetchParallelism),
	}
	for k, v := range envVars {
		_, err := ghClient.Actions.CreateEnvVariable(ctx, int(*ghGovPubRepo.ID), env.GetName(), &github.ActionsVariable{Name: k, Value: v})
		must.NoError(ctx, err)
	}
}

const (
	DefaultGithubFreq       = 120     // seconds
	DefaultCommunityFreq    = 60 * 60 // seconds
	DefaultFetchParallelism = 5
)

func encryptValue(ctx context.Context, pubKey *github.PublicKey, secretValue string) string {

	decodedPubKey, err := base64.StdEncoding.DecodeString(pubKey.GetKey())
	must.NoError(ctx, err)

	var boxKey [32]byte
	copy(boxKey[:], decodedPubKey)
	secretBytes := []byte(secretValue)
	encryptedBytes, err := box.SealAnonymous([]byte{}, secretBytes, &boxKey, crypto_rand.Reader)
	must.NoError(ctx, err)

	return base64.StdEncoding.EncodeToString(encryptedBytes)
}
