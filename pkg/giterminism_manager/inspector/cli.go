package inspector

import "github.com/werf/werf/pkg/giterminism_manager/errors"

func (i Inspector) InspectCustomTags() error {
	if i.sharedOptions.LooseGiterminism() {
		return nil
	}

	if i.giterminismConfig.IsCustomTagsAccepted() {
		return nil
	}

	return errors.NewError(`custom tags not allowed by giterminism

The use of --add-custom-tag and --use-custom-tag options might make previous deployments unreproducible and require extra configuration in the helm chart.`)
}
