package controllers

import "github.com/microsoftgraph/msgraph-sdk-go/models/microsoft/graph"

func NewUpdateRedirectUrisRequestBody(Uris []string) *graph.Application {
	reqBody := graph.NewApplication()
	reqBody.SetWeb(graph.NewWebApplication())
	reqBody.GetWeb().SetRedirectUris(
		Uris,
	)
	return reqBody
}
