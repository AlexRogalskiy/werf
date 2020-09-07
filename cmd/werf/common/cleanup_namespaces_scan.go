package common

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/werf/kubedog/pkg/kube"
	"github.com/werf/logboek"
)

func SetupScanContextNamespaceOnly(cmdData *CmdData, cmd *cobra.Command) {
	cmdData.ScanContextNamespaceOnly = new(bool)
	cmd.Flags().BoolVarP(cmdData.ScanContextNamespaceOnly, "scan-context-namespace-only", "", GetBoolEnvironmentDefaultFalse("WERF_SCAN_CONTEXT_NAMESPACE_ONLY"), "Scan for used images only in namespace linked with context for each available context in kube-config (or only for the context specified with option --kube-context). When disabled will scan all namespaces in all contexts (or only for the context specified with option --kube-context). (Default $WERF_SCAN_CONTEXT_NAMESPACE_ONLY)")
}

func GetKubernetesContextClients(cmdData *CmdData) ([]*kube.ContextClient, error) {
	var res []*kube.ContextClient
	if contextClients, err := kube.GetAllContextsClients(kube.GetAllContextsClientsOptions{KubeConfig: *cmdData.KubeConfig}); err != nil {
		return nil, err
	} else {
		if *cmdData.KubeContext != "" {
			for _, cc := range contextClients {
				if cc.ContextName == *cmdData.KubeContext {
					res = append(res, cc)
					break
				}
			}

			if len(res) == 0 {
				return nil, fmt.Errorf("cannot find specified kube context %q", *cmdData.KubeContext)
			}
		} else {
			res = contextClients
		}
	}

	for _, contextClient := range res {
		logboek.Debug().LogF("GetKubernetesContextClients -- context %q namespace %q\n", contextClient.ContextName, contextClient.ContextNamespace)
	}

	return res, nil
}

func GetKubernetesNamespaceRestrictionByContext(cmdData *CmdData, contextClients []*kube.ContextClient) map[string]string {
	res := map[string]string{}
	for _, contextClient := range contextClients {
		if *cmdData.ScanContextNamespaceOnly {
			res[contextClient.ContextName] = contextClient.ContextNamespace
		} else {
			// "" - cluster scope, therefore all namespaces
			res[contextClient.ContextName] = ""
		}
	}

	for contextName, restrictionNamespace := range res {
		logboek.Debug().LogF("GetKubernetesNamespaceRestrictionByContext -- context %q restriction namespace %q\n", contextName, restrictionNamespace)
	}

	return res
}
