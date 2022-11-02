/*
Copyright 2019 The Kubernetes Authors.

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

package hints

import (
	discovery "k8s.io/api/discovery/v1"
	"k8s.io/klog/v2"
	endpointsliceutil "k8s.io/kubernetes/pkg/controller/util/endpointslice"
)

type SliceInfo struct {
	ServiceKey  string
	AddressType discovery.AddressType
	ToCreate    []*discovery.EndpointSlice
	ToUpdate    []*discovery.EndpointSlice
	Unchanged   []*discovery.EndpointSlice
}

func AddAlwaysHints(si *SliceInfo) ([]*discovery.EndpointSlice, []*discovery.EndpointSlice) {
	allocatableSlices := si.ToCreate
	for _, slice := range si.ToUpdate {
		allocatableSlices = append(allocatableSlices, slice)
	}

	for _, slice := range allocatableSlices {
		for i, endpoint := range slice.Endpoints {
			if !endpointsliceutil.EndpointReady(endpoint) {
				endpoint.Hints = nil
				continue
			}
			// TODO: Clarify how to handle a missing zone. Do we tolerate empty zone and continue or
			// do we remove all hints (as done in "Auto")? Depending on this the `canUseTopology` func has to be adapted for kube-proxy.
			if endpoint.Zone == nil || *endpoint.Zone == "" {
				klog.InfoS("Endpoint found without zone specified, removing hints from service", "serviceKey", si.ServiceKey)
				return RemoveHintsFromSlices(si)
			}

			slice.Endpoints[i].Hints = &discovery.EndpointHints{ForZones: []discovery.ForZone{{Name: *endpoint.Zone}}}
		}
	}

	return si.ToCreate, si.ToUpdate
}

// RemoveHintsFromSlices removes topology hints on EndpointSlices and returns
// updated lists of EndpointSlices to create and update.
func RemoveHintsFromSlices(si *SliceInfo) ([]*discovery.EndpointSlice, []*discovery.EndpointSlice) {
	// Remove hints on all EndpointSlices we were already going to change.
	slices := append(si.ToCreate, si.ToUpdate...)
	for _, slice := range slices {
		for i := range slice.Endpoints {
			slice.Endpoints[i].Hints = nil
		}
	}

	// Remove hints on all unchanged EndpointSlices and mark them for update
	// if any already had hints. We use j to track the number/index of slices
	// that are still unchanged.
	j := 0
	for _, slice := range si.Unchanged {
		changed := false
		for i, endpoint := range slice.Endpoints {
			if endpoint.Hints != nil {
				// Unchanged slices are still direct copies from informer cache.
				// Need to deep copy before we make any modifications to avoid
				// accidentally changing informer cache.
				slice = slice.DeepCopy()
				slice.Endpoints[i].Hints = nil
				changed = true
			}
		}
		if changed {
			si.ToUpdate = append(si.ToUpdate, slice)
		} else {
			si.Unchanged[j] = slice
			j++
		}
	}

	// truncate si.Unchanged so it only includes slices that are still
	// unchanged.
	si.Unchanged = si.Unchanged[:j]

	return si.ToCreate, si.ToUpdate
}
