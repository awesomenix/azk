package cmd

import (
	"flag"
	"fmt"
	"os"

	enginev1alpha1 "github.com/awesomenix/azkube/pkg/apis/engine/v1alpha1"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes/scheme"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("azkube")

var RootCmd = &cobra.Command{
	Use:   "azkube",
	Short: "azure cluster management",
	Long:  `Simple kubernetes azure cluster manager`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		cmd.Flags().Set("logtostderr", "true")
	},
	Run: func(cmd *cobra.Command, args []string) {
		// Do Stuff Here
		cmd.Help()
	},
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func exitWithHelp(cmd *cobra.Command, err string) {
	fmt.Fprintln(os.Stderr, err)
	cmd.Help()
	os.Exit(1)
}

func init() {
	enginev1alpha1.AddToScheme(scheme.Scheme)
	flag.CommandLine.Set("logtostderr", "true")
	RootCmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)
}
