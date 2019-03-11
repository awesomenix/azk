package main

// Idea here is that
//  we create kind cluster
//  deploy crd and controllers into it
//  create target cluster
//  move all the crds and controllers to target cluster

import (
	"github.com/awesomenix/azk/cmd/azk/cmd"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func main() {
	logf.SetLogger(logf.ZapLogger(true))
	cmd.Execute()
}
