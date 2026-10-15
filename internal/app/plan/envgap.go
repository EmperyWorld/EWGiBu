package plan

import (
	"sort"

	"iacpack/internal/domain/ir"
)

// missingEnvKeys lists environment keys a service lacks compared with another
// service that uses the same image. environmentFrom copies those keys, so a
// service that inherits them is absent from the result.
func missingEnvKeys(dep ir.Deployment) map[string][]string {
	byImage := map[string][]string{}
	for id, svc := range dep.Services {
		byImage[svc.Image] = append(byImage[svc.Image], id)
	}
	out := map[string][]string{}
	images := make([]string, 0, len(byImage))
	for image := range byImage {
		images = append(images, image)
	}
	sort.Strings(images)
	for _, image := range images {
		ids := byImage[image]
		if len(ids) < 2 {
			continue
		}
		sort.Strings(ids)
		finals := map[string]map[string]struct{}{}
		union := map[string]struct{}{}
		for _, id := range ids {
			keys := envKeySet(dep, id, nil)
			finals[id] = keys
			for k := range keys {
				union[k] = struct{}{}
			}
		}
		for _, id := range ids {
			var miss []string
			for k := range union {
				if _, ok := finals[id][k]; !ok {
					miss = append(miss, k)
				}
			}
			sort.Strings(miss)
			if len(miss) > 0 {
				out[id] = miss
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func envKeySet(dep ir.Deployment, id string, stack []string) map[string]struct{} {
	for _, s := range stack {
		if s == id {
			return map[string]struct{}{}
		}
	}
	svc, ok := dep.Services[id]
	if !ok {
		return map[string]struct{}{}
	}
	out := map[string]struct{}{}
	if svc.EnvironmentFrom != "" {
		for k := range envKeySet(dep, svc.EnvironmentFrom, append(stack, id)) {
			out[k] = struct{}{}
		}
	}
	for k := range svc.Environment {
		out[k] = struct{}{}
	}
	return out
}

func sortedGapIDs(gaps map[string][]string) []string {
	ids := make([]string, 0, len(gaps))
	for id := range gaps {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
