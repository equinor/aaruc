/*
Copyright 2022.

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

package controllers

import (
	"bytes"
	"context"
	"sort"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"git.mills.io/prologic/bitcask"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/applications/item"
)

const dbPath string = "./db"

// IngressReconciler reconciles a Ingress object
type IngressReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	GraphClient *msgraphsdk.GraphServiceClient
	db          *bitcask.Bitcask
}

//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Ingress object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	key := []byte(req.NamespacedName.String())
	i := new(networkingv1.Ingress)
	err := r.Client.Get(ctx, req.NamespacedName, i)
	if err != nil {
		return ctrl.Result{}, err
	}

	id, ok := i.Annotations["azure-app-registration"]
	if ok {
		// add reply-url
		host := "https://" + i.Spec.Rules[0].Host

		app, err := r.GraphClient.ApplicationsById(id).Get(nil)
		if err != nil {
			return ctrl.Result{}, err
		}

		Uris := append(app.GetWeb().GetRedirectUris(), host)

		err = r.GraphClient.ApplicationsById(id).Patch(
			&item.ApplicationRequestBuilderPatchOptions{
				Body: NewUpdateRedirectUrisRequestBody(Uris),
			},
		)
		if err != nil {
			return ctrl.Result{}, err
		}

		e := &Entry{AppId: id, Uri: host}
		b, _ := e.Serialize()
		r.db.Put(key, b.Bytes())
		l.Info("Added reply-url", "Resource Name", req.Name, "Host", host)
	} else {
		// check if an entry is in db
		data, _ := r.db.Get(key)
		if len(data) != 0 {
			// if a value for the resource was present, then the annotation must have been removed
			e, err := DecodeEntry(bytes.NewBuffer(data))
			if err != nil {
				return ctrl.Result{}, err
			}
			host := e.Uri

			app, err := r.GraphClient.ApplicationsById(e.AppId).Get(nil)
			if err != nil {
				return ctrl.Result{}, err
			}

			Uris := app.GetWeb().GetRedirectUris()
			sort.Strings(Uris)
			i := sort.SearchStrings(Uris, host)
			if i < len(Uris) {
				err = r.GraphClient.ApplicationsById(e.AppId).Patch(
					&item.ApplicationRequestBuilderPatchOptions{
						Body: NewUpdateRedirectUrisRequestBody(append(Uris[:i], Uris[i+1:]...)),
					},
				)
				if err != nil {
					return ctrl.Result{}, err
				}
				r.db.Delete(key)
				l.Info("Removed reply-url", "Resource Name", req.Name, "Host", host)
			}
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	dbPtr, err := bitcask.Open(dbPath)
	if err != nil {
		return err
	}

	r.db = dbPtr

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.Ingress{}).
		Complete(r)
}
