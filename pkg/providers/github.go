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

package providers

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/google/go-github/v49/github"
)

const (
	skew = 10 * time.Second
)

func ListRunners(ctx context.Context, client *github.Client, repo *github.Repository) ([]*github.Runner, error) {
	opt := &github.ListOptions{
		PerPage: 100,
	}

	var result []*github.Runner
	for {
		runners, resp, err := client.Actions.ListRunners(ctx, repo.Owner.GetLogin(), repo.GetName(), opt)
		switch e := err.(type) {
		case *github.RateLimitError:
			time.Sleep(time.Until(e.Rate.Reset.Time.Add(skew)))
			continue
		case *github.AbuseRateLimitError:
			time.Sleep(e.GetRetryAfter())
			continue
		case *github.ErrorResponse:
			if e.Response.StatusCode == http.StatusNotFound {
				log.Println(err)
				break
			} else {
				return nil, err
			}
		default:
			if e != nil {
				return nil, err
			}
		}

		result = append(result, runners.Runners...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return result, nil
}

func DeleteRunner(ctx context.Context, client *github.Client, repo *github.Repository, name string) error {
	runners, err := ListRunners(ctx, client, repo)
	if err != nil {
		return err
	}
	for _, runner := range runners {
		if runner.GetName() == name {
			_, err := client.Actions.RemoveRunner(ctx, repo.Owner.GetLogin(), repo.GetName(), runner.GetID())
			return err
		}
	}
	return nil
}
