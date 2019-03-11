package main

// Idea here is that
//  we create a bootstrap vm, first master
//  deploy crd and controllers into it
//  continues to create other masters and nodepools

import (
	"github.com/awesomenix/azk/cmd/azk/cmd"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func main() {
	logf.SetLogger(logf.ZapLogger(true))
	cmd.Execute()
}
