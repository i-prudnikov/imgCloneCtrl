package main

import (
	"flag"
	"fmt"
	"strings"
)

var (
	argPrintVersion           bool
	argIgnoreNamespaces       = flagSet{"kube-system": struct{}{}}
	argBackupRegistry         string
	argBackupRegistryUser     string
	argBackupRegistryPassword string
	//Leader election
	argLeaderElectionID        string
	argLeaderElectionNamespace string
)

type flagSet map[string]struct{}

func (i *flagSet) String() string {
	if *i == nil {
		return ""
	}
	b := strings.Builder{}

	for key := range *i {
		fmt.Fprintf(&b, "%s,", key)
	}

	if b.String() != "" {
		return b.String()[:b.Len()-1]
	}
	return ""
}

func (i *flagSet) Set(value string) error {
	(*i)[value] = struct{}{}
	return nil
}

func init() {
	//Setting up flags
	flag.BoolVar(&argPrintVersion, "version", false, "Print version")
	flag.Var(&argIgnoreNamespaces, "ignoreNamespace",
		"Name of namespace to ignore. Multiple values supported.")
	flag.StringVar(&argBackupRegistry, "backupRegistry", "",
		"Backup registry to use (i.e. quay.io/my_favorite_registry)")
	flag.StringVar(&argBackupRegistryUser, "backupRegistryUser", "",
		"Backup registry user")
	flag.StringVar(&argBackupRegistryPassword, "backupRegistryPassword", "",
		"Backup registry password")

	flag.StringVar(&argLeaderElectionID, "leaderElectionID", "",
		"Leader election ID (configmap with this name will be created)")
	flag.StringVar(&argLeaderElectionNamespace, "leaderElectionNamespace", "",
		"Election namespace - in which leader election ID config map will be created")
}
