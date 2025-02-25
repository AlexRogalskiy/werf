package tmp_manager

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/werf/logboek"
	"github.com/werf/werf/pkg/util"
	"github.com/werf/werf/pkg/werf"
)

func Purge(ctx context.Context, dryRun bool) error {
	return logboek.Context(ctx).LogProcess("Running purge for tmp data").DoError(func() error { return purge(ctx, dryRun) })
}

func purge(ctx context.Context, dryRun bool) error {
	tmpFiles, err := ioutil.ReadDir(werf.GetTmpDir())
	if err != nil {
		return fmt.Errorf("unable to list tmp files in %s: %s", werf.GetTmpDir(), err)
	}

	filesToRemove := []string{}
	projectDirsToRemove := []string{}

	for _, finfo := range tmpFiles {
		if strings.HasPrefix(finfo.Name(), ProjectDirPrefix) {
			projectDirsToRemove = append(projectDirsToRemove, filepath.Join(werf.GetTmpDir(), finfo.Name()))
		}

		if strings.HasPrefix(finfo.Name(), CommonPrefix) {
			filesToRemove = append(filesToRemove, filepath.Join(werf.GetTmpDir(), finfo.Name()))
		}
	}

	var errors []error
	if len(projectDirsToRemove) > 0 {
		for _, projectDirToRemove := range projectDirsToRemove {
			logboek.Context(ctx).LogLn(projectDirToRemove)
		}
		if !dryRun {
			if runtime.GOOS == "windows" {
				for _, path := range projectDirsToRemove {
					if err := os.RemoveAll(path); err != nil {
						errors = append(errors, fmt.Errorf("unable to remove tmp project dir %s: %s", path, err))
					}
				}
			} else {
				if err := util.RemoveHostDirsWithLinuxContainer(ctx, werf.GetTmpDir(), projectDirsToRemove); err != nil {
					errors = append(errors, fmt.Errorf("unable to remove tmp projects dirs %s: %s", strings.Join(projectDirsToRemove, ", "), err))
				}
			}
		}
	}

	filesToRemove = append(filesToRemove, GetServiceTmpDir())

	for _, file := range filesToRemove {
		logboek.Context(ctx).LogLn(file)

		if !dryRun {
			err := os.RemoveAll(file)
			if err != nil {
				errors = append(errors, fmt.Errorf("unable to remove %s: %s", file, err))
			}
		}
	}

	if len(errors) > 0 {
		msg := ""
		for _, err := range errors {
			msg += fmt.Sprintf("%s\n", err)
		}
		return fmt.Errorf("%s", msg)
	}

	return nil
}
