/*
Copyright AppsCode Inc. and Contributors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package backend

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/nats-io/nats.go"
	"k8s.io/klog/v2"
)

const (
	NatsConnectionTimeout       = 350 * time.Millisecond
	NatsConnectionRetryInterval = 100 * time.Millisecond
	NatsRequestTimeout          = 10 * time.Second

	StreamPrefix = "gha_"
)

// NewConnection creates a new NATS connection
func NewConnection(addr, credFile string) (nc *nats.Conn, err error) {
	hostname, _ := os.Hostname()
	opts := []nats.Option{
		nats.Name(fmt.Sprintf("ghactions.%s", hostname)),
		nats.MaxReconnects(-1),
		nats.ErrorHandler(errorHandler),
		nats.ReconnectHandler(reconnectHandler),
		nats.DisconnectErrHandler(disconnectHandler),
		// nats.UseOldRequestStyle(),
	}

	if _, err := os.Stat(credFile); os.IsNotExist(err) {
		var username, password string
		if v, ok := os.LookupEnv("NATS_USERNAME"); ok {
			username = v
		} else {
			username = os.Getenv("THIS_IS_NATS_USERNAME")
		}
		if v, ok := os.LookupEnv("NATS_PASSWORD"); ok {
			password = v
		} else {
			password = os.Getenv("THIS_IS_NATS_PASSWORD")
		}
		if username != "" && password != "" {
			opts = append(opts, nats.UserInfo(username, password))
		}
	} else {
		opts = append(opts, nats.UserCredentials(credFile))
	}

	//if os.Getenv("NATS_CERTIFICATE") != "" && os.Getenv("NATS_KEY") != "" {
	//	opts = append(opts, nats.ClientCert(os.Getenv("NATS_CERTIFICATE"), os.Getenv("NATS_KEY")))
	//}
	//
	//if os.Getenv("NATS_CA") != "" {
	//	opts = append(opts, nats.RootCAs(os.Getenv("NATS_CA")))
	//}

	// initial connections can error due to DNS lookups etc, just retry, eventually with backoff
	ctx, cancel := context.WithTimeout(context.Background(), NatsConnectionTimeout)
	defer cancel()

	ticker := time.NewTicker(NatsConnectionRetryInterval)
	for {
		select {
		case <-ticker.C:
			nc, err := nats.Connect(addr, opts...)
			if err == nil {
				return nc, nil
			}
			klog.InfoS("failed to connect to event receiver", "error", err)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// called during errors subscriptions etc
func errorHandler(nc *nats.Conn, s *nats.Subscription, err error) {
	if s != nil {
		klog.Infof("error in event receiver connection: %s: subscription: %s: %s", nc.ConnectedUrl(), s.Subject, err)
		return
	}
	klog.Infof("Error in event receiver connection: %s: %s", nc.ConnectedUrl(), err)
}

// called after reconnection
func reconnectHandler(nc *nats.Conn) {
	klog.Infof("Reconnected to %s", nc.ConnectedUrl())
}

// called after disconnection
func disconnectHandler(nc *nats.Conn, err error) {
	if err != nil {
		klog.Infof("Disconnected from event receiver due to error: %v", err)
	} else {
		klog.Infof("Disconnected from event receiver")
	}
}
