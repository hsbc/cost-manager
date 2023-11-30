package logging

import (
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var Logger logr.Logger

func init() {
	log.SetLogger(zap.New())
	Logger = log.Log
}
