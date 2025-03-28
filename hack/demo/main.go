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

package main

import (
	"context"
	"fmt"
	"github.com/google/go-github/v70/github"
	"golang.org/x/oauth2"
	"os"
)

func main() {
	ghToken := os.Getenv("GITHUB_TOKEN")
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: ghToken})
	tc := oauth2.NewClient(ctx, ts)

	gh := github.NewClient(tc)
	org := "kubedb"

	ab, _, err := gh.Billing.GetActionsBillingOrg(context.Background(), org)
	if err != nil {
		panic(err)
	}
	fmt.Println(ab.IncludedMinutes)
	fmt.Println(ab.TotalPaidMinutesUsed)
}
