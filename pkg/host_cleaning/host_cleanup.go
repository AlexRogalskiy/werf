package host_cleaning

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/werf/logboek"
	"github.com/werf/werf/pkg/git_repo/gitdata"
	"github.com/werf/werf/pkg/tmp_manager"
)

const (
	DefaultAllowedDockerStorageVolumeUsagePercentage       float64 = 70.0
	DefaultAllowedDockerStorageVolumeUsageMarginPercentage float64 = 5.0
	DefaultAllowedLocalCacheVolumeUsagePercentage          float64 = 70.0
	DefaultAllowedLocalCacheVolumeUsageMarginPercentage    float64 = 5.0
)

type HostCleanupOptions struct {
	AllowedDockerStorageVolumeUsagePercentage       *uint
	AllowedDockerStorageVolumeUsageMarginPercentage *uint
	AllowedLocalCacheVolumeUsagePercentage          *uint
	AllowedLocalCacheVolumeUsageMarginPercentage    *uint
	DockerServerStoragePath                         *string

	DryRun bool
	Force  bool
}

type AutoHostCleanupOptions struct {
	HostCleanupOptions

	ForceShouldRun bool
}

func getOptionValueOrDefault(optionValue *uint, defaultValue float64) float64 {
	var res float64
	if optionValue != nil {
		res = float64(*optionValue)
	} else {
		res = defaultValue
	}
	return res
}

func RunAutoHostCleanup(ctx context.Context, options AutoHostCleanupOptions) error {
	if !options.ForceShouldRun {
		shouldRun, err := ShouldRunAutoHostCleanup(ctx, options.HostCleanupOptions)
		if err != nil {
			logboek.Context(ctx).Warn().LogF("WARNING: unable to check if auto host cleanup should be run: %s\n", err)
			return nil
		}
		if !shouldRun {
			return nil
		}
	}

	var args []string

	args = append(args,
		"host", "cleanup",
		fmt.Sprintf("--dry-run=%v", options.DryRun),
		fmt.Sprintf("--force=%v", options.Force),
	)

	if options.AllowedDockerStorageVolumeUsagePercentage != nil {
		args = append(args, "--allowed-docker-storage-volume-usage", fmt.Sprintf("%d", *options.AllowedDockerStorageVolumeUsagePercentage))
	}
	if options.AllowedDockerStorageVolumeUsageMarginPercentage != nil {
		args = append(args, "--allowed-docker-storage-volume-usage-margin", fmt.Sprintf("%d", *options.AllowedDockerStorageVolumeUsageMarginPercentage))
	}
	if options.AllowedLocalCacheVolumeUsagePercentage != nil {
		args = append(args, "--allowed-local-cache-volume-usage", fmt.Sprintf("%d", *options.AllowedLocalCacheVolumeUsagePercentage))
	}
	if options.AllowedLocalCacheVolumeUsageMarginPercentage != nil {
		args = append(args, "--allowed-docker-storage-volume-usage-margin", fmt.Sprintf("%d", *options.AllowedLocalCacheVolumeUsageMarginPercentage))
	}
	if options.DockerServerStoragePath != nil {
		args = append(args, "--docker-server-storage-path", *options.DockerServerStoragePath)
	}

	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(cmd.Env, os.Environ()...)
	cmd.Env = append(cmd.Env, "_WERF_BACKGROUND_MODE_ENABLED=1")

	if err := cmd.Start(); err != nil {
		logboek.Context(ctx).Warn().LogF("WARNING: unable to start background auto host cleanup process: %s\n", err)
		return nil
	}

	if err := cmd.Process.Release(); err != nil {
		logboek.Context(ctx).Warn().LogF("WARNING: unable to detach background auto host cleanup process: %s\n", err)
		return nil
	}

	return nil
}

func RunHostCleanup(ctx context.Context, options HostCleanupOptions) error {
	if err := logboek.Context(ctx).LogProcess("Running GC for tmp data").DoError(func() error {
		if err := tmp_manager.RunGC(ctx, options.DryRun); err != nil {
			return fmt.Errorf("tmp files GC failed: %s", err)
		}
		return nil
	}); err != nil {
		return err
	}

	allowedLocalCacheVolumeUsagePercentage := getOptionValueOrDefault(options.AllowedLocalCacheVolumeUsagePercentage, DefaultAllowedLocalCacheVolumeUsagePercentage)
	allowedLocalCacheVolumeUsageMarginPercentage := getOptionValueOrDefault(options.AllowedLocalCacheVolumeUsageMarginPercentage, DefaultAllowedLocalCacheVolumeUsageMarginPercentage)

	allowedDockerStorageVolumeUsagePercentage := getOptionValueOrDefault(options.AllowedDockerStorageVolumeUsagePercentage, DefaultAllowedDockerStorageVolumeUsagePercentage)
	allowedDockerStorageVolumeUsageMarginPercentage := getOptionValueOrDefault(options.AllowedDockerStorageVolumeUsageMarginPercentage, DefaultAllowedDockerStorageVolumeUsageMarginPercentage)

	if err := logboek.Context(ctx).Default().LogProcess("Running GC for git data").DoError(func() error {
		if err := gitdata.RunGC(ctx, allowedLocalCacheVolumeUsagePercentage, allowedLocalCacheVolumeUsageMarginPercentage); err != nil {
			return fmt.Errorf("git repo GC failed: %s", err)
		}
		return nil
	}); err != nil {
		return err
	}

	dockerServerStoragePath, err := getDockerServerStoragePath(ctx, options.DockerServerStoragePath)
	if err != nil {
		return fmt.Errorf("error getting local docker server storage path: %s", err)
	}

	return logboek.Context(ctx).Default().LogProcess("Running GC for local docker server").DoError(func() error {
		if err := RunGCForLocalDockerServer(ctx, allowedDockerStorageVolumeUsagePercentage, allowedDockerStorageVolumeUsageMarginPercentage, dockerServerStoragePath, options.Force, options.DryRun); err != nil {
			return fmt.Errorf("local docker server GC failed: %s", err)
		}
		return nil
	})
}

func ShouldRunAutoHostCleanup(ctx context.Context, options HostCleanupOptions) (bool, error) {
	shouldRun, err := tmp_manager.ShouldRunAutoGC()
	if err != nil {
		return false, fmt.Errorf("failed to check tmp manager GC: %s", err)
	}
	if shouldRun {
		return true, nil
	}

	allowedLocalCacheVolumeUsagePercentage := getOptionValueOrDefault(options.AllowedLocalCacheVolumeUsagePercentage, DefaultAllowedLocalCacheVolumeUsagePercentage)
	allowedDockerStorageVolumeUsagePercentage := getOptionValueOrDefault(options.AllowedDockerStorageVolumeUsagePercentage, DefaultAllowedDockerStorageVolumeUsagePercentage)

	shouldRun, err = gitdata.ShouldRunAutoGC(ctx, allowedLocalCacheVolumeUsagePercentage)
	if err != nil {
		return false, fmt.Errorf("failed to check git repo GC: %s", err)
	}
	if shouldRun {
		return true, nil
	}

	dockerServerStoragePath, err := getDockerServerStoragePath(ctx, options.DockerServerStoragePath)
	if err != nil {
		return false, fmt.Errorf("error getting local docker server storage path: %s", err)
	}

	shouldRun, err = ShouldRunAutoGCForLocalDockerServer(ctx, allowedDockerStorageVolumeUsagePercentage, dockerServerStoragePath)
	if err != nil {
		return false, fmt.Errorf("failed to check local docker server host cleaner GC: %s", err)
	}
	return shouldRun, nil
}
