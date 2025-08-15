/**

Copyright 2024 F5, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

**/

package crds

type Crd struct {
	Resource string
	Group    string
	Version  string
}

func GetNICCRDList() []Crd {
	crdList := []Crd{
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
	}
	return crdList
}

func GetNGFCRDList() []Crd {
	crdList := []Crd{
		{
			Resource: "clientsettingspolicies",
			Group:    "gateway.nginx.org",
			Version:  "v1alpha1",
		},
		{
			Resource: "nginxgateways",
			Group:    "gateway.nginx.org",
			Version:  "v1alpha1",
		},
		{
			Resource: "nginxproxies",
			Group:    "gateway.nginx.org",
			Version:  "v1alpha1",
		},
		{
			Resource: "observabilitypolicies",
			Group:    "gateway.nginx.org",
			Version:  "v1alpha1",
		},
	}
	return crdList
}
