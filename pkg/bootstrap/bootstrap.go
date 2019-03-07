package bootstrap

import (
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("bootstrap")

func (spec *Spec) Bootstrap() error {
	if err := spec.CreateBaseInfrastructure(); err != nil {
		log.Error(err, "Error creating base bootstrap infrastructure")
		return err
	}

	if err := spec.CreateInfrastructure(); err != nil {
		log.Error(err, "Error creating bootstrap infrastructure")
		return err
	}

	return nil
}
