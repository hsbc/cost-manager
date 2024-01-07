package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hsbc/cost-manager/pkg/kubernetes"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/prometheus/client_golang/api"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	prometheusAlertsInactiveDuration = 30 * time.Second
)

func TestPrometheusAlerts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	kubeClient, restConfig, err := kubernetes.NewClient()
	require.Nil(t, err)

	pod, err := kubernetes.WaitForAnyReadyPod(ctx, kubeClient, client.InNamespace("monitoring"), client.MatchingLabels{"app.kubernetes.io/name": "prometheus"})
	require.Nil(t, err)
	// Port forward to Prometheus in the background
	forwardedPort, close, err := kubernetes.PortForward(ctx, restConfig, pod.Namespace, pod.Name, 9090)
	require.Nil(t, err)
	defer func() {
		err := close()
		require.Nil(t, err)
	}()
	// Setup Prometheus client using local forwarded port
	prometheusAddress := fmt.Sprintf("http://127.0.0.1:%d", forwardedPort)
	prometheusClient, err := api.NewClient(api.Config{
		Address: prometheusAddress,
	})
	require.Nil(t, err)
	prometheusAPI := prometheusv1.NewAPI(prometheusClient)

	t.Log("Waiting for cost-manager alerts to be registered with Prometheus...")
	costManagerPrometheusRule := &monitoringv1.PrometheusRule{}
	err = kubeClient.Get(ctx, client.ObjectKey{Name: "cost-manager", Namespace: "cost-manager"}, costManagerPrometheusRule)
	require.Nil(t, err)
	for {
		prometheusRules, err := prometheusAPI.Rules(ctx)
		require.Nil(t, err)
		if prometheusHasAllCostManagerAlerts(costManagerPrometheusRule, prometheusRules) {
			break
		}
		time.Sleep(time.Second)
	}
	t.Log("All cost-manager alerts registered with Prometheus!")

	// Wait for Prometheus to scrape cost-manager
	for {
		results, _, err := prometheusAPI.Query(ctx, `up{job="cost-manager",namespace="cost-manager"}`, time.Now())
		require.Nil(t, err)
		if len(results.(model.Vector)) > 0 {
			break
		}
		time.Sleep(time.Second)
	}

	t.Logf("Ensuring all Prometheus alerts remain inactive for %s...", prometheusAlertsInactiveDuration)
	err = waitForAllPrometheusAlertsToRemainInactive(ctx, prometheusAPI)
	require.Nil(t, err)
	t.Logf("All Prometheus alerts remained inactive for %s!", prometheusAlertsInactiveDuration)
}

func prometheusHasAllCostManagerAlerts(costManagerPrometheusRule *monitoringv1.PrometheusRule, prometheusRules prometheusv1.RulesResult) bool {
	for _, group := range costManagerPrometheusRule.Spec.Groups {
		for _, rule := range group.Rules {
			if len(rule.Alert) > 0 {
				if !prometheusHasGroupAlert(group.Name, rule.Alert, prometheusRules) {
					return false
				}
			}
		}
	}
	return true
}

func prometheusHasGroupAlert(groupName string, alertName string, prometheusRules prometheusv1.RulesResult) bool {
	for _, group := range prometheusRules.Groups {
		if group.Name == groupName {
			for _, rule := range group.Rules {
				switch alert := rule.(type) {
				case prometheusv1.AlertingRule:
					if alertName == alert.Name {
						return true
					}
				default:
					continue
				}
			}
		}
	}
	return false
}

func waitForAllPrometheusAlertsToRemainInactive(ctx context.Context, prometheusAPI prometheusv1.API) error {
	ticker := time.NewTicker(time.Second)
	for {
		done := time.After(prometheusAlertsInactiveDuration)
		for {
			resetTimer := false
			select {
			case <-done:
				return nil
			case <-ticker.C:
				result, err := prometheusAPI.Alerts(ctx)
				if err != nil {
					return err
				}

				// If any alerts are firing then we return an error
				var firingAlertNames []string
				for _, alert := range result.Alerts {
					if alert.State == prometheusv1.AlertStateFiring {
						firingAlertNames = append(firingAlertNames, string(alert.Labels[model.AlertNameLabel]))
					}
				}
				if len(firingAlertNames) > 0 {
					return fmt.Errorf("Prometheus alerts firing: %s", strings.Join(firingAlertNames, ", "))
				}

				// If any alerts are not inactive then we reset our timer
				resetTimer = !allPrometheusAlertsInactive(result.Alerts)
			}
			if resetTimer {
				break
			}
		}
	}
}

func allPrometheusAlertsInactive(alerts []prometheusv1.Alert) bool {
	for _, alert := range alerts {
		if alert.State != prometheusv1.AlertStateInactive {
			return false
		}
	}
	return true
}
