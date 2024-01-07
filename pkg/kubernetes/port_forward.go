package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/hashicorp/go-multierror"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// PortForward port forwards to the specified Pod in the background. The forwarded port is a random
// available local port which is returned as well as a function to close the listener when finished
func PortForward(ctx context.Context, restConfig *rest.Config, podNamespace, podName string, port int) (uint16, func() error, error) {
	stopChan, readyChan, errChan := make(chan struct{}, 1), make(chan struct{}, 1), make(chan error, 1)
	forwarder, err := createForwarder(ctx, restConfig, stopChan, readyChan, podNamespace, podName, port)
	if err != nil {
		return 0, nil, err
	}
	go func() {
		errChan <- forwarder.ForwardPorts()
	}()
	// Wait for port forward to be ready or fail
	select {
	case <-readyChan:
	case err := <-errChan:
		if err != nil {
			return 0, nil, err
		}
		return 0, nil, errors.New("port forward finished")
	}
	// Create function for the caller to finish port forwarding
	close := func() error {
		// Make sure any started listeners are stopped...
		close(stopChan)
		// ...and wait for the port forward to finish
		return <-errChan
	}
	forwardedPorts, err := forwarder.GetPorts()
	if err != nil {
		return 0, nil, multierror.Append(err, close())
	}
	if len(forwardedPorts) != 1 {
		err := fmt.Errorf("unexpected number of forwarded ports: %d", len(forwardedPorts))
		return 0, nil, multierror.Append(err, close())
	}
	return forwardedPorts[0].Local, close, nil
}

func createForwarder(ctx context.Context, restConfig *rest.Config, stopChan, readyChan chan struct{}, podNamespace, podName string, port int) (*portforward.PortForwarder, error) {
	// Discard output to avoid race conditions
	out, errOut := io.Discard, io.Discard

	roundTripper, upgrader, err := spdy.RoundTripperFor(restConfig)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", podNamespace, podName)
	hostIP := strings.TrimLeft(restConfig.Host, "htps:/")
	serverURL := url.URL{Scheme: "https", Path: path, Host: hostIP}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: roundTripper}, http.MethodPost, &serverURL)
	// Listen on a random available local port to avoid collisions:
	// https://github.com/kubernetes/client-go/blob/86d49e7265f07676cb39f342595a858b032112de/tools/portforward/portforward.go#L75
	forwarderPort := fmt.Sprintf(":%d", port)
	forwarder, err := portforward.New(dialer, []string{forwarderPort}, stopChan, readyChan, out, errOut)
	if err != nil {
		return nil, err
	}

	return forwarder, nil
}
