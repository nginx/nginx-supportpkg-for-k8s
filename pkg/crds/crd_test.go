package crds

import (
	"reflect"
	"testing"
)

func TestGetNICCRDList(t *testing.T) {
	tests := []struct {
		name string
		want []Crd
	}{
		{
			name: "Correct CRD list",
			want: []Crd{
				{
					Resource: "apdoslogconfs",
					Group:    "appprotectdos.f5.com",
					Version:  "v1beta1",
				},
				{
					Resource: "apdospolicies",
					Group:    "appprotectdos.f5.com",
					Version:  "v1beta1",
				},
				{
					Resource: "dosprotectedresources",
					Group:    "appprotectdos.f5.com",
					Version:  "v1beta1",
				},
				{
					Resource: "aplogconfs",
					Group:    "appprotect.f5.com",
					Version:  "v1beta1",
				},
				{
					Resource: "appolicies",
					Group:    "appprotect.f5.com",
					Version:  "v1beta1",
				},
				{
					Resource: "apusersigs",
					Group:    "appprotect.f5.com",
					Version:  "v1beta1",
				},
				{
					Resource: "globalconfigurations",
					Group:    "k8s.nginx.org",
					Version:  "v1",
				},
				{
					Resource: "policies",
					Group:    "k8s.nginx.org",
					Version:  "v1",
				},
				{
					Resource: "transportservers",
					Group:    "k8s.nginx.org",
					Version:  "v1",
				},
				{
					Resource: "virtualserverroutes",
					Group:    "k8s.nginx.org",
					Version:  "v1",
				},
				{
					Resource: "virtualservers",
					Group:    "k8s.nginx.org",
					Version:  "v1",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetNICCRDList(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetNICCRDList() = %v, want %v", got, tt.want)
			}
		})
	}
}
