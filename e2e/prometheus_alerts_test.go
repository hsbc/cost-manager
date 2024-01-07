package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hsbc/cost-manager/pkg/kubernetes"
	"github.com/prometheus/client_golang/api"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	prometheusAlertsInactiveDuration = 30 * time.Second
)

func TestPrometheusAlerts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	restConfig := config.GetConfigOrDie()
	kubeClient, err := client.NewWithWatch(restConfig, client.Options{})
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
