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
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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

	// check the database
	entry, dbErr := r.Get(req.NamespacedName)
	if dbErr != nil {
		return ctrl.Result{}, dbErr
	}

	ingress := new(networkingv1.Ingress)
	r.Client.Get(ctx, req.NamespacedName, ingress)
	appObjectID, ok := ingress.Annotations["azure-app-registration"]

	if ok {
		replyUrls := GetHostsFromIngress(ingress)
		entry := Entry{AppId: appObjectID, ReplyUrls: replyUrls}
		appReplyUrls, err := r.getReplyUrls(entry.AppId)
		if err != nil {
			return ctrl.Result{}, err
		}

		sort.Strings(replyUrls)
		sort.Strings(appReplyUrls)
		diff, result := MergeStringArrays(replyUrls, appReplyUrls)
		if len(diff) > 0 {
			err = r.updateReplyUrl(result, entry.AppId)
			if err != nil {
				return ctrl.Result{}, err
			}

			l.Info("Added reply-urls", "Resource Name", req.Name, "Reply URLs", diff)
		}

		if err = r.Put(req.NamespacedName, entry); err != nil {
			return ctrl.Result{}, err
		}
	} else if entry != nil {
		appReplyUrls, err := r.getReplyUrls(entry.AppId)
		if err != nil {
			return ctrl.Result{}, err
		}

		sort.Strings(appReplyUrls) // entry.ReplyUrls are already sorted
		diff, result := DiffStringArrays(entry.ReplyUrls, appReplyUrls)
		if len(diff) > 0 {
			err = r.updateReplyUrl(result, entry.AppId)
			if err != nil {
				return ctrl.Result{}, err
			}

			if err := r.Delete(req.NamespacedName); err != nil {
				return ctrl.Result{}, err
			}

			l.Info("Removed reply-url", "Resource Name", req.Name, "Host", diff)
		}
	}
	return ctrl.Result{}, nil
}

func (r *IngressReconciler) getReplyUrls(appObjectID string) ([]string, error) {
	app, err := r.GraphClient.ApplicationsById(appObjectID).Get(nil)
	if err != nil {
		return make([]string, 0), err
	}
	return app.GetWeb().GetRedirectUris(), nil
}

func (r *IngressReconciler) updateReplyUrl(replyUrls []string, appObjectID string) error {
	if err := r.GraphClient.ApplicationsById(appObjectID).Patch(
		&item.ApplicationRequestBuilderPatchOptions{
			Body: NewUpdateRedirectUrisRequestBody(replyUrls),
		},
	); err != nil {
		return err
	}
	return nil
}

func (r *IngressReconciler) Keys() []types.NamespacedName {
	keys := []types.NamespacedName{}
	for data := range r.db.Keys() {
		keyPtr := new(types.NamespacedName)

		err := DecodeData(bytes.NewBuffer(data), keyPtr)
		if err != nil {
			continue
		}

		keys = append(keys, *keyPtr)
	}

	return keys
}

func (r *IngressReconciler) Put(key types.NamespacedName, entry Entry) error {
	keyBuf, err := EncodeData(key)
	if err != nil {
		return err
	}

	entryData, err := EncodeData(entry)
	if err != nil {
		return err
	}

	if err = r.db.Put(keyBuf.Bytes(), entryData.Bytes()); err != nil {
		return err
	}

	return nil
}

func (r *IngressReconciler) Get(key types.NamespacedName) (*Entry, error) {
	entryPtr := new(Entry)

	keyBuf, err := EncodeData(key)
	if err != nil {
		return nil, err
	}

	dataBytes, err := r.db.Get(keyBuf.Bytes())
	if err != nil {
		if err.Error() == "error: key not found" {
			return nil, nil
		}
	}

	err = DecodeData(bytes.NewBuffer(dataBytes), entryPtr)
	if err != nil {
		return nil, err
	}

	return entryPtr, err
}

func (r *IngressReconciler) Delete(key types.NamespacedName) error {
	keyBuf, err := EncodeData(key)
	if err != nil {
		return err
	}

	if err = r.db.Delete(keyBuf.Bytes()); err != nil {
		return err
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	dbPtr, err := bitcask.Open(dbPath, bitcask.WithMaxKeySize(1024))
	if err != nil {
		return err
	}

	r.db = dbPtr

	// reconcile the objects stored in the database
	for _, nsn := range r.Keys() {
		r.Reconcile(context.TODO(), reconcile.Request{NamespacedName: nsn})
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.Ingress{}).
		Complete(r)
}
