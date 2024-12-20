package controller

import (
	"context"
	"time"

	"github.com/hsbc/cost-manager/pkg/api/v1alpha1"
	"github.com/robfig/cron/v3"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	scheduledPodLimiterControllerName = "scheduled-pod-limiter"
)

// scheduledPodLimiter creates ResourceQuotas in selected Namespaces at the scheduled stop times to
// limit Pods to 0 and then deletes all Pods; this should trigger the cluster autoscaler to scale
// the cluster down. At the scheduled start times the ResourceQuotas are deleted, allowing the Pods
// to be recreated
type scheduledPodLimiter struct {
	Config        *v1alpha1.ScheduledPodLimiter
	Client        client.Client
	startSchedule cron.Schedule
	stopSchedule  cron.Schedule
}

var _ reconcile.Reconciler = &scheduledPodLimiter{}

func (s *scheduledPodLimiter) SetupWithManager(mgr ctrl.Manager) error {
	// Parse cron schedules
	startSchedule, err := cron.ParseStandard(s.Config.SchedulePolicy.StartSchedule)
	if err != nil {
		return err
	}
	s.startSchedule = startSchedule
	stopSchedule, err := cron.ParseStandard(s.Config.SchedulePolicy.StopSchedule)
	if err != nil {
		return err
	}
	s.stopSchedule = stopSchedule

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Complete(s)
}

func (s *scheduledPodLimiter) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	// TODO: Determine whether we should limit workloads

	return reconcile.Result{}, nil
}

func (s *scheduledPodLimiter) shouldLimitPods(now time.Time) bool {
	previousStartTime := s.startSchedule.Prev(now)
	previousStopTime := s.stopSchedule.Prev(now)

	return previousStopTime.After(previousStartTime)
}
