package true_git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Masterminds/semver"
)

type SyncSourceWorktreeWithServiceBranchOptions struct {
	ServiceBranchPrefix string
	GlobExcludeList     []string
}

func SyncSourceWorktreeWithServiceBranch(ctx context.Context, gitDir, sourceWorktreeDir, worktreeCacheDir, commit string, opts SyncSourceWorktreeWithServiceBranchOptions) (string, error) {
	var resultCommit string
	if err := withWorkTreeCacheLock(ctx, worktreeCacheDir, func() error {
		var err error
		if gitDir, err = filepath.Abs(gitDir); err != nil {
			return fmt.Errorf("bad git dir %s: %s", gitDir, err)
		}

		if worktreeCacheDir, err = filepath.Abs(worktreeCacheDir); err != nil {
			return fmt.Errorf("bad work tree cache dir %s: %s", worktreeCacheDir, err)
		}

		serviceWorktreeDir, err := prepareWorkTree(ctx, gitDir, worktreeCacheDir, commit, true)
		if err != nil {
			return fmt.Errorf("unable to prepare worktree for commit %v: %s", commit, err)
		}

		currentCommitPath := filepath.Join(worktreeCacheDir, "current_commit")
		if err := os.RemoveAll(currentCommitPath); err != nil {
			return fmt.Errorf("unable to remove %s: %s", currentCommitPath, err)
		}

		branchName := fmt.Sprintf("%s%s", opts.ServiceBranchPrefix, commit)
		resultCommit, err = syncWorktreeWithServiceWorktreeBranch(ctx, sourceWorktreeDir, serviceWorktreeDir, commit, branchName, opts.GlobExcludeList)
		if err != nil {
			return fmt.Errorf("unable to sync worktree with service branch %q: %s", branchName, err)
		}

		return nil
	}); err != nil {
		return "", err
	}

	return resultCommit, nil
}

func syncWorktreeWithServiceWorktreeBranch(ctx context.Context, sourceWorktreeDir, serviceWorktreeDir, sourceCommit, branchName string, globExcludeList []string) (string, error) {
	serviceBranchHeadCommit, err := getOrPrepareServiceBranchHeadCommit(ctx, serviceWorktreeDir, sourceCommit, branchName)
	if err != nil {
		return "", fmt.Errorf("unable to get or prepare service branch head commit: %s", err)
	}

	checkoutCmd := NewGitCmd(ctx, &GitCmdOptions{RepoDir: serviceWorktreeDir}, "checkout", branchName)
	if err = checkoutCmd.Run(ctx); err != nil {
		return "", fmt.Errorf("git checkout command failed: %s", err)
	}

	revertedChangesExist, err := revertExcludedChangesInServiceWorktreeIndex(ctx, sourceWorktreeDir, serviceWorktreeDir, sourceCommit, serviceBranchHeadCommit, globExcludeList)
	if err != nil {
		return "", fmt.Errorf("unable to revert excluded changes in service worktree index: %q", err)
	}

	newChangesExist, err := checkNewChangesInSourceWorktreeDir(ctx, sourceWorktreeDir, serviceWorktreeDir, globExcludeList)
	if err != nil {
		return "", fmt.Errorf("unable to check new changes in source worktree: %s", err)
	}

	if !revertedChangesExist && !newChangesExist {
		return serviceBranchHeadCommit, nil
	}

	if err = addNewChangesInServiceWorktreeDir(ctx, sourceWorktreeDir, serviceWorktreeDir, globExcludeList); err != nil {
		return "", fmt.Errorf("unable to add new changes in service worktree: %s", err)
	}

	newCommit, err := commitNewChangesInServiceBranch(ctx, serviceWorktreeDir, branchName)
	if err != nil {
		return "", fmt.Errorf("unable to commit new changes in service branch: %s", err)
	}

	return newCommit, nil
}

func getOrPrepareServiceBranchHeadCommit(ctx context.Context, serviceWorktreeDir string, sourceCommit string, branchName string) (string, error) {
	branchListCmd := NewGitCmd(ctx, &GitCmdOptions{RepoDir: serviceWorktreeDir}, "branch", "--list", branchName)
	if err := branchListCmd.Run(ctx); err != nil {
		return "", fmt.Errorf("git branch list command failed: %s", err)
	}

	var isServiceBranchExist bool
	isServiceBranchExist = branchListCmd.OutBuf.Len() != 0

	if !isServiceBranchExist {
		checkoutCmd := NewGitCmd(ctx, &GitCmdOptions{RepoDir: serviceWorktreeDir}, "checkout", "-b", branchName, sourceCommit)
		if err := checkoutCmd.Run(ctx); err != nil {
			return "", fmt.Errorf("git checkout command failed: %s", err)
		}

		return sourceCommit, nil
	}

	revParseCmd := NewGitCmd(ctx, &GitCmdOptions{RepoDir: serviceWorktreeDir}, "rev-parse", branchName)
	if err := revParseCmd.Run(ctx); err != nil {
		return "", fmt.Errorf("git rev parse branch command failed: %s", err)
	}

	serviceBranchHeadCommit := strings.TrimSpace(revParseCmd.OutBuf.String())
	return serviceBranchHeadCommit, nil
}

