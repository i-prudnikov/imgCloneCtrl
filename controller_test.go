package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)
//TODO(i-prudnikov): Add unit tests for reconciler object methods


// Test_Reconcile is overall test of reconciliation logic for deployment and daemonset
// Based on this, we can extend testing to cover various errors. This is not included here
// for the sake of simplicity.
// From the home exercise perspective, I'm aiming to show general approach only
func Test_Reconcile(t *testing.T) {
	// Fake client is buggy, and it looks like it more or less works for very basic and simple scenarios
	// https://github.com/kubernetes-sigs/controller-runtime/issues/348
	fakeClientBuilder := fake.NewClientBuilder()

	//mock registry
	mockRegistry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Fake registry")
	}))
	defer mockRegistry.Close()

	u,_ := url.Parse(mockRegistry.URL)

	reconc := reconciler{
		client:            nil,
		ignoredNamespaces: map[string]struct{}{"kube-system": {}},
		backupRegistry:    u.Host+"/namespace/backup",
	}

	tests := []struct {
		// test case short title
		title       string
		objects     []client.Object
		expetedImage string
		expectError bool
	}{
		{
			title: "reconcile deployment",
			expetedImage: reconc.getTargetImage(u.Host+"/nginx:latest"),
			objects: []client.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "server",
						Namespace: "test",
					},
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"deployment": "test" + "-deployment"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"deployment": "test" + "-deployment"}},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "nginx",
										Image: u.Host+"/nginx:latest",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			title: "reconcile daemonset",
			expetedImage: reconc.getTargetImage(u.Host+"/nginx:latest"),
			objects: []client.Object{
				&appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "server",
						Namespace: "test",
					},
					Spec: appsv1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"deployment": "test" + "-deployment"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"deployment": "test" + "-deployment"}},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "nginx",
										Image: u.Host+"/nginx:latest",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			//Put mock objects to fake client
			fakeClientBuilder.WithObjects(test.objects...)
			//Set fake client to reconciler
			reconc.client = fakeClientBuilder.Build()

			for _, o := range test.objects {
				kind := ""

				if _, isDeployment := o.(*appsv1.Deployment); isDeployment {
					kind = "Deployment"
				}else {
					kind = "DaemonSet"
				}

				r := reconcile.Request{NamespacedName: types.NamespacedName{
					Namespace: o.GetNamespace(),
					Name:      fmt.Sprintf("%s:%s", kind, o.GetName()),
				}}
				_, e := reconc.Reconcile(context.Background(), r)
				require.Nil(t, e)

				//Checking if reconciled object has the right image
				key := types.NamespacedName{
					Name: o.GetName(),
					Namespace: o.GetNamespace(),
				}
				switch kind {
				case "Deployment":
					dp := appsv1.Deployment{}
					reconc.client.Get(context.Background(),key,&dp)
					require.Equal(t,dp.Spec.Template.Spec.Containers[0].Image,test.expetedImage)

				case "DaemonSet":
					ds := appsv1.DaemonSet{}
					reconc.client.Get(context.Background(),key,&ds)
					require.Equal(t,ds.Spec.Template.Spec.Containers[0].Image,test.expetedImage)
				}
			}
		})
	}
}
