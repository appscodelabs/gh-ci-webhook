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

//nolint:goconst
package cmds

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/appscodelabs/gh-ci-webhook/pkg/backend"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/acme/autocert"
	shell "gomodules.xyz/go-sh"
	passgen "gomodules.xyz/password-generator"
)

var (
	secretToken = ""
	certDir     = "certs"
	email       = "tamal@appscode.com"
	hosts       = []string{"this-is-nats.appscode.ninja"}
	port        = 8080
	enableSSL   bool
	queueLength = 100
)

func NewCmdRun() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "run",
		Short:             "Run GitHub webhook server",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			nc, err = backend.NewConnection(addr, credFile)
			if err != nil {
				return err
			}
			defer nc.Drain() //nolint:errcheck

			if secretToken == "" {
				secretToken = passgen.GenerateForCharset(20, passgen.AlphaNum)
				fmt.Printf("using secret token %s\n", secretToken)
			}

			return runServer()
		},
	}

	cmd.Flags().StringVar(&secretToken, "secret-token", secretToken, "Secret token to verify webhook payloads")
	cmd.Flags().StringVar(&certDir, "cert-dir", certDir, "Directory where certs are stored")
	cmd.Flags().StringVar(&email, "email", email, "Email used by Let's Encrypt to notify about problems with issued certificates")
	cmd.Flags().StringSliceVar(&hosts, "hosts", hosts, "Hosts for which certificate will be issued")
	cmd.Flags().IntVar(&port, "port", port, "Port used when SSL is not enabled")
	cmd.Flags().BoolVar(&enableSSL, "ssl", enableSSL, "Set true to enable SSL via Let's Encrypt")
	cmd.Flags().IntVar(&queueLength, "queue-length", queueLength, "Length of queue used to hold pr events")

	cmd.Flags().StringVar(&addr, "nats-addr", addr, "NATS serve address")
	cmd.Flags().StringVar(&credFile, "nats-credential-file", credFile, "PATH to NATS credential file")

	return cmd
}

type Response struct {
	Type    string               `json:"type,omitempty"`
	Host    string               `json:"host,omitempty"`
	URL     string               `json:"url,omitempty"`
	Method  string               `json:"method,omitempty"`
	Headers http.Header          `json:"headers,omitempty"`
	Body    string               `json:"body,omitempty"`
	TLS     *tls.ConnectionState `json:"tls,omitempty"`
}

func runServer() error {
	sh := shell.NewSession()
	sh.ShowCMD = true
	sh.PipeFail = true
	sh.PipeStdErrors = true

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		resp := &Response{
			Type:    "http",
			Host:    r.Host,
			URL:     r.URL.String(),
			Method:  r.Method,
			Headers: r.Header,
			TLS:     r.TLS,
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(resp)
	})
	r.Post("/*", serveHTTP)

	if !enableSSL {
		addr := fmt.Sprintf(":%d", port)
		fmt.Println("Listening to addr", addr)
		return http.ListenAndServe(addr, r)
	}

	// ref:
	// - https://goenning.net/2017/11/08/free-and-automated-ssl-certificates-with-go/
	// - https://stackoverflow.com/a/40494806/244009
	certManager := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache(certDir),
		HostPolicy: autocert.HostWhitelist(hosts...),
		Email:      email,
	}
	server := &http.Server{
		Addr:         ":https",
		Handler:      r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  120 * time.Second,
		TLSConfig: &tls.Config{
			GetCertificate: certManager.GetCertificate,
		},
	}

	go func() {
		// does automatic http to https redirects
		err := http.ListenAndServe(":http", certManager.HTTPHandler(nil))
		if err != nil {
			panic(err)
		}
	}()

	fmt.Println("Listening to addr", server.Addr)
	return server.ListenAndServeTLS("", "") // Key and cert are coming from Let's Encrypt
}

func serveHTTP(w http.ResponseWriter, r *http.Request) {
	err := backend.SubmitPayload(nc, r, []byte(secretToken))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
}