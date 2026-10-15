package importpropose

import (
	"fmt"
	"sort"
	"strings"

	"iacpack/internal/diagnostic"
)

// linkEnvironmentFrom sets environmentFrom when two services share the same
// pull image ref and one env set is not a value-preserving superset of the
// other. The shorter or differing service inherits from the fuller one and
// keeps only keys whose values differ.
func linkEnvironmentFrom(services, images map[string]any, diags *[]diagnostic.Diagnostic) {
	type member struct {
		id   string
		keys map[string]any
	}
	byRef := map[string][]member{}
	ids := make([]string, 0, len(services))
	for id := range services {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		body, _ := services[id].(map[string]any)
		if body == nil {
			continue
		}
		if _, ok := body["environmentFrom"].(string); ok {
			continue
		}
		imgID, _ := body["image"].(string)
		img, _ := images[imgID].(map[string]any)
		if img == nil {
			continue
		}
		ref, _ := img["ref"].(string)
		if ref == "" {
			continue
		}
		env, _ := body["environment"].(map[string]any)
		byRef[ref] = append(byRef[ref], member{id: id, keys: env})
	}
	refIDs := make([]string, 0, len(byRef))
	for ref := range byRef {
		refIDs = append(refIDs, ref)
	}
	sort.Strings(refIDs)
	for _, ref := range refIDs {
		group := byRef[ref]
		if len(group) < 2 {
			continue
		}
		sort.SliceStable(group, func(i, j int) bool {
			if len(group[i].keys) != len(group[j].keys) {
				return len(group[i].keys) > len(group[j].keys)
			}
			return group[i].id < group[j].id
		})
		src := group[0]
		for _, dst := range group[1:] {
			if dst.id == src.id || len(src.keys) == 0 {
				continue
			}
			if equalEnvMaps(src.keys, dst.keys) {
				body, _ := services[dst.id].(map[string]any)
				body["environmentFrom"] = src.id
				delete(body, "environment")
				services[dst.id] = body
				*diags = append(*diags, diagnostic.Info(diagnostic.CodeImportEnvFrom,
					fmt.Sprintf("service %s: environmentFrom %s (identical env)", dst.id, src.id)))
				continue
			}
			if isEnvSuperset(dst.keys, src.keys) {
				allMatch := true
				for k, v := range src.keys {
					if !envValueEqual(v, dst.keys[k]) {
						allMatch = false
						break
					}
				}
				if allMatch {
					continue
				}
			}
			overrides := map[string]any{}
			var overrideKeys []string
			for k, v := range dst.keys {
				sv, ok := src.keys[k]
				if !ok || !envValueEqual(sv, v) {
					overrides[k] = v
					overrideKeys = append(overrideKeys, k)
				}
			}
			sort.Strings(overrideKeys)
			body, _ := services[dst.id].(map[string]any)
			body["environmentFrom"] = src.id
			if len(overrides) > 0 {
				body["environment"] = overrides
			} else {
				delete(body, "environment")
			}
			services[dst.id] = body
			msg := fmt.Sprintf("service %s: environmentFrom %s", dst.id, src.id)
			if len(overrideKeys) > 0 {
				msg += "; override " + strings.Join(overrideKeys, ", ")
			}
			*diags = append(*diags, diagnostic.Info(diagnostic.CodeImportEnvFrom, msg).
				WithSource("", "/spec/services/"+dst.id+"/environmentFrom", 0, 0))
		}
	}
}

func isEnvSuperset(a, b map[string]any) bool {
	for k := range b {
		if _, ok := a[k]; !ok {
			return false
		}
	}
	return true
}

func equalEnvMaps(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok || !envValueEqual(va, vb) {
			return false
		}
	}
	return true
}

func envValueEqual(a, b any) bool {
	am, aok := a.(map[string]any)
	bm, bok := b.(map[string]any)
	if aok && bok {
		if len(am) != len(bm) {
			return false
		}
		for k, v := range am {
			if !envValueEqual(v, bm[k]) {
				return false
			}
		}
		return true
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}
