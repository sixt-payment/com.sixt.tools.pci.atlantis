package events

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/pkg/errors"

	"path/filepath"

	"github.com/hootsuite/atlantis/server/events/models"
	"github.com/hootsuite/atlantis/server/events/run"
	"github.com/hootsuite/atlantis/server/events/terraform"
	"github.com/hootsuite/atlantis/server/events/vcs"
	"github.com/hootsuite/atlantis/server/events/webhooks"
)

type ApplyExecutor struct {
	VCSClient               vcs.ClientProxy
	Terraform               *terraform.Client
	RequireApproval         bool
	RequireExternalApproval bool
	ApprovalURL             string
	Run                     *run.Run
	Workspace               Workspace
	ProjectPreExecute       *ProjectPreExecute
	Webhooks                webhooks.Sender
}

type externalApproval struct {
	PullRequest string
	ApprovedBy  string
	Approved    bool
}

func (a *ApplyExecutor) checkExternalApproval(ctx *CommandContext, repo models.Repo, pull models.PullRequest) (bool, error) {
	client := &http.Client{
		Timeout: time.Second * 1,
	}

	payload := fmt.Sprintf("{\"repo_owner\": \"%s\", \"repo_name\": \"%s\", \"pull_request\": %d}", repo.Owner, repo.Name, pull.Num)
	req, err := http.NewRequest("POST", a.ApprovalURL, bytes.NewBuffer([]byte(payload)))
	req.Header.Set("Content-Type", "application/json")

	if err != nil {
		return false, err
	}

	req.Close = true

	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}

	if resp.StatusCode == 200 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return false, err
		}

		var approval externalApproval
		if json.Unmarshal(body, &approval) != nil {
			return false, err
		}

		if approval.Approved {
			return true, nil
		}

		return false, nil
	}

	if resp.StatusCode >= 400 || resp.StatusCode < 200 {
		return false, nil
	}

	return false, nil
}

func (a *ApplyExecutor) Execute(ctx *CommandContext) CommandResponse {
	if a.RequireApproval {
		approved, err := a.VCSClient.PullIsApproved(ctx.BaseRepo, ctx.Pull, ctx.VCSHost)
		if err != nil {
			return CommandResponse{Error: errors.Wrap(err, "checking if pull request was approved")}
		}
		if !approved {
			return CommandResponse{Failure: "Pull request must be approved before running apply."}
		}
		ctx.Log.Info("confirmed pull request was approved")
	}

	if a.RequireExternalApproval {
		approved, err := a.checkExternalApproval(ctx, ctx.BaseRepo, ctx.Pull)
		if err != nil {
			return CommandResponse{Error: errors.Wrap(err, "checking if pull request was approved (external)")}
		}
		if !approved {
			return CommandResponse{Failure: "Pull request must be approved before running apply. (external)"}
		}
		ctx.Log.Info("confirmed pull request was approved (external)")
	}

	repoDir, err := a.Workspace.GetWorkspace(ctx.BaseRepo, ctx.Pull, ctx.Command.Environment)
	if err != nil {
		return CommandResponse{Failure: "No workspace found. Did you run plan?"}
	}
	ctx.Log.Info("found workspace in %q", repoDir)

	// plans are stored at project roots by their environment names. We just need to find them
	var plans []models.Plan
	err = filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// if the plan is for the right env,
		if !info.IsDir() && info.Name() == ctx.Command.Environment+".tfplan" {
			rel, _ := filepath.Rel(repoDir, filepath.Dir(path))
			plans = append(plans, models.Plan{
				Project:   models.NewProject(ctx.BaseRepo.FullName, rel),
				LocalPath: path,
			})
		}
		return nil
	})
	if err != nil {
		return CommandResponse{Error: errors.Wrap(err, "finding plans")}
	}
	if len(plans) == 0 {
		return CommandResponse{Failure: "No plans found for that environment."}
	}
	var paths []string
	for _, p := range plans {
		paths = append(paths, p.LocalPath)
	}
	ctx.Log.Info("found %d plan(s) in our workspace: %v", len(plans), paths)

	results := []ProjectResult{}
	for _, plan := range plans {
		ctx.Log.Info("running apply for project at path %q", plan.Project.Path)
		result := a.apply(ctx, repoDir, plan)
		result.Path = plan.LocalPath
		results = append(results, result)
	}
	return CommandResponse{ProjectResults: results}
}

func (a *ApplyExecutor) apply(ctx *CommandContext, repoDir string, plan models.Plan) ProjectResult {
	preExecute := a.ProjectPreExecute.Execute(ctx, repoDir, plan.Project)
	if preExecute.ProjectResult != (ProjectResult{}) {
		return preExecute.ProjectResult
	}
	config := preExecute.ProjectConfig
	terraformVersion := preExecute.TerraformVersion

	applyExtraArgs := config.GetExtraArguments(ctx.Command.Name.String())
	absolutePath := filepath.Join(repoDir, plan.Project.Path)
	env := ctx.Command.Environment
	tfApplyCmd := append(append(append([]string{"apply", "-no-color"}, applyExtraArgs...), ctx.Command.Flags...), plan.LocalPath)
	output, err := a.Terraform.RunCommandWithVersion(ctx.Log, absolutePath, tfApplyCmd, terraformVersion, env)

	a.Webhooks.Send(ctx.Log, webhooks.ApplyResult{ // nolint: errcheck
		Workspace: env,
		User:      ctx.User,
		Repo:      ctx.BaseRepo,
		Pull:      ctx.Pull,
		Success:   err == nil,
	})

	if err != nil {
		return ProjectResult{Error: fmt.Errorf("%s\n%s", err.Error(), output)}
	}
	ctx.Log.Info("apply succeeded")

	if len(config.PostApply) > 0 {
		_, err := a.Run.Execute(ctx.Log, config.PostApply, absolutePath, env, terraformVersion, "post_apply")
		if err != nil {
			return ProjectResult{Error: errors.Wrap(err, "running post apply commands")}
		}
	}

	return ProjectResult{ApplySuccess: output}
}
