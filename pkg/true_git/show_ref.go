package true_git

import (
	"fmt"
	"os/exec"
	"strings"
)

type RefDescriptor struct {
	Commit   string
	FullName string
	IsHEAD   bool

	IsTag   bool
	TagName string

	IsBranch   bool
	BranchName string
	IsRemote   bool
	RemoteName string
}

type ShowRefResult struct {
	Refs []RefDescriptor
}

func ShowRef(repoDir string) (*ShowRefResult, error) {
	gitArgs := append(getCommonGitOptions(), "-C", repoDir, "show-ref", "--head")

	cmd := exec.Command("git", gitArgs...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%v failed: %s:\n%s", strings.Join(append([]string{"git"}, gitArgs...), " "), err, output)
	}

	res := &ShowRefResult{}

	outputLines := strings.Split(string(output), "\n")
	for _, line := range outputLines {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}

		ref := RefDescriptor{
			Commit:   parts[0],
			FullName: parts[1],
		}

		if ref.FullName == "HEAD" {
			ref.IsHEAD = true
		} else if strings.HasPrefix(ref.FullName, "refs/tags/") {
			ref.IsTag = true
			ref.TagName = strings.TrimPrefix(ref.FullName, "refs/tags/")
		} else if strings.HasPrefix(ref.FullName, "refs/heads/") {
			ref.IsBranch = true
			ref.BranchName = strings.TrimPrefix(ref.FullName, "refs/heads/")
		} else if strings.HasPrefix(ref.FullName, "refs/remotes/") {
			ref.IsBranch = true
			ref.IsRemote = true

			parts := strings.SplitN(strings.TrimPrefix(ref.FullName, "refs/remotes/"), "/", 2)
			if len(parts) != 2 {
				continue
			}
			ref.RemoteName, ref.BranchName = parts[0], parts[1]
		}

		res.Refs = append(res.Refs, ref)
	}

	return res, nil
}
