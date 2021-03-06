/*
Copyright AppsCode Inc.

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

package api

import (
	"sort"
)

type Addon struct {
	Name     string   `json:"name"`
	Versions []string `json:"versions"`
}

type StashCatalog struct {
	ChartRegistry    string  `json:"chart_registry"`
	ChartRegistryURL string  `json:"chart_registry_url"`
	Addons           []Addon `json:"addons"`
}

func (c *StashCatalog) Sort() {
	sort.Slice(c.Addons, func(i, j int) bool { return c.Addons[i].Name < c.Addons[j].Name })
	var err error
	for i, project := range c.Addons {
		c.Addons[i].Versions, err = SortVersions(project.Versions)
		if err != nil {
			panic(err)
		}
	}
}
