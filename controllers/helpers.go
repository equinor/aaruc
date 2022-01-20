package controllers

import (
	"bytes"
	"encoding/gob"
	"sort"

	"github.com/microsoftgraph/msgraph-sdk-go/models/microsoft/graph"
	networkingv1 "k8s.io/api/networking/v1"
)

func NewUpdateRedirectUrisRequestBody(Uris []string) *graph.Application {
	reqBody := graph.NewApplication()
	reqBody.SetWeb(graph.NewWebApplication())
	reqBody.GetWeb().SetRedirectUris(
		Uris,
	)
	return reqBody
}

func EncodeData(s interface{}) (*bytes.Buffer, error) {
	b := new(bytes.Buffer)
	e := gob.NewEncoder(b)
	if err := e.Encode(s); err != nil {
		return b, err
	}
	return b, nil
}

func DecodeData(b *bytes.Buffer, t interface{}) error {
	d := gob.NewDecoder(b)
	if err := d.Decode(t); err != nil {
		return err
	}
	return nil
}

func MergeStringArrays(source []string, target []string) ([]string, []string) {
	// iss:	index source string
	// ss:	source string
	// si:	search index
	diff := []string{}
	for _, ss := range source {
		si := sort.SearchStrings(target, ss)
		if si == len(target) {
			diff = append(diff, ss)
			target = append(target, ss)
			sort.Strings(target)
		}
	}
	return diff, target
}

func DiffStringArrays(source []string, target []string) ([]string, []string) {
	// iss:	index source string
	// ss:	source string
	// si:	search index
	diff := []string{}
	for _, ss := range source {
		si := sort.SearchStrings(target, ss)
		if si < len(target) {
			diff = append(diff, ss)
			target[si] = target[len(target)-1]
			target = target[:len(target)-1]
			sort.Strings(target)
		}
	}
	return diff, target
}

func GetHostsFromIngress(ingress *networkingv1.Ingress) []string {
	result := []string{}
	for _, r := range ingress.Spec.Rules {
		result = append(result, "https://"+r.Host)
	}
	return result
}
