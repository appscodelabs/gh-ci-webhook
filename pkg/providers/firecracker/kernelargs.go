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

package firecracker

import "strings"

// kernelArgs serializes+deserializes kernel boot parameters from/into a map.
// Kernel docs: https://www.kernel.org/doc/Documentation/admin-guide/kernel-parameters.txt
//
// "key=value" will result in map["key"] = &"value"
// "key=" will result in map["key"] = &""
// "key" will result in map["key"] = nil
type kernelArgs map[string]*string

// serialize the kernelArgs back to a string that can be provided
// to the kernel
func (kargs kernelArgs) String() string {
	var fields []string
	for key, value := range kargs {
		field := key
		if value != nil {
			field += "=" + *value
		}
		fields = append(fields, field)
	}
	return strings.Join(fields, " ")
}

// deserialize the provided string to a kernelArgs map
func parseKernelArgs(rawString string) kernelArgs {
	argMap := make(map[string]*string)
	for _, kv := range strings.Fields(rawString) {
		// only split into up to 2 fields (before and after the first "=")
		kvSplit := strings.SplitN(kv, "=", 2)

		key := kvSplit[0]

		var value *string
		if len(kvSplit) == 2 {
			value = &kvSplit[1]
		}

		argMap[key] = value
	}

	return argMap
}