func revertExcludedChangesInServiceWorktreeIndex(ctx context.Context, sourceWorktreeDir string, serviceWorktreeDir string, sourceCommit string, serviceBranchHeadCommit string, globExcludeList []string) (bool, error) {
	if len(globExcludeList) == 0 || serviceBranchHeadCommit == sourceCommit {
		return false, nil
	}

	gitDiffArgs := []string{
		"-c", "diff.renames=false",
		"-c", "core.quotePath=false",
		"diff",
		"--binary",
		serviceBranchHeadCommit, sourceCommit,
		"--",
	}
	gitDiffArgs = append(gitDiffArgs, globExcludeList...)

	diffCmd := NewGitCmd(ctx, &GitCmdOptions{RepoDir: serviceWorktreeDir}, gitDiffArgs...)
	if err := diffCmd.Run(ctx); err != nil {
		return false, fmt.Errorf("git diff command failed: %s", err)
	}

	if diffCmd.OutBuf.Len() == 0 {
		return false, nil
	}

	applyCmd := NewGitCmd(ctx, &GitCmdOptions{RepoDir: serviceWorktreeDir}, "apply", "--binary", "--index")
	applyCmd.Stdin = diffCmd.OutBuf
	if err := applyCmd.Run(ctx); err != nil {
		return false, fmt.Errorf("git apply command failed: %s", err)
	}

	return true, nil
}

func checkNewChangesInSourceWorktreeDir(ctx context.Context, sourceWorktreeDir string, serviceWorktreeDir string, globExcludeList []string) (bool, error) {
	output, err := runGitAddCmd(ctx, sourceWorktreeDir, serviceWorktreeDir, globExcludeList, true)
	if err != nil {
		return false, err
	}

	return len(output.Bytes()) != 0, nil
}

func addNewChangesInServiceWorktreeDir(ctx context.Context, sourceWorktreeDir string, serviceWorktreeDir string, globExcludeList []string) error {
	_, err := runGitAddCmd(ctx, sourceWorktreeDir, serviceWorktreeDir, globExcludeList, false)
	return err
}

func runGitAddCmd(ctx context.Context, sourceWorktreeDir string, serviceWorktreeDir string, globExcludeList []string, dryRun bool) (*bytes.Buffer, error) {
	gitAddArgs := []string{
		"--work-tree",
		sourceWorktreeDir,
		"add",
	}

	if dryRun {
		gitAddArgs = append(gitAddArgs, "--dry-run", "--ignore-missing")
	}

	var pathSpecList []string
	{
		pathSpecList = append(pathSpecList, ":.")
		for _, glob := range globExcludeList {
			pathSpecList = append(pathSpecList, ":!"+glob)
		}
	}

	var pathSpecFileBuf *bytes.Buffer
	if gitVersion.LessThan(semver.MustParse("2.25.0")) {
		gitAddArgs = append(gitAddArgs, "--")
		gitAddArgs = append(gitAddArgs, pathSpecList...)
	} else {
		gitAddArgs = append(gitAddArgs, "--pathspec-from-file=-", "--pathspec-file-nul")
		pathSpecFileBuf = bytes.NewBufferString(strings.Join(pathSpecList, "\000"))
	}

	addCmd := NewGitCmd(ctx, &GitCmdOptions{RepoDir: serviceWorktreeDir}, gitAddArgs...)
	if pathSpecFileBuf != nil {
		addCmd.Stdin = pathSpecFileBuf
	}
	if err := addCmd.Run(ctx); err != nil {
		return nil, err
	}

	return addCmd.OutBuf, nil
}

func commitNewChangesInServiceBranch(ctx context.Context, serviceWorktreeDir string, branchName string) (string, error) {
	commitCmd := NewGitCmd(ctx, &GitCmdOptions{RepoDir: serviceWorktreeDir}, "-c", "user.email=werf@werf.io", "-c", "user.name=werf", "commit", "--no-verify", "-m", time.Now().String())
	if err := commitCmd.Run(ctx); err != nil {
		return "", fmt.Errorf("git commit command failed: %s", err)
	}

	revParseCmd := NewGitCmd(ctx, &GitCmdOptions{RepoDir: serviceWorktreeDir}, "rev-parse", branchName)
	if err := revParseCmd.Run(ctx); err != nil {
		return "", fmt.Errorf("git rev parse branch command failed: %s", err)
	}

	serviceNewCommit := strings.TrimSpace(revParseCmd.OutBuf.String())

	checkoutCmd := NewGitCmd(ctx, &GitCmdOptions{RepoDir: serviceWorktreeDir}, "checkout", "--force", "--detach", serviceNewCommit)
	if err := checkoutCmd.Run(ctx); err != nil {
		return "", fmt.Errorf("git checkout command failed: %s", err)
	}

	return serviceNewCommit, nil
}
