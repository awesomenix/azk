package controller

import (
	"github.com/awesomenix/azkube/pkg/controller/nodepool"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, nodepool.Add)
}
